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
	"fmt"
	"time"

	"github.com/Fedosin/libkpa/algorithm"
	"github.com/Fedosin/libkpa/api"
	"github.com/Fedosin/libkpa/metrics"
)

// Scaler represents a single autoscaler instance that combines metric aggregation
// with a sliding window autoscaling algorithm.
type Scaler struct {
	name             string
	algorithm        *algorithm.SlidingWindowAutoscaler
	stableAggregator api.MetricAggregator
	burstAggregator  api.MetricAggregator
}

// NewScaler creates a new Scaler instance with the specified configuration.
// The algoType parameter determines which metric aggregation algorithm to use:
// - "linear": Uses TimeWindow for simple time-based aggregation
// - "weighted": Uses WeightedTimeWindow for weighted aggregation
func NewScaler(
	name string,
	cfg api.AutoscalerConfig,
	algoType string,
) (*Scaler, error) {
	if name == "" {
		return nil, fmt.Errorf("scaler name cannot be empty")
	}

	// Create the sliding window autoscaler
	algoScaler, err := algorithm.NewSlidingWindowAutoscaler(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create sliding window autoscaler: %w", err)
	}

	// Calculate burst window duration
	burstWindow := max(time.Second, time.Duration(float64(cfg.StableWindow)*cfg.BurstWindowPercentage/100.0))

	// Default granularity of 1 second
	granularity := time.Second

	// Create the appropriate metric aggregators based on algoType
	var stableAgg, burstAgg api.MetricAggregator
	switch algoType {
	case "linear":
		stableAgg, err = metrics.NewTimeWindow(cfg.StableWindow, granularity)
		if err != nil {
			return nil, fmt.Errorf("failed to create stable aggregator: %w", err)
		}
		burstAgg, err = metrics.NewTimeWindow(burstWindow, granularity)
		if err != nil {
			return nil, fmt.Errorf("failed to create burst aggregator: %w", err)
		}
	case "weighted":
		stableAgg, err = metrics.NewWeightedTimeWindow(cfg.StableWindow, granularity)
		if err != nil {
			return nil, fmt.Errorf("failed to create stable aggregator: %w", err)
		}
		burstAgg, err = metrics.NewWeightedTimeWindow(burstWindow, granularity)
		if err != nil {
			return nil, fmt.Errorf("failed to create burst aggregator: %w", err)
		}
	default:
		return nil, fmt.Errorf("unknown algorithm type: %s (expected 'linear' or 'weighted')", algoType)
	}

	return &Scaler{
		name:             name,
		algorithm:        algoScaler,
		stableAggregator: stableAgg,
		burstAggregator:  burstAgg,
	}, nil
}

// Name returns the scaler's name.
func (s *Scaler) Name() string {
	return s.name
}

// ChangeAggregationAlgorithm swaps the metric aggregator implementation without
// affecting the autoscaling algorithm. This allows runtime changes to how metrics
// are aggregated.
func (s *Scaler) ChangeAggregationAlgorithm(algoType string) error {
	cfg := s.algorithm.GetConfig()

	// Calculate burst window duration
	burstWindow := max(time.Second, time.Duration(float64(cfg.StableWindow)*cfg.BurstWindowPercentage/100.0))

	granularity := time.Second

	var err error

	switch algoType {
	case "linear":
		s.stableAggregator, err = metrics.NewTimeWindow(cfg.StableWindow, granularity)
		if err != nil {
			return fmt.Errorf("failed to create stable aggregator: %w", err)
		}
		s.burstAggregator, err = metrics.NewTimeWindow(burstWindow, granularity)
		if err != nil {
			return fmt.Errorf("failed to create burst aggregator: %w", err)
		}
	case "weighted":
		s.stableAggregator, err = metrics.NewWeightedTimeWindow(cfg.StableWindow, granularity)
		if err != nil {
			return fmt.Errorf("failed to create stable aggregator: %w", err)
		}
		s.burstAggregator, err = metrics.NewWeightedTimeWindow(burstWindow, granularity)
		if err != nil {
			return fmt.Errorf("failed to create burst aggregator: %w", err)
		}
	default:
		return fmt.Errorf("unknown algorithm type: %s (expected 'linear' or 'weighted')", algoType)
	}

	return nil
}

// Scale calculates the desired scale based on current metrics.
func (s *Scaler) Scale(readyPods int32, now time.Time) api.ScaleRecommendation {
	// Get average values from the aggregators
	stableValue := s.stableAggregator.WindowAverage(now)
	burstValue := s.burstAggregator.WindowAverage(now)

	// If either window is empty, return invalid scale
	if s.stableAggregator.IsEmpty(now) || s.burstAggregator.IsEmpty(now) {
		stableValue = -1
		burstValue = -1
	}

	// Create a metric snapshot
	snapshot := metrics.NewMetricSnapshot(stableValue, burstValue, readyPods, now)

	// Delegate to the algorithm
	return s.algorithm.Scale(snapshot, now)
}

// Config returns the current autoscaler configuration.
func (s *Scaler) Config() api.AutoscalerConfig {
	return s.algorithm.GetConfig()
}

// Update reconfigures the autoscaler with a new spec.
func (s *Scaler) Update(config api.AutoscalerConfig) error {
	// Update the algorithm
	if err := s.algorithm.Update(config); err != nil {
		return err
	}

	// Calculate burst window duration
	burstWindow := max(time.Second, time.Duration(float64(config.StableWindow)*config.BurstWindowPercentage/100.0))

	// Resize the aggregators
	s.stableAggregator.ResizeWindow(config.StableWindow)
	s.burstAggregator.ResizeWindow(burstWindow)

	return nil
}

// Record adds a metric value at the given time.
func (s *Scaler) Record(value float64, t time.Time) {
	s.stableAggregator.Record(t, value)
	s.burstAggregator.Record(t, value)
}
