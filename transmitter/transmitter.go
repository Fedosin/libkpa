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

// Package transmitter provides metric reporting capabilities for the autoscaler.
package transmitter

import (
	"context"
	"log"
)

// MetricTransmitter defines the interface for transmitting autoscaler metrics.
type MetricTransmitter interface {
	// RecordDesiredPods records the desired pod count metric.
	RecordDesiredPods(ctx context.Context, namespace, service string, value int32)

	// RecordStableValue records the stable window metric value.
	RecordStableValue(ctx context.Context, namespace, service string, metric string, value float64)

	// RecordBurstValue records the burst window metric value.
	RecordBurstValue(ctx context.Context, namespace, service string, metric string, value float64)

	// RecordTargetValue records the target metric value.
	RecordTargetValue(ctx context.Context, namespace, service string, metric string, value float64)

	// RecordBurstMode records whether the autoscaler is in burst mode.
	RecordBurstMode(ctx context.Context, namespace, service string, inBurst bool)
}

// LogTransmitter is a simple transmitter that logs metrics to stdout.
type LogTransmitter struct {
	logger *log.Logger
}

// NewLogTransmitter creates a new log-based metric transmitter.
func NewLogTransmitter(logger *log.Logger) *LogTransmitter {
	if logger == nil {
		logger = log.Default()
	}
	return &LogTransmitter{
		logger: logger,
	}
}

// RecordDesiredPods logs the desired pod count.
func (t *LogTransmitter) RecordDesiredPods(ctx context.Context, namespace, service string, value int32) {
	t.logger.Printf("metric: desired_pods{namespace=%s,service=%s} = %d\n", namespace, service, value)
}

// RecordStableValue logs the stable window metric value.
func (t *LogTransmitter) RecordStableValue(ctx context.Context, namespace, service string, metric string, value float64) {
	t.logger.Printf("metric: stable_%s{namespace=%s,service=%s} = %.2f\n", metric, namespace, service, value)
}

// RecordBurstValue logs the burst window metric value.
func (t *LogTransmitter) RecordBurstValue(ctx context.Context, namespace, service string, metric string, value float64) {
	t.logger.Printf("metric: burst_%s{namespace=%s,service=%s} = %.2f\n", metric, namespace, service, value)
}

// RecordTargetValue logs the target metric value.
func (t *LogTransmitter) RecordTargetValue(ctx context.Context, namespace, service string, metric string, value float64) {
	t.logger.Printf("metric: target_%s{namespace=%s,service=%s} = %.2f\n", metric, namespace, service, value)
}

// RecordBurstMode logs whether the autoscaler is in burst mode.
func (t *LogTransmitter) RecordBurstMode(ctx context.Context, namespace, service string, inBurst bool) {
	burstValue := 0
	if inBurst {
		burstValue = 1
	}
	t.logger.Printf("metric: burst_mode{namespace=%s,service=%s} = %d\n", namespace, service, burstValue)
}

// NoOpTransmitter is a transmitter that does nothing.
type NoOpTransmitter struct{}

// NewNoOpTransmitter creates a new no-op transmitter.
func NewNoOpTransmitter() *NoOpTransmitter {
	return &NoOpTransmitter{}
}

// RecordDesiredPods does nothing.
func (t *NoOpTransmitter) RecordDesiredPods(ctx context.Context, namespace, service string, value int32) {
}

// RecordStableValue does nothing.
func (t *NoOpTransmitter) RecordStableValue(ctx context.Context, namespace, service string, metric string, value float64) {
}

// RecordBurstValue does nothing.
func (t *NoOpTransmitter) RecordBurstValue(ctx context.Context, namespace, service string, metric string, value float64) {
}

// RecordTargetValue does nothing.
func (t *NoOpTransmitter) RecordTargetValue(ctx context.Context, namespace, service string, metric string, value float64) {
}

// RecordBurstMode does nothing.
func (t *NoOpTransmitter) RecordBurstMode(ctx context.Context, namespace, service string, inBurst bool) {
}
