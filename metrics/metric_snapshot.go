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

package metrics

import "time"

// MetricSnapshot represents a point-in-time view of metrics.
type MetricSnapshot struct {
	stableValue   float64
	burstValue    float64
	readyPodCount int32
	timestamp     time.Time
}

// NewMetricSnapshot creates a new metric snapshot.
func NewMetricSnapshot(stableValue, burstValue float64, readyPods int32, timestamp time.Time) *MetricSnapshot {
	return &MetricSnapshot{
		stableValue:   stableValue,
		burstValue:    burstValue,
		readyPodCount: readyPods,
		timestamp:     timestamp,
	}
}

// StableValue returns the metric value averaged over the stable window.
func (s *MetricSnapshot) StableValue() float64 {
	return s.stableValue
}

// BurstValue returns the metric value averaged over the burst window.
func (s *MetricSnapshot) BurstValue() float64 {
	return s.burstValue
}

// ReadyPodCount returns the number of ready pods.
func (s *MetricSnapshot) ReadyPodCount() int32 {
	return s.readyPodCount
}

// Timestamp returns when this snapshot was taken.
func (s *MetricSnapshot) Timestamp() time.Time {
	return s.timestamp
}
