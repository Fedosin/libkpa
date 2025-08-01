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

// Package api contains the API types and interfaces for the KPA autoscaler library.
package api

import (
	"time"
)

// AutoscalerConfig defines the parameters for autoscaling behavior.
type AutoscalerConfig struct {
	// MaxScaleUpRate is the maximum rate at which the autoscaler will scale up pods.
	// It must be greater than 1.0. For example, a value of 2.0 allows scaling up
	// by at most doubling the pod count. Default is 1000.0.
	MaxScaleUpRate float64

	// MaxScaleDownRate is the maximum rate at which the autoscaler will scale down pods.
	// It must be greater than 1.0. For example, a value of 2.0 allows scaling down
	// by at most halving the pod count. Default is 2.0.
	MaxScaleDownRate float64

	// TargetValue is the desired value of the scaling metric per pod that we aim to maintain.
	// Default is 100.0.
	TargetValue float64

	// TotalTargetValue is the total desired value of the scaling metric.
	// Default is 1000.0.
	TotalTargetValue float64

	// BurstThreshold is the threshold for entering burst mode, expressed as a
	// percentage of desired pod count. If the observed load over the burst window
	// exceeds this percentage of the current pod count capacity, burst mode is triggered.
	// Default is 200 (200%).
	BurstThreshold float64

	// BurstWindowPercentage is the percentage of the stable window used for
	// burst mode calculations. Must be in range [1.0, 100.0]. Default is 10.0.
	BurstWindowPercentage float64

	// StableWindow is the time window over which metrics are averaged for
	// scaling decisions. Must be between 5s and 600s. Default is 60s.
	StableWindow time.Duration

	// ScaleDownDelay is the minimum time that must pass at reduced load
	// before scaling down. Default is 0s (immediate scale down).
	ScaleDownDelay time.Duration

	// MinScale is the minimum number of pods to maintain. Must be >= 0.
	// Default is 0 (can scale to zero).
	MinScale int32

	// MaxScale is the maximum number of pods to maintain. 0 means unlimited.
	// Default is 0.
	MaxScale int32

	// ActivationScale is the minimum scale to use when scaling from zero.
	// Must be >= 1. Default is 1.
	ActivationScale int32

	// ScaleToZeroGracePeriod is the time to wait before scaling to zero
	// after the service becomes idle. Default is 30s.
	ScaleToZeroGracePeriod time.Duration
}

// Metrics represents collected metrics.
type Metrics struct {
	// Timestamp is when these metrics were collected.
	Timestamp time.Time

	// Value is the metric value.
	Value float64
}

// ScaleRecommendation represents the autoscaler's scaling recommendation.
type ScaleRecommendation struct {
	// DesiredPodCount is the recommended number of pods.
	DesiredPodCount int32

	// ScaleValid indicates whether the recommendation is valid.
	// False if insufficient data was available.
	ScaleValid bool

	// InBurstMode indicates whether the autoscaler is in burst mode.
	InBurstMode bool
}
