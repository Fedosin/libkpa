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
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

const granularity = time.Second

func TestComputeDecayMultiplier(t *testing.T) {
	tests := []struct {
		numBuckets float64
		want       float64
	}{{
		numBuckets: 100,
		want:       minExponent,
	}, {
		numBuckets: 60,
		want:       minExponent,
	}, {
		numBuckets: 40,
		want:       0.20567,
	}, {
		numBuckets: 6,
		want:       0.78456,
	}}

	for _, tc := range tests {
		t.Run(fmt.Sprint("nb=", tc.numBuckets), func(t *testing.T) {
			if got, want := computeSmoothingCoeff(tc.numBuckets), tc.want; math.Abs(got-want) > weightPrecision {
				t.Errorf("Decay multiplier = %v, want: %v", got, want)
			}
		})
	}
}

func TestTimeWindowSimple(t *testing.T) {
	trunc1 := time.Now().Truncate(1 * time.Second)
	trunc5 := time.Now().Truncate(5 * time.Second)

	type args struct {
		time  time.Time
		value float64
	}
	tests := []struct {
		name        string
		granularity time.Duration
		stats       []args
		want        map[time.Time]float64
	}{{
		name:        "granularity = 1s",
		granularity: time.Second,
		stats: []args{
			{trunc1, 1.0}, // activator scale from 0.
			{trunc1.Add(100 * time.Millisecond), 10.0}, // from scraping pod/sent by activator.
			{trunc1.Add(1 * time.Second), 1.0},         // next bucket
			{trunc1.Add(3 * time.Second), 1.0},         // nextnextnext bucket
		},
		want: map[time.Time]float64{
			trunc1:                      11.0,
			trunc1.Add(1 * time.Second): 1.0,
			trunc1.Add(3 * time.Second): 1.0,
		},
	}, {
		name:        "granularity = 5s",
		granularity: 5 * time.Second,
		stats: []args{
			{trunc5, 1.0},
			{trunc5.Add(3 * time.Second), 11.0}, // same bucket
			{trunc5.Add(6 * time.Second), 1.0},  // next bucket
		},
		want: map[time.Time]float64{
			trunc5:                      12.0,
			trunc5.Add(5 * time.Second): 1.0,
		},
	}, {
		name:        "empty",
		granularity: time.Second,
		stats:       []args{},
		want:        map[time.Time]float64{},
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// New implementation test.
			buckets := NewTimeWindow(2*time.Minute, tt.granularity)
			if !buckets.IsEmpty(trunc1) {
				t.Error("Unexpected non empty result")
			}
			for _, stat := range tt.stats {
				buckets.Record(stat.time, stat.value)
			}

			got := make(map[time.Time]float64)
			// Less time in future than our window is (2mins above), but more than any of the tests report.
			buckets.forEachBucket(trunc1.Add(time.Minute), func(t time.Time, b float64) {
				// Since we're storing 0s when there's no data, we need to exclude those
				// for this test.
				if b > 0 {
					got[t] = b
				}
			})

			if !reflect.DeepEqual(tt.want, got) {
				t.Error("Unexpected values (-want +got):", reflect.DeepEqual(tt.want, got))
			}
		})
	}
}

func TestTimeWindowManyReps(t *testing.T) {
	trunc1 := time.Now().Truncate(granularity)
	buckets := NewTimeWindow(time.Minute, granularity)
	for p := range 5 {
		trunc1 = trunc1.Add(granularity)
		for t := range 5 {
			buckets.Record(trunc1, float64(p+t))
		}
	}
	// So the buckets are:
	// t0: [0, 1, 2, 3, 4] = 10
	// t1: [1, 2, 3, 4, 5] = 15
	// t2: [2, 3, 4, 5, 6] = 20
	// t3: [3, 4, 5, 6, 7] = 25
	// t4: [4, 5, 6, 7, 8] = 30
	//                     = 100
	const want = 100.
	sum1, sum2 := 0., 0.
	buckets.forEachBucket(trunc1, func(_ time.Time, b float64) {
		sum1 += b
	})
	buckets.forEachBucket(trunc1, func(_ time.Time, b float64) {
		sum2 += b
	})
	if got, want := sum1, want; got != want {
		t.Errorf("Sum1 = %f, want: %f", got, want)
	}

	if got, want := sum2, want; got != want {
		t.Errorf("Sum2 = %f, want: %f", got, want)
	}
}

func TestTimeWindowManyRepsWithNonMonotonicalOrder(t *testing.T) {
	start := time.Now().Truncate(granularity)
	end := start
	buckets := NewTimeWindow(time.Minute, granularity)

	d := []int{0, 3, 2, 1, 4}
	for p := range 5 {
		end = start.Add(time.Duration(d[p]) * granularity)
		for t := range 5 {
			buckets.Record(end, float64(p+t))
		}
	}

	// So the buckets are:
	// t0: [0, 1, 2, 3, 4] = 10
	// t1: [3, 4, 5, 6, 7] = 25
	// t2: [2, 3, 4, 5, 6] = 20
	// t3: [1, 2, 3, 4, 5] = 15
	// t4: [4, 5, 6, 7, 8] = 30
	//                     = 100
	const want = 100.
	sum1, sum2 := 0., 0.
	buckets.forEachBucket(end, func(_ time.Time, b float64) {
		sum1 += b
	})
	buckets.forEachBucket(end, func(_ time.Time, b float64) {
		sum2 += b
	})
	if got, want := sum1, want; got != want {
		t.Errorf("Sum1 = %f, want: %f", got, want)
	}

	if got, want := sum2, want; got != want {
		t.Errorf("Sum2 = %f, want: %f", got, want)
	}
}

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

func TestTimeWindowWindowAverage(t *testing.T) {
	now := time.Now()
	buckets := NewTimeWindow(5*time.Second, granularity)

	// This verifies that we properly use firstWrite. Without that we'd get 0.2.
	buckets.Record(now, 1)
	if got, want := buckets.WindowAverage(now), 1.; got != want {
		t.Errorf("WindowAverage = %v, want: %v", got, want)
	}
	for i := 1; i < 5; i++ {
		buckets.Record(now.Add(time.Duration(i)*time.Second), float64(i+1))
	}

	if got, want := buckets.WindowAverage(now.Add(4*time.Second)), 15./5; got != want {
		t.Errorf("WindowAverage = %v, want: %v", got, want)
	}
	// Check when `now` lags behind.
	if got, want := buckets.WindowAverage(now.Add(3600*time.Millisecond)), 15./5; got != want {
		t.Errorf("WindowAverage = %v, want: %v", got, want)
	}

	// Check with short hole.
	if got, want := buckets.WindowAverage(now.Add(6*time.Second)), (15.-1-2)/(5-2); got != want {
		t.Errorf("WindowAverage = %v, want: %v", got, want)
	}

	// Check with a long hole.
	if got, want := buckets.WindowAverage(now.Add(10*time.Second)), 0.; got != want {
		t.Errorf("WindowAverage = %v, want: %v", got, want)
	}

	// Check write with holes.
	buckets.Record(now.Add(6*time.Second), 91)
	if got, want := buckets.WindowAverage(now.Add(6*time.Second)), (15.-1-2+91)/5; got != want {
		t.Errorf("WindowAverage = %v, want: %v", got, want)
	}

	// Advance much farther.
	now = now.Add(time.Minute)
	buckets.Record(now, 1984)
	if got, want := buckets.WindowAverage(now), 1984.; got != want {
		t.Errorf("WindowAverage = %v, want: %v", got, want)
	}

	// Check with an earlier time.
	buckets.Record(now.Add(-3*time.Second), 4)
	if got, want := buckets.WindowAverage(now), (4.+1984)/4; got != want {
		t.Errorf("WindowAverage = %v, want: %v", got, want)
	}

	// One more second pass.
	now = now.Add(time.Second)
	buckets.Record(now, 5)
	if got, want := buckets.WindowAverage(now), (4.+1984+5)/5; got != want {
		t.Errorf("WindowAverage = %v, want: %v", got, want)
	}

	// Insert an earlier time again.
	buckets.Record(now.Add(-3*time.Second), 10)
	if got, want := buckets.WindowAverage(now), (4.+10+1984+5)/5; got != want {
		t.Errorf("WindowAverage = %v, want: %v", got, want)
	}

	// Verify that we ignore the value which is too early.
	buckets.Record(now.Add(-6*time.Second), 10)
	if got, want := buckets.WindowAverage(now), (4.+10+1984+5)/5; got != want {
		t.Errorf("WindowAverage = %v, want: %v", got, want)
	}

	// Verify that we ignore the value with bound timestamp.
	buckets.Record(now.Add(-5*time.Second), 10)
	if got, want := buckets.WindowAverage(now), (4.+10+1984+5)/5; got != want {
		t.Errorf("WindowAverage = %v, want: %v", got, want)
	}

	// Verify we clear up the data when not receiving data for exact `window` peroid.
	buckets.Record(now.Add(5*time.Second), 10)
	if got, want := buckets.WindowAverage(now.Add(5*time.Second)), 10.; got != want {
		t.Errorf("WindowAverage = %v, want: %v", got, want)
	}
}

// TestTimeWindowAverageWithLargeGap tests the wraparound bug fix where the time gap
// between lastWrite and now exceeds the bucket array size.
func TestTimeWindowAverageWithLargeGap(t *testing.T) {
	now := time.Now()
	// Create a window with 30 buckets (60s window / 2s granularity)
	buckets := NewTimeWindow(60*time.Second, 2*time.Second)

	// Record some initial data
	for i := range 10 {
		buckets.Record(now.Add(time.Duration(i)*2*time.Second), float64(i+1))
	}

	// windowTotal should be 1+2+3+...+10 = 55

	// Now query with a gap larger than the bucket count (65 seconds = 32.5 buckets)
	// This should handle the wraparound correctly and not subtract buckets multiple times
	futureTime := now.Add(65 * time.Second)
	avg := buckets.WindowAverage(futureTime)

	// The average should be based on remaining valid buckets
	// Since the gap is 45 seconds (65-20), which is 22.5 buckets,
	// we should have ~7-8 valid buckets remaining
	// But since gap < window, case 2 applies
	if avg < 0 {
		t.Errorf("WindowAverage with large gap returned negative value: %v", avg)
	}

	// Test with an even larger gap that's still less than window
	futureTime2 := now.Add(75 * time.Second)
	avg2 := buckets.WindowAverage(futureTime2)

	// Should still handle correctly without going negative
	if avg2 < 0 {
		t.Errorf("WindowAverage with very large gap returned negative value: %v", avg2)
	}
}

// TestTimeWindowAverageNegativeValues tests that the window can handle and average negative values correctly.
func TestTimeWindowAverageNegativeValues(t *testing.T) {
	now := time.Now()
	buckets := NewTimeWindow(5*time.Second, granularity)

	// Record negative values
	buckets.Record(now, -10)
	buckets.Record(now.Add(1*time.Second), -20)
	buckets.Record(now.Add(2*time.Second), -30)

	// Average should be (-10 + -20 + -30) / 3 = -20
	if got, want := buckets.WindowAverage(now.Add(2*time.Second)), -20.0; got != want {
		t.Errorf("WindowAverage with negative values = %v, want: %v", got, want)
	}

	// Mix of positive and negative
	buckets.Record(now.Add(3*time.Second), 40)
	buckets.Record(now.Add(4*time.Second), 50)

	// Average should be (-10 + -20 + -30 + 40 + 50) / 5 = 30 / 5 = 6
	if got, want := buckets.WindowAverage(now.Add(4*time.Second)), 6.0; got != want {
		t.Errorf("WindowAverage with mixed values = %v, want: %v", got, want)
	}
}

// TestTimeWindowAverageBoundaryConditions tests edge cases around bucket boundaries.
func TestTimeWindowAverageBoundaryConditions(t *testing.T) {
	now := time.Now()
	// Small window to make wraparound easier to test
	buckets := NewTimeWindow(10*time.Second, 2*time.Second) // 5 buckets

	// Fill all buckets
	for i := range 5 {
		buckets.Record(now.Add(time.Duration(i)*2*time.Second), float64(i+1))
	}

	// Test querying at exactly the window boundary
	avg := buckets.WindowAverage(now.Add(8 * time.Second))
	if got, want := avg, 15.0/5; got != want {
		t.Errorf("WindowAverage at boundary = %v, want: %v", got, want)
	}

	// Test with gap that equals window size
	futureTime := now.Add(18 * time.Second) // 10 seconds after last write at 8s
	avg2 := buckets.WindowAverage(futureTime)

	// When gap equals window size, should return 0 (default case)
	if got, want := avg2, 0.0; got != want {
		t.Errorf("WindowAverage with gap equal to window size = %v, want: %v", got, want)
	}

	// Test with gap slightly less than window size
	futureTime3 := now.Add(17 * time.Second) // 9 seconds after last write
	avg3 := buckets.WindowAverage(futureTime3)

	// Should not be negative and should handle wraparound correctly
	if avg3 < 0 {
		t.Errorf("WindowAverage with gap near window size returned negative: %v", avg3)
	}
}

func TestDescendingRecord(t *testing.T) {
	now := time.Now()
	buckets := NewTimeWindow(5*time.Second, 1*time.Second)

	for i := 8 * time.Second; i >= 0*time.Second; i -= time.Second {
		buckets.Record(now.Add(i), 5)
	}

	if got, want := buckets.WindowAverage(now.Add(5*time.Second)), 5.; got != want {
		// we wrote a 5 every second, and we never wrote in the same second twice,
		// so the average _should_ be 5.
		t.Errorf("WindowAverage = %v, want: %v", got, want)
	}
}

func TestTimeWindowHoles(t *testing.T) {
	now := time.Now()
	buckets := NewTimeWindow(5*time.Second, granularity)

	for i := range 5 {
		buckets.Record(now.Add(time.Duration(i)*time.Second), float64(i+1))
	}

	sum := 0.

	buckets.forEachBucket(now.Add(4*time.Second),
		func(_ time.Time, b float64) {
			sum += b
		})

	if got, want := sum, 15.; got != want {
		t.Errorf("Sum = %v, want: %v", got, want)
	}
	if got, want := buckets.WindowAverage(now.Add(4*time.Second)), 15./5; got != want {
		t.Errorf("WindowAverage = %v, want: %v", got, want)
	}
	// Now write at 9th second. Which means that seconds
	// 5[0], 6[1], 7[2] become 0.
	buckets.Record(now.Add(8*time.Second), 2.)
	// So now we have [3] = 2, [4] = 5 and sum should be 7.
	sum = 0.

	buckets.forEachBucket(now.Add(8*time.Second),
		func(_ time.Time, b float64) {
			sum += b
		})
	if got, want := sum, 7.; got != want {
		t.Errorf("Sum = %v, want: %v", got, want)
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

func TestTimeWindowResizeWindow(t *testing.T) {
	startTime := time.Now()
	buckets := NewTimeWindow(5*time.Second, granularity)

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
	if got, want := buckets.WindowAverage(now), wantInitial/5; got != want {
		t.Fatalf("Initial data set Sum = %v, want: %v", got, want)
	}

	// Increase window.
	buckets.ResizeWindow(10 * time.Second)
	if got, want := len(buckets.buckets), 10; got != want {
		t.Fatalf("Resized bucket count = %d, want: %d", got, want)
	}
	if got, want := buckets.window, 10*time.Second; got != want {
		t.Fatalf("Resized bucket windows = %v, want: %v", got, want)
	}

	// Verify values were properly copied.
	sum = 0.
	buckets.forEachBucket(now, func(t time.Time, b float64) {
		sum += b
	})
	if got, want := sum, wantInitial; got != want {
		t.Fatalf("After first resize data set Sum = %v, want: %v", got, want)
	}
	// Note the average doesn't change, since we know we had at most 5 buckets.
	if got, want := buckets.WindowAverage(now), wantInitial/5; got != want {
		t.Errorf("Initial data set Sum = %v, want: %v", got, want)
	}

	// Add one more. Make sure all the data is preserved, since window is longer.
	now = now.Add(time.Second)
	buckets.Record(now, 7)
	const wantWithUpdate = wantInitial + 7
	sum = 0.
	buckets.forEachBucket(now, func(t time.Time, b float64) {
		sum += b
	})
	if got, want := sum, wantWithUpdate; got != want {
		t.Fatalf("Updated data set Sum = %v, want: %v", got, want)
	}
	// Same here. We just have at most 6 recorded buckets.
	if got, want := buckets.WindowAverage(now), roundToNDigits(6, wantWithUpdate/6); got != want {
		t.Fatalf("Initial data set Sum = %v, want: %v", got, want)
	}

	// Now let's reduce window size.
	buckets.ResizeWindow(4 * time.Second)
	if got, want := len(buckets.buckets), 4; got != want {
		t.Fatalf("Resized bucket count = %d, want: %d", got, want)
	}
	// Just last 4 buckets should have remained (so 2 oldest are expunged).
	const wantWithShrink = wantWithUpdate - 2 - 3
	sum = 0.
	buckets.forEachBucket(now, func(t time.Time, b float64) {
		sum += b
	})
	if got, want := sum, wantWithShrink; got != want {
		t.Fatalf("Updated data set Sum = %v, want: %v", got, want)
	}
	if got, want := buckets.WindowAverage(now), wantWithShrink/4; got != want {
		t.Fatalf("Initial data set Sum = %v, want: %v", got, want)
	}

	// Verify idempotence.
	ob := &buckets.buckets
	buckets.ResizeWindow(4 * time.Second)
	if ob != &buckets.buckets {
		t.Error("The buckets have changed, though window didn't")
	}
}

func TestTimeWindowWindowUpdate3sGranularity(t *testing.T) {
	const granularity = 3 * time.Second
	trunc1 := time.Now().Truncate(granularity)

	// So two buckets here (ceil(5/3)=ceil(1.6(6))=2).
	buckets := NewTimeWindow(5*time.Second, granularity)
	if got, want := len(buckets.buckets), 2; got != want {
		t.Fatalf("Initial bucket count = %d, want: %d", got, want)
	}

	// Fill the whole bucketing list.
	buckets.Record(trunc1, 10)
	buckets.Record(trunc1.Add(1*time.Second), 2)
	buckets.Record(trunc1.Add(2*time.Second), 3)
	buckets.Record(trunc1.Add(3*time.Second), 4)
	buckets.Record(trunc1.Add(4*time.Second), 5)
	buckets.Record(trunc1.Add(5*time.Second), 6)
	buckets.Record(trunc1.Add(6*time.Second), 7) // This overrides the initial 15 (10+2+3)
	sum := 0.
	buckets.forEachBucket(trunc1.Add(6*time.Second), func(t time.Time, b float64) {
		sum += b
	})
	expectedSum := (4. + 5 + 6) + 7
	if got, want := sum, expectedSum; got != want {
		t.Fatalf("Initial data set Sum = %v, want: %v", got, want)
	}

	// Increase window.
	buckets.ResizeWindow(10 * time.Second)
	if got, want := len(buckets.buckets), 4; got != want {
		t.Fatalf("Resized bucket count = %d, want: %d", got, want)
	}
	if got, want := buckets.window, 10*time.Second; got != want {
		t.Fatalf("Resized bucket windows = %v, want: %v", got, want)
	}

	// Verify values were properly copied.
	sum = 0
	buckets.forEachBucket(trunc1.Add(6*time.Second), func(t time.Time, b float64) {
		sum += b
	})
	if got, want := sum, expectedSum; got != want {
		t.Fatalf("After first resize data set Sum = %v, want: %v", got, want)
	}

	// Add one more. Make sure all the data is preserved, since window is longer.
	buckets.Record(trunc1.Add(9*time.Second+300*time.Millisecond), 42)
	sum = 0
	buckets.forEachBucket(trunc1.Add(9*time.Second), func(t time.Time, b float64) {
		sum += b
	})
	expectedSum += 42
	if got, want := sum, expectedSum; got != want {
		t.Fatalf("Updated data set Sum = %v, want: %v", got, want)
	}

	// Now let's reduce window size.
	buckets.ResizeWindow(4 * time.Second)

	sum = 0
	if got, want := len(buckets.buckets), 2; got != want {
		t.Fatalf("Resized bucket count = %d, want: %d", got, want)
	}

	// Just last 4 buckets should have remained.
	sum = 0.
	expectedSum = 42 + 7 // we drop oldest bucket and the one not yet utilized)
	buckets.forEachBucket(trunc1.Add(9*time.Second), func(t time.Time, b float64) {
		sum += b
	})
	if got, want := sum, expectedSum; got != want {
		t.Fatalf("Updated data set Sum = %v, want: %v", got, want)
	}

	// Verify idempotence.
	ob := &buckets.buckets
	buckets.ResizeWindow(4 * time.Second)
	if ob != &buckets.buckets {
		t.Error("The buckets have changed, though window didn't")
	}
}

func TestTimeWindowWindowUpdateNoOp(t *testing.T) {
	startTime := time.Now().Add(-time.Minute)
	buckets := NewTimeWindow(5*time.Second, granularity)
	buckets.Record(startTime, 19.82)
	if got, want := buckets.firstWrite, buckets.lastWrite; !got.Equal(want) {
		t.Errorf("FirstWrite = %v, want: %v", got, want)
	}
	buckets.ResizeWindow(10 * time.Second)

	if got, want := buckets.firstWrite, (time.Time{}); !got.Equal(want) {
		t.Errorf("FirstWrite after update = %v, want: %v", got, want)
	}
}

func BenchmarkWindowAverage(b *testing.B) {
	// Window lengths in secs.
	for _, wl := range []int{30, 60, 120, 240, 600} {
		b.Run(fmt.Sprintf("%v-win-len", wl), func(b *testing.B) {
			tn := time.Now().Truncate(time.Second) // To simplify everything.
			buckets := NewTimeWindow(time.Duration(wl)*time.Second,
				time.Second /*granularity*/)
			// Populate with some random data.
			for i := range wl {
				buckets.Record(tn.Add(time.Duration(i)*time.Second), rand.Float64()*100)
			}
			for b.Loop() {
				buckets.WindowAverage(tn.Add(time.Duration(wl) * time.Second))
			}
		})
	}
}

func TestRoundToNDigits(t *testing.T) {
	if got, want := roundToNDigits(6, 3.6e-17), 0.; got != want {
		t.Errorf("Rounding = %v, want: %v", got, want)
	}
	if got, want := roundToNDigits(3, 0.0004), 0.; got != want {
		t.Errorf("Rounding = %v, want: %v", got, want)
	}
	if got, want := roundToNDigits(3, 1.2345), 1.235; got != want {
		t.Errorf("Rounding = %v, want: %v", got, want)
	}
	if got, want := roundToNDigits(4, 1.2345), 1.2345; got != want {
		t.Errorf("Rounding = %v, want: %v", got, want)
	}
	if got, want := roundToNDigits(6, 12345), 12345.; got != want {
		t.Errorf("Rounding = %v, want: %v", got, want)
	}
}

func (t *TimeWindow) forEachBucket(now time.Time, acc func(time time.Time, bucket float64)) {
	now = now.Truncate(t.granularity)
	t.bucketsMutex.RLock()
	defer t.bucketsMutex.RUnlock()

	// So number of buckets we can process is len(buckets)-(now-lastWrite)/granularity.
	// Since empty check above failed, we know this is at least 1 bucket.
	numBuckets := len(t.buckets) - int(now.Sub(t.lastWrite)/t.granularity)
	bucketTime := t.lastWrite // Always aligned with granularity.
	si := t.timeToIndex(bucketTime)
	for range numBuckets {
		tIdx := si % len(t.buckets)
		acc(bucketTime, t.buckets[tIdx])
		si--
		bucketTime = bucketTime.Add(-t.granularity)
	}
}
