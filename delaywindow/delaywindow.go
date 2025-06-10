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

// Package delaywindow provides a delay window collection and aggregation.
package delaywindow

import (
	"sync"
	"time"
)

// DelayWindow maintains a sliding window of values over time, organized into fixed-size time slots.
// It allows recording values at specific times and retrieving the maximum value within the window.
type DelayWindow struct {
	// windowSize is the total duration of the sliding window
	windowSize time.Duration
	// granularity is the duration of each time slot
	granularity time.Duration
	// numSlots is the total number of slots in the window (windowSize/granularity)
	numSlots int
	// slots is a circular buffer storing values for each time slot
	slots []int32
	// slotTimes stores the truncated timestamp for each slot to track validity
	slotTimes []time.Time
	// lastRecordedTime tracks the most recent time a value was recorded
	lastRecordedTime time.Time
	// mu protects concurrent access
	mu sync.RWMutex
}

// NewDelayWindow creates a new DelayWindow with the specified window size and granularity.
// The windowSize must be divisible by granularity, and both must be positive.
func NewDelayWindow(windowSize, granularity time.Duration) *DelayWindow {
	if windowSize <= 0 || granularity <= 0 {
		panic("windowSize and granularity must be positive")
	}
	if windowSize%granularity != 0 {
		panic("windowSize must be divisible by granularity")
	}

	numSlots := int(windowSize / granularity)
	return &DelayWindow{
		windowSize:  windowSize,
		granularity: granularity,
		numSlots:    numSlots,
		slots:       make([]int32, numSlots),
		slotTimes:   make([]time.Time, numSlots),
	}
}

// Record stores a value at the specified time.
// The time is truncated to the granularity, and the value is stored in the corresponding slot.
// If a value already exists for that slot, it is overwritten.
func (dw *DelayWindow) Record(now time.Time, value int32) {
	dw.mu.Lock()
	defer dw.mu.Unlock()

	// Truncate time to granularity
	truncatedTime := now.Truncate(dw.granularity)

	// Calculate slot index using modulo for circular buffer behavior
	slot := dw.getSlotIndex(truncatedTime)

	// Store the value and timestamp
	dw.slots[slot] = value
	dw.slotTimes[slot] = truncatedTime

	// Update last recorded time
	if truncatedTime.After(dw.lastRecordedTime) {
		dw.lastRecordedTime = truncatedTime
	}
}

// CurrentMax returns the maximum value within the current window.
// The window extends from (now - windowSize) to now, where now is the latest recorded time
// or the current time if no recordings exist.
// Returns 0 if no values exist in the window.
func (dw *DelayWindow) CurrentMax() int32 {
	dw.mu.RLock()
	defer dw.mu.RUnlock()

	// Determine the current time (latest recorded or time.Now())
	now := dw.lastRecordedTime
	if now.IsZero() {
		now = time.Now().Truncate(dw.granularity)
	}

	// Calculate the start of the window
	windowStart := now.Add(-dw.windowSize + dw.granularity)

	maxValue := int32(0)

	// Scan all slots to find values within the window
	for i := range dw.numSlots {
		// Check if this slot has a valid timestamp within the window
		if !dw.slotTimes[i].IsZero() &&
			!dw.slotTimes[i].Before(windowStart) &&
			!dw.slotTimes[i].After(now) {
			if dw.slots[i] > maxValue {
				maxValue = dw.slots[i]
			}
		}
	}

	return maxValue
}

// getSlotIndex calculates the circular buffer index for a given time.
// This ensures proper wrap-around behavior.
func (dw *DelayWindow) getSlotIndex(t time.Time) int {
	// Convert time to a slot number based on Unix time and granularity
	slotNumber := t.Unix() / int64(dw.granularity.Seconds())
	// Use modulo to wrap around in the circular buffer
	return int(slotNumber % int64(dw.numSlots))
}

// Resize adjusts the window size while maintaining the same granularity.
// Existing data within the new window is preserved, while data outside
// the new window is discarded. The granularity remains unchanged.
func (dw *DelayWindow) Resize(newWindowSize time.Duration) {
	dw.mu.Lock()
	defer dw.mu.Unlock()

	// Validate new window size
	if newWindowSize <= 0 {
		panic("newWindowSize must be positive")
	}
	if newWindowSize%dw.granularity != 0 {
		panic("newWindowSize must be divisible by granularity")
	}

	newNumSlots := int(newWindowSize / dw.granularity)

	// If the number of slots hasn't changed, just update windowSize
	if newNumSlots == dw.numSlots {
		dw.windowSize = newWindowSize
		return
	}

	// Create new slots and slotTimes arrays
	newSlots := make([]int32, newNumSlots)
	newSlotTimes := make([]time.Time, newNumSlots)

	// Determine the current time for window calculation
	now := dw.lastRecordedTime
	if now.IsZero() {
		now = time.Now().Truncate(dw.granularity)
	}

	// Calculate the start of the new window
	windowStart := now.Add(-newWindowSize + dw.granularity)

	// Copy existing valid data to new arrays
	for i := range dw.numSlots {
		// Check if this slot has valid data within the new window
		if !dw.slotTimes[i].IsZero() &&
			!dw.slotTimes[i].Before(windowStart) &&
			!dw.slotTimes[i].After(now) {
			// Calculate new slot index based on the timestamp
			slotNumber := dw.slotTimes[i].Unix() / int64(dw.granularity.Seconds())
			newSlot := int(slotNumber % int64(newNumSlots))
			newSlots[newSlot] = dw.slots[i]
			newSlotTimes[newSlot] = dw.slotTimes[i]
		}
	}

	// Update the structure with new values
	dw.windowSize = newWindowSize
	dw.numSlots = newNumSlots
	dw.slots = newSlots
	dw.slotTimes = newSlotTimes
}
