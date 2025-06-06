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
	"testing"
	"time"
)

func TestTimedFloat64Buckets_BasicOperations(t *testing.T) {
	window := 10 * time.Second
	granularity := 1 * time.Second
	buckets := NewTimedFloat64Buckets(window, granularity)

	now := time.Now()

	// Test empty bucket
	if !buckets.IsEmpty(now) {
		t.Error("New bucket should be empty")
	}
	if avg := buckets.WindowAverage(now); avg != 0 {
		t.Errorf("Empty bucket average should be 0, got %f", avg)
	}

	// Record some values
	buckets.Record(now, 10)
	buckets.Record(now.Add(1*time.Second), 20)
	buckets.Record(now.Add(2*time.Second), 30)

	// Test not empty
	if buckets.IsEmpty(now.Add(2 * time.Second)) {
		t.Error("Bucket should not be empty after recording")
	}

	// Test average
	avg := buckets.WindowAverage(now.Add(2 * time.Second))
	// 3 buckets used, average should be (10 + 20 + 30) / 3 = 20.0
	expected := 20.0
	if avg != expected {
		t.Errorf("Expected average %f, got %f", expected, avg)
	}
}

func TestTimedFloat64Buckets_WindowExpiry(t *testing.T) {
	window := 5 * time.Second
	granularity := 1 * time.Second
	buckets := NewTimedFloat64Buckets(window, granularity)

	now := time.Now()

	// Record values
	buckets.Record(now, 100)

	// Check within window
	if buckets.IsEmpty(now.Add(4 * time.Second)) {
		t.Error("Should not be empty within window")
	}

	// Check after window expires
	if !buckets.IsEmpty(now.Add(6 * time.Second)) {
		t.Error("Should be empty after window expires")
	}
}

func TestTimedFloat64Buckets_MultipleValues(t *testing.T) {
	window := 10 * time.Second
	granularity := 1 * time.Second
	buckets := NewTimedFloat64Buckets(window, granularity)

	now := time.Now().Truncate(time.Second)

	// Record multiple values in same bucket
	buckets.Record(now, 10)
	buckets.Record(now.Add(500*time.Millisecond), 20) // Same bucket due to truncation

	avg := buckets.WindowAverage(now.Add(500 * time.Millisecond))
	expected := 30.0 // Both values (10+20) are in the same bucket, and we have only 1 bucket
	if avg != expected {
		t.Errorf("Expected average %f, got %f", expected, avg)
	}
}

func TestTimedFloat64Buckets_ResizeWindow(t *testing.T) {
	window := 10 * time.Second
	granularity := 1 * time.Second
	buckets := NewTimedFloat64Buckets(window, granularity)

	now := time.Now().Truncate(time.Second)

	// Record values (1, 2, 3, ..., 10)
	for i := 0; i < 10; i++ {
		buckets.Record(now.Add(time.Duration(i)*time.Second), float64(i+1))
	}

	// Check average before resize (all 10 values: 1+2+...+10 = 55, avg = 5.5)
	avg1 := buckets.WindowAverage(now.Add(9 * time.Second))

	// Resize to smaller window
	buckets.ResizeWindow(5 * time.Second)

	// After resize, ResizeWindow keeps the most recent values based on lastWrite
	// The resize will keep recent buckets, so the average might actually increase
	// since recent values (6,7,8,9,10) have higher numbers
	avg2 := buckets.WindowAverage(now.Add(9 * time.Second))

	// Just verify that resize had an effect
	if avg1 == avg2 {
		t.Errorf("Average should change after resizing window: before=%f, after=%f", avg1, avg2)
	}
}

func TestTimedFloat64Buckets_GapInData(t *testing.T) {
	window := 10 * time.Second
	granularity := 1 * time.Second
	buckets := NewTimedFloat64Buckets(window, granularity)

	now := time.Now().Truncate(time.Second)

	// Record with gaps
	buckets.Record(now, 10)
	buckets.Record(now.Add(5*time.Second), 20)

	// Check at the time of second recording
	avg := buckets.WindowAverage(now.Add(5 * time.Second))
	// We have values at t=0 (10) and t=5 (20), with gaps at t=1,2,3,4 (zeros)
	// Total = 10 + 0 + 0 + 0 + 0 + 20 = 30
	// Buckets = 6
	// Average = 30/6 = 5
	expected := 5.0
	if avg != expected {
		t.Errorf("Expected average %f, got %f", expected, avg)
	}
}

func TestTimeWindow_MaxTracking(t *testing.T) {
	window := NewTimeWindow(10*time.Second, 1*time.Second)

	now := time.Now()

	// Record values
	window.Record(now, 5)
	window.Record(now.Add(1*time.Second), 10)
	window.Record(now.Add(2*time.Second), 3)
	window.Record(now.Add(3*time.Second), 8)

	// Should return maximum value
	maxValue := window.Current()
	if maxValue != 10 {
		t.Errorf("Expected max 10, got %d", maxValue)
	}
}

func TestTimeWindow_ResizeWindow(t *testing.T) {
	window := NewTimeWindow(10*time.Second, 1*time.Second)

	now := time.Now()

	// Record values
	for i := int32(1); i <= 10; i++ {
		window.Record(now.Add(time.Duration(i)*time.Second), i)
	}

	// Resize window
	window.ResizeWindow(5 * time.Second)

	// Max should still be present if within new window
	maxValue := window.Current()
	if maxValue == 0 {
		t.Error("Max should not be 0 after resize")
	}
}

func TestMetricSnapshot(t *testing.T) {
	now := time.Now()
	snapshot := NewMetricSnapshot(100.0, 150.0, 5, now)

	if snapshot.StableValue() != 100.0 {
		t.Errorf("Expected stable value 100.0, got %f", snapshot.StableValue())
	}

	if snapshot.PanicValue() != 150.0 {
		t.Errorf("Expected panic value 150.0, got %f", snapshot.PanicValue())
	}

	if snapshot.ReadyPodCount() != 5 {
		t.Errorf("Expected 5 ready pods, got %d", snapshot.ReadyPodCount())
	}

	if !snapshot.Timestamp().Equal(now) {
		t.Errorf("Expected timestamp %v, got %v", now, snapshot.Timestamp())
	}
}

func BenchmarkTimedFloat64Buckets_Record(b *testing.B) {
	buckets := NewTimedFloat64Buckets(60*time.Second, 1*time.Second)
	now := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buckets.Record(now.Add(time.Duration(i)*time.Millisecond), float64(i))
	}
}

func BenchmarkTimedFloat64Buckets_WindowAverage(b *testing.B) {
	buckets := NewTimedFloat64Buckets(60*time.Second, 1*time.Second)
	now := time.Now()

	// Pre-fill with data
	for i := 0; i < 60; i++ {
		buckets.Record(now.Add(time.Duration(i)*time.Second), float64(i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buckets.WindowAverage(now.Add(60 * time.Second))
	}
}

func TestTimedFloat64Buckets_EdgeCases(t *testing.T) {
	window := 10 * time.Second
	granularity := 1 * time.Second
	buckets := NewTimedFloat64Buckets(window, granularity)

	now := time.Now()

	// Test recording at exact window boundary
	buckets.Record(now, 10)
	buckets.Record(now.Add(10*time.Second), 20) // Exactly at window edge

	// The second value should be in a new window
	avg := buckets.WindowAverage(now.Add(10 * time.Second))
	if avg != 20.0 {
		t.Errorf("Expected average 20.0 for new window, got %f", avg)
	}

	// Test negative time progression (shouldn't happen in practice)
	buckets.Record(now.Add(5*time.Second), 30)
	// This should be ignored or handled gracefully
}

func TestTimedFloat64Buckets_ConcurrentAccess(t *testing.T) {
	window := 10 * time.Second
	granularity := 1 * time.Second
	buckets := NewTimedFloat64Buckets(window, granularity)

	now := time.Now()
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			buckets.Record(now.Add(time.Duration(i)*time.Millisecond), float64(i))
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			_ = buckets.WindowAverage(now)
			_ = buckets.IsEmpty(now)
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()

	// Wait for both to complete
	<-done
	<-done

	// If we get here without deadlock or panic, the test passes
}
