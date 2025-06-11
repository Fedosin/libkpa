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
	"fmt"
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
	now := time.Now()

	// Test stable traffic - should maintain current scale
	snapshot := metrics.NewMetricSnapshot(
		300.0, // stable value
		300.0, // panic value
		3,     // current pods
		now,
	)

	result := autoscaler.Scale(snapshot, now)

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
	now := time.Now()

	// Test ramping up traffic
	snapshot := metrics.NewMetricSnapshot(
		500.0, // stable value
		600.0, // panic value
		3,     // current pods
		now,
	)

	result := autoscaler.Scale(snapshot, now)

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
	now := time.Now()

	// Test panic mode triggering - panic metric shows we need double the pods
	snapshot := metrics.NewMetricSnapshot(
		300.0, // stable value
		600.0, // panic value - needs 6 pods, we have 3
		3,     // current pods
		now,
	)

	result := autoscaler.Scale(snapshot, now)

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

	result2 := autoscaler.Scale(snapshot2, now.Add(10*time.Second))

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
	now := time.Now()

	// Start with high load
	snapshot1 := metrics.NewMetricSnapshot(
		500.0, // needs 5 pods
		500.0,
		5,
		now,
	)
	result1 := autoscaler.Scale(snapshot1, now)
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
	result2 := autoscaler.Scale(snapshot2, now.Add(10*time.Second))

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
	now := time.Now()

	// Test min scale
	snapshot1 := metrics.NewMetricSnapshot(
		50.0, // would need 1 pod
		50.0,
		3,
		now,
	)
	result1 := autoscaler.Scale(snapshot1, now)
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
	result2 := autoscaler.Scale(snapshot2, now)
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
	now := time.Now()

	// Test activation scale
	snapshot := metrics.NewMetricSnapshot(
		150.0, // would need 2 pods normally
		150.0,
		0, // scaling from zero
		now,
	)
	result := autoscaler.Scale(snapshot, now)

	// Should scale to activation scale instead of 2
	if result.DesiredPodCount != 3 {
		t.Errorf("Expected activation scale of 3 pods, got %d", result.DesiredPodCount)
	}
}

func TestSlidingWindowAutoscaler_ZeroValues(t *testing.T) {
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
	now := time.Now()

	// Test with zero values
	snapshot := metrics.NewMetricSnapshot(
		0.0,
		0.0,
		3,
		now,
	)

	result := autoscaler.Scale(snapshot, now)

	// Should return valid scale with zero values
	if !result.ScaleValid {
		t.Errorf("Expected valid scale with zero values, got %v", result)
	}

	// Should scale to min
	if result.DesiredPodCount != 1 {
		t.Errorf("Expected 1 pod, got %d", result.DesiredPodCount)
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

func TestSlidingWindowAutoscaler_RateLimiting(t *testing.T) {
	spec := api.AutoscalerSpec{
		MaxScaleUpRate:        1.5, // Can only scale up by 50%
		MaxScaleDownRate:      2.0, // Can scale down by 50%
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
	now := time.Now()

	// Test scale up rate limiting
	snapshot1 := metrics.NewMetricSnapshot(
		1000.0, // Would need 10 pods
		1000.0,
		4, // Current pods
		now,
	)
	result1 := autoscaler.Scale(snapshot1, now)
	// Should be limited to 4 * 1.5 = 6 pods
	if result1.DesiredPodCount != 6 {
		t.Errorf("Expected scale up to be rate-limited to 6 pods, got %d", result1.DesiredPodCount)
	}

	// Test scale down rate limiting
	snapshot2 := metrics.NewMetricSnapshot(
		100.0, // Would need 1 pod
		100.0,
		10, // Current pods
		now.Add(time.Minute),
	)
	result2 := autoscaler.Scale(snapshot2, now.Add(time.Minute))
	// Should be limited to 10 / 2.0 = 5 pods
	if result2.DesiredPodCount != 5 {
		t.Errorf("Expected scale down to be rate-limited to 5 pods, got %d", result2.DesiredPodCount)
	}
}

func TestSlidingWindowAutoscaler_UnreachableState(t *testing.T) {
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
		ActivationScale:       1,
		Reachable:             false, // Service is unreachable
	}

	autoscaler := NewSlidingWindowAutoscaler(spec)
	now := time.Now()

	// Test scaling to zero when unreachable with MinScale=0
	snapshot := metrics.NewMetricSnapshot(
		0.0,
		0.0,
		3, // Current pods
		now,
	)
	result := autoscaler.Scale(snapshot, now)

	// Result should be valid
	if !result.ScaleValid {
		t.Errorf("Expected valid scale with zero values, got %v", result)
	}
	// Should scale to zero
	if result.DesiredPodCount != 0 {
		t.Errorf("Expected 0 pod, got %d", result.DesiredPodCount)
	}

	// Test with MinScale > 0 and some traffic
	spec.MinScale = 2
	autoscaler.Update(spec)

	snapshot2 := metrics.NewMetricSnapshot(
		100.0, // Some traffic
		100.0,
		3, // Current pods
		now.Add(time.Second),
	)
	result2 := autoscaler.Scale(snapshot2, now.Add(time.Second))
	// Should maintain at least MinScale even if unreachable
	if result2.DesiredPodCount != 2 {
		t.Errorf("Expected MinScale pods for unreachable service, got %d", result2.DesiredPodCount)
	}
}

func TestSlidingWindowAutoscaler_RPSMetric(t *testing.T) {
	spec := api.AutoscalerSpec{
		MaxScaleUpRate:        10.0,
		MaxScaleDownRate:      2.0,
		ScalingMetric:         api.RPS, // Using RPS instead of Concurrency
		TargetValue:           50.0,    // 50 RPS per pod
		TotalValue:            100.0,   // Total capacity per pod
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
	now := time.Now()

	// Test with RPS metric
	snapshot := metrics.NewMetricSnapshot(
		250.0, // 250 RPS total
		250.0,
		3, // Current pods
		now,
	)
	result := autoscaler.Scale(snapshot, now)

	// Should scale to 250 / 50 = 5 pods
	if result.DesiredPodCount != 5 {
		t.Errorf("Expected 5 pods for 250 RPS with 50 RPS target, got %d", result.DesiredPodCount)
	}
}

func TestSlidingWindowAutoscaler_ScaleToZero(t *testing.T) {
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
		MinScale:              0, // Allow scale to zero
		MaxScale:              10,
		ActivationScale:       0, // No activation scale
		Reachable:             true,
	}

	autoscaler := NewSlidingWindowAutoscaler(spec)
	now := time.Now()

	// Test scaling to zero
	snapshot1 := metrics.NewMetricSnapshot(
		0.0,
		0.0,
		1, // Current pods
		now,
	)
	result1 := autoscaler.Scale(snapshot1, now)

	if !result1.ScaleValid {
		t.Errorf("Expected valid scale with zero values, got %v", result1)
	}

	if result1.DesiredPodCount != 0 {
		t.Errorf("Expected 0 pods with no traffic and MinScale=0, got %d", result1.DesiredPodCount)
	}
}

func TestSlidingWindowAutoscaler_RapidMetricChanges(t *testing.T) {
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
		ScaleDownDelay:        30 * time.Second,
		MinScale:              1,
		MaxScale:              10,
		ActivationScale:       1,
		Reachable:             true,
	}

	autoscaler := NewSlidingWindowAutoscaler(spec)
	now := time.Now()

	// Simulate rapid metric changes
	changes := []struct {
		value    float64
		expected int32
		desc     string
	}{
		{300.0, 3, "initial state"},
		{800.0, 8, "spike up"},
		{200.0, 8, "drop but within scale-down delay"},
		{900.0, 9, "spike up again"},
		{100.0, 9, "drop again but still in delay"},
	}

	currentTime := now
	for i, change := range changes {
		snapshot := metrics.NewMetricSnapshot(
			change.value,
			change.value,
			changes[max(0, i-1)].expected, // Use previous expected as current
			currentTime,
		)
		result := autoscaler.Scale(snapshot, currentTime)

		if result.DesiredPodCount != change.expected {
			t.Errorf("%s: expected %d pods, got %d", change.desc, change.expected, result.DesiredPodCount)
		}

		currentTime = currentTime.Add(5 * time.Second)
	}
}

func TestSlidingWindowAutoscaler_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		spec     api.AutoscalerSpec
		snapshot metrics.MetricSnapshot
		expected int32
		valid    bool
	}{
		{
			name: "zero target value",
			spec: api.AutoscalerSpec{
				MaxScaleUpRate:   10.0,
				MaxScaleDownRate: 2.0,
				ScalingMetric:    api.Concurrency,
				TargetValue:      0.0, // Zero target value
				TotalValue:       1000.0,
				MinScale:         1,
				MaxScale:         10,
				Reachable:        true,
			},
			snapshot: *metrics.NewMetricSnapshot(
				100.0, // stable value
				100.0, // panic value
				3,     // ready pods
				time.Now(),
			),
			expected: 10, // Should hit max scale due to infinity from division by zero
			valid:    true,
		},
		{
			name: "negative metric values",
			spec: api.AutoscalerSpec{
				MaxScaleUpRate:   10.0,
				MaxScaleDownRate: 2.0,
				ScalingMetric:    api.Concurrency,
				TargetValue:      100.0,
				TotalValue:       1000.0,
				MinScale:         1,
				MaxScale:         10,
				Reachable:        true,
			},
			snapshot: *metrics.NewMetricSnapshot(
				-50.0, // negative stable value
				-50.0, // negative panic value
				3,     // ready pods
				time.Now(),
			),
			expected: 0,
			valid:    false,
		},
		{
			name: "very large metric values",
			spec: api.AutoscalerSpec{
				MaxScaleUpRate:   10.0,
				MaxScaleDownRate: 2.0,
				ScalingMetric:    api.Concurrency,
				TargetValue:      100.0,
				TotalValue:       1000.0,
				MinScale:         1,
				MaxScale:         1000,
				Reachable:        true,
			},
			snapshot: *metrics.NewMetricSnapshot(
				1000000.0, // very large stable value
				1000000.0, // very large panic value
				10,        // ready pods
				time.Now(),
			),
			expected: 100, // Limited by rate limiting
			valid:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			autoscaler := NewSlidingWindowAutoscaler(tt.spec)
			now := time.Now()

			result := autoscaler.Scale(&tt.snapshot, now)

			if result.ScaleValid != tt.valid {
				t.Errorf("Expected valid=%v, got %v", tt.valid, result.ScaleValid)
			}
			if tt.valid && result.DesiredPodCount != tt.expected {
				t.Errorf("Expected %d pods, got %d", tt.expected, result.DesiredPodCount)
			}
		})
	}
}

func TestSlidingWindowAutoscaler_PanicModeTransitions(t *testing.T) {
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
	now := time.Now()

	// Enter panic mode
	snapshot1 := metrics.NewMetricSnapshot(
		300.0,
		600.0, // Panic value needs 6 pods
		3,     // Current pods
		now,
	)
	result1 := autoscaler.Scale(snapshot1, now)
	if !result1.InPanicMode {
		t.Error("Should enter panic mode")
	}

	// Stay in panic mode even with lower stable value
	snapshot2 := metrics.NewMetricSnapshot(
		200.0, // Stable value dropped
		400.0, // Still needs more pods than we have
		6,
		now.Add(30*time.Second),
	)
	result2 := autoscaler.Scale(snapshot2, now.Add(30*time.Second))
	if !result2.InPanicMode {
		t.Error("Should stay in panic mode")
	}

	// Exit panic mode after stable window
	snapshot3 := metrics.NewMetricSnapshot(
		300.0,
		300.0, // Panic value normalized
		6,
		now.Add(65*time.Second), // After stable window
	)
	result3 := autoscaler.Scale(snapshot3, now.Add(65*time.Second))
	if result3.InPanicMode {
		t.Error("Should exit panic mode after stable window")
	}
}

func TestSlidingWindowAutoscaler_InvalidMetricSnapshot(t *testing.T) {
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
	now := time.Now()

	snapshot1 := metrics.NewMetricSnapshot(
		-1.0,
		-1.0,
		3,
		now,
	)
	result1 := autoscaler.Scale(snapshot1, now)
	if result1.ScaleValid {
		t.Error("Expected invalid scale result with negative values")
	}

	snapshot2 := metrics.NewMetricSnapshot(
		-1.0,
		0.0,
		3,
		now,
	)
	result2 := autoscaler.Scale(snapshot2, now)
	if result2.ScaleValid {
		t.Error("Expected invalid scale result with negative values")
	}

	snapshot3 := metrics.NewMetricSnapshot(
		0.0,
		-1.0,
		3,
		now,
	)
	result3 := autoscaler.Scale(snapshot3, now)
	if result3.ScaleValid {
		t.Error("Expected invalid scale result with zero values")
	}
}

func TestSlidingWindowAutoscaler_ConcurrentAccess(t *testing.T) {
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

	// Run multiple goroutines to test concurrent access
	done := make(chan bool)
	errors := make(chan error, 10)

	for i := range 10 {
		go func(id int) {
			defer func() { done <- true }()

			now := time.Now()
			snapshot := metrics.NewMetricSnapshot(
				float64(100+id*10),
				float64(100+id*10),
				3,
				now,
			)

			// Perform scaling
			result := autoscaler.Scale(snapshot, now)
			if !result.ScaleValid {
				errors <- fmt.Errorf("goroutine %d: invalid scale result", id)
			}

			// Update spec
			newSpec := spec
			newSpec.TargetValue = float64(100 + id)
			if err := autoscaler.Update(newSpec); err != nil {
				errors <- fmt.Errorf("goroutine %d: update failed: %w", id, err)
			}
		}(i)
	}

	// Wait for all goroutines
	for range 10 {
		<-done
	}
	close(errors)

	// Check for errors
	for err := range errors {
		t.Error(err)
	}
}

func TestSlidingWindowAutoscaler_InvalidConfigurations(t *testing.T) {
	tests := []struct {
		name        string
		spec        api.AutoscalerSpec
		shouldError bool
	}{
		{
			name: "negative max scale up rate",
			spec: api.AutoscalerSpec{
				MaxScaleUpRate:   -1.0, // Invalid
				MaxScaleDownRate: 2.0,
				ScalingMetric:    api.Concurrency,
				TargetValue:      100.0,
				MinScale:         1,
				MaxScale:         10,
			},
			shouldError: false, // Should handle gracefully
		},
		{
			name: "min scale greater than max scale",
			spec: api.AutoscalerSpec{
				MaxScaleUpRate:   10.0,
				MaxScaleDownRate: 2.0,
				ScalingMetric:    api.Concurrency,
				TargetValue:      100.0,
				MinScale:         10,
				MaxScale:         5, // Less than MinScale
			},
			shouldError: false, // Should handle gracefully
		},
		{
			name: "negative stable window",
			spec: api.AutoscalerSpec{
				MaxScaleUpRate:   10.0,
				MaxScaleDownRate: 2.0,
				ScalingMetric:    api.Concurrency,
				TargetValue:      100.0,
				StableWindow:     -60 * time.Second, // Invalid
				MinScale:         1,
				MaxScale:         10,
			},
			shouldError: false, // Should handle gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic on creation
			autoscaler := NewSlidingWindowAutoscaler(tt.spec)
			if autoscaler == nil {
				t.Error("Expected autoscaler to be created")
			}
		})
	}
}
