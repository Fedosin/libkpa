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
	"strings"
	"testing"
	"time"

	"github.com/Fedosin/libkpa/api"
)

func TestConfigErrors(t *testing.T) {
	tests := []struct {
		name     string
		errors   []error
		expected string
		hasError bool
	}{
		{
			name:     "no errors",
			errors:   []error{},
			expected: "",
			hasError: false,
		},
		{
			name:     "single error",
			errors:   []error{fmt.Errorf("error 1")},
			expected: "configuration errors:\n  - error 1",
			hasError: true,
		},
		{
			name:     "multiple errors",
			errors:   []error{fmt.Errorf("error 1"), fmt.Errorf("error 2"), fmt.Errorf("error 3")},
			expected: "configuration errors:\n  - error 1\n  - error 2\n  - error 3",
			hasError: true,
		},
		{
			name:     "nil errors are ignored",
			errors:   []error{fmt.Errorf("error 1"), nil, fmt.Errorf("error 2")},
			expected: "configuration errors:\n  - error 1\n  - error 2",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ce := &configErrors{}
			for _, err := range tt.errors {
				ce.add(err)
			}

			if got := ce.hasErrors(); got != tt.hasError {
				t.Errorf("hasErrors() = %v, want %v", got, tt.hasError)
			}

			if got := ce.Error(); got != tt.expected {
				t.Errorf("Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	// Save original env and restore after test
	originalEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, e := range originalEnv {
			pair := splitEnvPair(e)
			if len(pair) == 2 {
				os.Setenv(pair[0], pair[1])
			}
		}
	}()

	tests := []struct {
		name    string
		envVars map[string]string
		want    *api.AutoscalerConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "default values when no env vars set",
			envVars: map[string]string{},
			want: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: 30 * time.Second,
				MaxScaleUpRate:         1000.0,
				MaxScaleDownRate:       2.0,
				TargetValue:            100.0,
				TotalTargetValue:       0.0,
				BurstThreshold:         2.0, // 200% converted to fraction
				BurstWindowPercentage:  10.0,
				StableWindow:           60 * time.Second,
				ScaleDownDelay:         0 * time.Second,
				MinScale:               0,
				MaxScale:               0,
				ActivationScale:        1,
			},
		},
		{
			name: "custom values from env vars",
			envVars: map[string]string{
				"AUTOSCALER_SCALE_TO_ZERO_GRACE_PERIOD": "45s",
				"AUTOSCALER_MAX_SCALE_UP_RATE":          "500.5",
				"AUTOSCALER_MAX_SCALE_DOWN_RATE":        "3.5",
				"AUTOSCALER_TARGET_VALUE":               "100.0",
				"AUTOSCALER_BURST_THRESHOLD_PERCENTAGE": "150.0",
				"AUTOSCALER_BURST_WINDOW_PERCENTAGE":    "20.0",
				"AUTOSCALER_STABLE_WINDOW":              "120s",
				"AUTOSCALER_SCALE_DOWN_DELAY":           "10s",
				"AUTOSCALER_MIN_SCALE":                  "1",
				"AUTOSCALER_MAX_SCALE":                  "10",
				"AUTOSCALER_ACTIVATION_SCALE":           "2",
			},
			want: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: 45 * time.Second,
				MaxScaleUpRate:         500.5,
				MaxScaleDownRate:       3.5,
				TargetValue:            100.0,
				TotalTargetValue:       0.0,
				BurstThreshold:         1.5, // 150% converted to fraction
				BurstWindowPercentage:  20.0,
				StableWindow:           120 * time.Second,
				ScaleDownDelay:         10 * time.Second,
				MinScale:               1,
				MaxScale:               10,
				ActivationScale:        2,
			},
		},
		{
			name: "burst threshold already as fraction",
			envVars: map[string]string{
				"AUTOSCALER_BURST_THRESHOLD_PERCENTAGE": "2.5",
			},
			want: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: 30 * time.Second,
				MaxScaleUpRate:         1000.0,
				MaxScaleDownRate:       2.0,
				TargetValue:            100.0,
				TotalTargetValue:       0.0,
				BurstThreshold:         2.5, // Already a fraction, not converted
				BurstWindowPercentage:  10.0,
				StableWindow:           60 * time.Second,
				ScaleDownDelay:         0 * time.Second,
				MinScale:               0,
				MaxScale:               0,
				ActivationScale:        1,
			},
		},
		{
			name: "total target value set",
			envVars: map[string]string{
				"AUTOSCALER_TARGET_VALUE":       "0", // Explicitly set to 0
				"AUTOSCALER_TOTAL_TARGET_VALUE": "2000.0",
			},
			want: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: 30 * time.Second,
				MaxScaleUpRate:         1000.0,
				MaxScaleDownRate:       2.0,
				TargetValue:            0.0,
				TotalTargetValue:       2000.0,
				BurstThreshold:         2.0,
				BurstWindowPercentage:  10.0,
				StableWindow:           60 * time.Second,
				ScaleDownDelay:         0 * time.Second,
				MinScale:               0,
				MaxScale:               0,
				ActivationScale:        1,
			},
		},
		{
			name: "invalid float value",
			envVars: map[string]string{
				"AUTOSCALER_MAX_SCALE_UP_RATE": "not-a-number",
			},
			wantErr: true,
			errMsg:  "invalid float value",
		},
		{
			name: "invalid duration value",
			envVars: map[string]string{
				"AUTOSCALER_STABLE_WINDOW": "invalid-duration",
			},
			wantErr: true,
			errMsg:  "invalid duration value",
		},
		{
			name: "invalid int32 value",
			envVars: map[string]string{
				"AUTOSCALER_MIN_SCALE": "not-an-int",
			},
			wantErr: true,
			errMsg:  "invalid int32 value",
		},
		{
			name: "multiple errors",
			envVars: map[string]string{
				"AUTOSCALER_MIN_SCALE":     "invalid",
				"AUTOSCALER_STABLE_WINDOW": "also-invalid",
			},
			wantErr: true,
			errMsg:  "configuration errors:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear env and set test values
			os.Clearenv()
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			got, err := Load()

			if tt.wantErr {
				if err == nil {
					t.Errorf("Load() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if tt.errMsg != "" && !containsString(err.Error(), tt.errMsg) {
					t.Errorf("Load() error = %v, should contain %v", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("Load() unexpected error = %v", err)
				return
			}

			if !configsEqual(got, tt.want) {
				t.Errorf("Load() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestLoadFromMap(t *testing.T) {
	tests := []struct {
		name    string
		data    map[string]string
		want    *api.AutoscalerConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "default values with empty map",
			data: map[string]string{},
			want: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: 30 * time.Second,
				MaxScaleUpRate:         1000.0,
				MaxScaleDownRate:       2.0,
				TargetValue:            100.0,
				TotalTargetValue:       0.0,
				BurstThreshold:         2.0,
				BurstWindowPercentage:  10.0,
				StableWindow:           60 * time.Second,
				ScaleDownDelay:         0 * time.Second,
				MinScale:               0,
				MaxScale:               0,
				ActivationScale:        1,
			},
		},
		{
			name: "custom values from map",
			data: map[string]string{
				"scale-to-zero-grace-period": "45s",
				"max-scale-up-rate":          "500.5",
				"max-scale-down-rate":        "3.5",
				"target-value":               "100.0",
				"burst-threshold-percentage": "150.0",
				"burst-window-percentage":    "20.0",
				"stable-window":              "120s",
				"scale-down-delay":           "10s",
				"min-scale":                  "1",
				"max-scale":                  "10",
				"activation-scale":           "2",
			},
			want: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: 45 * time.Second,
				MaxScaleUpRate:         500.5,
				MaxScaleDownRate:       3.5,
				TargetValue:            100.0,
				TotalTargetValue:       0.0,
				BurstThreshold:         1.5,
				BurstWindowPercentage:  20.0,
				StableWindow:           120 * time.Second,
				ScaleDownDelay:         10 * time.Second,
				MinScale:               1,
				MaxScale:               10,
				ActivationScale:        2,
			},
		},
		{
			name: "values with whitespace",
			data: map[string]string{
				"max-scale-up-rate": "  500.5  ",
				"min-scale":         " 5 ",
				"stable-window":     " 30s ",
			},
			want: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: 30 * time.Second,
				MaxScaleUpRate:         500.5,
				MaxScaleDownRate:       2.0,
				TargetValue:            100.0,
				TotalTargetValue:       0.0,
				BurstThreshold:         2.0,
				BurstWindowPercentage:  10.0,
				StableWindow:           30 * time.Second,
				ScaleDownDelay:         0 * time.Second,
				MinScale:               5,
				MaxScale:               0,
				ActivationScale:        1,
			},
		},
		{
			name: "total target value from map",
			data: map[string]string{
				"target-value":       "0", // Explicitly set to 0
				"total-target-value": "1500.0",
			},
			want: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: 30 * time.Second,
				MaxScaleUpRate:         1000.0,
				MaxScaleDownRate:       2.0,
				TargetValue:            0.0,
				TotalTargetValue:       1500.0,
				BurstThreshold:         2.0,
				BurstWindowPercentage:  10.0,
				StableWindow:           60 * time.Second,
				ScaleDownDelay:         0 * time.Second,
				MinScale:               0,
				MaxScale:               0,
				ActivationScale:        1,
			},
		},
		{
			name: "invalid float",
			data: map[string]string{
				"target-value": "abc",
			},
			wantErr: true,
			errMsg:  "invalid float value",
		},
		{
			name: "invalid duration",
			data: map[string]string{
				"scale-down-delay": "10 minutes",
			},
			wantErr: true,
			errMsg:  "invalid duration value",
		},
		{
			name: "invalid int32",
			data: map[string]string{
				"max-scale": "10.5",
			},
			wantErr: true,
			errMsg:  "invalid int32 value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadFromMap(tt.data)

			if tt.wantErr {
				if err == nil {
					t.Errorf("LoadFromMap() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if tt.errMsg != "" && !containsString(err.Error(), tt.errMsg) {
					t.Errorf("LoadFromMap() error = %v, should contain %v", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("LoadFromMap() unexpected error = %v", err)
				return
			}

			if !configsEqual(got, tt.want) {
				t.Errorf("LoadFromMap() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *api.AutoscalerConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: 30 * time.Second,
				MaxScaleUpRate:         1000.0,
				MaxScaleDownRate:       2.0,
				TargetValue:            100.0,
				BurstThreshold:         2.0,
				BurstWindowPercentage:  10.0,
				StableWindow:           60 * time.Second,
				ScaleDownDelay:         10 * time.Second,
				MinScale:               0,
				MaxScale:               10,
				ActivationScale:        1,
			},
			wantErr: false,
		},
		{
			name: "negative scale-to-zero grace period",
			config: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: -1 * time.Second,
				MaxScaleUpRate:         2.0,
				MaxScaleDownRate:       2.0,
				TargetValue:            1.0,
				StableWindow:           60 * time.Second,
				ActivationScale:        1,
			},
			wantErr: true,
			errMsg:  "scale-to-zero-grace-period must be positive",
		},
		{
			name: "negative scale-down delay",
			config: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: 30 * time.Second,
				ScaleDownDelay:         -5 * time.Second,
				MaxScaleUpRate:         2.0,
				MaxScaleDownRate:       2.0,
				TargetValue:            1.0,
				StableWindow:           60 * time.Second,
				ActivationScale:        1,
			},
			wantErr: true,
			errMsg:  "scale-down-delay cannot be negative",
		},
		{
			name: "scale-down delay with sub-second precision",
			config: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: 30 * time.Second,
				ScaleDownDelay:         5*time.Second + 500*time.Millisecond,
				MaxScaleUpRate:         2.0,
				MaxScaleDownRate:       2.0,
				TargetValue:            1.0,
				StableWindow:           60 * time.Second,
				ActivationScale:        1,
			},
			wantErr: true,
			errMsg:  "must be specified with at most second precision",
		},
		{
			name: "target value unlimited (-1)",
			config: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: 30 * time.Second,
				MaxScaleUpRate:         2.0,
				MaxScaleDownRate:       2.0,
				TargetValue:            1.0,
				StableWindow:           60 * time.Second,
				BurstWindowPercentage:  10.0,
				ActivationScale:        1,
			},
			wantErr: false,
		},
		{
			name: "both target values zero",
			config: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: 30 * time.Second,
				MaxScaleUpRate:         2.0,
				MaxScaleDownRate:       2.0,
				TargetValue:            0.0,
				TotalTargetValue:       0.0,
				StableWindow:           60 * time.Second,
				ActivationScale:        1,
			},
			wantErr: true,
			errMsg:  "either target-value or total-target-value must be positive",
		},
		{
			name: "both target values set",
			config: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: 30 * time.Second,
				MaxScaleUpRate:         2.0,
				MaxScaleDownRate:       2.0,
				TargetValue:            100.0,
				TotalTargetValue:       1000.0,
				StableWindow:           60 * time.Second,
				ActivationScale:        1,
			},
			wantErr: true,
			errMsg:  "cannot specify both target-value",
		},
		{
			name: "max scale up rate too low",
			config: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: 30 * time.Second,
				MaxScaleUpRate:         0.5,
				MaxScaleDownRate:       2.0,
				TargetValue:            1.0,
				StableWindow:           60 * time.Second,
				ActivationScale:        1,
			},
			wantErr: true,
			errMsg:  "max-scale-up-rate = 0.5, must be greater than 1.0",
		},
		{
			name: "max scale down rate too low",
			config: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: 30 * time.Second,
				MaxScaleUpRate:         2.0,
				MaxScaleDownRate:       1.0,
				TargetValue:            1.0,
				StableWindow:           60 * time.Second,
				ActivationScale:        1,
			},
			wantErr: true,
			errMsg:  "max-scale-down-rate = 1, must be greater than 1.0",
		},
		{
			name: "stable window too short",
			config: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: 30 * time.Second,
				MaxScaleUpRate:         2.0,
				MaxScaleDownRate:       2.0,
				TargetValue:            1.0,
				StableWindow:           2 * time.Second,
				ActivationScale:        1,
			},
			wantErr: true,
			errMsg:  "stable-window = 2s, must be in [5s; 10m0s] range",
		},
		{
			name: "stable window too long",
			config: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: 30 * time.Second,
				MaxScaleUpRate:         2.0,
				MaxScaleDownRate:       2.0,
				TargetValue:            1.0,
				StableWindow:           700 * time.Second,
				ActivationScale:        1,
			},
			wantErr: true,
			errMsg:  "stable-window = 11m40s, must be in [5s; 10m0s] range",
		},
		{
			name: "stable window with sub-second precision",
			config: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: 30 * time.Second,
				MaxScaleUpRate:         2.0,
				MaxScaleDownRate:       2.0,
				TargetValue:            1.0,
				StableWindow:           60*time.Second + 100*time.Millisecond,
				ActivationScale:        1,
			},
			wantErr: true,
			errMsg:  "stable-window = 1m0.1s, must be specified with at most second precision",
		},
		{
			name: "burst window percentage too low",
			config: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: 30 * time.Second,
				MaxScaleUpRate:         2.0,
				MaxScaleDownRate:       2.0,
				TargetValue:            1.0,
				StableWindow:           60 * time.Second,
				BurstWindowPercentage:  0.5,
				ActivationScale:        1,
			},
			wantErr: true,
			errMsg:  "burst-window-percentage = 0.5, must be in [1.0, 100.0] interval",
		},
		{
			name: "burst window percentage too high",
			config: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: 30 * time.Second,
				MaxScaleUpRate:         2.0,
				MaxScaleDownRate:       2.0,
				TargetValue:            1.0,
				StableWindow:           60 * time.Second,
				BurstWindowPercentage:  101.0,
				ActivationScale:        1,
			},
			wantErr: true,
			errMsg:  "burst-window-percentage = 101, must be in [1.0, 100.0] interval",
		},
		{
			name: "negative min scale",
			config: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: 30 * time.Second,
				MaxScaleUpRate:         2.0,
				MaxScaleDownRate:       2.0,
				TargetValue:            1.0,
				StableWindow:           60 * time.Second,
				MinScale:               -1,
				ActivationScale:        1,
			},
			wantErr: true,
			errMsg:  "min-scale = -1, must be at least 0",
		},
		{
			name: "negative max scale",
			config: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: 30 * time.Second,
				MaxScaleUpRate:         2.0,
				MaxScaleDownRate:       2.0,
				TargetValue:            1.0,
				StableWindow:           60 * time.Second,
				MaxScale:               -1,
				ActivationScale:        1,
			},
			wantErr: true,
			errMsg:  "max-scale = -1, must be at least 0",
		},
		{
			name: "min scale greater than max scale",
			config: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: 30 * time.Second,
				MaxScaleUpRate:         2.0,
				MaxScaleDownRate:       2.0,
				TargetValue:            1.0,
				StableWindow:           60 * time.Second,
				MinScale:               10,
				MaxScale:               5,
				ActivationScale:        1,
			},
			wantErr: true,
			errMsg:  "min-scale (10) must be less than or equal to max-scale (5)",
		},
		{
			name: "min scale greater than max scale but max scale is 0",
			config: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: 30 * time.Second,
				MaxScaleUpRate:         2.0,
				MaxScaleDownRate:       2.0,
				TargetValue:            1.0,
				StableWindow:           60 * time.Second,
				BurstWindowPercentage:  10.0,
				MinScale:               10,
				MaxScale:               0,
				ActivationScale:        1,
			},
			wantErr: false, // This should be valid as max scale 0 means unlimited
		},
		{
			name: "activation scale less than 1",
			config: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: 30 * time.Second,
				MaxScaleUpRate:         2.0,
				MaxScaleDownRate:       2.0,
				TargetValue:            1.0,
				StableWindow:           60 * time.Second,
				ActivationScale:        0,
			},
			wantErr: true,
			errMsg:  "activation-scale = 0, must be at least 1",
		},
		{
			name: "multiple validation errors",
			config: &api.AutoscalerConfig{
				ScaleToZeroGracePeriod: -1 * time.Second,
				MaxScaleUpRate:         0.5,
				MaxScaleDownRate:       0.5,
				TargetValue:            -1.0,
				StableWindow:           1 * time.Second,
				MinScale:               -1,
				MaxScale:               -1,
				ActivationScale:        0,
			},
			wantErr: true,
			errMsg:  "configuration errors:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.config)

			if tt.wantErr {
				if err == nil {
					t.Errorf("validate() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if tt.errMsg != "" && !containsString(err.Error(), tt.errMsg) {
					t.Errorf("validate() error = %v, should contain %v", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("validate() unexpected error = %v", err)
			}
		})
	}
}

func TestHelperFunctions(t *testing.T) {
	// Test getEnvString
	t.Run("getEnvString", func(t *testing.T) {
		os.Setenv("AUTOSCALER_TEST_STRING", "test-value")
		defer os.Unsetenv("AUTOSCALER_TEST_STRING")

		if got := getEnvString("TEST_STRING", "default"); got != "test-value" {
			t.Errorf("getEnvString() = %v, want %v", got, "test-value")
		}

		if got := getEnvString("NON_EXISTENT", "default"); got != "default" {
			t.Errorf("getEnvString() = %v, want %v", got, "default")
		}
	})

	// Test parseString
	t.Run("parseString", func(t *testing.T) {
		if got := parseString("test", "default"); got != "test" {
			t.Errorf("parseString() = %v, want %v", got, "test")
		}

		if got := parseString("", "default"); got != "default" {
			t.Errorf("parseString() = %v, want %v", got, "default")
		}
	})
}

// Helper functions for testing

func splitEnvPair(e string) []string {
	if idx := strings.Index(e, "="); idx != -1 {
		return []string{e[:idx], e[idx+1:]}
	}
	return []string{e}
}

func containsString(s, substr string) bool {
	return strings.Contains(s, substr)
}

func configsEqual(a, b *api.AutoscalerConfig) bool {
	if a == nil || b == nil {
		return a == b
	}

	return a.ScaleToZeroGracePeriod == b.ScaleToZeroGracePeriod &&
		a.MaxScaleUpRate == b.MaxScaleUpRate &&
		a.MaxScaleDownRate == b.MaxScaleDownRate &&
		a.TargetValue == b.TargetValue &&
		a.TotalTargetValue == b.TotalTargetValue &&
		a.BurstThreshold == b.BurstThreshold &&
		a.BurstWindowPercentage == b.BurstWindowPercentage &&
		a.StableWindow == b.StableWindow &&
		a.ScaleDownDelay == b.ScaleDownDelay &&
		a.MinScale == b.MinScale &&
		a.MaxScale == b.MaxScale &&
		a.ActivationScale == b.ActivationScale
}
