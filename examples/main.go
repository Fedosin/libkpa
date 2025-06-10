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

// MockMetricCollector simulates collecting metrics from pods
type MockMetricCollector struct {
	baseLoad float64
}

func (m *MockMetricCollector) CollectMetrics() []api.PodMetrics {
	// Simulate 3 pods with varying load
	pods := []api.PodMetrics{
		{
			PodName:            "app-pod-1",
			Timestamp:          time.Now(),
			ConcurrentRequests: m.baseLoad + rand.Float64()*20,
			RequestsPerSecond:  (m.baseLoad + rand.Float64()*20) * 2,
			ProcessUptime:      5 * time.Minute,
		},
		{
			PodName:            "app-pod-2",
			Timestamp:          time.Now(),
			ConcurrentRequests: m.baseLoad + rand.Float64()*20,
			RequestsPerSecond:  (m.baseLoad + rand.Float64()*20) * 2,
			ProcessUptime:      5 * time.Minute,
		},
		{
			PodName:            "app-pod-3",
			Timestamp:          time.Now(),
			ConcurrentRequests: m.baseLoad + rand.Float64()*20,
			RequestsPerSecond:  (m.baseLoad + rand.Float64()*20) * 2,
			ProcessUptime:      5 * time.Minute,
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
	cfg.PanicWindowPercentage = 10.0
	cfg.MinScale = 1
	cfg.MaxScale = 10
	cfg.ScaleDownDelay = 5 * time.Second

	fmt.Println("=== Knative Pod Autoscaler Library Demo ===")
	fmt.Printf("Configuration:\n")
	fmt.Printf("  Scaling Metric: %s\n", cfg.ScalingMetric)
	fmt.Printf("  Target Value: %.0f\n", cfg.TargetValue)
	fmt.Printf("  Stable Window: %s\n", cfg.StableWindow)
	fmt.Printf("  Min/Max Scale: %d/%d\n", cfg.MinScale, cfg.MaxScale)
	fmt.Println()

	// Create the autoscaler
	autoscaler := algorithm.NewSlidingWindowAutoscaler(cfg.AutoscalerSpec)

	// Create a metric transmitter for logging
	metricTransmitter := transmitter.NewLogTransmitter(nil)

	// Create metric windows for stable and panic averages
	stableWindow := metrics.NewTimeWindow(cfg.StableWindow, time.Second)
	panicWindow := metrics.NewTimeWindow(
		time.Duration(float64(cfg.StableWindow)*cfg.PanicWindowPercentage/100),
		time.Second,
	)

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
			totalRPS := 0.0
			for _, pm := range podMetrics {
				totalConcurrency += pm.ConcurrentRequests
				totalRPS += pm.RequestsPerSecond
			}

			// Record in windows
			stableWindow.Record(now, totalConcurrency)
			panicWindow.Record(now, totalConcurrency)

			// Get window averages
			stableAvg := stableWindow.WindowAverage(now)
			panicAvg := panicWindow.WindowAverage(now)

			// Create metric snapshot
			snapshot := metrics.NewMetricSnapshot(
				stableAvg,
				panicAvg,
				currentPods,
				now,
			)

			// Get scaling recommendation
			recommendation := autoscaler.Scale(ctx, snapshot, now)

			// Log current state
			fmt.Printf("[%s] Metrics: stable=%.1f, panic=%.1f, current=%d pods\n",
				now.Format("15:04:05"),
				stableAvg,
				panicAvg,
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
				if recommendation.InPanicMode {
					fmt.Print(" [PANIC MODE]")
				}
				fmt.Println()

				// Record metrics
				metricTransmitter.RecordDesiredPods(ctx, "default", "example-app", recommendation.DesiredPodCount)
				metricTransmitter.RecordStableValue(ctx, "default", "example-app", cfg.ScalingMetric, stableAvg)
				metricTransmitter.RecordPanicValue(ctx, "default", "example-app", cfg.ScalingMetric, panicAvg)
				metricTransmitter.RecordPanicMode(ctx, "default", "example-app", recommendation.InPanicMode)

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
