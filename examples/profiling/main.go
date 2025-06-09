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

// Package main provides a detailed profiling example for the libkpa autoscaler.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"sync/atomic"
	"time"

	"github.com/Fedosin/libkpa/algorithm"
	"github.com/Fedosin/libkpa/api"
	"github.com/Fedosin/libkpa/config"
	"github.com/Fedosin/libkpa/metrics"
)

// DetailedProfilingStats tracks detailed performance metrics
type DetailedProfilingStats struct {
	// Scaling decision breakdown
	ScalingDecisions int64
	TotalScalingTime int64 // nanoseconds
	MinScalingTime   int64
	MaxScalingTime   int64

	// Component timing
	WindowRecordTime     int64
	WindowAverageTime    int64
	SnapshotCreationTime int64
	PanicModeCheckTime   int64
	RateLimitingTime     int64

	// Memory tracking
	InitialMemStats  runtime.MemStats
	CurrentMemStats  runtime.MemStats
	PeakHeapAlloc    uint64
	TotalAllocations uint64

	// Delay tracking
	ScaleDownDelays int64
	TotalDelayTime  int64
}

func (d *DetailedProfilingStats) RecordScaling(duration time.Duration) {
	atomic.AddInt64(&d.ScalingDecisions, 1)
	ns := duration.Nanoseconds()
	atomic.AddInt64(&d.TotalScalingTime, ns)

	// Update min/max (not thread-safe for exact values but good enough for profiling)
	if min := atomic.LoadInt64(&d.MinScalingTime); min == 0 || ns < min {
		atomic.StoreInt64(&d.MinScalingTime, ns)
	}
	if max := atomic.LoadInt64(&d.MaxScalingTime); ns > max {
		atomic.StoreInt64(&d.MaxScalingTime, ns)
	}
}

func (d *DetailedProfilingStats) PrintDetailedSummary() {
	decisions := atomic.LoadInt64(&d.ScalingDecisions)
	if decisions == 0 {
		fmt.Println("No scaling decisions made")
		return
	}

	totalTime := atomic.LoadInt64(&d.TotalScalingTime)
	avgTime := totalTime / decisions

	fmt.Println("\n=== Detailed Profiling Summary ===")
	fmt.Printf("\nScaling Performance:\n")
	fmt.Printf("  Total Decisions: %d\n", decisions)
	fmt.Printf("  Average Time: %v\n", time.Duration(avgTime))
	fmt.Printf("  Min Time: %v\n", time.Duration(atomic.LoadInt64(&d.MinScalingTime)))
	fmt.Printf("  Max Time: %v\n", time.Duration(atomic.LoadInt64(&d.MaxScalingTime)))

	fmt.Printf("\nComponent Breakdown:\n")
	fmt.Printf("  Window Record: %v (%.1f%%)\n",
		time.Duration(atomic.LoadInt64(&d.WindowRecordTime)),
		float64(d.WindowRecordTime)*100/float64(totalTime))
	fmt.Printf("  Window Average: %v (%.1f%%)\n",
		time.Duration(atomic.LoadInt64(&d.WindowAverageTime)),
		float64(d.WindowAverageTime)*100/float64(totalTime))
	fmt.Printf("  Snapshot Creation: %v (%.1f%%)\n",
		time.Duration(atomic.LoadInt64(&d.SnapshotCreationTime)),
		float64(d.SnapshotCreationTime)*100/float64(totalTime))

	fmt.Printf("\nMemory Usage:\n")
	fmt.Printf("  Initial Heap: %.2f MB\n", float64(d.InitialMemStats.HeapAlloc)/1024/1024)
	fmt.Printf("  Current Heap: %.2f MB\n", float64(d.CurrentMemStats.HeapAlloc)/1024/1024)
	fmt.Printf("  Peak Heap: %.2f MB\n", float64(atomic.LoadUint64(&d.PeakHeapAlloc))/1024/1024)
	fmt.Printf("  Total Allocations: %.2f MB\n", float64(atomic.LoadUint64(&d.TotalAllocations))/1024/1024)
	fmt.Printf("  GC Runs: %d\n", d.CurrentMemStats.NumGC-d.InitialMemStats.NumGC)
	fmt.Printf("  GC Pause: %v\n", time.Duration(d.CurrentMemStats.PauseTotalNs-d.InitialMemStats.PauseTotalNs))

	if delays := atomic.LoadInt64(&d.ScaleDownDelays); delays > 0 {
		fmt.Printf("\nScale-Down Delays:\n")
		fmt.Printf("  Total Delays: %d\n", delays)
		fmt.Printf("  Total Delay Time: %v\n", time.Duration(atomic.LoadInt64(&d.TotalDelayTime)))
		fmt.Printf("  Average Delay: %v\n", time.Duration(d.TotalDelayTime/delays))
	}
}

// ProfiledAutoscaler wraps the autoscaler to add profiling
type ProfiledAutoscaler struct {
	autoscaler *algorithm.SlidingWindowAutoscaler
	stats      *DetailedProfilingStats
}

// Scale is the main function that scales the number of pods
func (p *ProfiledAutoscaler) Scale(ctx context.Context, snapshot api.MetricSnapshot, now time.Time) api.ScaleRecommendation {
	start := time.Now()
	result := p.autoscaler.Scale(ctx, snapshot, now)
	p.stats.RecordScaling(time.Since(start))
	return result
}

func main() {
	var (
		cpuProfile   = flag.String("cpuprofile", "", "write cpu profile to file")
		memProfile   = flag.String("memprofile", "", "write memory profile to file")
		traceFile    = flag.String("trace", "", "write execution trace to file")
		pprofAddr    = flag.String("pprof", "", "pprof server address (e.g., :6060)")
		duration     = flag.Duration("duration", 2*time.Minute, "simulation duration")
		tickInterval = flag.Duration("interval", 500*time.Millisecond, "tick interval")
	)
	flag.Parse()

	// Start pprof server if requested
	if *pprofAddr != "" {
		go func() {
			log.Printf("Starting pprof server on %s", *pprofAddr)
			log.Println(http.ListenAndServe(*pprofAddr, nil))
		}()
	}

	// Start CPU profiling if requested
	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	// Start execution tracing if requested
	var traceStop func()
	if *traceFile != "" {
		f, err := os.Create(*traceFile)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		trace.Start(f)
		traceStop = func() { trace.Stop() }
	}

	// Initialize profiling stats
	stats := &DetailedProfilingStats{}
	runtime.ReadMemStats(&stats.InitialMemStats)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Configure for profiling
	cfg.TargetValue = 100.0
	cfg.StableWindow = 60 * time.Second
	cfg.PanicWindowPercentage = 10.0
	cfg.ScaleDownDelay = 30 * time.Second
	cfg.MinScale = 1
	cfg.MaxScale = 20

	fmt.Printf("=== Autoscaler Profiling Example ===\n")
	fmt.Printf("Duration: %v\n", *duration)
	fmt.Printf("Tick Interval: %v\n", *tickInterval)
	fmt.Printf("CPU Profile: %v\n", *cpuProfile != "")
	fmt.Printf("Memory Profile: %v\n", *memProfile != "")
	fmt.Printf("Trace: %v\n", *traceFile != "")
	fmt.Printf("pprof Server: %s\n", *pprofAddr)
	fmt.Println()

	// Create profiled autoscaler
	baseAutoscaler := algorithm.NewSlidingWindowAutoscaler(cfg.AutoscalerSpec)
	autoscaler := &ProfiledAutoscaler{
		autoscaler: baseAutoscaler,
		stats:      stats,
	}

	// Create metric windows
	stableWindow := metrics.NewTimedFloat64Buckets(cfg.StableWindow, time.Second)
	panicWindow := metrics.NewTimedFloat64Buckets(
		time.Duration(float64(cfg.StableWindow)*cfg.PanicWindowPercentage/100),
		time.Second,
	)

	// Simulation
	ctx := context.Background()
	ticker := time.NewTicker(*tickInterval)
	defer ticker.Stop()

	timeout := time.NewTimer(*duration)
	defer timeout.Stop()

	// Memory monitoring
	memTicker := time.NewTicker(time.Second)
	defer memTicker.Stop()
	go func() {
		for range memTicker.C {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			if current := atomic.LoadUint64(&stats.PeakHeapAlloc); m.HeapAlloc > current {
				atomic.StoreUint64(&stats.PeakHeapAlloc, m.HeapAlloc)
			}
			atomic.AddUint64(&stats.TotalAllocations, m.TotalAlloc)
		}
	}()

	currentPods := int32(3)
	iteration := 0

	fmt.Println("Starting profiling simulation...")

	for {
		select {
		case <-timeout.C:
			fmt.Println("\nSimulation complete!")

			// Final memory stats
			runtime.ReadMemStats(&stats.CurrentMemStats)

			// Write memory profile if requested
			if *memProfile != "" {
				f, err := os.Create(*memProfile)
				if err != nil {
					log.Fatal(err)
				}
				defer f.Close()
				runtime.GC()
				if err := pprof.WriteHeapProfile(f); err != nil {
					log.Fatal(err)
				}
			}

			// Stop trace if running
			if traceStop != nil {
				traceStop()
			}

			stats.PrintDetailedSummary()
			return

		case <-ticker.C:
			now := time.Now()
			iteration++

			// Simulate varying load
			load := 80.0 + 200.0*math.Sin(float64(iteration)*0.1) + rand.Float64()*50

			// Time window recording
			recordStart := time.Now()
			stableWindow.Record(now, load)
			panicWindow.Record(now, load)
			atomic.AddInt64(&stats.WindowRecordTime, time.Since(recordStart).Nanoseconds())

			// Time window averaging
			avgStart := time.Now()
			stableAvg := stableWindow.WindowAverage(now)
			panicAvg := panicWindow.WindowAverage(now)
			atomic.AddInt64(&stats.WindowAverageTime, time.Since(avgStart).Nanoseconds())

			// Time snapshot creation
			snapshotStart := time.Now()
			snapshot := metrics.NewMetricSnapshot(
				stableAvg,
				panicAvg,
				currentPods,
				now,
			)
			atomic.AddInt64(&stats.SnapshotCreationTime, time.Since(snapshotStart).Nanoseconds())

			// Get scaling recommendation
			recommendation := autoscaler.Scale(ctx, snapshot, now)

			if recommendation.ScaleValid && recommendation.DesiredPodCount != currentPods {
				if recommendation.DesiredPodCount < currentPods {
					// Track scale-down delays
					atomic.AddInt64(&stats.ScaleDownDelays, 1)
					if cfg.ScaleDownDelay > 0 {
						atomic.AddInt64(&stats.TotalDelayTime, int64(cfg.ScaleDownDelay))
					}
				}
				currentPods = recommendation.DesiredPodCount
			}

			// Print progress every 10 seconds
			if iteration%(int(10*time.Second / *tickInterval)) == 0 {
				fmt.Printf("Progress: %v elapsed, %d decisions, current pods: %d\n",
					time.Duration(iteration)*(*tickInterval),
					atomic.LoadInt64(&stats.ScalingDecisions),
					currentPods)
			}
		}
	}
}
