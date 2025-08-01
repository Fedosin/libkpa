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

// An example of how to use the libkpa library.
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/Fedosin/libkpa/algorithm"
	"github.com/Fedosin/libkpa/api"
	"github.com/Fedosin/libkpa/config"
	"github.com/Fedosin/libkpa/metrics"
	"github.com/Fedosin/libkpa/transmitter"
)

const (
	scalingMetric = "concurrency"
)

// MockMetricCollector simulates collecting metrics from pods
type MockMetricCollector struct {
	baseLoad float64
}

func (m *MockMetricCollector) CollectMetrics() []api.Metrics {
	// Simulate 3 pods with varying load
	pods := []api.Metrics{
		{
			Timestamp: time.Now(),
			Value:     m.baseLoad + rand.Float64()*20,
		},
		{
			Timestamp: time.Now(),
			Value:     m.baseLoad + rand.Float64()*20,
		},
		{
			Timestamp: time.Now(),
			Value:     m.baseLoad + rand.Float64()*20,
		},
	}
	return pods
}

func main() {
	// Load configuration from environment or use defaults
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Override some values for demonstration
	cfg.TargetValue = 100.0
	cfg.StableWindow = 30 * time.Second
	cfg.BurstWindowPercentage = 10.0
	cfg.MinScale = 1
	cfg.MaxScale = 10
	cfg.ScaleDownDelay = 5 * time.Second

	fmt.Println("=== Knative Pod Autoscaler Library Demo ===")
	fmt.Printf("Configuration:\n")
	fmt.Printf("  Scaling Metric: %s\n", scalingMetric)
	fmt.Printf("  Target Value: %.0f\n", cfg.TargetValue)
	fmt.Printf("  Stable Window: %s\n", cfg.StableWindow)
	fmt.Printf("  Min/Max Scale: %d/%d\n", cfg.MinScale, cfg.MaxScale)
	fmt.Println()

	// Create the autoscaler
	autoscaler, err := algorithm.NewSlidingWindowAutoscaler(*cfg)
	if err != nil {
		log.Fatalf("Failed to create autoscaler: %v", err)
	}

	// Create a metric transmitter for logging
	metricTransmitter := transmitter.NewLogTransmitter(nil)

	// Create metric windows for stable and burst averages
	stableWindow, err := metrics.NewTimeWindow(cfg.StableWindow, time.Second)
	if err != nil {
		log.Fatalf("Failed to create new stable time window: %v", err)
	}

	burstWindow, err := metrics.NewTimeWindow(
		max(time.Second, time.Duration(float64(cfg.StableWindow)*cfg.BurstWindowPercentage/100)),
		time.Second,
	)
	if err != nil {
		log.Fatalf("Failed to create new burst time window: %v", err)
	}

	// Create a mock metric collector
	collector := &MockMetricCollector{baseLoad: 80}

	// Simulation parameters
	ctx := context.Background()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	fmt.Println("Starting autoscaler simulation...")
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println()

	// Track current pod count
	currentPods := int32(3)

	// Simulate different load patterns
	loadPhases := []struct {
		name     string
		duration time.Duration
		load     float64
	}{
		{"Normal Load", 20 * time.Second, 80},
		{"High Load", 20 * time.Second, 250},
		{"Spike Load", 10 * time.Second, 500},
		{"Decreasing Load", 20 * time.Second, 50},
		{"Idle", 15 * time.Second, 0},
	}

	phaseIndex := 0
	phaseStart := time.Now()

	for {
		select {
		case <-ticker.C:
			now := time.Now()

			// Update load based on current phase
			if phaseIndex < len(loadPhases) {
				phase := loadPhases[phaseIndex]
				if now.Sub(phaseStart) > phase.duration {
					phaseIndex++
					phaseStart = now
					if phaseIndex < len(loadPhases) {
						fmt.Printf("\n=== Phase: %s ===\n", loadPhases[phaseIndex].name)
					}
				}
				if phaseIndex < len(loadPhases) {
					collector.baseLoad = loadPhases[phaseIndex].load
				}
			}

			// Collect metrics
			podMetrics := collector.CollectMetrics()

			// Calculate total load
			totalConcurrency := 0.0
			for _, pm := range podMetrics {
				totalConcurrency += pm.Value
			}

			// Record in windows
			stableWindow.Record(now, totalConcurrency)
			burstWindow.Record(now, totalConcurrency)

			// Get window averages
			stableAvg := stableWindow.WindowAverage(now)
			burstAvg := burstWindow.WindowAverage(now)

			// Create metric snapshot
			snapshot := metrics.NewMetricSnapshot(
				stableAvg,
				burstAvg,
				currentPods,
				now,
			)

			// Get scaling recommendation
			recommendation := autoscaler.Scale(snapshot, now)

			// Log current state
			fmt.Printf("[%s] Metrics: stable=%.1f, burst=%.1f, current=%d pods\n",
				now.Format("15:04:05"),
				stableAvg,
				burstAvg,
				currentPods,
			)

			if recommendation.ScaleValid {
				// Log recommendation
				action := "maintain"
				if recommendation.DesiredPodCount > currentPods {
					action = "scale up"
				} else if recommendation.DesiredPodCount < currentPods {
					action = "scale down"
				}

				fmt.Printf("  → Recommendation: %s to %d pods", action, recommendation.DesiredPodCount)
				if recommendation.InBurstMode {
					fmt.Print(" [BURST MODE]")
				}
				fmt.Println()

				// Record metrics
				metricTransmitter.RecordDesiredPods(ctx, "default", "example-app", recommendation.DesiredPodCount)
				metricTransmitter.RecordStableValue(ctx, "default", "example-app", scalingMetric, stableAvg)
				metricTransmitter.RecordBurstValue(ctx, "default", "example-app", scalingMetric, burstAvg)
				metricTransmitter.RecordBurstMode(ctx, "default", "example-app", recommendation.InBurstMode)

				// Simulate applying the recommendation
				if recommendation.DesiredPodCount != currentPods {
					fmt.Printf("  → Scaling from %d to %d pods...\n", currentPods, recommendation.DesiredPodCount)
					currentPods = recommendation.DesiredPodCount
				}
			} else {
				fmt.Println("  → No valid recommendation (insufficient data)")
			}

			// Exit after all phases
			if phaseIndex >= len(loadPhases) {
				fmt.Println("\nSimulation complete!")
				return
			}

		case <-ctx.Done():
			return
		}
	}
}
