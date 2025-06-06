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
	"context"
	"math"
	"sync"
	"time"

	"github.com/Fedosin/libkpa/api"
	"github.com/Fedosin/libkpa/metrics"
)

// SlidingWindowAutoscaler implements the sliding window autoscaling algorithm
// used by Knative's KPA (Knative Pod Autoscaler).
type SlidingWindowAutoscaler struct {
	mu sync.RWMutex

	// Configuration
	spec *api.AutoscalerSpec

	// State for panic mode
	panicTime    time.Time
	maxPanicPods int32

	// Delay window for scale-down decisions
	delayWindow *metrics.TimeWindow
}

// NewSlidingWindowAutoscaler creates a new sliding window autoscaler.
func NewSlidingWindowAutoscaler(spec *api.AutoscalerSpec) *SlidingWindowAutoscaler {
	var delayWindow *metrics.TimeWindow
	if spec.ScaleDownDelay > 0 {
		delayWindow = metrics.NewTimeWindow(spec.ScaleDownDelay, 2*time.Second)
	}

	return &SlidingWindowAutoscaler{
		spec:        spec,
		delayWindow: delayWindow,
	}
}

// Scale calculates the desired scale based on current metrics.
func (a *SlidingWindowAutoscaler) Scale(ctx context.Context, snapshot api.MetricSnapshot, now time.Time) api.ScaleRecommendation {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Get current ready pod count
	readyPodCount := float64(snapshot.ReadyPodCount())
	if readyPodCount == 0 {
		readyPodCount = 1 // Avoid division by zero
	}

	// Get metric values
	observedStableValue := snapshot.StableValue()
	observedPanicValue := snapshot.PanicValue()

	// If no data, return invalid recommendation
	if observedStableValue == 0 && observedPanicValue == 0 {
		return api.ScaleRecommendation{
			ScaleValid: false,
		}
	}

	// Calculate scale limits based on current pod count
	maxScaleUp := math.Ceil(a.spec.MaxScaleUpRate * readyPodCount)
	maxScaleDown := float64(0)
	if a.spec.Reachable {
		maxScaleDown = math.Floor(readyPodCount / a.spec.MaxScaleDownRate)
	}

	// Calculate desired pod counts
	desiredStablePodCount := math.Ceil(observedStableValue / a.spec.TargetValue)
	desiredPanicPodCount := math.Ceil(observedPanicValue / a.spec.TargetValue)

	// Apply scale limits
	desiredStablePodCount = math.Min(math.Max(desiredStablePodCount, maxScaleDown), maxScaleUp)
	desiredPanicPodCount = math.Min(math.Max(desiredPanicPodCount, maxScaleDown), maxScaleUp)

	// Apply activation scale if needed
	if a.spec.ActivationScale > 1 {
		if desiredStablePodCount > 0 && float64(a.spec.ActivationScale) > desiredStablePodCount {
			desiredStablePodCount = float64(a.spec.ActivationScale)
		}
		if desiredPanicPodCount > 0 && float64(a.spec.ActivationScale) > desiredPanicPodCount {
			desiredPanicPodCount = float64(a.spec.ActivationScale)
		}
	}

	// Check panic mode conditions
	isOverPanicThreshold := desiredPanicPodCount/readyPodCount >= a.spec.PanicThreshold
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
	case inPanicMode && !isOverPanicThreshold && a.panicTime.Add(a.spec.StableWindow).Before(now):
		// Exit panic mode
		a.panicTime = time.Time{}
		a.maxPanicPods = 0
		inPanicMode = false
	}

	// Determine final desired pod count
	desiredPodCount := int32(desiredStablePodCount)
	if inPanicMode {
		// Use the higher of stable or panic pod count
		if int32(desiredPanicPodCount) > desiredPodCount {
			desiredPodCount = int32(desiredPanicPodCount)
		}
		// Never scale down in panic mode
		if desiredPodCount > a.maxPanicPods {
			a.maxPanicPods = desiredPodCount
		} else {
			desiredPodCount = a.maxPanicPods
		}
	}

	// Apply scale-down delay if configured
	if a.spec.Reachable && a.delayWindow != nil {
		a.delayWindow.Record(now, desiredPodCount)
		delayedPodCount := a.delayWindow.Current()
		desiredPodCount = delayedPodCount
	}

	// Apply min/max scale bounds
	if a.spec.MinScale > 0 && desiredPodCount < a.spec.MinScale {
		desiredPodCount = a.spec.MinScale
	}
	if a.spec.MaxScale > 0 && desiredPodCount > a.spec.MaxScale {
		desiredPodCount = a.spec.MaxScale
	}

	// Calculate excess burst capacity
	excessBurstCapacity := calculateExcessBurstCapacity(
		snapshot.ReadyPodCount(),
		a.spec.TotalValue,
		a.spec.TargetBurstCapacity,
		observedPanicValue,
	)

	return api.ScaleRecommendation{
		DesiredPodCount:     desiredPodCount,
		ExcessBurstCapacity: excessBurstCapacity,
		ScaleValid:          true,
		InPanicMode:         inPanicMode,
		ObservedStableValue: observedStableValue,
		ObservedPanicValue:  observedPanicValue,
		CurrentPodCount:     snapshot.ReadyPodCount(),
	}
}

// Update reconfigures the autoscaler with a new spec.
func (a *SlidingWindowAutoscaler) Update(spec *api.AutoscalerSpec) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.spec = spec

	// Update delay window if needed
	if spec.ScaleDownDelay > 0 {
		if a.delayWindow == nil {
			a.delayWindow = metrics.NewTimeWindow(spec.ScaleDownDelay, 2*time.Second)
		} else {
			a.delayWindow.ResizeWindow(spec.ScaleDownDelay)
		}
	} else {
		a.delayWindow = nil
	}

	return nil
}

// GetSpec returns the current autoscaler spec.
func (a *SlidingWindowAutoscaler) GetSpec() api.AutoscalerSpec {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return *a.spec
}

// calculateExcessBurstCapacity computes the excess burst capacity.
// A negative value means the deployment doesn't have enough capacity
// to handle the target burst capacity.
func calculateExcessBurstCapacity(readyPods int32, totalValue, targetBurstCapacity, observedPanicValue float64) int32 {
	if targetBurstCapacity == 0 {
		return 0
	}
	if targetBurstCapacity < 0 {
		return -1 // Unlimited
	}

	totalCapacity := float64(readyPods) * totalValue
	excessBurstCapacity := math.Floor(totalCapacity - targetBurstCapacity - observedPanicValue)
	return int32(excessBurstCapacity)
}
