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

// Package config handles autoscaler configuration loading and validation.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Fedosin/libkpa/api"
)

const (
	// Environment variable names
	EnvPrefix = "AUTOSCALER_"

	// Default values
	defaultMaxScaleUpRate           = 1000.0
	defaultMaxScaleDownRate         = 2.0
	defaultBurstWindowPercentage    = 10.0
	defaultBurstThresholdPercentage = 200.0
	defaultStableWindow             = 60 * time.Second
	defaultScaleToZeroGracePeriod   = 30 * time.Second
	defaultScaleDownDelay           = 0 * time.Second
	defaultInitialScale             = int32(1)
	defaultMinScale                 = int32(0)
	defaultMaxScale                 = int32(0)
	defaultActivationScale          = int32(1)
	defaultTargetValue              = 100.0
	defaultTotalTargetValue         = 0.0

	// Validation constraints
	minStableWindow = 5 * time.Second
	maxStableWindow = 600 * time.Second
	minTargetValue  = 0.01
)

// configErrors aggregates multiple configuration errors
type configErrors struct {
	errors []error
}

func (ce *configErrors) add(err error) {
	if err != nil {
		ce.errors = append(ce.errors, err)
	}
}

func (ce *configErrors) hasErrors() bool {
	return len(ce.errors) > 0
}

func (ce *configErrors) Error() string {
	if len(ce.errors) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("configuration errors:")
	for _, err := range ce.errors {
		sb.WriteString("\n  - ")
		sb.WriteString(err.Error())
	}
	return sb.String()
}

// Load creates a Config from environment variables and validates it.
func Load() (*api.AutoscalerConfig, error) {
	errs := &configErrors{}

	scaleToZeroGracePeriod, err := getEnvDuration("SCALE_TO_ZERO_GRACE_PERIOD", defaultScaleToZeroGracePeriod)
	errs.add(err)

	maxScaleUpRate, err := getEnvFloat("MAX_SCALE_UP_RATE", defaultMaxScaleUpRate)
	errs.add(err)

	maxScaleDownRate, err := getEnvFloat("MAX_SCALE_DOWN_RATE", defaultMaxScaleDownRate)
	errs.add(err)

	targetValue, err := getEnvFloat("TARGET_VALUE", defaultTargetValue)
	errs.add(err)

	totalTargetValue, err := getEnvFloat("TOTAL_TARGET_VALUE", defaultTotalTargetValue)
	errs.add(err)

	burstThreshold, err := getEnvFloat("BURST_THRESHOLD_PERCENTAGE", defaultBurstThresholdPercentage)
	errs.add(err)

	burstWindowPercentage, err := getEnvFloat("BURST_WINDOW_PERCENTAGE", defaultBurstWindowPercentage)
	errs.add(err)

	stableWindow, err := getEnvDuration("STABLE_WINDOW", defaultStableWindow)
	errs.add(err)

	scaleDownDelay, err := getEnvDuration("SCALE_DOWN_DELAY", defaultScaleDownDelay)
	errs.add(err)

	minScale, err := getEnvInt32("MIN_SCALE", defaultMinScale)
	errs.add(err)

	maxScale, err := getEnvInt32("MAX_SCALE", defaultMaxScale)
	errs.add(err)

	activationScale, err := getEnvInt32("ACTIVATION_SCALE", defaultActivationScale)
	errs.add(err)

	if errs.hasErrors() {
		return nil, errs
	}

	cfg := &api.AutoscalerConfig{
		ScaleToZeroGracePeriod: scaleToZeroGracePeriod,
		MaxScaleUpRate:         maxScaleUpRate,
		MaxScaleDownRate:       maxScaleDownRate,
		TargetValue:            targetValue,
		TotalTargetValue:       totalTargetValue,
		BurstThreshold:         burstThreshold,
		BurstWindowPercentage:  burstWindowPercentage,
		StableWindow:           stableWindow,
		ScaleDownDelay:         scaleDownDelay,
		MinScale:               minScale,
		MaxScale:               maxScale,
		ActivationScale:        activationScale,
	}

	// Adjust percentage to fraction if needed
	if cfg.BurstThreshold > 10.0 {
		cfg.BurstThreshold /= 100.0
	}

	// Validate the configuration
	if err = Validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// NewDefaultAutoscalerConfig creates an AutoscalerConfig with all default values.
func NewDefaultAutoscalerConfig() *api.AutoscalerConfig {
	cfg := &api.AutoscalerConfig{
		ScaleToZeroGracePeriod: defaultScaleToZeroGracePeriod,
		MaxScaleUpRate:         defaultMaxScaleUpRate,
		MaxScaleDownRate:       defaultMaxScaleDownRate,
		TargetValue:            defaultTargetValue,
		TotalTargetValue:       defaultTotalTargetValue,
		BurstThreshold:         defaultBurstThresholdPercentage,
		BurstWindowPercentage:  defaultBurstWindowPercentage,
		StableWindow:           defaultStableWindow,
		ScaleDownDelay:         defaultScaleDownDelay,
		MinScale:               defaultMinScale,
		MaxScale:               defaultMaxScale,
		ActivationScale:        defaultActivationScale,
	}

	// Adjust percentage to fraction if needed
	if cfg.BurstThreshold > 10.0 {
		cfg.BurstThreshold /= 100.0
	}

	return cfg
}

// LoadFromMap creates a Config from a map of string values.
func LoadFromMap(data map[string]string) (*api.AutoscalerConfig, error) {
	errs := &configErrors{}

	scaleToZeroGracePeriod, err := parseDuration(data["scale-to-zero-grace-period"], defaultScaleToZeroGracePeriod)
	errs.add(err)

	maxScaleUpRate, err := parseFloat(data["max-scale-up-rate"], defaultMaxScaleUpRate)
	errs.add(err)

	maxScaleDownRate, err := parseFloat(data["max-scale-down-rate"], defaultMaxScaleDownRate)
	errs.add(err)

	targetValue, err := parseFloat(data["target-value"], defaultTargetValue)
	errs.add(err)

	totalTargetValue, err := parseFloat(data["total-target-value"], defaultTotalTargetValue)
	errs.add(err)

	burstThreshold, err := parseFloat(data["burst-threshold-percentage"], defaultBurstThresholdPercentage)
	errs.add(err)

	burstWindowPercentage, err := parseFloat(data["burst-window-percentage"], defaultBurstWindowPercentage)
	errs.add(err)

	stableWindow, err := parseDuration(data["stable-window"], defaultStableWindow)
	errs.add(err)

	scaleDownDelay, err := parseDuration(data["scale-down-delay"], defaultScaleDownDelay)
	errs.add(err)

	minScale, err := parseInt32(data["min-scale"], defaultMinScale)
	errs.add(err)

	maxScale, err := parseInt32(data["max-scale"], defaultMaxScale)
	errs.add(err)

	activationScale, err := parseInt32(data["activation-scale"], defaultActivationScale)
	errs.add(err)

	if errs.hasErrors() {
		return nil, errs
	}

	cfg := &api.AutoscalerConfig{
		ScaleToZeroGracePeriod: scaleToZeroGracePeriod,
		MaxScaleUpRate:         maxScaleUpRate,
		MaxScaleDownRate:       maxScaleDownRate,
		TargetValue:            targetValue,
		TotalTargetValue:       totalTargetValue,
		BurstThreshold:         burstThreshold,
		BurstWindowPercentage:  burstWindowPercentage,
		StableWindow:           stableWindow,
		ScaleDownDelay:         scaleDownDelay,
		MinScale:               minScale,
		MaxScale:               maxScale,
		ActivationScale:        activationScale,
	}

	// Adjust percentage to fraction if needed
	if cfg.BurstThreshold > 10.0 {
		cfg.BurstThreshold /= 100.0
	}

	// Validate the configuration
	if err = Validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate ensures all configuration values are valid.
func Validate(cfg *api.AutoscalerConfig) error {
	errs := &configErrors{}

	// Validate scale-to-zero grace period
	if cfg.ScaleToZeroGracePeriod <= 0 {
		errs.add(fmt.Errorf("scale-to-zero-grace-period must be positive, was: %v", cfg.ScaleToZeroGracePeriod))
	}

	// Validate scale-down delay
	if cfg.ScaleDownDelay < 0 {
		errs.add(fmt.Errorf("scale-down-delay cannot be negative, was: %v", cfg.ScaleDownDelay))
	}
	if cfg.ScaleDownDelay.Round(time.Second) != cfg.ScaleDownDelay {
		errs.add(fmt.Errorf("scale-down-delay = %v, must be specified with at most second precision", cfg.ScaleDownDelay))
	}

	// Validate target values
	if cfg.TargetValue <= 0 && cfg.TotalTargetValue <= 0 {
		errs.add(fmt.Errorf("either target-value or total-target-value must be positive"))
	}
	if cfg.TargetValue > 0 && cfg.TotalTargetValue > 0 {
		errs.add(fmt.Errorf("cannot specify both target-value (%v) and total-target-value (%v)", cfg.TargetValue, cfg.TotalTargetValue))
	}

	// Validate scale rates
	if cfg.MaxScaleUpRate <= 1.0 {
		errs.add(fmt.Errorf("max-scale-up-rate = %v, must be greater than 1.0", cfg.MaxScaleUpRate))
	}
	if cfg.MaxScaleDownRate <= 1.0 {
		errs.add(fmt.Errorf("max-scale-down-rate = %v, must be greater than 1.0", cfg.MaxScaleDownRate))
	}

	// Validate stable window
	if cfg.StableWindow < minStableWindow || cfg.StableWindow > maxStableWindow {
		errs.add(fmt.Errorf("stable-window = %v, must be in [%v; %v] range", cfg.StableWindow, minStableWindow, maxStableWindow))
	}
	if cfg.StableWindow.Round(time.Second) != cfg.StableWindow {
		errs.add(fmt.Errorf("stable-window = %v, must be specified with at most second precision", cfg.StableWindow))
	}

	// Validate burst window percentage
	if cfg.BurstWindowPercentage < 1.0 || cfg.BurstWindowPercentage > 100.0 {
		errs.add(fmt.Errorf("burst-window-percentage = %v, must be in [1.0, 100.0] interval", cfg.BurstWindowPercentage))
	}

	// Validate scale bounds
	if cfg.MinScale < 0 {
		errs.add(fmt.Errorf("min-scale = %v, must be at least 0", cfg.MinScale))
	}
	if cfg.MaxScale < 0 {
		errs.add(fmt.Errorf("max-scale = %v, must be at least 0", cfg.MaxScale))
	}
	if cfg.MinScale > cfg.MaxScale && cfg.MaxScale > 0 {
		errs.add(fmt.Errorf("min-scale (%d) must be less than or equal to max-scale (%d)", cfg.MinScale, cfg.MaxScale))
	}
	if cfg.ActivationScale < 1 {
		errs.add(fmt.Errorf("activation-scale = %v, must be at least 1", cfg.ActivationScale))
	}

	if errs.hasErrors() {
		return errs
	}

	return nil
}

// Helper functions for environment variable parsing
func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(EnvPrefix + key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) (float64, error) {
	value := os.Getenv(EnvPrefix + key)
	if value == "" {
		return defaultValue, nil
	}
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return defaultValue, fmt.Errorf("invalid float value for %s%s: %q", EnvPrefix, key, value)
	}
	return f, nil
}

func getEnvInt32(key string, defaultValue int32) (int32, error) {
	value := os.Getenv(EnvPrefix + key)
	if value == "" {
		return defaultValue, nil
	}
	i, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return defaultValue, fmt.Errorf("invalid int32 value for %s%s: %q", EnvPrefix, key, value)
	}
	return int32(i), nil
}

func getEnvDuration(key string, defaultValue time.Duration) (time.Duration, error) {
	value := os.Getenv(EnvPrefix + key)
	if value == "" {
		return defaultValue, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return defaultValue, fmt.Errorf("invalid duration value for %s%s: %q", EnvPrefix, key, value)
	}
	return d, nil
}

// Helper functions for map parsing
func parseString(value, defaultValue string) string {
	if value != "" {
		return value
	}
	return defaultValue
}

func parseFloat(value string, defaultValue float64) (float64, error) {
	if value == "" {
		return defaultValue, nil
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return defaultValue, fmt.Errorf("invalid float value: %q", value)
	}
	return f, nil
}

func parseInt32(value string, defaultValue int32) (int32, error) {
	if value == "" {
		return defaultValue, nil
	}
	i, err := strconv.ParseInt(strings.TrimSpace(value), 10, 32)
	if err != nil {
		return defaultValue, fmt.Errorf("invalid int32 value: %q", value)
	}
	return int32(i), nil
}

func parseDuration(value string, defaultValue time.Duration) (time.Duration, error) {
	if value == "" {
		return defaultValue, nil
	}
	d, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return defaultValue, fmt.Errorf("invalid duration value: %q", value)
	}
	return d, nil
}
