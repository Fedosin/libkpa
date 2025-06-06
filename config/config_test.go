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

package config

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Fedosin/libkpa/api"
)

func TestLoadFromMap(t *testing.T) {
	tests := []struct {
		name    string
		data    map[string]string
		wantErr bool
		check   func(*testing.T, *api.Config)
	}{
		{
			name: "valid config with all values",
			data: map[string]string{
				"scaling-metric":                          "rps",
				"target-value":                            "150",
				"max-scale-up-rate":                       "5.0",
				"max-scale-down-rate":                     "3.0",
				"stable-window":                           "120s",
				"scale-down-delay":                        "30s",
				"panic-threshold-percentage":              "150",
				"panic-window-percentage":                 "20",
				"initial-scale":                           "2",
				"min-scale":                               "1",
				"max-scale":                               "20",
				"activation-scale":                        "2",
				"target-burst-capacity":                   "300",
				"container-concurrency-target-percentage": "80",
				"enable-scale-to-zero":                    "false",
			},
			wantErr: false,
			check: func(t *testing.T, cfg *api.Config) {
				if cfg.ScalingMetric != api.RPS {
					t.Errorf("ScalingMetric = %v, want %v", cfg.ScalingMetric, api.RPS)
				}
				if cfg.TargetValue != 150 {
					t.Errorf("TargetValue = %v, want 150", cfg.TargetValue)
				}
				if cfg.MaxScaleUpRate != 5.0 {
					t.Errorf("MaxScaleUpRate = %v, want 5.0", cfg.MaxScaleUpRate)
				}
				if cfg.StableWindow != 120*time.Second {
					t.Errorf("StableWindow = %v, want 120s", cfg.StableWindow)
				}
				if cfg.PanicThreshold != 1.5 {
					t.Errorf("PanicThreshold = %v, want 1.5", cfg.PanicThreshold)
				}
				if cfg.MinScale != 1 {
					t.Errorf("MinScale = %v, want 1", cfg.MinScale)
				}
				if cfg.EnableScaleToZero != false {
					t.Errorf("EnableScaleToZero = %v, want false", cfg.EnableScaleToZero)
				}
			},
		},
		{
			name: "default values",
			data: map[string]string{},
			check: func(t *testing.T, cfg *api.Config) {
				if cfg.ScalingMetric != api.Concurrency {
					t.Errorf("ScalingMetric = %v, want %v", cfg.ScalingMetric, api.Concurrency)
				}
				if cfg.TargetValue != 100.0 {
					t.Errorf("TargetValue = %v, want 100.0", cfg.TargetValue)
				}
				if cfg.MaxScaleUpRate != 1000.0 {
					t.Errorf("MaxScaleUpRate = %v, want 1000.0", cfg.MaxScaleUpRate)
				}
				if cfg.StableWindow != 60*time.Second {
					t.Errorf("StableWindow = %v, want 60s", cfg.StableWindow)
				}
			},
		},
		{
			name: "invalid max-scale-up-rate",
			data: map[string]string{
				"max-scale-up-rate": "0.5",
			},
			wantErr: true,
		},
		{
			name: "invalid max-scale-down-rate",
			data: map[string]string{
				"max-scale-down-rate": "1.0",
			},
			wantErr: true,
		},
		{
			name: "invalid stable window too small",
			data: map[string]string{
				"stable-window": "2s",
			},
			wantErr: true,
		},
		{
			name: "invalid stable window too large",
			data: map[string]string{
				"stable-window": "700s",
			},
			wantErr: true,
		},
		{
			name: "invalid panic window percentage",
			data: map[string]string{
				"panic-window-percentage": "0.5",
			},
			wantErr: true,
		},
		{
			name: "invalid min-scale negative",
			data: map[string]string{
				"min-scale": "-1",
			},
			wantErr: true,
		},
		{
			name: "invalid min-scale greater than max-scale",
			data: map[string]string{
				"min-scale": "10",
				"max-scale": "5",
			},
			wantErr: true,
		},
		{
			name: "invalid activation-scale",
			data: map[string]string{
				"activation-scale": "0",
			},
			wantErr: true,
		},
		{
			name: "invalid scaling metric",
			data: map[string]string{
				"scaling-metric": "invalid",
			},
			wantErr: true,
		},
		{
			name: "percentage conversion",
			data: map[string]string{
				"container-concurrency-target-percentage": "70",
				"panic-threshold-percentage":              "200",
			},
			check: func(t *testing.T, cfg *api.Config) {
				if cfg.ContainerConcurrencyTargetFraction != 0.7 {
					t.Errorf("ContainerConcurrencyTargetFraction = %v, want 0.7", cfg.ContainerConcurrencyTargetFraction)
				}
				if cfg.PanicThreshold != 2.0 {
					t.Errorf("PanicThreshold = %v, want 2.0", cfg.PanicThreshold)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadFromMap(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadFromMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}

func TestLoadFromEnvironment(t *testing.T) {
	// Save current environment
	envVars := []string{
		"AUTOSCALER_SCALING_METRIC",
		"AUTOSCALER_TARGET_VALUE",
		"AUTOSCALER_MAX_SCALE_UP_RATE",
		"AUTOSCALER_STABLE_WINDOW",
		"AUTOSCALER_MIN_SCALE",
		"AUTOSCALER_ENABLE_SCALE_TO_ZERO",
	}
	saved := make(map[string]string)
	for _, key := range envVars {
		saved[key] = os.Getenv(key)
		os.Unsetenv(key)
	}
	defer func() {
		for key, value := range saved {
			if value != "" {
				os.Setenv(key, value)
			}
		}
	}()

	// Set test environment
	os.Setenv("AUTOSCALER_SCALING_METRIC", "rps")
	os.Setenv("AUTOSCALER_TARGET_VALUE", "200")
	os.Setenv("AUTOSCALER_MAX_SCALE_UP_RATE", "10")
	os.Setenv("AUTOSCALER_STABLE_WINDOW", "90s")
	os.Setenv("AUTOSCALER_MIN_SCALE", "2")
	os.Setenv("AUTOSCALER_ENABLE_SCALE_TO_ZERO", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Check loaded values
	if cfg.ScalingMetric != api.RPS {
		t.Errorf("ScalingMetric = %v, want %v", cfg.ScalingMetric, api.RPS)
	}
	if cfg.TargetValue != 200 {
		t.Errorf("TargetValue = %v, want 200", cfg.TargetValue)
	}
	if cfg.MaxScaleUpRate != 10 {
		t.Errorf("MaxScaleUpRate = %v, want 10", cfg.MaxScaleUpRate)
	}
	if cfg.StableWindow != 90*time.Second {
		t.Errorf("StableWindow = %v, want 90s", cfg.StableWindow)
	}
	if cfg.MinScale != 2 {
		t.Errorf("MinScale = %v, want 2", cfg.MinScale)
	}
	if cfg.EnableScaleToZero != false {
		t.Errorf("EnableScaleToZero = %v, want false", cfg.EnableScaleToZero)
	}
}

func TestValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*api.Config)
		wantErr string
	}{
		{
			name: "negative scale-to-zero-grace-period",
			modify: func(cfg *api.Config) {
				cfg.ScaleToZeroGracePeriod = -1 * time.Second
			},
			wantErr: "scale-to-zero-grace-period must be positive",
		},
		{
			name: "negative scale-down-delay",
			modify: func(cfg *api.Config) {
				cfg.ScaleDownDelay = -1 * time.Second
			},
			wantErr: "scale-down-delay cannot be negative",
		},
		{
			name: "sub-second scale-down-delay",
			modify: func(cfg *api.Config) {
				cfg.ScaleDownDelay = 500 * time.Millisecond
			},
			wantErr: "must be specified with at most second precision",
		},
		{
			name: "invalid target-burst-capacity",
			modify: func(cfg *api.Config) {
				cfg.TargetBurstCapacity = -2
			},
			wantErr: "target-burst-capacity must be either non-negative or -1",
		},
		{
			name: "container-concurrency-target-fraction too low",
			modify: func(cfg *api.Config) {
				cfg.ContainerConcurrencyTargetFraction = 0
			},
			wantErr: "outside of valid range of (0, 1]",
		},
		{
			name: "container-concurrency-target-fraction too high",
			modify: func(cfg *api.Config) {
				cfg.ContainerConcurrencyTargetFraction = 1.1
			},
			wantErr: "outside of valid range of (0, 1]",
		},
		{
			name: "target concurrency too low",
			modify: func(cfg *api.Config) {
				cfg.ContainerConcurrencyTargetFraction = 0.0001
				cfg.ContainerConcurrencyTargetDefault = 1
			},
			wantErr: "can't be less than",
		},
		{
			name: "rps target too low",
			modify: func(cfg *api.Config) {
				cfg.RPSTargetDefault = 0.001
			},
			wantErr: "must be at least",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &api.Config{
				EnableScaleToZero:                  true,
				ScaleToZeroGracePeriod:             30 * time.Second,
				ContainerConcurrencyTargetFraction: 0.7,
				ContainerConcurrencyTargetDefault:  100,
				RPSTargetDefault:                   200,
				TargetUtilization:                  0.7,
				AutoscalerSpec: api.AutoscalerSpec{
					MaxScaleUpRate:        1000,
					MaxScaleDownRate:      2,
					ScalingMetric:         api.Concurrency,
					TargetValue:           100,
					TotalValue:            1000,
					TargetBurstCapacity:   211,
					PanicThreshold:        2,
					PanicWindowPercentage: 10,
					StableWindow:          60 * time.Second,
					ScaleDownDelay:        0,
					InitialScale:          1,
					MinScale:              0,
					MaxScale:              0,
					ActivationScale:       1,
					Reachable:             true,
				},
			}

			tt.modify(cfg)
			_, err := validate(cfg)
			if err == nil {
				t.Errorf("validate() error = nil, want error containing %q", tt.wantErr)
			} else if !contains(err.Error(), tt.wantErr) {
				t.Errorf("validate() error = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestErrorAggregation(t *testing.T) {
	// Set up some invalid environment variables
	os.Setenv("AUTOSCALER_ENABLE_SCALE_TO_ZERO", "invalid-bool")
	os.Setenv("AUTOSCALER_MAX_SCALE_UP_RATE", "not-a-number")
	os.Setenv("AUTOSCALER_STABLE_WINDOW", "invalid-duration")
	os.Setenv("AUTOSCALER_MIN_SCALE", "abc")
	defer func() {
		os.Unsetenv("AUTOSCALER_ENABLE_SCALE_TO_ZERO")
		os.Unsetenv("AUTOSCALER_MAX_SCALE_UP_RATE")
		os.Unsetenv("AUTOSCALER_STABLE_WINDOW")
		os.Unsetenv("AUTOSCALER_MIN_SCALE")
	}()

	// Try to load configuration
	cfg, err := Load()
	if err == nil {
		t.Fatal("expected error but got none")
	}

	// Print the aggregated error
	fmt.Printf("Error message:\n%v\n", err)

	// Verify that cfg is nil when there are errors
	if cfg != nil {
		t.Fatal("expected nil config when errors occur")
	}
}

func TestErrorAggregationFromMap(t *testing.T) {
	// Create a map with invalid values
	data := map[string]string{
		"enable-scale-to-zero":    "not-bool",
		"max-scale-up-rate":       "invalid-float",
		"stable-window":           "bad-duration",
		"initial-scale":           "negative-one",
		"panic-window-percentage": "not-a-percentage",
	}

	// Try to load configuration
	cfg, err := LoadFromMap(data)
	if err == nil {
		t.Fatal("expected error but got none")
	}

	// Print the aggregated error
	fmt.Printf("Error message from map:\n%v\n", err)

	// Verify that cfg is nil when there are errors
	if cfg != nil {
		t.Fatal("expected nil config when errors occur")
	}
}


func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr || len(s) > len(substr) && contains(s[1:], substr)
}
