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

// Package metrics provides metric collection and aggregation for the autoscaler.
package metrics

import (
	"sync"
	"time"
)

// TimedFloat64Buckets manages time-windowed buckets of float64 values.
type TimedFloat64Buckets struct {
	mu sync.RWMutex

	// buckets is a ring buffer indexed by timeToIndex() % len(buckets).
	buckets []float64

	// firstWrite holds the time when the first write has been made.
	firstWrite time.Time

	// lastWrite stores the time when the last write was made.
	lastWrite time.Time

	// granularity is the duration represented by each bucket.
	granularity time.Duration

	// window is the total time represented by all buckets.
	window time.Duration

	// windowTotal is the sum of all buckets within the window.
	windowTotal float64
}

// NewTimedFloat64Buckets creates a new TimedFloat64Buckets with the given window and granularity.
func NewTimedFloat64Buckets(window, granularity time.Duration) *TimedFloat64Buckets {
	numBuckets := (window + granularity - 1) / granularity
	return &TimedFloat64Buckets{
		buckets:     make([]float64, numBuckets),
		granularity: granularity,
		window:      window,
	}
}

// Record adds a value at the given time to the buckets.
func (t *TimedFloat64Buckets) Record(now time.Time, value float64) {
	bucketTime := now.Truncate(t.granularity)

	t.mu.Lock()
	defer t.mu.Unlock()

	writeIdx := t.timeToIndex(now)

	// If the last write is the same as the bucket time, we can just add the value to the bucket
	if t.lastWrite.Equal(bucketTime) {
		t.buckets[writeIdx%len(t.buckets)] += value
		t.windowTotal += value
		return
	}

	// Ignore values older than a window
	if bucketTime.Add(t.window).Before(t.lastWrite) {
		return
	}

	// Update firstWrite if this is the first write or if it's before the current firstWrite
	if t.firstWrite.IsZero() || t.firstWrite.After(bucketTime) {
		t.firstWrite = bucketTime
	}

	if bucketTime.After(t.lastWrite) {
		if bucketTime.Sub(t.lastWrite) >= t.window {
			// Reset all buckets if we haven't written for a full window
			t.firstWrite = bucketTime
			for i := range t.buckets {
				t.buckets[i] = 0
			}
			t.windowTotal = 0
		} else {
			// Clear buckets between lastWrite and now
			for i := t.timeToIndex(t.lastWrite) + 1; i <= writeIdx; i++ {
				idx := i % len(t.buckets)
				t.windowTotal -= t.buckets[idx]
				t.buckets[idx] = 0
			}
		}
		t.lastWrite = bucketTime
	}

	t.buckets[writeIdx%len(t.buckets)] += value
	t.windowTotal += value
}

// WindowAverage returns the average value over the window.
func (t *TimedFloat64Buckets) WindowAverage(now time.Time) float64 {
	now = now.Truncate(t.granularity)
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.IsEmptyLocked(now) {
		return 0
	}

	switch d := now.Sub(t.lastWrite); {
	case d <= 0:
		// Current or future time - use current window total
		numBuckets := min(
			int(t.lastWrite.Sub(t.firstWrite)/t.granularity)+1,
			len(t.buckets))
		return t.windowTotal / float64(numBuckets)
	case d < t.window:
		// Recent past - remove outdated buckets
		startIdx := t.timeToIndex(t.lastWrite)
		endIdx := t.timeToIndex(now)
		total := t.windowTotal
		for i := startIdx + 1; i <= endIdx; i++ {
			total -= t.buckets[i%len(t.buckets)]
		}
		numBuckets := min(
			int(t.lastWrite.Sub(t.firstWrite)/t.granularity)+1,
			len(t.buckets)-(endIdx-startIdx))
		return total / float64(numBuckets)
	default:
		// No data within window
		return 0
	}
}

// IsEmpty returns true if no data has been recorded within the window.
func (t *TimedFloat64Buckets) IsEmpty(now time.Time) bool {
	now = now.Truncate(t.granularity)
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.IsEmptyLocked(now)
}

// IsEmptyLocked returns true if no data has been recorded within the window.
// Caller must hold at least a read lock.
func (t *TimedFloat64Buckets) IsEmptyLocked(now time.Time) bool {
	return now.Sub(t.lastWrite) > t.window
}

// ResizeWindow changes the window duration.
func (t *TimedFloat64Buckets) ResizeWindow(newWindow time.Duration) {
	if func() bool {
		t.mu.RLock()
		defer t.mu.RUnlock()
		return newWindow == t.window
	}() {
		return
	}

	numBuckets := int((newWindow + t.granularity - 1) / t.granularity)
	newBuckets := make([]float64, numBuckets)
	newTotal := 0.0

	t.mu.Lock()
	defer t.mu.Unlock()

	// Copy existing data if within window
	if time.Now().Truncate(t.granularity).Sub(t.lastWrite) <= t.window {
		oldNumBuckets := len(t.buckets)
		tIdx := t.timeToIndex(t.lastWrite)

		for range min(numBuckets, oldNumBuckets) {
			oldIdx := tIdx % oldNumBuckets
			newIdx := tIdx % numBuckets
			newBuckets[newIdx] = t.buckets[oldIdx]
			newTotal += t.buckets[oldIdx]
			tIdx--
		}
		t.firstWrite = t.lastWrite.Add(-time.Duration(oldNumBuckets-1) * t.granularity)
	} else {
		t.firstWrite = time.Time{}
	}

	t.window = newWindow
	t.buckets = newBuckets
	t.windowTotal = newTotal
}

// timeToIndex converts a time to a bucket index.
func (t *TimedFloat64Buckets) timeToIndex(tm time.Time) int {
	return int(tm.Unix()) / int(t.granularity.Seconds())
}

// TimeWindow represents a max/delay time window for scaling decisions.
type TimeWindow struct {
	window *TimedFloat64Buckets
}

// NewTimeWindow creates a new TimeWindow.
func NewTimeWindow(duration, granularity time.Duration) *TimeWindow {
	return &TimeWindow{
		window: NewTimedFloat64Buckets(duration, granularity),
	}
}

// Record adds a value at the current time.
func (t *TimeWindow) Record(now time.Time, value int32) {
	t.window.Record(now, float64(value))
}

// Current returns the current maximum value in the window.
func (t *TimeWindow) Current() int32 {
	// Find the maximum by iterating through buckets
	t.window.mu.RLock()
	defer t.window.mu.RUnlock()

	maxValue := int32(0)
	for _, v := range t.window.buckets {
		val := int32(v)
		if val > maxValue {
			maxValue = val
		}
	}
	return maxValue
}

// ResizeWindow changes the window duration.
func (t *TimeWindow) ResizeWindow(newDuration time.Duration) {
	t.window.ResizeWindow(newDuration)
}

// MetricSnapshot represents a point-in-time view of metrics.
type MetricSnapshot struct {
	stableValue   float64
	panicValue    float64
	readyPodCount int32
	timestamp     time.Time
}

// NewMetricSnapshot creates a new metric snapshot.
func NewMetricSnapshot(stableValue, panicValue float64, readyPods int32, timestamp time.Time) *MetricSnapshot {
	return &MetricSnapshot{
		stableValue:   stableValue,
		panicValue:    panicValue,
		readyPodCount: readyPods,
		timestamp:     timestamp,
	}
}

// StableValue returns the metric value averaged over the stable window.
func (s *MetricSnapshot) StableValue() float64 {
	return s.stableValue
}

// PanicValue returns the metric value averaged over the panic window.
func (s *MetricSnapshot) PanicValue() float64 {
	return s.panicValue
}

// ReadyPodCount returns the number of ready pods.
func (s *MetricSnapshot) ReadyPodCount() int32 {
	return s.readyPodCount
}

// Timestamp returns when this snapshot was taken.
func (s *MetricSnapshot) Timestamp() time.Time {
	return s.timestamp
}
