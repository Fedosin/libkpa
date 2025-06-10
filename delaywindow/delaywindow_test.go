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

package delaywindow

import (
	"fmt"
	"testing"
	"time"
)

func TestDelayWindow_BasicFunctionality(t *testing.T) {
	// Create a window with 30 second window and 1 second granularity
	dw := NewDelayWindow(30*time.Second, 1*time.Second)

	// Test base time
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Record some values
	dw.Record(baseTime, 10)
	dw.Record(baseTime.Add(5*time.Second), 20)
	dw.Record(baseTime.Add(10*time.Second), 15)

	// Check that CurrentMax returns the highest value
	maxDelay := dw.CurrentMax()
	if maxDelay != 20 {
		t.Errorf("Expected max value 20, got %d", maxDelay)
	}
}

func TestDelayWindow_OverwriteSameSecond(t *testing.T) {
	// Test that recording at the same second overwrites previous value
	dw := NewDelayWindow(30*time.Second, 1*time.Second)

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Record initial value
	dw.Record(baseTime, 10)
	maxDelay := dw.CurrentMax()
	if maxDelay != 10 {
		t.Errorf("Expected max value 10, got %d", maxDelay)
	}

	// Record new value at same second (should overwrite)
	dw.Record(baseTime.Add(500*time.Millisecond), 25) // Same second after truncation
	maxDelay = dw.CurrentMax()
	if maxDelay != 25 {
		t.Errorf("Expected max value 25 after overwrite, got %d", maxDelay)
	}

	// Verify only one value exists
	dw.Record(baseTime.Add(1*time.Second), 5)
	maxDelay = dw.CurrentMax()
	if maxDelay != 25 {
		t.Errorf("Expected max value 25 (not 10+25), got %d", maxDelay)
	}
}

func TestDelayWindow_SparseRecordings(t *testing.T) {
	// Test that sparse recordings default missing slots to zero
	dw := NewDelayWindow(10*time.Second, 1*time.Second)

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Record values with gaps
	dw.Record(baseTime, 15)                    // slot 0
	dw.Record(baseTime.Add(3*time.Second), 8)  // slot 3
	dw.Record(baseTime.Add(7*time.Second), 12) // slot 7

	// CurrentMax should be 15
	maxDelay := dw.CurrentMax()
	if maxDelay != 15 {
		t.Errorf("Expected max value 15, got %d", maxDelay)
	}

	// Move time forward so first value is outside window
	dw.Record(baseTime.Add(11*time.Second), 5)

	// Now max should be 12 (from slot 7)
	maxDelay = dw.CurrentMax()
	if maxDelay != 12 {
		t.Errorf("Expected max value 12 after window slide, got %d", maxDelay)
	}
}

func TestDelayWindow_WindowSliding(t *testing.T) {
	// Test that CurrentMax correctly reflects values in the sliding window
	dw := NewDelayWindow(5*time.Second, 1*time.Second)

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Fill the window
	dw.Record(baseTime, 1)
	dw.Record(baseTime.Add(1*time.Second), 2)
	dw.Record(baseTime.Add(2*time.Second), 3)
	dw.Record(baseTime.Add(3*time.Second), 4)
	dw.Record(baseTime.Add(4*time.Second), 5)

	maxDelay := dw.CurrentMax()
	if maxDelay != 5 {
		t.Errorf("Expected max value 5, got %d", maxDelay)
	}

	// Add value that causes window to slide
	dw.Record(baseTime.Add(5*time.Second), 3)

	// Window now contains values 2,3,4,5,3 (value 1 is outside)
	maxDelay = dw.CurrentMax()
	if maxDelay != 5 {
		t.Errorf("Expected max value 5 after slide, got %d", maxDelay)
	}

	// Add another value
	dw.Record(baseTime.Add(6*time.Second), 2)

	// Window now contains values 3,4,5,3,2 (values 1,2 are outside)
	maxDelay = dw.CurrentMax()
	if maxDelay != 5 {
		t.Errorf("Expected max value 5 after second slide, got %d", maxDelay)
	}

	// Move forward enough to exclude value 5
	dw.Record(baseTime.Add(9*time.Second), 1)

	// Window now contains values 3,2,0,0,1 (assuming 0 for slots 7,8)
	maxDelay = dw.CurrentMax()
	if maxDelay != 3 {
		t.Errorf("Expected max value 3 after excluding 5, got %d", maxDelay)
	}
}

func TestDelayWindow_CircularBuffer(t *testing.T) {
	// Test circular buffer wrap-around behavior
	dw := NewDelayWindow(3*time.Second, 1*time.Second)

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Fill buffer multiple times to test wrap-around
	for i := range 10 {
		dw.Record(baseTime.Add(time.Duration(i)*time.Second), int32(i+1))
	}

	// Window should contain values 8, 9, 10 (for times 7s, 8s, 9s)
	maxDelay := dw.CurrentMax()
	if maxDelay != 10 {
		t.Errorf("Expected max value 10, got %d", maxDelay)
	}
}

func TestDelayWindow_EmptyWindow(t *testing.T) {
	// Test empty window returns 0
	dw := NewDelayWindow(10*time.Second, 1*time.Second)

	maxDelay := dw.CurrentMax()
	if maxDelay != 0 {
		t.Errorf("Expected max value 0 for empty window, got %d", maxDelay)
	}
}

func TestDelayWindow_AllValuesExpired(t *testing.T) {
	// Test that expired values are not included in max calculation
	dw := NewDelayWindow(5*time.Second, 1*time.Second)

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Record some values
	dw.Record(baseTime, 10)
	dw.Record(baseTime.Add(1*time.Second), 20)

	// Move time forward beyond window
	dw.Record(baseTime.Add(10*time.Second), 5)

	// Only value 5 should be in window
	maxDelay := dw.CurrentMax()
	if maxDelay != 5 {
		t.Errorf("Expected max value 5 after expiry, got %d", maxDelay)
	}
}

func TestDelayWindow_Granularity(t *testing.T) {
	// Test different granularities
	dw := NewDelayWindow(1*time.Minute, 10*time.Second)

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Values within same 10-second slot
	dw.Record(baseTime.Add(2*time.Second), 5)
	dw.Record(baseTime.Add(7*time.Second), 15) // Should overwrite

	maxDelay := dw.CurrentMax()
	if maxDelay != 15 {
		t.Errorf("Expected max value 15, got %d", maxDelay)
	}

	// Value in different slot
	dw.Record(baseTime.Add(12*time.Second), 10)

	maxDelay = dw.CurrentMax()
	if maxDelay != 15 {
		t.Errorf("Expected max value still 15, got %d", maxDelay)
	}
}

func TestDelayWindow_Constructor(t *testing.T) {
	// Test constructor validation
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic for invalid window size")
		}
	}()

	// This should panic - window size not divisible by granularity
	NewDelayWindow(31*time.Second, 10*time.Second)
}

func TestDelayWindow_NegativeValues(t *testing.T) {
	// Test that only non-negative values are considered per requirement
	dw := NewDelayWindow(10*time.Second, 1*time.Second)

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Record mix of positive and zero values
	dw.Record(baseTime, 0)
	dw.Record(baseTime.Add(1*time.Second), 10)
	dw.Record(baseTime.Add(2*time.Second), 0)
	dw.Record(baseTime.Add(3*time.Second), 5)

	maxDelay := dw.CurrentMax()
	if maxDelay != 10 {
		t.Errorf("Expected max value 10, got %d", maxDelay)
	}
}

func TestDelayWindow_ResizeExpansion(t *testing.T) {
	// Test expanding the window preserves all data
	dw := NewDelayWindow(5*time.Second, 1*time.Second)

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Fill the window
	dw.Record(baseTime, 1)
	dw.Record(baseTime.Add(1*time.Second), 2)
	dw.Record(baseTime.Add(2*time.Second), 3)
	dw.Record(baseTime.Add(3*time.Second), 4)
	dw.Record(baseTime.Add(4*time.Second), 5)

	// Verify initial max
	if maxDelay := dw.CurrentMax(); maxDelay != 5 {
		t.Errorf("Expected initial max 5, got %d", maxDelay)
	}

	// Expand window to 10 seconds
	dw.Resize(10 * time.Second)

	// All values should still be present
	if maxDelay := dw.CurrentMax(); maxDelay != 5 {
		t.Errorf("Expected max 5 after expansion, got %d", maxDelay)
	}

	// Add more values beyond original window
	dw.Record(baseTime.Add(5*time.Second), 6)
	dw.Record(baseTime.Add(6*time.Second), 7)

	if maxDelay := dw.CurrentMax(); maxDelay != 7 {
		t.Errorf("Expected max 7 after adding more values, got %d", maxDelay)
	}
}

func TestDelayWindow_ResizeContraction(t *testing.T) {
	// Test shrinking the window discards old data
	dw := NewDelayWindow(10*time.Second, 1*time.Second)

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Fill the window
	for i := 0; i < 10; i++ {
		dw.Record(baseTime.Add(time.Duration(i)*time.Second), int32(i+1))
	}

	// Verify initial max
	if maxDelay := dw.CurrentMax(); maxDelay != 10 {
		t.Errorf("Expected initial max 10, got %d", maxDelay)
	}

	// Shrink window to 5 seconds
	dw.Resize(5 * time.Second)

	// Only values 6-10 should remain (last 5 seconds)
	if maxDelay := dw.CurrentMax(); maxDelay != 10 {
		t.Errorf("Expected max 10 after contraction, got %d", maxDelay)
	}

	// Verify old values are gone by adding a new value and checking
	dw.Record(baseTime.Add(10*time.Second), 3)

	// Window should now contain values 7,8,9,10,3
	if maxDelay := dw.CurrentMax(); maxDelay != 10 {
		t.Errorf("Expected max still 10, got %d", maxDelay)
	}
}

func TestDelayWindow_ResizeSameSize(t *testing.T) {
	// Test resizing to same size doesn't affect data
	dw := NewDelayWindow(5*time.Second, 1*time.Second)

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Add some values
	dw.Record(baseTime, 10)
	dw.Record(baseTime.Add(2*time.Second), 20)

	// Resize to same size
	dw.Resize(5 * time.Second)

	// Values should be unchanged
	if maxDelay := dw.CurrentMax(); maxDelay != 20 {
		t.Errorf("Expected max 20 after same-size resize, got %d", maxDelay)
	}
}

func TestDelayWindow_ResizeWithSparseData(t *testing.T) {
	// Test resize with sparse data in the window
	dw := NewDelayWindow(20*time.Second, 2*time.Second)

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Add sparse values
	dw.Record(baseTime, 5)
	dw.Record(baseTime.Add(6*time.Second), 15)
	dw.Record(baseTime.Add(14*time.Second), 10)
	dw.Record(baseTime.Add(18*time.Second), 20)

	// Shrink to 10 seconds
	dw.Resize(10 * time.Second)

	// Only values at 14s and 18s should remain
	if maxDelay := dw.CurrentMax(); maxDelay != 20 {
		t.Errorf("Expected max 20 after resize with sparse data, got %d", maxDelay)
	}
}

func TestDelayWindow_ResizeCircularBufferMapping(t *testing.T) {
	// Test that circular buffer indices are correctly remapped during resize
	dw := NewDelayWindow(6*time.Second, 1*time.Second)

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Fill buffer to trigger wrap-around
	for i := 0; i < 10; i++ {
		dw.Record(baseTime.Add(time.Duration(i)*time.Second), int32(i*10))
	}

	// Resize to different size to force index remapping
	dw.Resize(8 * time.Second)

	// Check that recent values are preserved correctly
	if maxDelay := dw.CurrentMax(); maxDelay != 90 {
		t.Errorf("Expected max 90 after resize with circular buffer, got %d", maxDelay)
	}
}

func TestDelayWindow_ResizeInvalidInput(t *testing.T) {
	dw := NewDelayWindow(10*time.Second, 2*time.Second)

	// Test negative window size
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("Expected panic for negative window size")
			}
		}()
		dw.Resize(-5 * time.Second)
	}()

	// Test window size not divisible by granularity
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("Expected panic for window size not divisible by granularity")
			}
		}()
		dw.Resize(11 * time.Second) // Not divisible by 2 seconds
	}()

	// Test zero window size
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("Expected panic for zero window size")
			}
		}()
		dw.Resize(0)
	}()
}

func TestDelayWindow_ResizeEmptyWindow(t *testing.T) {
	// Test resizing an empty window
	dw := NewDelayWindow(10*time.Second, 1*time.Second)

	// Resize empty window
	dw.Resize(5 * time.Second)

	// Should still return 0 for empty window
	if maxDelay := dw.CurrentMax(); maxDelay != 0 {
		t.Errorf("Expected max 0 for empty resized window, got %d", maxDelay)
	}

	// Add value and verify it works
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	dw.Record(baseTime, 42)

	if maxDelay := dw.CurrentMax(); maxDelay != 42 {
		t.Errorf("Expected max 42 after adding to resized window, got %d", maxDelay)
	}
}

func TestDelayWindow_ResizeMultipleTimes(t *testing.T) {
	// Test multiple consecutive resizes
	dw := NewDelayWindow(10*time.Second, 1*time.Second)

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Add values across the window
	for i := 0; i < 10; i++ {
		dw.Record(baseTime.Add(time.Duration(i)*time.Second), int32((i+1)*5))
	}

	// Initial max should be 50
	if maxDelay := dw.CurrentMax(); maxDelay != 50 {
		t.Errorf("Expected initial max 50, got %d", maxDelay)
	}

	// Resize smaller
	dw.Resize(6 * time.Second)
	if maxDelay := dw.CurrentMax(); maxDelay != 50 {
		t.Errorf("Expected max 50 after first resize, got %d", maxDelay)
	}

	// Resize even smaller
	dw.Resize(3 * time.Second)
	if maxDelay := dw.CurrentMax(); maxDelay != 50 {
		t.Errorf("Expected max 50 after second resize, got %d", maxDelay)
	}

	// Resize larger again
	dw.Resize(8 * time.Second)
	if maxDelay := dw.CurrentMax(); maxDelay != 50 {
		t.Errorf("Expected max 50 after expansion, got %d", maxDelay)
	}
}

func ExampleDelayWindow() {
	// Create a window that tracks values over the last 30 seconds
	// with 1-second granularity
	dw := NewDelayWindow(30*time.Second, 1*time.Second)

	// Simulate recording values at different times
	baseTime := time.Now()

	// Record some metric values
	dw.Record(baseTime, 100)
	dw.Record(baseTime.Add(5*time.Second), 150)
	dw.Record(baseTime.Add(10*time.Second), 120)
	dw.Record(baseTime.Add(15*time.Second), 180)

	// Get the maximum value in the window
	maxDelay := dw.CurrentMax()
	fmt.Printf("Maximum value in the last 30 seconds: %d\n", maxDelay)

	// Recording at the same second overwrites the previous value
	dw.Record(baseTime.Add(15*time.Second), 200) // Overwrites the 180
	maxDelay = dw.CurrentMax()
	fmt.Printf("Maximum value after overwrite: %d\n", maxDelay)

	// Output:
	// Maximum value in the last 30 seconds: 180
	// Maximum value after overwrite: 200
}

func ExampleDelayWindow_monitoring() {
	// Example: Using DelayWindow for monitoring request rates

	// Track maximum requests per second over the last minute
	// with 1-second granularity
	requestTracker := NewDelayWindow(60*time.Second, 1*time.Second)

	// Simulate incoming requests
	requestCounts := map[time.Time]int32{
		time.Now():                      45,
		time.Now().Add(1 * time.Second): 52,
		time.Now().Add(2 * time.Second): 38,
		time.Now().Add(3 * time.Second): 61,
		time.Now().Add(4 * time.Second): 55,
	}

	// Record request counts
	for timestamp, count := range requestCounts {
		requestTracker.Record(timestamp, count)
	}

	// Check peak request rate
	peakRate := requestTracker.CurrentMax()
	fmt.Printf("Peak request rate in the last minute: %d requests/second\n", peakRate)

	// This could be used for autoscaling decisions
	if peakRate > 50 {
		fmt.Println("High load detected - consider scaling up")
	}

	// Output:
	// Peak request rate in the last minute: 61 requests/second
	// High load detected - consider scaling up
}
