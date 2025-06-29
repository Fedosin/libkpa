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

// Package manager provides a Manager type that manages multiple Scaler instances.
// It allows registering and unregistering Scalers, setting scale bounds,
// changing aggregation algorithms, and recording metrics.
package manager

import (
	"fmt"
	"sync"
	"time"
)

// Manager manages multiple autoscalers and coordinates their scaling decisions.
type Manager struct {
	mu          sync.RWMutex
	minReplicas int32
	maxReplicas int32
	scalers     map[string]*Scaler
}

// NewManager creates a new Manager instance with the specified replica bounds.
// The initialScalers parameter allows registering scalers during construction.
func NewManager(
	minReplicas, maxReplicas int32,
	initialScalers ...*Scaler,
) *Manager {
	// Validate replica bounds
	if minReplicas < 0 {
		minReplicas = 0
	}
	if maxReplicas > 0 && maxReplicas < minReplicas {
		maxReplicas = minReplicas
	}

	m := &Manager{
		minReplicas: minReplicas,
		maxReplicas: maxReplicas,
		scalers:     make(map[string]*Scaler),
	}

	// Register initial scalers
	for _, s := range initialScalers {
		m.Register(s)
	}

	return m
}

// Register adds a scaler to the manager.
// If a scaler with the same name already exists, it will be replaced.
func (m *Manager) Register(s *Scaler) {
	if s == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.scalers[s.Name()] = s
}

// Unregister removes a scaler from the manager by name.
func (m *Manager) Unregister(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.scalers, name)
}

// GetMinScale returns the minimum replica count.
func (m *Manager) GetMinScale() int32 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.minReplicas
}

// GetMaxScale returns the maximum replica count.
func (m *Manager) GetMaxScale() int32 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.maxReplicas
}

// SetMinScale updates the minimum replica count.
func (m *Manager) SetMinScale(minValue int32) {
	if minValue < 0 {
		minValue = 0
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.minReplicas = minValue

	// Ensure max is still valid
	if m.maxReplicas > 0 && m.maxReplicas < m.minReplicas {
		m.maxReplicas = m.minReplicas
	}
}

// SetMaxScale updates the maximum replica count.
// A value of 0 means no upper limit.
func (m *Manager) SetMaxScale(maxValue int32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maxReplicas = maxValue

	// Ensure min is still valid
	if m.maxReplicas > 0 && m.minReplicas > m.maxReplicas {
		m.minReplicas = m.maxReplicas
	}
}

// ChangeAggregationAlgorithm changes the aggregation algorithm for a specific scaler.
func (m *Manager) ChangeAggregationAlgorithm(name, algoType string) error {
	m.mu.RLock()
	scaler, exists := m.scalers[name]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("scaler %q not found", name)
	}

	return scaler.ChangeAggregationAlgorithm(algoType)
}

// Record records a metric value for a specific scaler.
func (m *Manager) Record(name string, value float64, t time.Time) error {
	m.mu.RLock()
	scaler, exists := m.scalers[name]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("scaler %q not found", name)
	}

	scaler.Record(value, t)
	return nil
}

// Scale computes the desired replica count by taking the maximum of all scalers' recommendations.
// The context parameter is provided for future extensibility but is not currently used.
func (m *Manager) Scale(readyPods int32, now time.Time) int32 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.scalers) == 0 {
		// No scalers registered, return minimum replicas
		return m.minReplicas
	}

	// Start with the minimum possible value
	maxDesired := int32(0)
	validScalers := 0

	// Iterate through all scalers and get their recommendations
	for name, scaler := range m.scalers {
		recommendation := scaler.Scale(readyPods, now)

		// Only consider valid recommendations
		if recommendation.ScaleValid {
			validScalers++
			if recommendation.DesiredPodCount > maxDesired {
				maxDesired = recommendation.DesiredPodCount
			}
		} else {
			// Log or handle invalid recommendations if needed
			_ = name // avoid unused variable warning
		}
	}

	// If no valid scalers, return current scale
	if validScalers == 0 {
		return readyPods
	}

	// Apply min/max bounds
	if maxDesired < m.minReplicas {
		maxDesired = m.minReplicas
	}
	if m.maxReplicas > 0 && maxDesired > m.maxReplicas {
		maxDesired = m.maxReplicas
	}

	return maxDesired
}
