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

	// State for burst mode
	burstTime    time.Time
	maxBurstPods int32

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

	// We always start in the burst mode.
	// When Autoscaler restarts we lose metric history, which causes us to
	// momentarily scale down, and that is not a desired behavior.
	// Thus, we're keeping at least the current scale until we
	// accumulate enough data to make conscious decisions.
	result.burstTime = time.Now()

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
	observedBurstValue := snapshot.BurstValue()

	// If no data, return invalid recommendation
	if observedStableValue < 0 || observedBurstValue < 0 {
		return api.ScaleRecommendation{
			ScaleValid: false,
		}
	}

	// Calculate scale limits based on current pod count
	maxScaleUp := int32(math.Ceil(a.config.MaxScaleUpRate * float64(readyPodCount)))
	maxScaleDown := int32(math.Floor(float64(readyPodCount) / a.config.MaxScaleDownRate))

	// raw pod counts calculated directly from metrics, prior to applying any rate limits.
	var rawStablePodCount, rawBurstPodCount int32

	if a.config.TargetValue > 0 {
		rawStablePodCount = int32(math.Ceil(observedStableValue / a.config.TargetValue))
		rawBurstPodCount = int32(math.Ceil(observedBurstValue / a.config.TargetValue))
	} else if a.config.TotalTargetValue > 0 {
		rawStablePodCount = int32(math.Ceil(float64(readyPodCount) * observedStableValue / a.config.TotalTargetValue))
		rawBurstPodCount = int32(math.Ceil(float64(readyPodCount) * observedBurstValue / a.config.TotalTargetValue))
	}

	// Apply scale limits
	desiredStablePodCount := min(max(rawStablePodCount, maxScaleDown), maxScaleUp)
	desiredBurstPodCount := min(max(rawBurstPodCount, maxScaleDown), maxScaleUp)

	// Apply activation scale if needed
	if a.config.ActivationScale > 1 {
		// Activation scale should apply only when there is actual demand (i.e. raw counts > 0).
		// This prevents the activation scale from blocking scale-to-zero.
		if rawStablePodCount > 0 && a.config.ActivationScale > desiredStablePodCount {
			desiredStablePodCount = a.config.ActivationScale
		}
		if rawBurstPodCount > 0 && a.config.ActivationScale > desiredBurstPodCount {
			desiredBurstPodCount = a.config.ActivationScale
		}
	}

	// Check burst mode conditions
	isOverBurstThreshold := float64(rawBurstPodCount)/float64(readyPodCount) >= a.config.BurstThreshold
	inBurstMode := !a.burstTime.IsZero()

	// Update burst mode state
	switch {
	case !inBurstMode && isOverBurstThreshold:
		// Enter burst mode
		a.burstTime = now
		inBurstMode = true
	case isOverBurstThreshold:
		// Extend burst mode
		a.burstTime = now
	case inBurstMode && !isOverBurstThreshold && a.burstTime.Add(a.config.StableWindow).Before(now):
		// Exit burst mode
		a.burstTime = time.Time{}
		a.maxBurstPods = 0
		inBurstMode = false
	}

	// Determine final desired pod count
	desiredPodCount := desiredStablePodCount
	if inBurstMode {
		// Use the higher of stable or burst pod count
		if desiredBurstPodCount > desiredPodCount {
			desiredPodCount = desiredBurstPodCount
		}
		// Never scale down in burst mode
		if desiredPodCount > a.maxBurstPods {
			a.maxBurstPods = desiredPodCount
		} else {
			desiredPodCount = a.maxBurstPods
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
		InBurstMode:     inBurstMode,
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
