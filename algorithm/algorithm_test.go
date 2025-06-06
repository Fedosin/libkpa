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

package algorithm

import (
	"context"
	"testing"
	"time"

	"github.com/Fedosin/libkpa/api"
	"github.com/Fedosin/libkpa/metrics"
)

func TestSlidingWindowAutoscaler_StableTraffic(t *testing.T) {
	spec := api.AutoscalerSpec{
		MaxScaleUpRate:        10.0,
		MaxScaleDownRate:      2.0,
		ScalingMetric:         api.Concurrency,
		TargetValue:           100.0,
		TotalValue:            1000.0,
		TargetBurstCapacity:   200.0,
		PanicThreshold:        2.0,
		PanicWindowPercentage: 10.0,
		StableWindow:          60 * time.Second,
		ScaleDownDelay:        0,
		MinScale:              1,
		MaxScale:              10,
		ActivationScale:       1,
		Reachable:             true,
	}

	autoscaler := NewSlidingWindowAutoscaler(spec)
	ctx := context.Background()
	now := time.Now()

	// Test stable traffic - should maintain current scale
	snapshot := metrics.NewMetricSnapshot(
		300.0, // stable value
		300.0, // panic value
		3,     // current pods
		now,
	)

	result := autoscaler.Scale(ctx, snapshot, now)

	if !result.ScaleValid {
		t.Error("Expected valid scale result")
	}
	if result.DesiredPodCount != 3 {
		t.Errorf("Expected 3 pods, got %d", result.DesiredPodCount)
	}
	if result.InPanicMode {
		t.Error("Should not be in panic mode for stable traffic")
	}
}

func TestSlidingWindowAutoscaler_RampingTraffic(t *testing.T) {
	spec := api.AutoscalerSpec{
		MaxScaleUpRate:        10.0,
		MaxScaleDownRate:      2.0,
		ScalingMetric:         api.Concurrency,
		TargetValue:           100.0,
		TotalValue:            1000.0,
		TargetBurstCapacity:   200.0,
		PanicThreshold:        2.0,
		PanicWindowPercentage: 10.0,
		StableWindow:          60 * time.Second,
		ScaleDownDelay:        0,
		MinScale:              1,
		MaxScale:              10,
		ActivationScale:       1,
		Reachable:             true,
	}

	autoscaler := NewSlidingWindowAutoscaler(spec)
	ctx := context.Background()
	now := time.Now()

	// Test ramping up traffic
	snapshot := metrics.NewMetricSnapshot(
		500.0, // stable value
		600.0, // panic value
		3,     // current pods
		now,
	)

	result := autoscaler.Scale(ctx, snapshot, now)

	if !result.ScaleValid {
		t.Error("Expected valid scale result")
	}
	// Should scale up to handle the increased load
	if result.DesiredPodCount < 5 {
		t.Errorf("Expected at least 5 pods for increased load, got %d", result.DesiredPodCount)
	}
}

func TestSlidingWindowAutoscaler_PanicMode(t *testing.T) {
	spec := api.AutoscalerSpec{
		MaxScaleUpRate:        10.0,
		MaxScaleDownRate:      2.0,
		ScalingMetric:         api.Concurrency,
		TargetValue:           100.0,
		TotalValue:            1000.0,
		TargetBurstCapacity:   200.0,
		PanicThreshold:        2.0,
		PanicWindowPercentage: 10.0,
		StableWindow:          60 * time.Second,
		ScaleDownDelay:        0,
		MinScale:              1,
		MaxScale:              20,
		ActivationScale:       1,
		Reachable:             true,
	}

	autoscaler := NewSlidingWindowAutoscaler(spec)
	ctx := context.Background()
	now := time.Now()

	// Test panic mode triggering - panic metric shows we need double the pods
	snapshot := metrics.NewMetricSnapshot(
		300.0, // stable value
		600.0, // panic value - needs 6 pods, we have 3
		3,     // current pods
		now,
	)

	result := autoscaler.Scale(ctx, snapshot, now)

	if !result.ScaleValid {
		t.Error("Expected valid scale result")
	}
	if !result.InPanicMode {
		t.Error("Should be in panic mode when desired/current > panic threshold")
	}
	if result.DesiredPodCount != 6 {
		t.Errorf("Expected 6 pods in panic mode, got %d", result.DesiredPodCount)
	}

	// Test that panic mode prevents scale down
	snapshot2 := metrics.NewMetricSnapshot(
		200.0, // stable value decreased
		250.0, // panic value decreased but still in panic
		6,     // current pods (after scale up)
		now.Add(10*time.Second),
	)

	result2 := autoscaler.Scale(ctx, snapshot2, now.Add(10*time.Second))

	if !result2.InPanicMode {
		t.Error("Should still be in panic mode")
	}
	if result2.DesiredPodCount < 6 {
		t.Errorf("Should not scale down in panic mode, got %d pods", result2.DesiredPodCount)
	}
}

func TestSlidingWindowAutoscaler_ScaleDownDelay(t *testing.T) {
	spec := api.AutoscalerSpec{
		MaxScaleUpRate:        10.0,
		MaxScaleDownRate:      2.0,
		ScalingMetric:         api.Concurrency,
		TargetValue:           100.0,
		TotalValue:            1000.0,
		TargetBurstCapacity:   200.0,
		PanicThreshold:        2.0,
		PanicWindowPercentage: 10.0,
		StableWindow:          60 * time.Second,
		ScaleDownDelay:        30 * time.Second, // 30s delay
		MinScale:              1,
		MaxScale:              10,
		ActivationScale:       1,
		Reachable:             true,
	}

	autoscaler := NewSlidingWindowAutoscaler(spec)
	ctx := context.Background()
	now := time.Now()

	// Start with high load
	snapshot1 := metrics.NewMetricSnapshot(
		500.0, // needs 5 pods
		500.0,
		5,
		now,
	)
	result1 := autoscaler.Scale(ctx, snapshot1, now)
	if result1.DesiredPodCount != 5 {
		t.Errorf("Expected 5 pods initially, got %d", result1.DesiredPodCount)
	}

	// Load drops but within delay window
	snapshot2 := metrics.NewMetricSnapshot(
		200.0, // needs 2 pods
		200.0,
		5,
		now.Add(10*time.Second),
	)
	result2 := autoscaler.Scale(ctx, snapshot2, now.Add(10*time.Second))

	// Should maintain 5 pods due to scale-down delay
	if result2.DesiredPodCount != 5 {
		t.Errorf("Expected 5 pods due to scale-down delay, got %d", result2.DesiredPodCount)
	}
}

func TestSlidingWindowAutoscaler_MinMaxScale(t *testing.T) {
	spec := api.AutoscalerSpec{
		MaxScaleUpRate:        10.0,
		MaxScaleDownRate:      2.0,
		ScalingMetric:         api.Concurrency,
		TargetValue:           100.0,
		TotalValue:            1000.0,
		TargetBurstCapacity:   200.0,
		PanicThreshold:        2.0,
		PanicWindowPercentage: 10.0,
		StableWindow:          60 * time.Second,
		ScaleDownDelay:        0,
		MinScale:              2,
		MaxScale:              5,
		ActivationScale:       1,
		Reachable:             true,
	}

	autoscaler := NewSlidingWindowAutoscaler(spec)
	ctx := context.Background()
	now := time.Now()

	// Test min scale
	snapshot1 := metrics.NewMetricSnapshot(
		50.0, // would need 1 pod
		50.0,
		3,
		now,
	)
	result1 := autoscaler.Scale(ctx, snapshot1, now)
	if result1.DesiredPodCount != 2 {
		t.Errorf("Expected min scale of 2 pods, got %d", result1.DesiredPodCount)
	}

	// Test max scale
	snapshot2 := metrics.NewMetricSnapshot(
		1000.0, // would need 10 pods
		1000.0,
		3,
		now,
	)
	result2 := autoscaler.Scale(ctx, snapshot2, now)
	if result2.DesiredPodCount != 5 {
		t.Errorf("Expected max scale of 5 pods, got %d", result2.DesiredPodCount)
	}
}

func TestSlidingWindowAutoscaler_ActivationScale(t *testing.T) {
	spec := api.AutoscalerSpec{
		MaxScaleUpRate:        10.0,
		MaxScaleDownRate:      2.0,
		ScalingMetric:         api.Concurrency,
		TargetValue:           100.0,
		TotalValue:            1000.0,
		TargetBurstCapacity:   200.0,
		PanicThreshold:        2.0,
		PanicWindowPercentage: 10.0,
		StableWindow:          60 * time.Second,
		ScaleDownDelay:        0,
		MinScale:              0,
		MaxScale:              10,
		ActivationScale:       3, // Minimum 3 pods when scaling from zero
		Reachable:             true,
	}

	autoscaler := NewSlidingWindowAutoscaler(spec)
	ctx := context.Background()
	now := time.Now()

	// Test activation scale
	snapshot := metrics.NewMetricSnapshot(
		150.0, // would need 2 pods normally
		150.0,
		0, // scaling from zero
		now,
	)
	result := autoscaler.Scale(ctx, snapshot, now)

	// Should scale to activation scale instead of 2
	if result.DesiredPodCount != 3 {
		t.Errorf("Expected activation scale of 3 pods, got %d", result.DesiredPodCount)
	}
}

func TestSlidingWindowAutoscaler_NoData(t *testing.T) {
	spec := api.AutoscalerSpec{
		MaxScaleUpRate:        10.0,
		MaxScaleDownRate:      2.0,
		ScalingMetric:         api.Concurrency,
		TargetValue:           100.0,
		TotalValue:            1000.0,
		TargetBurstCapacity:   200.0,
		PanicThreshold:        2.0,
		PanicWindowPercentage: 10.0,
		StableWindow:          60 * time.Second,
		ScaleDownDelay:        0,
		MinScale:              1,
		MaxScale:              10,
		ActivationScale:       1,
		Reachable:             true,
	}

	autoscaler := NewSlidingWindowAutoscaler(spec)
	ctx := context.Background()
	now := time.Now()

	// Test with no metric data
	snapshot := metrics.NewMetricSnapshot(
		0.0, // no stable data
		0.0, // no panic data
		3,
		now,
	)

	result := autoscaler.Scale(ctx, snapshot, now)

	if result.ScaleValid {
		t.Error("Should not return valid scale with no data")
	}
}

func TestExcessBurstCapacity(t *testing.T) {
	tests := []struct {
		name                string
		readyPods           int32
		totalValue          float64
		targetBurstCapacity float64
		observedPanicValue  float64
		expected            int32
	}{
		{
			name:                "positive excess",
			readyPods:           5,
			totalValue:          1000,
			targetBurstCapacity: 2000,
			observedPanicValue:  1500,
			expected:            1500, // 5*1000 - 2000 - 1500 = 1500
		},
		{
			name:                "negative excess",
			readyPods:           2,
			totalValue:          1000,
			targetBurstCapacity: 2000,
			observedPanicValue:  1500,
			expected:            -1500, // 2*1000 - 2000 - 1500 = -1500
		},
		{
			name:                "zero burst capacity",
			readyPods:           3,
			totalValue:          1000,
			targetBurstCapacity: 0,
			observedPanicValue:  1000,
			expected:            0,
		},
		{
			name:                "unlimited burst capacity",
			readyPods:           3,
			totalValue:          1000,
			targetBurstCapacity: -1,
			observedPanicValue:  1000,
			expected:            -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateExcessBurstCapacity(
				tt.readyPods,
				tt.totalValue,
				tt.targetBurstCapacity,
				tt.observedPanicValue,
			)
			if result != tt.expected {
				t.Errorf("Expected excess burst capacity %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestPanicModeCalculator(t *testing.T) {
	spec := &api.AutoscalerSpec{
		PanicThreshold:        2.0,
		PanicWindowPercentage: 10.0,
		StableWindow:          60 * time.Second,
	}

	calc := NewPanicModeCalculator(spec)

	// Test panic window calculation
	panicWindow := calc.CalculatePanicWindow()
	expectedWindow := 6 * time.Second // 10% of 60s
	if panicWindow != expectedWindow {
		t.Errorf("Expected panic window %v, got %v", expectedWindow, panicWindow)
	}

	// Test should enter panic mode
	if !calc.ShouldEnterPanicMode(6, 3) {
		t.Error("Should enter panic mode when desired/current >= threshold")
	}
	if calc.ShouldEnterPanicMode(3, 3) {
		t.Error("Should not enter panic mode when desired/current < threshold")
	}

	// Test should exit panic mode
	now := time.Now()
	panicStart := now.Add(-70 * time.Second) // Started 70s ago
	if !calc.ShouldExitPanicMode(panicStart, now, false) {
		t.Error("Should exit panic mode after stable window when not over threshold")
	}
	if calc.ShouldExitPanicMode(panicStart, now, true) {
		t.Error("Should not exit panic mode when still over threshold")
	}

	// Test calculate desired pods
	result := calc.CalculateDesiredPods(3, 5, true, 4)
	if result != 5 {
		t.Errorf("Expected 5 pods in panic mode, got %d", result)
	}
}

func TestSlidingWindowAutoscaler_Update(t *testing.T) {
	spec := api.AutoscalerSpec{
		MaxScaleUpRate:        10.0,
		MaxScaleDownRate:      2.0,
		ScalingMetric:         api.Concurrency,
		TargetValue:           100.0,
		TotalValue:            1000.0,
		TargetBurstCapacity:   200.0,
		PanicThreshold:        2.0,
		PanicWindowPercentage: 10.0,
		StableWindow:          60 * time.Second,
		ScaleDownDelay:        0,
		MinScale:              1,
		MaxScale:              10,
		ActivationScale:       1,
		Reachable:             true,
	}

	autoscaler := NewSlidingWindowAutoscaler(spec)

	// Update configuration
	newSpec := spec
	newSpec.TargetValue = 150.0
	newSpec.ScaleDownDelay = 30 * time.Second

	err := autoscaler.Update(newSpec)
	if err != nil {
		t.Errorf("Update failed: %v", err)
	}

	// Verify update
	currentSpec := autoscaler.GetSpec()
	if currentSpec.TargetValue != 150.0 {
		t.Errorf("Expected updated target value 150.0, got %v", currentSpec.TargetValue)
	}
	if currentSpec.ScaleDownDelay != 30*time.Second {
		t.Errorf("Expected updated scale down delay 30s, got %v", currentSpec.ScaleDownDelay)
	}
}
