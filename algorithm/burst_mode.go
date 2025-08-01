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

package algorithm

import (
	"time"

	"github.com/Fedosin/libkpa/api"
)

// BurstModeCalculator handles burst mode calculations for the autoscaler.
type BurstModeCalculator struct {
	config *api.AutoscalerConfig
}

// NewBurstModeCalculator creates a new burst mode calculator.
func NewBurstModeCalculator(config *api.AutoscalerConfig) *BurstModeCalculator {
	return &BurstModeCalculator{
		config: config,
	}
}

// CalculateBurstWindow calculates the burst window duration based on the stable window
// and burst window percentage.
func (p *BurstModeCalculator) CalculateBurstWindow() time.Duration {
	return time.Duration(float64(p.config.StableWindow) * p.config.BurstWindowPercentage / 100.0)
}

// ShouldEnterBurstMode determines if the autoscaler should enter burst mode
// based on the current load and capacity.
func (p *BurstModeCalculator) ShouldEnterBurstMode(desiredPodCount, currentPodCount float64) bool {
	if currentPodCount == 0 {
		return false
	}
	// Enter burst mode if desired pods divided by current pods exceeds the burst threshold
	return desiredPodCount/currentPodCount >= p.config.BurstThreshold
}

// ShouldExitBurstMode determines if the autoscaler should exit burst mode.
func (p *BurstModeCalculator) ShouldExitBurstMode(burstStartTime time.Time, now time.Time, isOverThreshold bool) bool {
	// Exit burst mode if:
	// 1. We're no longer over the burst threshold, AND
	// 2. A full stable window has passed since we were last over the threshold
	if !isOverThreshold && burstStartTime.Add(p.config.StableWindow).Before(now) {
		return true
	}
	return false
}

// CalculateDesiredPods calculates the desired pod count considering burst mode.
func (p *BurstModeCalculator) CalculateDesiredPods(stableDesired, burstDesired int32, inBurstMode bool, maxBurstPods int32) int32 {
	if !inBurstMode {
		return stableDesired
	}

	// In burst mode, use the higher of stable or burst desired count
	desired := stableDesired
	if burstDesired > desired {
		desired = burstDesired
	}

	// Never scale down in burst mode
	if desired < maxBurstPods {
		desired = maxBurstPods
	}

	return desired
}
