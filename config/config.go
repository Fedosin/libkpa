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
	defaultTargetUtilization                  = 0.7
	defaultContainerConcurrencyTargetFraction = 0.7
	defaultContainerConcurrencyTargetDefault  = 100.0
	defaultRPSTargetDefault                   = 200.0
	defaultMaxScaleUpRate                     = 1000.0
	defaultMaxScaleDownRate                   = 2.0
	defaultTargetBurstCapacity                = 211.0
	defaultPanicWindowPercentage              = 10.0
	defaultPanicThresholdPercentage           = 200.0
	defaultStableWindow                       = 60 * time.Second
	defaultScaleToZeroGracePeriod             = 30 * time.Second
	defaultScaleDownDelay                     = 0 * time.Second
	defaultInitialScale                       = int32(1)
	defaultMinScale                           = int32(0)
	defaultMaxScale                           = int32(0)
	defaultActivationScale                    = int32(1)

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
func Load() (*api.Config, error) {
	errs := &configErrors{}

	enableScaleToZero, err := getEnvBool("ENABLE_SCALE_TO_ZERO", true)
	errs.add(err)

	scaleToZeroGracePeriod, err := getEnvDuration("SCALE_TO_ZERO_GRACE_PERIOD", defaultScaleToZeroGracePeriod)
	errs.add(err)

	containerConcurrencyTargetFraction, err := getEnvFloat("CONTAINER_CONCURRENCY_TARGET_PERCENTAGE", defaultContainerConcurrencyTargetFraction)
	errs.add(err)

	containerConcurrencyTargetDefault, err := getEnvFloat("CONTAINER_CONCURRENCY_TARGET_DEFAULT", defaultContainerConcurrencyTargetDefault)
	errs.add(err)

	rpsTargetDefault, err := getEnvFloat("RPS_TARGET_DEFAULT", defaultRPSTargetDefault)
	errs.add(err)

	targetUtilization, err := getEnvFloat("TARGET_UTILIZATION", defaultTargetUtilization)
	errs.add(err)

	maxScaleUpRate, err := getEnvFloat("MAX_SCALE_UP_RATE", defaultMaxScaleUpRate)
	errs.add(err)

	maxScaleDownRate, err := getEnvFloat("MAX_SCALE_DOWN_RATE", defaultMaxScaleDownRate)
	errs.add(err)

	targetValue, err := getEnvFloat("TARGET_VALUE", defaultContainerConcurrencyTargetDefault)
	errs.add(err)

	totalValue, err := getEnvFloat("TOTAL_VALUE", 1000.0)
	errs.add(err)

	targetBurstCapacity, err := getEnvFloat("TARGET_BURST_CAPACITY", defaultTargetBurstCapacity)
	errs.add(err)

	panicThreshold, err := getEnvFloat("PANIC_THRESHOLD_PERCENTAGE", defaultPanicThresholdPercentage)
	errs.add(err)

	panicWindowPercentage, err := getEnvFloat("PANIC_WINDOW_PERCENTAGE", defaultPanicWindowPercentage)
	errs.add(err)

	stableWindow, err := getEnvDuration("STABLE_WINDOW", defaultStableWindow)
	errs.add(err)

	scaleDownDelay, err := getEnvDuration("SCALE_DOWN_DELAY", defaultScaleDownDelay)
	errs.add(err)

	initialScale, err := getEnvInt32("INITIAL_SCALE", defaultInitialScale)
	errs.add(err)

	minScale, err := getEnvInt32("MIN_SCALE", defaultMinScale)
	errs.add(err)

	maxScale, err := getEnvInt32("MAX_SCALE", defaultMaxScale)
	errs.add(err)

	activationScale, err := getEnvInt32("ACTIVATION_SCALE", defaultActivationScale)
	errs.add(err)

	reachable, err := getEnvBool("REACHABLE", true)
	errs.add(err)

	if errs.hasErrors() {
		return nil, errs
	}

	cfg := &api.Config{
		EnableScaleToZero:                  enableScaleToZero,
		ScaleToZeroGracePeriod:             scaleToZeroGracePeriod,
		ContainerConcurrencyTargetFraction: containerConcurrencyTargetFraction,
		ContainerConcurrencyTargetDefault:  containerConcurrencyTargetDefault,
		RPSTargetDefault:                   rpsTargetDefault,
		TargetUtilization:                  targetUtilization,
		AutoscalerSpec: api.AutoscalerSpec{
			MaxScaleUpRate:        maxScaleUpRate,
			MaxScaleDownRate:      maxScaleDownRate,
			ScalingMetric:         api.ScalingMetric(getEnvString("SCALING_METRIC", string(api.Concurrency))),
			TargetValue:           targetValue,
			TotalValue:            totalValue,
			TargetBurstCapacity:   targetBurstCapacity,
			PanicThreshold:        panicThreshold,
			PanicWindowPercentage: panicWindowPercentage,
			StableWindow:          stableWindow,
			ScaleDownDelay:        scaleDownDelay,
			InitialScale:          initialScale,
			MinScale:              minScale,
			MaxScale:              maxScale,
			ActivationScale:       activationScale,
			Reachable:             reachable,
		},
	}

	// Adjust percentage to fraction if needed
	if cfg.ContainerConcurrencyTargetFraction > 1.0 {
		cfg.ContainerConcurrencyTargetFraction /= 100.0
	}
	if cfg.PanicThreshold > 10.0 {
		cfg.PanicThreshold /= 100.0
	}

	return validate(cfg)
}

// LoadFromMap creates a Config from a map of string values.
func LoadFromMap(data map[string]string) (*api.Config, error) {
	errs := &configErrors{}

	enableScaleToZero, err := parseBool(data["enable-scale-to-zero"], true)
	errs.add(err)

	scaleToZeroGracePeriod, err := parseDuration(data["scale-to-zero-grace-period"], defaultScaleToZeroGracePeriod)
	errs.add(err)

	containerConcurrencyTargetFraction, err := parseFloat(data["container-concurrency-target-percentage"], defaultContainerConcurrencyTargetFraction)
	errs.add(err)

	containerConcurrencyTargetDefault, err := parseFloat(data["container-concurrency-target-default"], defaultContainerConcurrencyTargetDefault)
	errs.add(err)

	rpsTargetDefault, err := parseFloat(data["requests-per-second-target-default"], defaultRPSTargetDefault)
	errs.add(err)

	targetUtilization, err := parseFloat(data["target-utilization"], defaultTargetUtilization)
	errs.add(err)

	maxScaleUpRate, err := parseFloat(data["max-scale-up-rate"], defaultMaxScaleUpRate)
	errs.add(err)

	maxScaleDownRate, err := parseFloat(data["max-scale-down-rate"], defaultMaxScaleDownRate)
	errs.add(err)

	targetValue, err := parseFloat(data["target-value"], defaultContainerConcurrencyTargetDefault)
	errs.add(err)

	totalValue, err := parseFloat(data["total-value"], 1000.0)
	errs.add(err)

	targetBurstCapacity, err := parseFloat(data["target-burst-capacity"], defaultTargetBurstCapacity)
	errs.add(err)

	panicThreshold, err := parseFloat(data["panic-threshold-percentage"], defaultPanicThresholdPercentage)
	errs.add(err)

	panicWindowPercentage, err := parseFloat(data["panic-window-percentage"], defaultPanicWindowPercentage)
	errs.add(err)

	stableWindow, err := parseDuration(data["stable-window"], defaultStableWindow)
	errs.add(err)

	scaleDownDelay, err := parseDuration(data["scale-down-delay"], defaultScaleDownDelay)
	errs.add(err)

	initialScale, err := parseInt32(data["initial-scale"], defaultInitialScale)
	errs.add(err)

	minScale, err := parseInt32(data["min-scale"], defaultMinScale)
	errs.add(err)

	maxScale, err := parseInt32(data["max-scale"], defaultMaxScale)
	errs.add(err)

	activationScale, err := parseInt32(data["activation-scale"], defaultActivationScale)
	errs.add(err)

	reachable, err := parseBool(data["reachable"], true)
	errs.add(err)

	if errs.hasErrors() {
		return nil, errs
	}

	cfg := &api.Config{
		EnableScaleToZero:                  enableScaleToZero,
		ScaleToZeroGracePeriod:             scaleToZeroGracePeriod,
		ContainerConcurrencyTargetFraction: containerConcurrencyTargetFraction,
		ContainerConcurrencyTargetDefault:  containerConcurrencyTargetDefault,
		RPSTargetDefault:                   rpsTargetDefault,
		TargetUtilization:                  targetUtilization,
		AutoscalerSpec: api.AutoscalerSpec{
			MaxScaleUpRate:        maxScaleUpRate,
			MaxScaleDownRate:      maxScaleDownRate,
			ScalingMetric:         api.ScalingMetric(parseString(data["scaling-metric"], string(api.Concurrency))),
			TargetValue:           targetValue,
			TotalValue:            totalValue,
			TargetBurstCapacity:   targetBurstCapacity,
			PanicThreshold:        panicThreshold,
			PanicWindowPercentage: panicWindowPercentage,
			StableWindow:          stableWindow,
			ScaleDownDelay:        scaleDownDelay,
			InitialScale:          initialScale,
			MinScale:              minScale,
			MaxScale:              maxScale,
			ActivationScale:       activationScale,
			Reachable:             reachable,
		},
	}

	// Adjust percentage to fraction if needed
	if cfg.ContainerConcurrencyTargetFraction > 1.0 {
		cfg.ContainerConcurrencyTargetFraction /= 100.0
	}
	if cfg.PanicThreshold > 10.0 {
		cfg.PanicThreshold /= 100.0
	}

	return validate(cfg)
}

// validate ensures all configuration values are valid.
func validate(cfg *api.Config) (*api.Config, error) {
	// Validate scale-to-zero grace period
	if cfg.ScaleToZeroGracePeriod <= 0 {
		return nil, fmt.Errorf("scale-to-zero-grace-period must be positive, was: %v", cfg.ScaleToZeroGracePeriod)
	}

	// Validate scale-down delay
	if cfg.ScaleDownDelay < 0 {
		return nil, fmt.Errorf("scale-down-delay cannot be negative, was: %v", cfg.ScaleDownDelay)
	}
	if cfg.ScaleDownDelay.Round(time.Second) != cfg.ScaleDownDelay {
		return nil, fmt.Errorf("scale-down-delay = %v, must be specified with at most second precision", cfg.ScaleDownDelay)
	}

	// Validate target burst capacity
	if cfg.TargetBurstCapacity < 0 && cfg.TargetBurstCapacity != -1 {
		return nil, fmt.Errorf("target-burst-capacity must be either non-negative or -1 (for unlimited), was: %f", cfg.TargetBurstCapacity)
	}

	// Validate container concurrency target fraction
	if cfg.ContainerConcurrencyTargetFraction <= 0 || cfg.ContainerConcurrencyTargetFraction > 1 {
		return nil, fmt.Errorf("container-concurrency-target-fraction = %f is outside of valid range of (0, 1]", cfg.ContainerConcurrencyTargetFraction)
	}

	// Validate target values
	if x := cfg.ContainerConcurrencyTargetFraction * cfg.ContainerConcurrencyTargetDefault; x < minTargetValue {
		return nil, fmt.Errorf("container-concurrency-target-fraction and container-concurrency-target-default yield target concurrency of %v, can't be less than %v", x, minTargetValue)
	}
	if cfg.RPSTargetDefault < minTargetValue {
		return nil, fmt.Errorf("rps-target-default must be at least %v, was: %v", minTargetValue, cfg.RPSTargetDefault)
	}
	if cfg.TargetValue > cfg.TotalValue {
		return nil, fmt.Errorf("target-value = %v, must be less than or equal to total-value = %v", cfg.TargetValue, cfg.TotalValue)
	}
	if cfg.TargetValue <= 0 {
		return nil, fmt.Errorf("target-value = %v, must be positive", cfg.TargetValue)
	}
	if cfg.TotalValue <= 0 {
		return nil, fmt.Errorf("total-value = %v, must be positive", cfg.TotalValue)
	}

	// Validate scale rates
	if cfg.MaxScaleUpRate <= 1.0 {
		return nil, fmt.Errorf("max-scale-up-rate = %v, must be greater than 1.0", cfg.MaxScaleUpRate)
	}
	if cfg.MaxScaleDownRate <= 1.0 {
		return nil, fmt.Errorf("max-scale-down-rate = %v, must be greater than 1.0", cfg.MaxScaleDownRate)
	}

	// Validate stable window
	if cfg.StableWindow < minStableWindow || cfg.StableWindow > maxStableWindow {
		return nil, fmt.Errorf("stable-window = %v, must be in [%v; %v] range", cfg.StableWindow, minStableWindow, maxStableWindow)
	}
	if cfg.StableWindow.Round(time.Second) != cfg.StableWindow {
		return nil, fmt.Errorf("stable-window = %v, must be specified with at most second precision", cfg.StableWindow)
	}

	// Validate panic window percentage
	if cfg.PanicWindowPercentage < 1.0 || cfg.PanicWindowPercentage > 100.0 {
		return nil, fmt.Errorf("panic-window-percentage = %v, must be in [1.0, 100.0] interval", cfg.PanicWindowPercentage)
	}

	// Validate scale bounds
	if cfg.InitialScale < 0 {
		return nil, fmt.Errorf("initial-scale = %v, must be at least 0", cfg.InitialScale)
	}
	if cfg.MinScale < 0 {
		return nil, fmt.Errorf("min-scale = %v, must be at least 0", cfg.MinScale)
	}
	if cfg.MaxScale < 0 {
		return nil, fmt.Errorf("max-scale = %v, must be at least 0", cfg.MaxScale)
	}
	if cfg.MinScale > cfg.MaxScale && cfg.MaxScale > 0 {
		return nil, fmt.Errorf("min-scale (%d) must be less than or equal to max-scale (%d)", cfg.MinScale, cfg.MaxScale)
	}
	if cfg.ActivationScale < 1 {
		return nil, fmt.Errorf("activation-scale = %v, must be at least 1", cfg.ActivationScale)
	}

	// Validate scaling metric
	switch cfg.ScalingMetric {
	case api.Concurrency, api.RPS:
		// Valid
	default:
		return nil, fmt.Errorf("scaling-metric = %s, must be either 'concurrency' or 'rps'", cfg.ScalingMetric)
	}

	return cfg, nil
}

// Helper functions for environment variable parsing
func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(EnvPrefix + key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) (bool, error) {
	value := os.Getenv(EnvPrefix + key)
	if value == "" {
		return defaultValue, nil
	}
	b, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue, fmt.Errorf("invalid boolean value for %s%s: %q", EnvPrefix, key, value)
	}
	return b, nil
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

func parseBool(value string, defaultValue bool) (bool, error) {
	if value == "" {
		return defaultValue, nil
	}
	b, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue, fmt.Errorf("invalid boolean value: %q", value)
	}
	return b, nil
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
