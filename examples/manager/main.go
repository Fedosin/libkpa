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

// An example of how to use high level manager and scaler.
package main

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	libkpaconfig "github.com/Fedosin/libkpa/config"
	"github.com/Fedosin/libkpa/manager"
)

func main() {
	// Configure autoscaler settings
	config := libkpaconfig.NewDefaultAutoscalerConfig()
	config.StableWindow = 6 * time.Second
	config.PanicWindowPercentage = 40.0
	config.TargetValue = 280.0
	config.PanicThreshold = 2.0
	config.MaxScaleUpRate = 1000.0
	config.MaxScaleDownRate = 2.0

	// Create scalers for different metrics
	cpuScaler, err := manager.NewScaler("cpu", *config, "linear")
	if err != nil {
		log.Fatal(err)
	}

	// Memory scaler with weighted algorithm for faster response
	memConfig := config
	memConfig.TargetValue = 270.0 // Target 270 Mb memory per pod
	memoryScaler, err := manager.NewScaler("memory", *memConfig, "weighted")
	if err != nil {
		log.Fatal(err)
	}

	// Request rate scaler
	reqConfig := config
	reqConfig.TargetValue = 1000.0 // Target 1000 requests/sec per pod
	requestScaler, err := manager.NewScaler("requests", *reqConfig, "weighted")
	if err != nil {
		log.Fatal(err)
	}

	// Create manager with initial scalers
	mgr := manager.NewManager(2, 20, cpuScaler, memoryScaler, requestScaler)

	// Simulate metric collection and scaling loop
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	fmt.Println("Starting autoscaler simulation...")
	fmt.Println("Press Ctrl+C to stop")

	currentPods := int32(5) // In real usage, get from Kubernetes

	// Simulate some workload patterns
	iteration := 0
	for range ticker.C {
		iteration++
		now := time.Now()

		// Simulate varying workload
		var cpuUsage, memUsage, reqRate float64

		switch {
		case iteration < 6:
			// Normal load
			cpuUsage = 40.0 + rand.Float64()*20  // 40-60 mCPU
			memUsage = 50.0 + rand.Float64()*10  // 50-60 Mb
			reqRate = 500.0 + rand.Float64()*200 // 500-700 req/s

		case iteration < 12:
			// High load spike
			cpuUsage = 180.0 + rand.Float64()*15   // 180-195 mCPU
			memUsage = 170.0 + rand.Float64()*20   // 170-190 Mb
			reqRate = 25000.0 + rand.Float64()*500 // 25000-25500 req/s

		case iteration < 18:
			// Gradually decreasing
			cpuUsage = 60.0 + rand.Float64()*20  // 60-80 mCPU
			memUsage = 55.0 + rand.Float64()*15  // 55-70 Mb
			reqRate = 800.0 + rand.Float64()*200 // 800-1000 req/s

		default:
			// Back to normal
			cpuUsage = 40.0 + rand.Float64()*20  // 40-60 mCPU
			memUsage = 50.0 + rand.Float64()*10  // 50-60 Mb
			reqRate = 500.0 + rand.Float64()*200 // 500-700 req/s
		}

		// Record metrics (multiply by current pod count to get total)
		err = mgr.Record("cpu", cpuUsage*float64(currentPods), now)
		if err != nil {
			log.Printf("Record error: %v", err)
		}
		err = mgr.Record("memory", memUsage*float64(currentPods), now)
		if err != nil {
			log.Printf("Record error: %v", err)
		}

		// Record total servicerequests per second
		err = mgr.Record("requests", reqRate, now)
		if err != nil {
			log.Printf("Record error: %v", err)
		}

		// Calculate desired scale
		desiredPods := mgr.Scale(currentPods, now)

		// Print status
		fmt.Printf("\n[%s] Iteration %d:\n", now.Format("15:04:05"), iteration)
		fmt.Printf("  Metrics: Total CPU=%.0f mCPU, Total Memory=%.0f Mb, Total Requests=%.0f/s\n",
			cpuUsage*float64(currentPods), memUsage*float64(currentPods), reqRate)
		fmt.Printf("  Current pods: %d → Desired pods: %d\n", currentPods, desiredPods)

		// Update current pods to the desired pods
		currentPods = desiredPods

		if iteration == 5 {
			fmt.Println("\n  >>> Adding more load")
		}

		if iteration == 6 {
			fmt.Println("\n  >>> Adjusting scale bounds for off-peak")
			mgr.SetMinScale(1)
			mgr.SetMaxScale(30)
		}

		if iteration > 20 {
			fmt.Println("\nSimulation complete!")
			break
		}
	}
}
