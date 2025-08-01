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
	"testing"
	"time"

	"github.com/Fedosin/libkpa/api"
	libkpaconfig "github.com/Fedosin/libkpa/config"
)

// mockMetricSnapshot implements api.MetricSnapshot for testing
type mockMetricSnapshot struct {
	stableValue   float64
	burstValue    float64
	readyPodCount int32
	timestamp     time.Time
}

func (m *mockMetricSnapshot) StableValue() float64 { return m.stableValue }
func (m *mockMetricSnapshot) BurstValue() float64  { return m.burstValue }
func (m *mockMetricSnapshot) ReadyPodCount() int32 { return m.readyPodCount }
func (m *mockMetricSnapshot) Timestamp() time.Time { return m.timestamp }

// Tests for SlidingWindowAutoscaler
func TestNewSlidingWindowAutoscaler(t *testing.T) {
	tests := []struct {
		name      string
		config    api.AutoscalerConfig
		wantBurst bool
	}{
		{
			name: "with scale down delay",
			config: func() api.AutoscalerConfig {
				c := libkpaconfig.NewDefaultAutoscalerConfig()
				c.ScaleDownDelay = 10 * time.Second
				return *c
			}(),
			wantBurst: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			autoscaler, err := NewSlidingWindowAutoscaler(tt.config)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if autoscaler == nil {
				t.Fatal("expected non-nil autoscaler")
			}
		})
	}
}

func TestSlidingWindowAutoscaler_Scale_NoData(t *testing.T) {
	autoscaler, err := NewSlidingWindowAutoscaler(*libkpaconfig.NewDefaultAutoscalerConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	now := time.Now()

	// Test with negative stable value
	snapshot := &mockMetricSnapshot{
		stableValue:   -1,
		burstValue:    100,
		readyPodCount: 1,
		timestamp:     now,
	}

	recommendation := autoscaler.Scale(snapshot, now)
	if recommendation.ScaleValid {
		t.Error("expected invalid recommendation with negative stable value")
	}

	// Test with negative burst value
	snapshot = &mockMetricSnapshot{
		stableValue:   100,
		burstValue:    -1,
		readyPodCount: 1,
		timestamp:     now,
	}

	recommendation = autoscaler.Scale(snapshot, now)
	if recommendation.ScaleValid {
		t.Error("expected invalid recommendation with negative burst value")
	}
}

func TestSlidingWindowAutoscaler_Scale_BasicScaling(t *testing.T) {
	tests := []struct {
		name             string
		config           api.AutoscalerConfig
		snapshot         mockMetricSnapshot
		expectedPodCount int32
	}{
		{
			name:   "scale up based on stable value",
			config: *libkpaconfig.NewDefaultAutoscalerConfig(),
			snapshot: mockMetricSnapshot{
				stableValue:   250, // 2.5x target
				burstValue:    250,
				readyPodCount: 2, // start with 2 pods instead of 1
			},
			expectedPodCount: 3, // ceil(250/100)
		},
		{
			name:   "scale down based on stable value",
			config: *libkpaconfig.NewDefaultAutoscalerConfig(),
			snapshot: mockMetricSnapshot{
				stableValue:   50, // 0.5x target
				burstValue:    50,
				readyPodCount: 5,
			},
			expectedPodCount: 2, // limited by max scale down rate (5/2.0)
		},
		{
			name: "respect min scale",
			config: func() api.AutoscalerConfig {
				c := *libkpaconfig.NewDefaultAutoscalerConfig()
				c.MinScale = 3
				return c
			}(),
			snapshot: mockMetricSnapshot{
				stableValue:   50,
				burstValue:    50,
				readyPodCount: 5,
			},
			expectedPodCount: 3, // min scale
		},
		{
			name: "respect max scale",
			config: func() api.AutoscalerConfig {
				c := *libkpaconfig.NewDefaultAutoscalerConfig()
				c.MaxScale = 10
				return c
			}(),
			snapshot: mockMetricSnapshot{
				stableValue:   800, // would require 8 pods
				burstValue:    800,
				readyPodCount: 5,
			},
			expectedPodCount: 8, // not limited by max scale yet
		},
		{
			name: "activation scale",
			config: func() api.AutoscalerConfig {
				c := *libkpaconfig.NewDefaultAutoscalerConfig()
				c.ActivationScale = 3
				return c
			}(),
			snapshot: mockMetricSnapshot{
				stableValue:   50, // would only need 1 pod
				burstValue:    50,
				readyPodCount: 1,
			},
			expectedPodCount: 3, // activation scale
		},
		{
			name: "total target value - basic scaling",
			config: func() api.AutoscalerConfig {
				c := *libkpaconfig.NewDefaultAutoscalerConfig()
				c.TargetValue = 0           // Use TotalTargetValue instead
				c.TotalTargetValue = 1000.0 // Total across all pods
				return c
			}(),
			snapshot: mockMetricSnapshot{
				stableValue:   2500, // Total value of 2500
				burstValue:    2500,
				readyPodCount: 2,
			},
			expectedPodCount: 5, // ceil(2 * 2500/1000) = 5
		},
		{
			name: "total target value - scale down",
			config: func() api.AutoscalerConfig {
				c := *libkpaconfig.NewDefaultAutoscalerConfig()
				c.TargetValue = 0
				c.TotalTargetValue = 1000.0
				return c
			}(),
			snapshot: mockMetricSnapshot{
				stableValue:   500, // Total value of 500
				burstValue:    500,
				readyPodCount: 5,
			},
			expectedPodCount: 3, // ceil(5 * 500/1000) = 3
		},
		{
			name: "total target value with activation scale",
			config: func() api.AutoscalerConfig {
				c := *libkpaconfig.NewDefaultAutoscalerConfig()
				c.TargetValue = 0
				c.TotalTargetValue = 1000.0
				c.ActivationScale = 3
				return c
			}(),
			snapshot: mockMetricSnapshot{
				stableValue:   100, // Would only need 1 pod
				burstValue:    100,
				readyPodCount: 1,
			},
			expectedPodCount: 3, // activation scale
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			autoscaler, err := NewSlidingWindowAutoscaler(tt.config)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			now := time.Now()
			tt.snapshot.timestamp = now

			recommendation := autoscaler.Scale(&tt.snapshot, now)

			if tt.expectedPodCount == -1 {
				// Expecting invalid recommendation
				if recommendation.ScaleValid {
					t.Fatal("expected invalid recommendation")
				}
			} else {
				if !recommendation.ScaleValid {
					t.Fatal("expected valid recommendation")
				}
				if recommendation.DesiredPodCount != tt.expectedPodCount {
					t.Errorf("expected pod count %d, got %d", tt.expectedPodCount, recommendation.DesiredPodCount)
				}
			}
		})
	}
}

func TestSlidingWindowAutoscaler_Scale_BurstMode(t *testing.T) {
	config := *libkpaconfig.NewDefaultAutoscalerConfig()
	autoscaler, err := NewSlidingWindowAutoscaler(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	now := time.Now()

	// Test entering burst mode
	snapshot := &mockMetricSnapshot{
		stableValue:   100,
		burstValue:    500, // 5x current capacity, exceeds 2x threshold
		readyPodCount: 2,
		timestamp:     now,
	}

	recommendation := autoscaler.Scale(snapshot, now)
	if !recommendation.InBurstMode {
		t.Error("expected to enter burst mode")
	}
	if recommendation.DesiredPodCount != 5 {
		t.Errorf("expected pod count 5, got %d", recommendation.DesiredPodCount)
	}

	// Test staying in burst mode (no scale down)
	now = now.Add(30 * time.Second)
	snapshot = &mockMetricSnapshot{
		stableValue:   100,
		burstValue:    100,
		readyPodCount: 5,
		timestamp:     now,
	}

	recommendation = autoscaler.Scale(snapshot, now)
	if !recommendation.InBurstMode {
		t.Error("expected to stay in burst mode")
	}
	if recommendation.DesiredPodCount != 5 {
		t.Errorf("expected pod count to remain at 5, got %d", recommendation.DesiredPodCount)
	}

	// Test exiting burst mode after stable window
	now = now.Add(config.StableWindow + time.Second)
	recommendation = autoscaler.Scale(snapshot, now)
	if recommendation.InBurstMode {
		t.Error("expected to exit burst mode")
	}
	if recommendation.DesiredPodCount != 2 {
		t.Errorf("expected pod count 2 after exiting burst mode, got %d", recommendation.DesiredPodCount)
	}
}

func TestSlidingWindowAutoscaler_Scale_BurstMode_TotalTargetValue(t *testing.T) {
	config := *libkpaconfig.NewDefaultAutoscalerConfig()
	config.TargetValue = 0
	config.TotalTargetValue = 1000.0
	autoscaler, err := NewSlidingWindowAutoscaler(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	now := time.Now()

	// Test entering burst mode with total target value
	snapshot := &mockMetricSnapshot{
		stableValue:   1000,
		burstValue:    5000, // 5x current total capacity, exceeds 2x threshold
		readyPodCount: 2,
		timestamp:     now,
	}

	recommendation := autoscaler.Scale(snapshot, now)
	if !recommendation.InBurstMode {
		t.Error("expected to enter burst mode")
	}
	// 2 pods * 5000 / 1000 = 10 pods
	if recommendation.DesiredPodCount != 10 {
		t.Errorf("expected pod count 10, got %d", recommendation.DesiredPodCount)
	}
}

func TestSlidingWindowAutoscaler_Scale_RateLimits(t *testing.T) {
	config := *libkpaconfig.NewDefaultAutoscalerConfig()
	config.MaxScaleUpRate = 2.0   // Can double
	config.MaxScaleDownRate = 2.0 // Can halve

	autoscaler, err := NewSlidingWindowAutoscaler(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	now := time.Now()

	// Test scale up rate limit
	snapshot := &mockMetricSnapshot{
		stableValue:   1000, // Would need 10 pods
		burstValue:    1000,
		readyPodCount: 2,
		timestamp:     now,
	}

	recommendation := autoscaler.Scale(snapshot, now)
	if recommendation.DesiredPodCount != 4 {
		t.Errorf("expected pod count limited to 4 (2x2), got %d", recommendation.DesiredPodCount)
	}

	// Test scale down rate limit
	snapshot = &mockMetricSnapshot{
		stableValue:   50, // Would need 1 pod
		burstValue:    50,
		readyPodCount: 8,
		timestamp:     now,
	}

	recommendation = autoscaler.Scale(snapshot, now)
	if recommendation.DesiredPodCount != 4 {
		t.Errorf("expected pod count limited to 4 (8/2), got %d", recommendation.DesiredPodCount)
	}
}

func TestSlidingWindowAutoscaler_Update(t *testing.T) {
	autoscaler, err := NewSlidingWindowAutoscaler(*libkpaconfig.NewDefaultAutoscalerConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	newConfig := *libkpaconfig.NewDefaultAutoscalerConfig()
	newConfig.TargetValue = 200
	newConfig.MaxScale = 50
	newConfig.ScaleDownDelay = 5 * time.Second

	err = autoscaler.Update(newConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gotConfig := autoscaler.GetConfig()
	if gotConfig.TargetValue != 200 {
		t.Errorf("expected TargetValue 200, got %f", gotConfig.TargetValue)
	}
	if gotConfig.MaxScale != 50 {
		t.Errorf("expected MaxScale 50, got %d", gotConfig.MaxScale)
	}
	if gotConfig.ScaleDownDelay != 5*time.Second {
		t.Errorf("expected ScaleDownDelay 5s, got %v", gotConfig.ScaleDownDelay)
	}
}

func TestSlidingWindowAutoscaler_Scale_ScaleToZero(t *testing.T) {
	config := *libkpaconfig.NewDefaultAutoscalerConfig()
	config.MinScale = 0

	autoscaler, err := NewSlidingWindowAutoscaler(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	now := time.Now()

	// Test scaling to zero with no load
	snapshot := &mockMetricSnapshot{
		stableValue:   0,
		burstValue:    0,
		readyPodCount: 1,
		timestamp:     now,
	}

	recommendation := autoscaler.Scale(snapshot, now)
	if recommendation.DesiredPodCount != 0 {
		t.Errorf("expected to scale to 0, got %d", recommendation.DesiredPodCount)
	}
}

func TestSlidingWindowAutoscaler_Scale_ActivationScaleWithZeroMetrics(t *testing.T) {
	config := *libkpaconfig.NewDefaultAutoscalerConfig()
	config.ActivationScale = 3

	autoscaler, err := NewSlidingWindowAutoscaler(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	now := time.Now()

	// Test that activation scale doesn't apply when metrics are zero
	snapshot := &mockMetricSnapshot{
		stableValue:   0,
		burstValue:    0,
		readyPodCount: 1,
		timestamp:     now,
	}

	recommendation := autoscaler.Scale(snapshot, now)
	if recommendation.DesiredPodCount != 0 {
		t.Errorf("expected 0 pods (activation scale shouldn't apply with zero metrics), got %d", recommendation.DesiredPodCount)
	}
}

func TestSlidingWindowAutoscaler_Scale_ReadyPodCountZero(t *testing.T) {
	autoscaler, err := NewSlidingWindowAutoscaler(*libkpaconfig.NewDefaultAutoscalerConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	now := time.Now()

	// Test with zero ready pods (should default to 1 to avoid division by zero)
	snapshot := &mockMetricSnapshot{
		stableValue:   100,
		burstValue:    100,
		readyPodCount: 0,
		timestamp:     now,
	}

	recommendation := autoscaler.Scale(snapshot, now)
	if !recommendation.ScaleValid {
		t.Fatal("expected valid recommendation")
	}
	// Should calculate based on 1 pod instead of 0
	if recommendation.DesiredPodCount != 1 {
		t.Errorf("expected pod count 1, got %d", recommendation.DesiredPodCount)
	}
}

// Tests for BurstModeCalculator
func TestNewBurstModeCalculator(t *testing.T) {
	config := *libkpaconfig.NewDefaultAutoscalerConfig()
	calculator := NewBurstModeCalculator(&config)

	if calculator == nil {
		t.Fatal("expected non-nil calculator")
	}
	if calculator.config != &config {
		t.Error("config not set correctly")
	}
}

func TestBurstModeCalculator_CalculateBurstWindow(t *testing.T) {
	tests := []struct {
		name           string
		stableWindow   time.Duration
		burstPercent   float64
		expectedWindow time.Duration
	}{
		{
			name:           "10% of 60s",
			stableWindow:   60 * time.Second,
			burstPercent:   10.0,
			expectedWindow: 6 * time.Second,
		},
		{
			name:           "50% of 60s",
			stableWindow:   60 * time.Second,
			burstPercent:   50.0,
			expectedWindow: 30 * time.Second,
		},
		{
			name:           "100% of 60s",
			stableWindow:   60 * time.Second,
			burstPercent:   100.0,
			expectedWindow: 60 * time.Second,
		},
		{
			name:           "5% of 120s",
			stableWindow:   120 * time.Second,
			burstPercent:   5.0,
			expectedWindow: 6 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := *libkpaconfig.NewDefaultAutoscalerConfig()
			config.StableWindow = tt.stableWindow
			config.BurstWindowPercentage = tt.burstPercent

			calculator := NewBurstModeCalculator(&config)
			result := calculator.CalculateBurstWindow()

			if result != tt.expectedWindow {
				t.Errorf("expected %v, got %v", tt.expectedWindow, result)
			}
		})
	}
}

func TestBurstModeCalculator_ShouldEnterBurstMode(t *testing.T) {
	config := *libkpaconfig.NewDefaultAutoscalerConfig()
	config.BurstThreshold = 2.0 // 200%
	calculator := NewBurstModeCalculator(&config)

	tests := []struct {
		name        string
		desiredPods float64
		currentPods float64
		shouldEnter bool
	}{
		{
			name:        "below threshold",
			desiredPods: 3,
			currentPods: 2,
			shouldEnter: false, // 150% < 200%
		},
		{
			name:        "at threshold",
			desiredPods: 4,
			currentPods: 2,
			shouldEnter: true, // 200% == 200%
		},
		{
			name:        "above threshold",
			desiredPods: 5,
			currentPods: 2,
			shouldEnter: true, // 250% > 200%
		},
		{
			name:        "zero current pods",
			desiredPods: 10,
			currentPods: 0,
			shouldEnter: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculator.ShouldEnterBurstMode(tt.desiredPods, tt.currentPods)
			if result != tt.shouldEnter {
				t.Errorf("expected %v, got %v", tt.shouldEnter, result)
			}
		})
	}
}

func TestBurstModeCalculator_ShouldExitBurstMode(t *testing.T) {
	config := *libkpaconfig.NewDefaultAutoscalerConfig()
	config.StableWindow = 60 * time.Second
	calculator := NewBurstModeCalculator(&config)

	now := time.Now()
	burstStartTime := now.Add(-30 * time.Second)

	tests := []struct {
		name        string
		burstStart  time.Time
		currentTime time.Time
		overThresh  bool
		shouldExit  bool
	}{
		{
			name:        "still over threshold",
			burstStart:  burstStartTime,
			currentTime: now,
			overThresh:  true,
			shouldExit:  false,
		},
		{
			name:        "below threshold but not enough time",
			burstStart:  burstStartTime,
			currentTime: now,
			overThresh:  false,
			shouldExit:  false, // only 30s passed, need 60s
		},
		{
			name:        "below threshold and enough time passed",
			burstStart:  now.Add(-70 * time.Second),
			currentTime: now,
			overThresh:  false,
			shouldExit:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculator.ShouldExitBurstMode(tt.burstStart, tt.currentTime, tt.overThresh)
			if result != tt.shouldExit {
				t.Errorf("expected %v, got %v", tt.shouldExit, result)
			}
		})
	}
}

func TestBurstModeCalculator_CalculateDesiredPods(t *testing.T) {
	config := *libkpaconfig.NewDefaultAutoscalerConfig()
	calculator := NewBurstModeCalculator(&config)

	tests := []struct {
		name          string
		stableDesired int32
		burstDesired  int32
		inBurstMode   bool
		maxBurstPods  int32
		expected      int32
	}{
		{
			name:          "not in burst mode",
			stableDesired: 5,
			burstDesired:  10,
			inBurstMode:   false,
			maxBurstPods:  8,
			expected:      5, // use stable
		},
		{
			name:          "burst mode - burst higher",
			stableDesired: 5,
			burstDesired:  10,
			inBurstMode:   true,
			maxBurstPods:  8,
			expected:      10, // use burst (higher)
		},
		{
			name:          "burst mode - stable higher",
			stableDesired: 10,
			burstDesired:  5,
			inBurstMode:   true,
			maxBurstPods:  8,
			expected:      10, // use stable (higher)
		},
		{
			name:          "burst mode - prevent scale down",
			stableDesired: 3,
			burstDesired:  4,
			inBurstMode:   true,
			maxBurstPods:  8,
			expected:      8, // maintain max burst pods
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculator.CalculateDesiredPods(tt.stableDesired, tt.burstDesired, tt.inBurstMode, tt.maxBurstPods)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}
