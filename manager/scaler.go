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

// timeWindowInterface provides a common interface for TimeWindow operations
type timeWindowInterface interface {
	Record(now time.Time, value float64)
	WindowAverage(now time.Time) float64
	IsEmpty(now time.Time) bool
	ResizeWindow(w time.Duration)
}

// Scaler represents a single autoscaler instance that combines metric aggregation
// with a sliding window autoscaling algorithm.
type Scaler struct {
	name             string
	algorithm        *algorithm.SlidingWindowAutoscaler
	stableAggregator timeWindowInterface
	panicAggregator  timeWindowInterface
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
	algoScaler := algorithm.NewSlidingWindowAutoscaler(cfg)

	// Calculate panic window duration
	panicWindow := max(time.Second, time.Duration(float64(cfg.StableWindow) * cfg.PanicWindowPercentage / 100.0))

	// Default granularity of 1 second
	granularity := time.Second

	// Create the appropriate metric aggregators based on algoType
	var stableAgg, panicAgg timeWindowInterface
	switch algoType {
	case "linear":
		stableAgg = metrics.NewTimeWindow(cfg.StableWindow, granularity)
		panicAgg = metrics.NewTimeWindow(panicWindow, granularity)
	case "weighted":
		stableAgg = metrics.NewWeightedTimeWindow(cfg.StableWindow, granularity)
		panicAgg = metrics.NewWeightedTimeWindow(panicWindow, granularity)
	default:
		return nil, fmt.Errorf("unknown algorithm type: %s (expected 'linear' or 'weighted')", algoType)
	}

	return &Scaler{
		name:             name,
		algorithm:        algoScaler,
		stableAggregator: stableAgg,
		panicAggregator:  panicAgg,
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

	// Calculate panic window duration
	panicWindow := max(time.Second, time.Duration(float64(cfg.StableWindow) * cfg.PanicWindowPercentage / 100.0))

	granularity := time.Second

	switch algoType {
	case "linear":
		s.stableAggregator = metrics.NewTimeWindow(cfg.StableWindow, granularity)
		s.panicAggregator = metrics.NewTimeWindow(panicWindow, granularity)
	case "weighted":
		s.stableAggregator = metrics.NewWeightedTimeWindow(cfg.StableWindow, granularity)
		s.panicAggregator = metrics.NewWeightedTimeWindow(panicWindow, granularity)
	default:
		return fmt.Errorf("unknown algorithm type: %s (expected 'linear' or 'weighted')", algoType)
	}

	return nil
}

// Scale calculates the desired scale based on current metrics.
func (s *Scaler) Scale(readyPods int32, now time.Time) api.ScaleRecommendation {
	// Get average values from the aggregators
	stableValue := s.stableAggregator.WindowAverage(now)
	panicValue := s.panicAggregator.WindowAverage(now)

	// If either window is empty, return invalid scale
	if s.stableAggregator.IsEmpty(now) || s.panicAggregator.IsEmpty(now) {
		stableValue = -1
		panicValue = -1
	}

	// Create a metric snapshot
	snapshot := metrics.NewMetricSnapshot(stableValue, panicValue, readyPods, now)

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

	// Calculate panic window duration
	panicWindow := max(time.Second, time.Duration(float64(config.StableWindow) * config.PanicWindowPercentage / 100.0))

	// Resize the aggregators
	s.stableAggregator.ResizeWindow(config.StableWindow)
	s.panicAggregator.ResizeWindow(panicWindow)

	return nil
}

// Record adds a metric value at the given time.
func (s *Scaler) Record(value float64, t time.Time) {
	s.stableAggregator.Record(t, value)
	s.panicAggregator.Record(t, value)
}
