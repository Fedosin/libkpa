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

package manager

import (
	"testing"
	"time"

	libkpaconfig "github.com/Fedosin/libkpa/config"
)

func TestNewScaler(t *testing.T) {
	tests := []struct {
		name       string
		scalerName string
		algoType   string
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "valid linear scaler",
			scalerName: "cpu-scaler",
			algoType:   "linear",
			wantErr:    false,
		},
		{
			name:       "valid weighted scaler",
			scalerName: "memory-scaler",
			algoType:   "weighted",
			wantErr:    false,
		},
		{
			name:       "empty scaler name",
			scalerName: "",
			algoType:   "linear",
			wantErr:    true,
			errMsg:     "scaler name cannot be empty",
		},
		{
			name:       "invalid algorithm type",
			scalerName: "test-scaler",
			algoType:   "unknown",
			wantErr:    true,
			errMsg:     "unknown algorithm type: unknown (expected 'linear' or 'weighted')",
		},
	}

	config := libkpaconfig.NewDefaultAutoscalerConfig()
	config.StableWindow = 60 * time.Second
	config.PanicWindowPercentage = 10.0
	config.TargetValue = 100.0
	config.PanicThreshold = 2.0

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scaler, err := NewScaler(tt.scalerName, *config, tt.algoType)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("expected error message %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if scaler == nil {
					t.Errorf("expected scaler but got nil")
				}
				if scaler != nil && scaler.Name() != tt.scalerName {
					t.Errorf("expected scaler name %q, got %q", tt.scalerName, scaler.Name())
				}
			}
		})
	}
}

func TestScalerChangeAggregationAlgorithm(t *testing.T) {
	config := libkpaconfig.NewDefaultAutoscalerConfig()
	config.StableWindow = 60 * time.Second
	config.PanicWindowPercentage = 10.0
	config.TargetValue = 100.0

	scaler, err := NewScaler("test-scaler", *config, "linear")
	if err != nil {
		t.Fatalf("failed to create scaler: %v", err)
	}

	// Test changing to weighted
	err = scaler.ChangeAggregationAlgorithm("weighted")
	if err != nil {
		t.Errorf("failed to change to weighted: %v", err)
	}

	// Test changing back to linear
	err = scaler.ChangeAggregationAlgorithm("linear")
	if err != nil {
		t.Errorf("failed to change to linear: %v", err)
	}

	// Test invalid algorithm type
	err = scaler.ChangeAggregationAlgorithm("invalid")
	if err == nil {
		t.Errorf("expected error for invalid algorithm type")
	}
}

func TestScalerRecordAndScale(t *testing.T) {
	config := libkpaconfig.NewDefaultAutoscalerConfig()
	config.StableWindow = 10 * time.Second
	config.PanicWindowPercentage = 10.0
	config.TargetValue = 100.0
	config.PanicThreshold = 2.0
	config.MaxScaleUpRate = 1000.0
	config.MaxScaleDownRate = 2.0

	scaler, err := NewScaler("test-scaler", *config, "linear")
	if err != nil {
		t.Fatalf("failed to create scaler: %v", err)
	}

	now := time.Now()

	// Initially, scale should be invalid (no data)
	recommendation := scaler.Scale(3, now)
	if recommendation.ScaleValid {
		t.Errorf("expected invalid scale with no data")
	}

	// Record some metrics
	for i := range 10 {
		scaler.Record(300.0, now.Add(time.Duration(i)*time.Second))
	}

	// Now scale should be valid
	recommendation = scaler.Scale(3, now.Add(10*time.Second))
	if !recommendation.ScaleValid {
		t.Errorf("expected valid scale after recording data")
	}

	// With average of 300 and target of 100, we should want 3 pods
	if recommendation.DesiredPodCount != 3 {
		t.Errorf("expected 3 pods, got %d", recommendation.DesiredPodCount)
	}
}

func TestNewManager(t *testing.T) {
	// Test basic creation
	manager := NewManager(1, 10)
	if manager.GetMinScale() != 1 {
		t.Errorf("expected min scale 1, got %d", manager.GetMinScale())
	}
	if manager.GetMaxScale() != 10 {
		t.Errorf("expected max scale 10, got %d", manager.GetMaxScale())
	}

	// Test with invalid bounds (max < min)
	manager = NewManager(10, 5)
	if manager.GetMaxScale() != 10 {
		t.Errorf("expected max scale to be adjusted to 10, got %d", manager.GetMaxScale())
	}

	// Test with negative min
	manager = NewManager(-5, 10)
	if manager.GetMinScale() != 0 {
		t.Errorf("expected min scale to be adjusted to 0, got %d", manager.GetMinScale())
	}

	// Test with initial scalers
	config := libkpaconfig.NewDefaultAutoscalerConfig()
	config.StableWindow = 60 * time.Second
	config.PanicWindowPercentage = 10.0
	config.TargetValue = 100.0

	scaler1, _ := NewScaler("cpu", *config, "linear")
	scaler2, _ := NewScaler("memory", *config, "weighted")

	manager = NewManager(1, 10, scaler1, scaler2)
	if len(manager.scalers) != 2 {
		t.Errorf("expected 2 scalers, got %d", len(manager.scalers))
	}
}

func TestManagerRegisterUnregister(t *testing.T) {
	manager := NewManager(1, 10)
	config := libkpaconfig.NewDefaultAutoscalerConfig()
	config.StableWindow = 60 * time.Second
	config.PanicWindowPercentage = 10.0
	config.TargetValue = 100.0

	// Register a scaler
	scaler, _ := NewScaler("cpu", *config, "linear")
	manager.Register(scaler)

	if len(manager.scalers) != 1 {
		t.Errorf("expected 1 scaler after register, got %d", len(manager.scalers))
	}

	// Register nil should be a no-op
	manager.Register(nil)
	if len(manager.scalers) != 1 {
		t.Errorf("expected 1 scaler after nil register, got %d", len(manager.scalers))
	}

	// Register with same name should replace
	scaler2, _ := NewScaler("cpu", *config, "weighted")
	manager.Register(scaler2)
	if len(manager.scalers) != 1 {
		t.Errorf("expected 1 scaler after replace, got %d", len(manager.scalers))
	}

	// Unregister
	manager.Unregister("cpu")
	if len(manager.scalers) != 0 {
		t.Errorf("expected 0 scalers after unregister, got %d", len(manager.scalers))
	}

	// Unregister non-existent should be a no-op
	manager.Unregister("nonexistent")
}

func TestManagerSetScaleBounds(t *testing.T) {
	manager := NewManager(5, 10)

	// Test SetMinScale
	manager.SetMinScale(3)
	if manager.GetMinScale() != 3 {
		t.Errorf("expected min scale 3, got %d", manager.GetMinScale())
	}

	// Test SetMinScale with negative
	manager.SetMinScale(-1)
	if manager.GetMinScale() != 0 {
		t.Errorf("expected min scale 0, got %d", manager.GetMinScale())
	}

	// Test SetMinScale greater than max
	manager.SetMaxScale(5)
	manager.SetMinScale(10)
	if manager.GetMaxScale() != 10 {
		t.Errorf("expected max scale to be adjusted to 10, got %d", manager.GetMaxScale())
	}

	// Test SetMaxScale
	manager.SetMaxScale(20)
	if manager.GetMaxScale() != 20 {
		t.Errorf("expected max scale 20, got %d", manager.GetMaxScale())
	}

	// Test SetMaxScale less than min
	manager.SetMinScale(15)
	manager.SetMaxScale(10)
	if manager.GetMinScale() != 10 {
		t.Errorf("expected min scale to be adjusted to 10, got %d", manager.GetMinScale())
	}
}

func TestManagerChangeAggregationAlgorithm(t *testing.T) {
	manager := NewManager(1, 10)
	config := libkpaconfig.NewDefaultAutoscalerConfig()
	config.StableWindow = 60 * time.Second
	config.PanicWindowPercentage = 10.0
	config.TargetValue = 100.0

	scaler, _ := NewScaler("cpu", *config, "linear")
	manager.Register(scaler)

	// Test changing existing scaler
	err := manager.ChangeAggregationAlgorithm("cpu", "weighted")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Test changing non-existent scaler
	err = manager.ChangeAggregationAlgorithm("nonexistent", "weighted")
	if err == nil {
		t.Errorf("expected error for non-existent scaler")
	}
}

func TestManagerRecord(t *testing.T) {
	manager := NewManager(1, 10)
	config := libkpaconfig.NewDefaultAutoscalerConfig()
	config.StableWindow = 60 * time.Second
	config.PanicWindowPercentage = 10.0
	config.TargetValue = 100.0

	scaler, _ := NewScaler("cpu", *config, "linear")
	manager.Register(scaler)

	// Test recording to existing scaler
	err := manager.Record("cpu", 150.0, time.Now())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Test recording to non-existent scaler
	err = manager.Record("nonexistent", 150.0, time.Now())
	if err == nil {
		t.Errorf("expected error for non-existent scaler")
	}
}

func TestManagerScale(t *testing.T) {
	now := time.Now()

	// Test with no scalers
	manager := NewManager(2, 10)
	result := manager.Scale(2, now)
	if result != 2 {
		t.Errorf("expected min replicas (2) with no scalers, got %d", result)
	}

	// Create scalers
	config := libkpaconfig.NewDefaultAutoscalerConfig()
	config.StableWindow = 10 * time.Second
	config.PanicWindowPercentage = 10.0
	config.TargetValue = 100.0
	config.PanicThreshold = 2.0
	config.MaxScaleUpRate = 1000.0
	config.MaxScaleDownRate = 2.0

	cpuScaler, err := NewScaler("cpu", *config, "linear")
	if err != nil {
		t.Fatalf("failed to create scaler: %v", err)
	}
	memoryScaler, err := NewScaler("memory", *config, "linear")
	if err != nil {
		t.Fatalf("failed to create scaler: %v", err)
	}

	manager.Register(cpuScaler)
	manager.Register(memoryScaler)

	// Record metrics for both scalers
	for i := range 10 {
		cpuScaler.Record(300.0, now.Add(time.Duration(i)*time.Second))    // Would want 3 pods
		memoryScaler.Record(500.0, now.Add(time.Duration(i)*time.Second)) // Would want 5 pods
	}

	// Scale should return the maximum (5)
	result = manager.Scale(3, now.Add(10*time.Second))
	if result != 5 {
		t.Errorf("expected 5 pods (max of 3 and 5), got %d", result)
	}

	// Test with max replicas constraint
	manager.SetMaxScale(4)
	result = manager.Scale(3, now.Add(10*time.Second))
	if result != 4 {
		t.Errorf("expected 4 pods (clamped by max), got %d", result)
	}

	// Test with all invalid scalers (no data)
	manager2 := NewManager(1, 10)
	emptyScaler1, _ := NewScaler("empty1", *config, "linear")
	emptyScaler2, _ := NewScaler("empty2", *config, "linear")
	manager2.Register(emptyScaler1)
	manager2.Register(emptyScaler2)

	result = manager2.Scale(1, now)
	if result != 1 {
		t.Errorf("expected current scale (1) with all invalid scalers, got %d", result)
	}
}

func TestManagerScaleMultipleScenarios(t *testing.T) {
	now := time.Now()

	config := libkpaconfig.NewDefaultAutoscalerConfig()
	config.StableWindow = 10 * time.Second
	config.PanicWindowPercentage = 10.0
	config.TargetValue = 100.0
	config.PanicThreshold = 2.0
	config.MaxScaleUpRate = 1000.0
	config.MaxScaleDownRate = 2.0

	// Scenario 1: CPU high, Memory low
	manager := NewManager(1, 10)
	cpuScaler, _ := NewScaler("cpu", *config, "linear")
	memoryScaler, _ := NewScaler("memory", *config, "weighted")

	manager.Register(cpuScaler)
	manager.Register(memoryScaler)

	for i := range 10 {
		cpuScaler.Record(800.0, now.Add(time.Duration(i)*time.Second))    // Would want 8 pods
		memoryScaler.Record(200.0, now.Add(time.Duration(i)*time.Second)) // Would want 2 pods
	}

	result := manager.Scale(5, now.Add(10*time.Second))
	if result != 8 {
		t.Errorf("expected 8 pods (max of CPU), got %d", result)
	}

	// Scenario 2: Both metrics want to scale to zero
	manager2 := NewManager(0, 10)
	cpuScaler2, _ := NewScaler("cpu", *config, "linear")
	memoryScaler2, _ := NewScaler("memory", *config, "linear")

	manager2.Register(cpuScaler2)
	manager2.Register(memoryScaler2)

	for i := range 10 {
		cpuScaler2.Record(0.0, now.Add(time.Duration(i)*time.Second))
		memoryScaler2.Record(0.0, now.Add(time.Duration(i)*time.Second))
	}

	result = manager2.Scale(1, now.Add(10*time.Second))
	if result != 0 {
		t.Errorf("expected 0 pods (scale to zero), got %d", result)
	}
}

func TestConcurrentAccess(t *testing.T) {
	manager := NewManager(1, 100)
	config := libkpaconfig.NewDefaultAutoscalerConfig()
	config.StableWindow = 10 * time.Second
	config.PanicWindowPercentage = 10.0
	config.TargetValue = 100.0

	// Create multiple scalers
	for i := range 5 {
		scaler, _ := NewScaler(string(rune('a'+i)), *config, "linear")
		manager.Register(scaler)
	}

	done := make(chan bool)

	// Concurrent operations
	go func() {
		for i := range 100 {
			manager.SetMinScale(int32(i % 10))
			manager.SetMaxScale(int32(50 + i%50))
		}
		done <- true
	}()

	go func() {
		for i := range 100 {
			manager.Record(string(rune('a'+i%5)), float64(i*10), time.Now())
		}
		done <- true
	}()

	go func() {
		for range 100 {
			manager.Scale(80, time.Now())
		}
		done <- true
	}()

	go func() {
		for i := range 50 {
			scaler, _ := NewScaler(string(rune('z'-i%5)), *config, "linear")
			manager.Register(scaler)
			manager.Unregister(string(rune('z' - i%5)))
		}
		done <- true
	}()

	// Wait for all goroutines
	for range 4 {
		<-done
	}
}
