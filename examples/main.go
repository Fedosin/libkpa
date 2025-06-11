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
	"runtime"
	"time"

	"github.com/Fedosin/libkpa/algorithm"
	"github.com/Fedosin/libkpa/api"
	"github.com/Fedosin/libkpa/config"
	"github.com/Fedosin/libkpa/metrics"
	"github.com/Fedosin/libkpa/transmitter"
)

// ProfilingStats tracks performance metrics
type ProfilingStats struct {
	ScalingDecisions     int
	TotalScalingTime     time.Duration
	MinScalingTime       time.Duration
	MaxScalingTime       time.Duration
	WindowOperationTime  time.Duration
	MetricCollectionTime time.Duration
	MemoryAllocations    []runtime.MemStats
}

func (p *ProfilingStats) RecordScalingTime(duration time.Duration) {
	p.ScalingDecisions++
	p.TotalScalingTime += duration
	if p.MinScalingTime == 0 || duration < p.MinScalingTime {
		p.MinScalingTime = duration
	}
	if duration > p.MaxScalingTime {
		p.MaxScalingTime = duration
	}
}

func (p *ProfilingStats) AverageScalingTime() time.Duration {
	if p.ScalingDecisions == 0 {
		return 0
	}
	return p.TotalScalingTime / time.Duration(p.ScalingDecisions)
}

func (p *ProfilingStats) PrintSummary() {
	fmt.Println("\n=== Profiling Summary ===")
	fmt.Printf("Total Scaling Decisions: %d\n", p.ScalingDecisions)
	fmt.Printf("Average Scaling Time: %v\n", p.AverageScalingTime())
	fmt.Printf("Min Scaling Time: %v\n", p.MinScalingTime)
	fmt.Printf("Max Scaling Time: %v\n", p.MaxScalingTime)
	fmt.Printf("Total Window Operations Time: %v\n", p.WindowOperationTime)
	fmt.Printf("Total Metric Collection Time: %v\n", p.MetricCollectionTime)

	if len(p.MemoryAllocations) > 0 {
		first := p.MemoryAllocations[0]
		last := p.MemoryAllocations[len(p.MemoryAllocations)-1]
		fmt.Printf("\nMemory Usage:\n")
		fmt.Printf("  Initial Heap: %.2f MB\n", float64(first.HeapAlloc)/1024/1024)
		fmt.Printf("  Final Heap: %.2f MB\n", float64(last.HeapAlloc)/1024/1024)
		fmt.Printf("  Peak Heap: %.2f MB\n", func() float64 {
			max := uint64(0)
			for _, m := range p.MemoryAllocations {
				if m.HeapAlloc > max {
					max = m.HeapAlloc
				}
			}
			return float64(max) / 1024 / 1024
		}())
		fmt.Printf("  Total GC Runs: %d\n", last.NumGC-first.NumGC)
		fmt.Printf("  Total GC Pause: %v\n", time.Duration(last.PauseTotalNs-first.PauseTotalNs))
	}
}

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

	// Initialize profiling stats
	profStats := &ProfilingStats{}

	// Simulation parameters
	ctx := context.Background()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Memory profiling ticker
	memTicker := time.NewTicker(5 * time.Second)
	defer memTicker.Stop()

	fmt.Println("Starting autoscaler simulation with profiling...")
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
	simulationStart := time.Now()

	// Collect initial memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	profStats.MemoryAllocations = append(profStats.MemoryAllocations, memStats)

	for {
		select {
		case <-memTicker.C:
			// Collect memory stats periodically
			runtime.ReadMemStats(&memStats)
			profStats.MemoryAllocations = append(profStats.MemoryAllocations, memStats)

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

			// Time metric collection
			collectionStart := time.Now()

			// Collect metrics
			podMetrics := collector.CollectMetrics()

			// Calculate total load
			totalConcurrency := 0.0
			totalRPS := 0.0
			for _, pm := range podMetrics {
				totalConcurrency += pm.ConcurrentRequests
				totalRPS += pm.RequestsPerSecond
			}

			profStats.MetricCollectionTime += time.Since(collectionStart)

			// Time window operations
			windowStart := time.Now()

			// Record in windows
			stableWindow.Record(now, totalConcurrency)
			panicWindow.Record(now, totalConcurrency)

			// Get window averages
			stableAvg := stableWindow.WindowAverage(now)
			panicAvg := panicWindow.WindowAverage(now)

			profStats.WindowOperationTime += time.Since(windowStart)

			// Create metric snapshot
			snapshot := metrics.NewMetricSnapshot(
				stableAvg,
				panicAvg,
				currentPods,
				now,
			)

			// Time the scaling decision
			scalingStart := time.Now()

			// Get scaling recommendation
			recommendation := autoscaler.Scale(ctx, snapshot, now)

			scalingDuration := time.Since(scalingStart)
			profStats.RecordScalingTime(scalingDuration)

			// Log current state with timing info
			fmt.Printf("[%s] Metrics: stable=%.1f, panic=%.1f, current=%d pods (scaling took %v)\n",
				now.Format("15:04:05"),
				stableAvg,
				panicAvg,
				currentPods,
				scalingDuration,
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
				// Collect final memory stats
				runtime.ReadMemStats(&memStats)
				profStats.MemoryAllocations = append(profStats.MemoryAllocations, memStats)

				fmt.Printf("\nSimulation complete! Total time: %v\n", time.Since(simulationStart))
				profStats.PrintSummary()
				return
			}

		case <-ctx.Done():
			return
		}
	}
}
