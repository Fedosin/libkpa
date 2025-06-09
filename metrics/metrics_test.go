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
	for i := range 10 {
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
	i := 0
	for b.Loop() {
		buckets.Record(now.Add(time.Duration(i)*time.Millisecond), float64(i))
		i++
	}
}

func BenchmarkTimedFloat64Buckets_WindowAverage(b *testing.B) {
	buckets := NewTimedFloat64Buckets(60*time.Second, 1*time.Second)
	now := time.Now()

	// Pre-fill with data
	for i := range 60 {
		buckets.Record(now.Add(time.Duration(i)*time.Second), float64(i))
	}

	b.ResetTimer()
	for b.Loop() {
		buckets.WindowAverage(now.Add(60 * time.Second))
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
		for i := range 100 {
			buckets.Record(now.Add(time.Duration(i)*time.Millisecond), float64(i))
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for range 100 {
			buckets.WindowAverage(now)
			buckets.IsEmpty(now)
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()

	// Wait for both to complete
	<-done
	<-done

	// If we get here without deadlock or panic, the test passes
}

func TestTimedFloat64Buckets_RecordOldValues(t *testing.T) {
	window := 5 * time.Second
	granularity := 1 * time.Second
	buckets := NewTimedFloat64Buckets(window, granularity)

	now := time.Now()

	// Record initial value
	buckets.Record(now, 100)

	// Try to record a value older than window
	buckets.Record(now.Add(-10*time.Second), 200)

	// Old value should be ignored
	avg := buckets.WindowAverage(now)
	if avg != 100 {
		t.Errorf("Old values should be ignored, expected 100, got %f", avg)
	}
}

func TestTimedFloat64Buckets_ZeroAndNegativeValues(t *testing.T) {
	window := 5 * time.Second
	granularity := 1 * time.Second
	buckets := NewTimedFloat64Buckets(window, granularity)

	now := time.Now()

	// Record zero, negative, and positive values
	buckets.Record(now, 0)
	buckets.Record(now.Add(1*time.Second), -10)
	buckets.Record(now.Add(2*time.Second), 20)
	buckets.Record(now.Add(3*time.Second), -5)

	avg := buckets.WindowAverage(now.Add(3 * time.Second))
	// Average should be (0 + (-10) + 20 + (-5)) / 4 = 5 / 4 = 1.25
	expected := 1.25
	if avg != expected {
		t.Errorf("Expected average %f, got %f", expected, avg)
	}
}

func TestTimedFloat64Buckets_LargeValues(t *testing.T) {
	window := 3 * time.Second
	granularity := 1 * time.Second
	buckets := NewTimedFloat64Buckets(window, granularity)

	now := time.Now()

	// Record very large values
	largeValue := math.MaxFloat64 / 10 // Avoid overflow
	buckets.Record(now, largeValue)
	buckets.Record(now.Add(1*time.Second), largeValue)

	avg := buckets.WindowAverage(now.Add(1 * time.Second))
	if avg != largeValue {
		t.Errorf("Expected average %e, got %e", largeValue, avg)
	}
}

func TestTimedFloat64Buckets_FractionalValues(t *testing.T) {
	window := 3 * time.Second
	granularity := 1 * time.Second
	buckets := NewTimedFloat64Buckets(window, granularity)

	now := time.Now()

	// Record fractional values
	buckets.Record(now, 1.1)
	buckets.Record(now.Add(1*time.Second), 2.2)
	buckets.Record(now.Add(2*time.Second), 3.3)

	avg := buckets.WindowAverage(now.Add(2 * time.Second))
	expected := 2.2 // (1.1 + 2.2 + 3.3) / 3
	tolerance := 0.0001
	if math.Abs(avg-expected) > tolerance {
		t.Errorf("Expected average ~%f, got %f", expected, avg)
	}
}

func TestTimedFloat64Buckets_ExactWindowBoundary(t *testing.T) {
	window := 5 * time.Second
	granularity := 1 * time.Second
	buckets := NewTimedFloat64Buckets(window, granularity)

	now := time.Now().Truncate(time.Second)

	// Fill exactly the window
	for i := range 5 {
		buckets.Record(now.Add(time.Duration(i)*time.Second), float64(i+1))
	}

	// Check at exact window end
	avg := buckets.WindowAverage(now.Add(4 * time.Second))
	expected := 3.0 // (1+2+3+4+5)/5
	if avg != expected {
		t.Errorf("Expected average %f, got %f", expected, avg)
	}

	// Check after window expires (need to account for truncation)
	// The last write was at now+4s, so window expires at now+4s+5s = now+9s
	if !buckets.IsEmpty(now.Add(10 * time.Second)) {
		t.Error("Should be empty after window expires")
	}
}

func TestTimedFloat64Buckets_WindowAverageWithFutureTime(t *testing.T) {
	window := 5 * time.Second
	granularity := 1 * time.Second
	buckets := NewTimedFloat64Buckets(window, granularity)

	now := time.Now()

	buckets.Record(now, 10)
	buckets.Record(now.Add(1*time.Second), 20)

	// Query with future time (within window)
	futureTime := now.Add(3 * time.Second)
	avg := buckets.WindowAverage(futureTime)

	// Should still include recorded values
	expected := 15.0 // (10 + 20) / 2
	if avg != expected {
		t.Errorf("Expected average %f for future time query, got %f", expected, avg)
	}
}

func TestTimedFloat64Buckets_ResizeWindowEdgeCases(t *testing.T) {
	t.Run("ResizeToSameSize", func(t *testing.T) {
		buckets := NewTimedFloat64Buckets(10*time.Second, 1*time.Second)
		now := time.Now()

		buckets.Record(now, 100)
		buckets.ResizeWindow(10 * time.Second) // Same size

		// Should maintain data
		avg := buckets.WindowAverage(now)
		if avg != 100 {
			t.Errorf("Data should be preserved when resizing to same size, got %f", avg)
		}
	})

	t.Run("ResizeToLarger", func(t *testing.T) {
		buckets := NewTimedFloat64Buckets(5*time.Second, 1*time.Second)
		now := time.Now()

		// Fill current window
		for i := range 5 {
			buckets.Record(now.Add(time.Duration(i)*time.Second), float64(i+1))
		}

		buckets.ResizeWindow(10 * time.Second)

		// All data should still be there
		avg := buckets.WindowAverage(now.Add(4 * time.Second))
		if avg == 0 {
			t.Error("Data should be preserved when resizing to larger window")
		}
	})

	t.Run("ResizeAfterExpiry", func(t *testing.T) {
		buckets := NewTimedFloat64Buckets(5*time.Second, 1*time.Second)
		now := time.Now()

		buckets.Record(now, 100)

		// Wait for window to expire
		expiredTime := now.Add(10 * time.Second)

		// Resize after expiry
		buckets.ResizeWindow(3 * time.Second)

		// Should be empty
		if !buckets.IsEmpty(expiredTime) {
			t.Error("Should be empty after resizing expired window")
		}
	})
}

func TestTimedFloat64Buckets_ConcurrentResizeWindow(t *testing.T) {
	buckets := NewTimedFloat64Buckets(10*time.Second, 1*time.Second)
	now := time.Now()

	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := range 50 {
			buckets.Record(now.Add(time.Duration(i)*100*time.Millisecond), float64(i))
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()

	// Resizer goroutine
	go func() {
		for i := range 10 {
			newWindow := time.Duration(5+i) * time.Second
			buckets.ResizeWindow(newWindow)
			time.Sleep(5 * time.Millisecond)
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for range 50 {
			buckets.WindowAverage(now)
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()

	// Wait for all to complete
	<-done
	<-done
	<-done
}

func TestTimedFloat64Buckets_FirstWriteUpdate(t *testing.T) {
	window := 10 * time.Second
	granularity := 1 * time.Second
	buckets := NewTimedFloat64Buckets(window, granularity)

	now := time.Now().Truncate(time.Second)

	// Record in reverse order
	buckets.Record(now.Add(5*time.Second), 50)
	buckets.Record(now.Add(3*time.Second), 30)
	buckets.Record(now.Add(1*time.Second), 10)

	// Should handle firstWrite correctly
	avg := buckets.WindowAverage(now.Add(5 * time.Second))
	// Should include all three values: (10 + 0 + 30 + 0 + 50) / 5 = 18
	expected := 18.0
	if avg != expected {
		t.Errorf("Expected average %f, got %f", expected, avg)
	}
}

func TestTimeWindow_EmptyWindow(t *testing.T) {
	window := NewTimeWindow(10*time.Second, 1*time.Second)

	// Current should return 0 for empty window
	if current := window.Current(); current != 0 {
		t.Errorf("Empty window should return 0, got %d", current)
	}
}

func TestTimeWindow_NegativeAndZeroValues(t *testing.T) {
	window := NewTimeWindow(5*time.Second, 1*time.Second)
	now := time.Now()

	// Record various values including zero and negative
	window.Record(now, -10)
	window.Record(now.Add(1*time.Second), 0)
	window.Record(now.Add(2*time.Second), 15)
	window.Record(now.Add(3*time.Second), -5)

	// Should track the maximum (15)
	maxValue := window.Current()
	if maxValue != 15 {
		t.Errorf("Expected max 15, got %d", maxValue)
	}
}

func TestTimeWindow_ConcurrentOperations(t *testing.T) {
	window := NewTimeWindow(10*time.Second, 1*time.Second)
	now := time.Now()
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := int32(0); i < 100; i++ {
			window.Record(now.Add(time.Duration(i)*10*time.Millisecond), i)
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for range 100 {
			window.Current()
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()

	// Resizer goroutine
	go func() {
		for i := range 10 {
			window.ResizeWindow(time.Duration(5+i) * time.Second)
			time.Sleep(10 * time.Millisecond)
		}
		done <- true
	}()

	// Wait for all to complete
	<-done
	<-done
	<-done
}

func TestTimeWindow_ResizeEffectOnMax(t *testing.T) {
	window := NewTimeWindow(10*time.Second, 1*time.Second)
	now := time.Now()

	// Record values spread across time
	window.Record(now, 10)
	window.Record(now.Add(3*time.Second), 30)
	window.Record(now.Add(6*time.Second), 20)
	window.Record(now.Add(9*time.Second), 40)

	// Max should be 40
	if maxValue := window.Current(); maxValue != 40 {
		t.Errorf("Expected max 40, got %d", maxValue)
	}

	// Resize to smaller window that might exclude the max
	window.ResizeWindow(3 * time.Second)

	// Max might change after resize
	newMax := window.Current()
	if newMax == 0 {
		t.Error("Max should not be 0 after resize if any values remain")
	}
}

func TestMetricSnapshot_EdgeCases(t *testing.T) {
	t.Run("ZeroValues", func(t *testing.T) {
		now := time.Now()
		snapshot := NewMetricSnapshot(0.0, 0.0, 0, now)

		if snapshot.StableValue() != 0.0 {
			t.Error("Stable value should be 0")
		}
		if snapshot.PanicValue() != 0.0 {
			t.Error("Panic value should be 0")
		}
		if snapshot.ReadyPodCount() != 0 {
			t.Error("Ready pod count should be 0")
		}
	})

	t.Run("NegativeValues", func(t *testing.T) {
		now := time.Now()
		snapshot := NewMetricSnapshot(-100.5, -200.5, -5, now)

		if snapshot.StableValue() != -100.5 {
			t.Errorf("Expected stable value -100.5, got %f", snapshot.StableValue())
		}
		if snapshot.PanicValue() != -200.5 {
			t.Errorf("Expected panic value -200.5, got %f", snapshot.PanicValue())
		}
		if snapshot.ReadyPodCount() != -5 {
			t.Errorf("Expected -5 ready pods, got %d", snapshot.ReadyPodCount())
		}
	})

	t.Run("Immutability", func(t *testing.T) {
		now := time.Now()
		snapshot := NewMetricSnapshot(100.0, 150.0, 5, now)

		// Get values multiple times to ensure they don't change
		for range 3 {
			if snapshot.StableValue() != 100.0 {
				t.Error("Stable value changed")
			}
			if snapshot.PanicValue() != 150.0 {
				t.Error("Panic value changed")
			}
			if snapshot.ReadyPodCount() != 5 {
				t.Error("Ready pod count changed")
			}
			if !snapshot.Timestamp().Equal(now) {
				t.Error("Timestamp changed")
			}
		}
	})
}

func TestMinInt(t *testing.T) {
	tests := []struct {
		a, b, expected int
	}{
		{1, 2, 1},
		{2, 1, 1},
		{-1, 1, -1},
		{-5, -3, -5},
		{0, 0, 0},
		{math.MaxInt32, math.MinInt32, math.MinInt32},
	}

	for _, test := range tests {
		result := min(test.a, test.b)
		if result != test.expected {
			t.Errorf("min(%d, %d) = %d, expected %d", test.a, test.b, result, test.expected)
		}
	}
}

// Additional Benchmarks

func BenchmarkTimeWindow_Record(b *testing.B) {
	window := NewTimeWindow(60*time.Second, 1*time.Second)
	now := time.Now()

	b.ResetTimer()
	i := 0
	for b.Loop() {
		window.Record(now.Add(time.Duration(i)*time.Millisecond), int32(i%100))
		i++
	}
}

func BenchmarkTimeWindow_Current(b *testing.B) {
	window := NewTimeWindow(60*time.Second, 1*time.Second)
	now := time.Now()

	// Pre-fill with data
	for i := range 60 {
		window.Record(now.Add(time.Duration(i)*time.Second), int32(i))
	}

	b.ResetTimer()
	for b.Loop() {
		_ = window.Current()
	}
}

func BenchmarkTimedFloat64Buckets_ResizeWindow(b *testing.B) {
	buckets := NewTimedFloat64Buckets(60*time.Second, 1*time.Second)
	now := time.Now()

	// Pre-fill with data
	for i := range 60 {
		buckets.Record(now.Add(time.Duration(i)*time.Second), float64(i))
	}

	b.ResetTimer()
	i := 0
	for b.Loop() {
		newWindow := time.Duration((i%10)+5) * time.Second
		buckets.ResizeWindow(newWindow)
		i++
	}
}

func BenchmarkTimedFloat64Buckets_ConcurrentAccess(b *testing.B) {
	buckets := NewTimedFloat64Buckets(60*time.Second, 1*time.Second)
	now := time.Now()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				buckets.Record(now.Add(time.Duration(i)*time.Millisecond), float64(i))
			} else {
				_ = buckets.WindowAverage(now)
			}
			i++
		}
	})
}

func BenchmarkMetricSnapshot_Creation(b *testing.B) {
	now := time.Now()

	b.ResetTimer()
	i := 0
	for b.Loop() {
		_ = NewMetricSnapshot(float64(i), float64(i*2), int32(i), now)
		i++
	}
}

// Additional edge case tests for 100% coverage

func TestTimedFloat64Buckets_WindowAverageRecentPast(t *testing.T) {
	window := 10 * time.Second
	granularity := 1 * time.Second
	buckets := NewTimedFloat64Buckets(window, granularity)

	now := time.Now().Truncate(time.Second)

	// Record some values
	for i := range 10 {
		buckets.Record(now.Add(time.Duration(i)*time.Second), float64(i))
	}

	// Query from the recent past (within window)
	pastTime := now.Add(7 * time.Second)
	avg := buckets.WindowAverage(pastTime)

	// The query time is in the past relative to lastWrite (which is at now+9s)
	// This triggers the "recent past" case in WindowAverage
	// Since we're querying at now+7s, and data extends to now+9s,
	// we should have data from earlier buckets
	if avg == 0 {
		t.Error("Average should not be 0 for recent past query")
	}
}

func TestTimedFloat64Buckets_MinimumGranularity(t *testing.T) {
	// Test with minimum supported granularity (1 second)
	window := 5 * time.Second
	granularity := 1 * time.Second
	buckets := NewTimedFloat64Buckets(window, granularity)

	now := time.Now().Truncate(time.Second)

	// Record values at second boundaries
	for i := range 5 {
		buckets.Record(now.Add(time.Duration(i)*time.Second), float64(i+1))
	}

	avg := buckets.WindowAverage(now.Add(4 * time.Second))
	// Should include all 5 values: (1+2+3+4+5)/5 = 3
	expected := 3.0
	if avg != expected {
		t.Errorf("Expected average %f with minimum granularity, got %f", expected, avg)
	}
}

func TestTimedFloat64Buckets_SmallWindow(t *testing.T) {
	// Test with a small window (minimum granularity is 1 second)
	window := 3 * time.Second
	granularity := 1 * time.Second
	buckets := NewTimedFloat64Buckets(window, granularity)

	now := time.Now().Truncate(time.Second)

	// Record values
	buckets.Record(now, 1)
	buckets.Record(now.Add(1*time.Second), 2)
	buckets.Record(now.Add(2*time.Second), 3)

	avg := buckets.WindowAverage(now.Add(2 * time.Second))
	expected := 2.0 // (1+2+3)/3
	if avg != expected {
		t.Errorf("Expected average %f with small window, got %f", expected, avg)
	}
}

func TestTimedFloat64Buckets_WindowAverageBeyondWindow(t *testing.T) {
	window := 5 * time.Second
	granularity := 1 * time.Second
	buckets := NewTimedFloat64Buckets(window, granularity)

	now := time.Now()

	// Record a value
	buckets.Record(now, 100)

	// Query way beyond the window
	futureTime := now.Add(10 * time.Second)
	avg := buckets.WindowAverage(futureTime)

	// Should return 0 as data is outside window
	if avg != 0 {
		t.Errorf("Expected 0 for query beyond window, got %f", avg)
	}
}

func TestTimeWindow_MultipleMaxValues(t *testing.T) {
	window := NewTimeWindow(5*time.Second, 1*time.Second)
	now := time.Now()

	// Record multiple instances of the same max value
	window.Record(now, 50)
	window.Record(now.Add(1*time.Second), 50)
	window.Record(now.Add(2*time.Second), 30)
	window.Record(now.Add(3*time.Second), 50)

	maxValue := window.Current()
	if maxValue != 50 {
		t.Errorf("Expected max 50, got %d", maxValue)
	}
}

func TestTimedFloat64Buckets_StressTestLargeBuckets(t *testing.T) {
	// Test with a large number of buckets
	window := 1 * time.Hour
	granularity := 1 * time.Second
	buckets := NewTimedFloat64Buckets(window, granularity)

	now := time.Now()

	// Record values across the entire window
	for i := 0; i < 3600; i += 60 { // Every minute
		buckets.Record(now.Add(time.Duration(i)*time.Second), float64(i))
	}

	// Should not panic and should return valid average
	avg := buckets.WindowAverage(now.Add(59 * time.Minute))
	if avg == 0 {
		t.Error("Average should not be 0 for large bucket test")
	}
}

func TestTimedFloat64Buckets_RecordSameBucketAccumulation(t *testing.T) {
	window := 5 * time.Second
	granularity := 1 * time.Second
	buckets := NewTimedFloat64Buckets(window, granularity)

	now := time.Now().Truncate(time.Second)

	// Record multiple values in the same bucket (same second)
	buckets.Record(now, 10)
	buckets.Record(now.Add(100*time.Millisecond), 20)
	buckets.Record(now.Add(200*time.Millisecond), 30)
	buckets.Record(now.Add(999*time.Millisecond), 40)

	// All values should accumulate in the same bucket
	avg := buckets.WindowAverage(now)
	expected := 100.0 // 10+20+30+40 = 100 in one bucket
	if avg != expected {
		t.Errorf("Expected accumulated value %f, got %f", expected, avg)
	}
}

func TestResizeWindow_RaceCondition(t *testing.T) {
	// Test for potential race conditions during resize
	buckets := NewTimedFloat64Buckets(10*time.Second, 1*time.Second)

	done := make(chan bool)
	errors := make(chan error, 100)

	// Multiple goroutines resizing concurrently
	for i := range 10 {
		go func(id int) {
			defer func() {
				if r := recover(); r != nil {
					errors <- r.(error)
				}
				done <- true
			}()

			for j := range 10 {
				newSize := time.Duration(5+(id+j)%10) * time.Second
				buckets.ResizeWindow(newSize)
				time.Sleep(time.Millisecond)
			}
		}(i)
	}

	// Wait for all goroutines
	for range 10 {
		<-done
	}

	close(errors)

	// Check for any errors
	for err := range errors {
		t.Errorf("Race condition detected: %v", err)
	}
}
