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

	"github.com/Fedosin/libkpa/api"
)

// MetricTransmitter defines the interface for transmitting autoscaler metrics.
type MetricTransmitter interface {
	// RecordDesiredPods records the desired pod count metric.
	RecordDesiredPods(ctx context.Context, namespace, service string, value int32)

	// RecordStableValue records the stable window metric value.
	RecordStableValue(ctx context.Context, namespace, service string, metric api.ScalingMetric, value float64)

	// RecordPanicValue records the panic window metric value.
	RecordPanicValue(ctx context.Context, namespace, service string, metric api.ScalingMetric, value float64)

	// RecordTargetValue records the target metric value.
	RecordTargetValue(ctx context.Context, namespace, service string, metric api.ScalingMetric, value float64)

	// RecordExcessBurstCapacity records the excess burst capacity.
	RecordExcessBurstCapacity(ctx context.Context, namespace, service string, value float64)

	// RecordPanicMode records whether the autoscaler is in panic mode.
	RecordPanicMode(ctx context.Context, namespace, service string, inPanic bool)
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
func (t *LogTransmitter) RecordStableValue(ctx context.Context, namespace, service string, metric api.ScalingMetric, value float64) {
	t.logger.Printf("metric: stable_%s{namespace=%s,service=%s} = %.2f\n", metric, namespace, service, value)
}

// RecordPanicValue logs the panic window metric value.
func (t *LogTransmitter) RecordPanicValue(ctx context.Context, namespace, service string, metric api.ScalingMetric, value float64) {
	t.logger.Printf("metric: panic_%s{namespace=%s,service=%s} = %.2f\n", metric, namespace, service, value)
}

// RecordTargetValue logs the target metric value.
func (t *LogTransmitter) RecordTargetValue(ctx context.Context, namespace, service string, metric api.ScalingMetric, value float64) {
	t.logger.Printf("metric: target_%s{namespace=%s,service=%s} = %.2f\n", metric, namespace, service, value)
}

// RecordExcessBurstCapacity logs the excess burst capacity.
func (t *LogTransmitter) RecordExcessBurstCapacity(ctx context.Context, namespace, service string, value float64) {
	t.logger.Printf("metric: excess_burst_capacity{namespace=%s,service=%s} = %.2f\n", namespace, service, value)
}

// RecordPanicMode logs whether the autoscaler is in panic mode.
func (t *LogTransmitter) RecordPanicMode(ctx context.Context, namespace, service string, inPanic bool) {
	panicValue := 0
	if inPanic {
		panicValue = 1
	}
	t.logger.Printf("metric: panic_mode{namespace=%s,service=%s} = %d\n", namespace, service, panicValue)
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
func (t *NoOpTransmitter) RecordStableValue(ctx context.Context, namespace, service string, metric api.ScalingMetric, value float64) {
}

// RecordPanicValue does nothing.
func (t *NoOpTransmitter) RecordPanicValue(ctx context.Context, namespace, service string, metric api.ScalingMetric, value float64) {
}

// RecordTargetValue does nothing.
func (t *NoOpTransmitter) RecordTargetValue(ctx context.Context, namespace, service string, metric api.ScalingMetric, value float64) {
}

// RecordExcessBurstCapacity does nothing.
func (t *NoOpTransmitter) RecordExcessBurstCapacity(ctx context.Context, namespace, service string, value float64) {
}

// RecordPanicMode does nothing.
func (t *NoOpTransmitter) RecordPanicMode(ctx context.Context, namespace, service string, inPanic bool) {
}
