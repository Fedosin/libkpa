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

import (
	"math"
	"time"
)

// WeightedTimeWindow is the implementation of buckets, that
// uses weighted average algorithm.
type WeightedTimeWindow struct {
	*TimeWindow

	// smoothingCoeff contains the speed with which the importance
	// of items in the past decays. The larger the faster weights decay.
	// It is autocomputed from window size and weightPrecision constant
	// and is bounded by minExponent below.
	smoothingCoeff float64
}

// NewWeightedTimeWindow generates a new WeightedTimeWindow with the given
// granularity.
func NewWeightedTimeWindow(window, granularity time.Duration) *WeightedTimeWindow {
	// Number of buckets is `window` divided by `granularity`, rounded up.
	// e.g. 60s / 2s = 30.
	nb := math.Ceil(float64(window) / float64(granularity))
	return &WeightedTimeWindow{
		TimeWindow:     NewTimeWindow(window, granularity),
		smoothingCoeff: computeSmoothingCoeff(nb),
	}
}

// WindowAverage returns the exponential weighted average. This means
// that more recent items have much greater impact on the average than
// the older ones.
// TODO(vagababov): optimize for O(1) computation, if possible.
// E.g. with data  [10, 10, 5, 5] (newest last), then
// the `WindowAverage` would return (10+10+5+5)/4 = 7.5
// This with exponent of 0.6 would return 5*0.6+5*0.6*0.4+10*0.6*0.4^2+10*0.6*0.4^3 = 5.544
// If we reverse the data to [5, 5, 10, 10] the simple average would remain the same,
// but this one would change to 9.072.
func (t *WeightedTimeWindow) WindowAverage(now time.Time) float64 {
	now = now.Truncate(t.granularity)
	t.bucketsMutex.RLock()
	defer t.bucketsMutex.RUnlock()
	if t.isEmptyLocked(now) {
		return 0
	}

	totalB := len(t.buckets)
	numB := len(t.buckets)

	multiplier := t.smoothingCoeff
	// We start with 0es. But we know that we have _some_ data because
	// IsEmpty returned false.
	if now.After(t.lastWrite) {
		numZ := now.Sub(t.lastWrite) / t.granularity
		// Skip to this multiplier directly: m*(1-m)^(nz-1).
		multiplier *= math.Pow(1-t.smoothingCoeff, float64(numZ))
		// Reduce effective number of buckets.
		numB -= int(numZ)
	}
	startIdx := t.timeToIndex(t.lastWrite) + totalB // To ensure always positive % operation.
	ret := 0.
	for i := range numB {
		effectiveIdx := (startIdx - i) % totalB
		v := t.buckets[effectiveIdx] * multiplier
		ret += v
		multiplier *= (1 - t.smoothingCoeff)
		// TODO(vagababov): bail out if sm > weightPrecision?
	}
	return ret
}

// ResizeWindow implements window resizing for the weighted averaging buckets object.
func (t *WeightedTimeWindow) ResizeWindow(w time.Duration) {
	t.TimeWindow.ResizeWindow(w)
	t.smoothingCoeff = computeSmoothingCoeff(math.Ceil(float64(w) / float64(t.granularity)))
}
