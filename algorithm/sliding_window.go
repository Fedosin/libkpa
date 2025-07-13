/*
Copyright 2025 The libkpa Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package algorithm implements the KPA autoscaling algorithms.
package algorithm

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/Fedosin/libkpa/api"
	libkpaconfig "github.com/Fedosin/libkpa/config"
	"github.com/Fedosin/libkpa/maxtimewindow"
)

// SlidingWindowAutoscaler implements the sliding window autoscaling algorithm
// used by Knative's KPA (Knative Pod Autoscaler).
type SlidingWindowAutoscaler struct {
	mu sync.RWMutex

	// Configuration
	config api.AutoscalerConfig

	// State for panic mode
	panicTime    time.Time
	maxPanicPods int32

	// Delay window for scale-down decisions
	maxTimeWindow *maxtimewindow.TimeWindow
}

const (
	scaleDownDelayGranularity = 2 * time.Second
)

// NewSlidingWindowAutoscaler creates a new sliding window autoscaler.
func NewSlidingWindowAutoscaler(config api.AutoscalerConfig) (*SlidingWindowAutoscaler, error) {
	if err := libkpaconfig.Validate(&config); err != nil {
		return nil, err
	}

	var maxTimeWindow *maxtimewindow.TimeWindow
	if config.ScaleDownDelay > 0 {
		maxTimeWindow = maxtimewindow.NewTimeWindow(config.ScaleDownDelay, scaleDownDelayGranularity)
	}

	result := &SlidingWindowAutoscaler{
		config:        config,
		maxTimeWindow: maxTimeWindow,
	}

	// We always start in the panic mode.
	// When Autoscaler restarts we lose metric history, which causes us to
	// momentarily scale down, and that is not a desired behavior.
	// Thus, we're keeping at least the current scale until we
	// accumulate enough data to make conscious decisions.
	result.panicTime = time.Now()

	return result, nil
}

// Scale calculates the desired scale based on current metrics.
func (a *SlidingWindowAutoscaler) Scale(snapshot api.MetricSnapshot, now time.Time) api.ScaleRecommendation {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Get current ready pod count
	readyPodCount := snapshot.ReadyPodCount()
	if readyPodCount == 0 {
		readyPodCount = 1 // Avoid division by zero
	}

	// Get metric values
	observedStableValue := snapshot.StableValue()
	observedPanicValue := snapshot.PanicValue()

	// If no data, return invalid recommendation
	if observedStableValue < 0 || observedPanicValue < 0 {
		return api.ScaleRecommendation{
			ScaleValid: false,
		}
	}

	// Calculate scale limits based on current pod count
	maxScaleUp := int32(math.Ceil(a.config.MaxScaleUpRate * float64(readyPodCount)))
	maxScaleDown := int32(math.Floor(float64(readyPodCount) / a.config.MaxScaleDownRate))

	// raw pod counts calculated directly from metrics, prior to applying any rate limits.
	var rawStablePodCount, rawPanicPodCount int32

	if a.config.TargetValue > 0 {
		rawStablePodCount = int32(math.Ceil(observedStableValue / a.config.TargetValue))
		rawPanicPodCount = int32(math.Ceil(observedPanicValue / a.config.TargetValue))
	} else if a.config.TotalTargetValue > 0 {
		rawStablePodCount = int32(math.Ceil(observedStableValue / a.config.TotalTargetValue))
		rawPanicPodCount = int32(math.Ceil(observedPanicValue / a.config.TotalTargetValue))
	}

	// Apply scale limits
	desiredStablePodCount := min(max(rawStablePodCount, maxScaleDown), maxScaleUp)
	desiredPanicPodCount := min(max(rawPanicPodCount, maxScaleDown), maxScaleUp)

	// Apply activation scale if needed
	if a.config.ActivationScale > 1 {
		// Activation scale should apply only when there is actual demand (i.e. raw counts > 0).
		// This prevents the activation scale from blocking scale-to-zero.
		if rawStablePodCount > 0 && a.config.ActivationScale > desiredStablePodCount {
			desiredStablePodCount = a.config.ActivationScale
		}
		if rawPanicPodCount > 0 && a.config.ActivationScale > desiredPanicPodCount {
			desiredPanicPodCount = a.config.ActivationScale
		}
	}

	// Check panic mode conditions
	isOverPanicThreshold := float64(rawPanicPodCount)/float64(readyPodCount) >= a.config.PanicThreshold
	inPanicMode := !a.panicTime.IsZero()

	// Update panic mode state
	switch {
	case !inPanicMode && isOverPanicThreshold:
		// Enter panic mode
		a.panicTime = now
		inPanicMode = true
	case isOverPanicThreshold:
		// Extend panic mode
		a.panicTime = now
	case inPanicMode && !isOverPanicThreshold && a.panicTime.Add(a.config.StableWindow).Before(now):
		// Exit panic mode
		a.panicTime = time.Time{}
		a.maxPanicPods = 0
		inPanicMode = false
	}

	// Determine final desired pod count
	desiredPodCount := desiredStablePodCount
	if inPanicMode {
		// Use the higher of stable or panic pod count
		if desiredPanicPodCount > desiredPodCount {
			desiredPodCount = desiredPanicPodCount
		}
		// Never scale down in panic mode
		if desiredPodCount > a.maxPanicPods {
			a.maxPanicPods = desiredPodCount
		} else {
			desiredPodCount = a.maxPanicPods
		}
	}

	// Apply scale-down delay if configured
	if a.maxTimeWindow != nil {
		a.maxTimeWindow.Record(now, desiredPodCount)
		desiredPodCount = a.maxTimeWindow.Current()
	}

	// Apply min/max scale bounds
	if a.config.MinScale > 0 && desiredPodCount < a.config.MinScale {
		desiredPodCount = a.config.MinScale
	}
	if a.config.MaxScale > 0 && desiredPodCount > a.config.MaxScale {
		desiredPodCount = a.config.MaxScale
	}

	return api.ScaleRecommendation{
		DesiredPodCount: desiredPodCount,
		ScaleValid:      true,
		InPanicMode:     inPanicMode,
	}
}

// Update reconfigures the autoscaler with a new spec.
func (a *SlidingWindowAutoscaler) Update(config api.AutoscalerConfig) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if err := libkpaconfig.Validate(&config); err != nil {
		return fmt.Errorf("failed to validate config: %w", err)
	}

	a.config = config

	// Update delay window if needed
	if config.ScaleDownDelay > 0 {
		a.maxTimeWindow = maxtimewindow.NewTimeWindow(config.ScaleDownDelay, scaleDownDelayGranularity)
	}

	return nil
}

// GetSpec returns the current autoscaler spec.
func (a *SlidingWindowAutoscaler) GetConfig() api.AutoscalerConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.config
}
