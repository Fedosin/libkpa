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
	"math/rand"
	"testing"
	"time"
)

func TestTimeWindowWeightedAverage(t *testing.T) {
	now := time.Now()
	buckets := NewWeightedTimeWindow(5*time.Second, granularity)

	buckets.Record(now, 2)
	expectedAvg := 2 * buckets.smoothingCoeff // 2*dm = dm.
	if got, want := buckets.WindowAverage(now), expectedAvg; got != want {
		t.Errorf("WeightedAverage = %v, want: %v", got, want)
	}

	// Let's read one second in future, but no writes.
	expectedAvg *= (1 - buckets.smoothingCoeff)
	if got, want := buckets.WindowAverage(now.Add(time.Second)), expectedAvg; got != want {
		t.Errorf("WeightedAverage = %v, want: %v", got, want)
	}
	// Record some more data.
	buckets.Record(now.Add(time.Second), 2)
	expectedAvg += 2 * buckets.smoothingCoeff
	if got, want := buckets.WindowAverage(now.Add(time.Second)), expectedAvg; got != want {
		t.Errorf("WeightedAverage = %v, want: %v", got, want)
	}

	// Fill the whole window, with [2, 3, 4, 5, 6]
	for i := range 5 {
		buckets.Record(now.Add(time.Duration(2+i)*time.Second), float64(i+2))
	}
	// Manually compute wanted average.
	m := buckets.smoothingCoeff
	expectedAvg = 6*m +
		5*m*(1-m) +
		4*m*(1-m)*(1-m) +
		3*m*(1-m)*(1-m)*(1-m) +
		2*m*(1-m)*(1-m)*(1-m)*(1-m)
	if got, want := buckets.WindowAverage(now.Add(6*time.Second)), expectedAvg; got != want {
		t.Errorf("WeightedAverage = %v, want: %v", got, want)
	}

	// Read from an empty window.
	if got, want := buckets.WindowAverage(now.Add(16*time.Second)), 0.; got != want {
		t.Errorf("WeightedAverage = %v, want: %v", got, want)
	}
}

func TestWeightedTimeWindowResizeWindow(t *testing.T) {
	startTime := time.Now()
	buckets := NewWeightedTimeWindow(5*time.Second, granularity)

	if got, want := buckets.smoothingCoeff, computeSmoothingCoeff(5); math.Abs(got-want) > weightPrecision {
		t.Errorf("DecayMultipler = %v, want: %v", got, want)
	}

	// Fill the whole bucketing list with rollover.
	buckets.Record(startTime, 1)
	buckets.Record(startTime.Add(1*time.Second), 2)
	buckets.Record(startTime.Add(2*time.Second), 3)
	buckets.Record(startTime.Add(3*time.Second), 4)
	buckets.Record(startTime.Add(4*time.Second), 5)
	buckets.Record(startTime.Add(5*time.Second), 6)
	now := startTime.Add(5 * time.Second)

	sum := 0.
	buckets.forEachBucket(now, func(t time.Time, b float64) {
		sum += b
	})
	const wantInitial = 2. + 3 + 4 + 5 + 6
	if got, want := sum, wantInitial; got != want {
		t.Fatalf("Initial data set Sum = %v, want: %v", got, want)
	}
	if got, want := roundToNDigits(3, buckets.WindowAverage(now)), 5.812; /*computed a mano*/ got != want {
		t.Fatalf("Initial data set Sum = %v, want: %v", got, want)
	}

	// Increase window. Most of the heavy lifting is delegated to regular buckets
	// so just do a cursory check.
	buckets.ResizeWindow(10 * time.Second)
	if got, want := len(buckets.buckets), 10; got != want {
		t.Fatalf("Resized bucket count = %d, want: %d", got, want)
	}
	if got, want := buckets.window, 10*time.Second; got != want {
		t.Fatalf("Resized bucket windows = %v, want: %v", got, want)
	}

	// And this is the main logic that was added in this type.
	if got, want := buckets.smoothingCoeff, computeSmoothingCoeff(10); math.Abs(got-want) > weightPrecision {
		t.Errorf("DecayMultipler = %v, want: %v", got, want)
	}
}

func TestWeightedTimeWindowAverageWithZeros(t *testing.T) {
	now := time.Now()
	buckets := NewWeightedTimeWindow(10*time.Second, granularity)

	// Fill the window with zeros
	for i := range 10 {
		buckets.Record(now.Add(time.Duration(i)*time.Second), 0)
	}

	// The average of all zeros should be 0
	if got, want := buckets.WindowAverage(now.Add(9*time.Second)), 0.0; got != want {
		t.Errorf("WindowAverage of zeros = %v, want: %v", got, want)
	}

	// Test with partial window (first 5 seconds)
	if got, want := buckets.WindowAverage(now.Add(4*time.Second)), 0.0; got != want {
		t.Errorf("WindowAverage of zeros (partial window) = %v, want: %v", got, want)
	}

	// Test after some time has passed without new data
	if got, want := buckets.WindowAverage(now.Add(12*time.Second)), 0.0; got != want {
		t.Errorf("WindowAverage of zeros (with gap) = %v, want: %v", got, want)
	}
}

func TestWeightedTimeWindowAverageWithPositiveValuesThenZeros(t *testing.T) {
	now := time.Now()
	buckets := NewWeightedTimeWindow(10*time.Second, granularity)

	// Fill the window with random positive values
	for i := range 10 {
		buckets.Record(now.Add(time.Duration(i)*time.Second), rand.Float64()*100)
	}

	now = now.Add(10 * time.Second)

	// Fill the window with zeros
	for i := range 10 {
		buckets.Record(now.Add(time.Duration(i)*time.Second), 0)
	}

	// The average of all zeros should be 0
	if got, want := buckets.WindowAverage(now.Add(9*time.Second)), 0.0; got != want {
		t.Errorf("WindowAverage of zeros = %v, want: %v", got, want)
	}

	// Test with partial window (first 5 seconds)
	if got, want := buckets.WindowAverage(now.Add(4*time.Second)), 0.0; got != want {
		t.Errorf("WindowAverage of zeros (partial window) = %v, want: %v", got, want)
	}

	// Test after some time has passed without new data
	if got, want := buckets.WindowAverage(now.Add(12*time.Second)), 0.0; got != want {
		t.Errorf("WindowAverage of zeros (with gap) = %v, want: %v", got, want)
	}
}
