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
)

// mockMetricSnapshot implements api.MetricSnapshot for testing
type mockMetricSnapshot struct {
	stableValue   float64
	panicValue    float64
	readyPodCount int32
	timestamp     time.Time
}

func (m *mockMetricSnapshot) StableValue() float64 { return m.stableValue }
func (m *mockMetricSnapshot) PanicValue() float64  { return m.panicValue }
func (m *mockMetricSnapshot) ReadyPodCount() int32 { return m.readyPodCount }
func (m *mockMetricSnapshot) Timestamp() time.Time { return m.timestamp }

// Test fixtures
func defaultConfig() api.AutoscalerConfig {
	return api.AutoscalerConfig{
		MaxScaleUpRate:        1000.0,
		MaxScaleDownRate:      2.0,
		TargetValue:           100.0,
		TotalValue:            1000.0,
		TargetBurstCapacity:   211.0,
		PanicThreshold:        2.0, // 200%
		PanicWindowPercentage: 10.0,
		StableWindow:          60 * time.Second,
		ScaleDownDelay:        0,
		MinScale:              0,
		MaxScale:              0,
		ActivationScale:       1,
		EnableScaleToZero:     true,
		Reachable:             true,
	}
}

// Tests for SlidingWindowAutoscaler
func TestNewSlidingWindowAutoscaler(t *testing.T) {
	tests := []struct {
		name         string
		config       api.AutoscalerConfig
		initialScale int32
		wantPanic    bool
	}{
		{
			name:         "initial scale 0",
			config:       defaultConfig(),
			initialScale: 0,
			wantPanic:    false,
		},
		{
			name:         "initial scale 1",
			config:       defaultConfig(),
			initialScale: 1,
			wantPanic:    false,
		},
		{
			name:         "initial scale > 1 starts in panic mode",
			config:       defaultConfig(),
			initialScale: 5,
			wantPanic:    true,
		},
		{
			name: "with scale down delay",
			config: func() api.AutoscalerConfig {
				c := defaultConfig()
				c.ScaleDownDelay = 10 * time.Second
				return c
			}(),
			initialScale: 1,
			wantPanic:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			autoscaler := NewSlidingWindowAutoscaler(tt.config, tt.initialScale)
			if autoscaler == nil {
				t.Fatal("expected non-nil autoscaler")
			}
			if tt.wantPanic && autoscaler.panicTime.IsZero() {
				t.Error("expected to start in panic mode")
			}
			if !tt.wantPanic && !autoscaler.panicTime.IsZero() {
				t.Error("expected not to start in panic mode")
			}
			if tt.wantPanic && autoscaler.maxPanicPods != tt.initialScale {
				t.Errorf("expected maxPanicPods=%d, got %d", tt.initialScale, autoscaler.maxPanicPods)
			}
		})
	}
}

func TestSlidingWindowAutoscaler_Scale_NoData(t *testing.T) {
	autoscaler := NewSlidingWindowAutoscaler(defaultConfig(), 1)
	now := time.Now()

	// Test with negative stable value
	snapshot := &mockMetricSnapshot{
		stableValue:   -1,
		panicValue:    100,
		readyPodCount: 1,
		timestamp:     now,
	}

	recommendation := autoscaler.Scale(snapshot, now)
	if recommendation.ScaleValid {
		t.Error("expected invalid recommendation with negative stable value")
	}

	// Test with negative panic value
	snapshot = &mockMetricSnapshot{
		stableValue:   100,
		panicValue:    -1,
		readyPodCount: 1,
		timestamp:     now,
	}

	recommendation = autoscaler.Scale(snapshot, now)
	if recommendation.ScaleValid {
		t.Error("expected invalid recommendation with negative panic value")
	}
}

func TestSlidingWindowAutoscaler_Scale_BasicScaling(t *testing.T) {
	tests := []struct {
		name              string
		config            api.AutoscalerConfig
		snapshot          mockMetricSnapshot
		expectedPodCount  int32
		expectedPanicMode bool
		expectedEBC       int32 // excess burst capacity
	}{
		{
			name:   "scale up based on stable value",
			config: defaultConfig(),
			snapshot: mockMetricSnapshot{
				stableValue:   250, // 2.5x target
				panicValue:    250,
				readyPodCount: 2, // start with 2 pods instead of 1
			},
			expectedPodCount:  3, // ceil(250/100)
			expectedPanicMode: false,
			expectedEBC:       1539, // floor(2*1000 - 211 - 250)
		},
		{
			name:   "scale down based on stable value",
			config: defaultConfig(),
			snapshot: mockMetricSnapshot{
				stableValue:   50, // 0.5x target
				panicValue:    50,
				readyPodCount: 5,
			},
			expectedPodCount:  2, // limited by max scale down rate (5/2.0)
			expectedPanicMode: false,
			expectedEBC:       4739, // floor(5*1000 - 211 - 50)
		},
		{
			name: "respect min scale",
			config: func() api.AutoscalerConfig {
				c := defaultConfig()
				c.MinScale = 3
				return c
			}(),
			snapshot: mockMetricSnapshot{
				stableValue:   50,
				panicValue:    50,
				readyPodCount: 5,
			},
			expectedPodCount:  3, // min scale
			expectedPanicMode: false,
		},
		{
			name: "respect max scale",
			config: func() api.AutoscalerConfig {
				c := defaultConfig()
				c.MaxScale = 10
				return c
			}(),
			snapshot: mockMetricSnapshot{
				stableValue:   800, // would require 8 pods
				panicValue:    800,
				readyPodCount: 5,
			},
			expectedPodCount:  8, // not limited by max scale yet
			expectedPanicMode: false,
		},
		{
			name: "activation scale",
			config: func() api.AutoscalerConfig {
				c := defaultConfig()
				c.ActivationScale = 3
				return c
			}(),
			snapshot: mockMetricSnapshot{
				stableValue:   50, // would only need 1 pod
				panicValue:    50,
				readyPodCount: 1,
			},
			expectedPodCount:  3, // activation scale
			expectedPanicMode: false,
		},
		{
			name: "zero target value",
			config: func() api.AutoscalerConfig {
				c := defaultConfig()
				c.TargetValue = 0
				c.MaxScale = 100
				return c
			}(),
			snapshot: mockMetricSnapshot{
				stableValue:   50,
				panicValue:    50,
				readyPodCount: 1,
			},
			expectedPodCount:  100,  // limited by max scale
			expectedPanicMode: true, // zero target value triggers panic mode
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			autoscaler := NewSlidingWindowAutoscaler(tt.config, 1)
			now := time.Now()
			tt.snapshot.timestamp = now

			recommendation := autoscaler.Scale(&tt.snapshot, now)

			if !recommendation.ScaleValid {
				t.Fatal("expected valid recommendation")
			}
			if recommendation.DesiredPodCount != tt.expectedPodCount {
				t.Errorf("expected pod count %d, got %d", tt.expectedPodCount, recommendation.DesiredPodCount)
			}
			if recommendation.InPanicMode != tt.expectedPanicMode {
				t.Errorf("expected panic mode %v, got %v", tt.expectedPanicMode, recommendation.InPanicMode)
			}
			if tt.expectedEBC != 0 && (tt.name == "scale up based on stable value" || tt.name == "scale down based on stable value") {
				if recommendation.ExcessBurstCapacity != tt.expectedEBC {
					t.Errorf("expected excess burst capacity %d, got %d", tt.expectedEBC, recommendation.ExcessBurstCapacity)
				}
			}
		})
	}
}

func TestSlidingWindowAutoscaler_Scale_PanicMode(t *testing.T) {
	config := defaultConfig()
	autoscaler := NewSlidingWindowAutoscaler(config, 1)
	now := time.Now()

	// Test entering panic mode
	snapshot := &mockMetricSnapshot{
		stableValue:   100,
		panicValue:    500, // 5x current capacity, exceeds 2x threshold
		readyPodCount: 2,
		timestamp:     now,
	}

	recommendation := autoscaler.Scale(snapshot, now)
	if !recommendation.InPanicMode {
		t.Error("expected to enter panic mode")
	}
	if recommendation.DesiredPodCount != 5 {
		t.Errorf("expected pod count 5, got %d", recommendation.DesiredPodCount)
	}

	// Test staying in panic mode (no scale down)
	now = now.Add(30 * time.Second)
	snapshot = &mockMetricSnapshot{
		stableValue:   100,
		panicValue:    100,
		readyPodCount: 5,
		timestamp:     now,
	}

	recommendation = autoscaler.Scale(snapshot, now)
	if !recommendation.InPanicMode {
		t.Error("expected to stay in panic mode")
	}
	if recommendation.DesiredPodCount != 5 {
		t.Errorf("expected pod count to remain at 5, got %d", recommendation.DesiredPodCount)
	}

	// Test exiting panic mode after stable window
	now = now.Add(config.StableWindow + time.Second)
	recommendation = autoscaler.Scale(snapshot, now)
	if recommendation.InPanicMode {
		t.Error("expected to exit panic mode")
	}
	if recommendation.DesiredPodCount != 2 {
		t.Errorf("expected pod count 2 after exiting panic mode, got %d", recommendation.DesiredPodCount)
	}
}

func TestSlidingWindowAutoscaler_Scale_RateLimits(t *testing.T) {
	config := defaultConfig()
	config.MaxScaleUpRate = 2.0   // Can double
	config.MaxScaleDownRate = 2.0 // Can halve

	autoscaler := NewSlidingWindowAutoscaler(config, 1)
	now := time.Now()

	// Test scale up rate limit
	snapshot := &mockMetricSnapshot{
		stableValue:   1000, // Would need 10 pods
		panicValue:    1000,
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
		panicValue:    50,
		readyPodCount: 8,
		timestamp:     now,
	}

	recommendation = autoscaler.Scale(snapshot, now)
	if recommendation.DesiredPodCount != 4 {
		t.Errorf("expected pod count limited to 4 (8/2), got %d", recommendation.DesiredPodCount)
	}
}

func TestSlidingWindowAutoscaler_Update(t *testing.T) {
	autoscaler := NewSlidingWindowAutoscaler(defaultConfig(), 1)

	newConfig := defaultConfig()
	newConfig.TargetValue = 200
	newConfig.MaxScale = 50
	newConfig.ScaleDownDelay = 5 * time.Second

	err := autoscaler.Update(newConfig)
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
	config := defaultConfig()
	config.MinScale = 0
	config.EnableScaleToZero = true

	autoscaler := NewSlidingWindowAutoscaler(config, 1)
	now := time.Now()

	// Test scaling to zero with no load
	snapshot := &mockMetricSnapshot{
		stableValue:   0,
		panicValue:    0,
		readyPodCount: 1,
		timestamp:     now,
	}

	recommendation := autoscaler.Scale(snapshot, now)
	if recommendation.DesiredPodCount != 0 {
		t.Errorf("expected to scale to 0, got %d", recommendation.DesiredPodCount)
	}
}

func TestSlidingWindowAutoscaler_Scale_NotReachable(t *testing.T) {
	config := defaultConfig()
	config.Reachable = false

	autoscaler := NewSlidingWindowAutoscaler(config, 1)
	now := time.Now()

	// When not reachable, maxScaleDown should be 0
	snapshot := &mockMetricSnapshot{
		stableValue:   10, // very low load
		panicValue:    10,
		readyPodCount: 5,
		timestamp:     now,
	}

	recommendation := autoscaler.Scale(snapshot, now)
	if recommendation.DesiredPodCount != 1 {
		t.Errorf("expected pod count 1 (no scale down limit when not reachable), got %d", recommendation.DesiredPodCount)
	}
}

func TestSlidingWindowAutoscaler_Scale_ActivationScaleWithZeroMetrics(t *testing.T) {
	config := defaultConfig()
	config.ActivationScale = 3

	autoscaler := NewSlidingWindowAutoscaler(config, 1)
	now := time.Now()

	// Test that activation scale doesn't apply when metrics are zero
	snapshot := &mockMetricSnapshot{
		stableValue:   0,
		panicValue:    0,
		readyPodCount: 1,
		timestamp:     now,
	}

	recommendation := autoscaler.Scale(snapshot, now)
	if recommendation.DesiredPodCount != 0 {
		t.Errorf("expected 0 pods (activation scale shouldn't apply with zero metrics), got %d", recommendation.DesiredPodCount)
	}
}

func TestSlidingWindowAutoscaler_Scale_ReadyPodCountZero(t *testing.T) {
	autoscaler := NewSlidingWindowAutoscaler(defaultConfig(), 1)
	now := time.Now()

	// Test with zero ready pods (should default to 1 to avoid division by zero)
	snapshot := &mockMetricSnapshot{
		stableValue:   100,
		panicValue:    100,
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

func TestCalculateExcessBurstCapacity(t *testing.T) {
	tests := []struct {
		name                string
		readyPods           int32
		totalValue          float64
		targetBurstCapacity float64
		observedPanicValue  float64
		expected            int32
	}{
		{
			name:                "positive excess capacity",
			readyPods:           5,
			totalValue:          1000,
			targetBurstCapacity: 211,
			observedPanicValue:  500,
			expected:            4289, // floor(5*1000 - 211 - 500)
		},
		{
			name:                "negative excess capacity",
			readyPods:           1,
			totalValue:          1000,
			targetBurstCapacity: 211,
			observedPanicValue:  1000,
			expected:            -211, // floor(1*1000 - 211 - 1000)
		},
		{
			name:                "zero target burst capacity",
			readyPods:           5,
			totalValue:          1000,
			targetBurstCapacity: 0,
			observedPanicValue:  500,
			expected:            0,
		},
		{
			name:                "negative target burst capacity (unlimited)",
			readyPods:           5,
			totalValue:          1000,
			targetBurstCapacity: -1,
			observedPanicValue:  500,
			expected:            -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateExcessBurstCapacity(tt.readyPods, tt.totalValue, tt.targetBurstCapacity, tt.observedPanicValue)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// Tests for PanicModeCalculator
func TestNewPanicModeCalculator(t *testing.T) {
	config := defaultConfig()
	calculator := NewPanicModeCalculator(&config)

	if calculator == nil {
		t.Fatal("expected non-nil calculator")
	}
	if calculator.config != &config {
		t.Error("config not set correctly")
	}
}

func TestPanicModeCalculator_CalculatePanicWindow(t *testing.T) {
	tests := []struct {
		name           string
		stableWindow   time.Duration
		panicPercent   float64
		expectedWindow time.Duration
	}{
		{
			name:           "10% of 60s",
			stableWindow:   60 * time.Second,
			panicPercent:   10.0,
			expectedWindow: 6 * time.Second,
		},
		{
			name:           "50% of 60s",
			stableWindow:   60 * time.Second,
			panicPercent:   50.0,
			expectedWindow: 30 * time.Second,
		},
		{
			name:           "100% of 60s",
			stableWindow:   60 * time.Second,
			panicPercent:   100.0,
			expectedWindow: 60 * time.Second,
		},
		{
			name:           "5% of 120s",
			stableWindow:   120 * time.Second,
			panicPercent:   5.0,
			expectedWindow: 6 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := defaultConfig()
			config.StableWindow = tt.stableWindow
			config.PanicWindowPercentage = tt.panicPercent

			calculator := NewPanicModeCalculator(&config)
			result := calculator.CalculatePanicWindow()

			if result != tt.expectedWindow {
				t.Errorf("expected %v, got %v", tt.expectedWindow, result)
			}
		})
	}
}

func TestPanicModeCalculator_ShouldEnterPanicMode(t *testing.T) {
	config := defaultConfig()
	config.PanicThreshold = 2.0 // 200%
	calculator := NewPanicModeCalculator(&config)

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
			result := calculator.ShouldEnterPanicMode(tt.desiredPods, tt.currentPods)
			if result != tt.shouldEnter {
				t.Errorf("expected %v, got %v", tt.shouldEnter, result)
			}
		})
	}
}

func TestPanicModeCalculator_ShouldExitPanicMode(t *testing.T) {
	config := defaultConfig()
	config.StableWindow = 60 * time.Second
	calculator := NewPanicModeCalculator(&config)

	now := time.Now()
	panicStartTime := now.Add(-30 * time.Second)

	tests := []struct {
		name        string
		panicStart  time.Time
		currentTime time.Time
		overThresh  bool
		shouldExit  bool
	}{
		{
			name:        "still over threshold",
			panicStart:  panicStartTime,
			currentTime: now,
			overThresh:  true,
			shouldExit:  false,
		},
		{
			name:        "below threshold but not enough time",
			panicStart:  panicStartTime,
			currentTime: now,
			overThresh:  false,
			shouldExit:  false, // only 30s passed, need 60s
		},
		{
			name:        "below threshold and enough time passed",
			panicStart:  now.Add(-70 * time.Second),
			currentTime: now,
			overThresh:  false,
			shouldExit:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculator.ShouldExitPanicMode(tt.panicStart, tt.currentTime, tt.overThresh)
			if result != tt.shouldExit {
				t.Errorf("expected %v, got %v", tt.shouldExit, result)
			}
		})
	}
}

func TestPanicModeCalculator_CalculateDesiredPods(t *testing.T) {
	config := defaultConfig()
	calculator := NewPanicModeCalculator(&config)

	tests := []struct {
		name          string
		stableDesired int32
		panicDesired  int32
		inPanicMode   bool
		maxPanicPods  int32
		expected      int32
	}{
		{
			name:          "not in panic mode",
			stableDesired: 5,
			panicDesired:  10,
			inPanicMode:   false,
			maxPanicPods:  8,
			expected:      5, // use stable
		},
		{
			name:          "panic mode - panic higher",
			stableDesired: 5,
			panicDesired:  10,
			inPanicMode:   true,
			maxPanicPods:  8,
			expected:      10, // use panic (higher)
		},
		{
			name:          "panic mode - stable higher",
			stableDesired: 10,
			panicDesired:  5,
			inPanicMode:   true,
			maxPanicPods:  8,
			expected:      10, // use stable (higher)
		},
		{
			name:          "panic mode - prevent scale down",
			stableDesired: 3,
			panicDesired:  4,
			inPanicMode:   true,
			maxPanicPods:  8,
			expected:      8, // maintain max panic pods
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculator.CalculateDesiredPods(tt.stableDesired, tt.panicDesired, tt.inPanicMode, tt.maxPanicPods)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}
