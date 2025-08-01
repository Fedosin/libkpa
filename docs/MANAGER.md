# Scaling Manager

The manager package provides a high-level abstraction for managing multiple autoscalers in libkpa. It allows you to run multiple scaling algorithms simultaneously (e.g., CPU and memory-based scaling) and coordinates their decisions to ensure adequate capacity.

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Usage Examples](#usage-examples)
4. [API Reference](#api-reference)
5. [Aggregation Algorithms](#aggregation-algorithms)
6. [Best Practices](#best-practices)
7. [Advanced Topics](#advanced-topics)

## Overview

The manager package consists of two main components:

- **Scaler**: Combines metric aggregation with the sliding window autoscaling algorithm
- **Manager**: Coordinates multiple scalers and enforces global replica bounds

### Key Features

- **Multiple Metrics**: Support for scaling based on multiple metrics simultaneously
- **Dynamic Configuration**: Change aggregation algorithms and bounds at runtime
- **Thread-Safe**: Safe for concurrent access from multiple goroutines
- **Flexible Aggregation**: Choose between linear and weighted time window algorithms

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                      Manager                            │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐   │
│  │   Scaler 1   │  │   Scaler 2   │  │   Scaler N   │   │
│  │     (CPU)    │  │   (Memory)   │  │    (...)     │   │
│  └──────────────┘  └──────────────┘  └──────────────┘   │
│         │                  │                  │         │
│         ├──────────────────┴──────────────────┘         │
│         │                                               │
│         ▼                                               │
│   Max(recommendations) → Apply bounds → Final result    │
└─────────────────────────────────────────────────────────┘

Each Scaler contains:
┌────────────────────────────────┐
│            Scaler              │
│  ┌──────────────────────────┐  │
│  │  SlidingWindowAutoscaler │  │
│  └──────────────────────────┘  │
│  ┌──────────────────────────┐  │
│  │   Stable Aggregator      │  │
│  │  (TimeWindow/Weighted)   │  │
│  └──────────────────────────┘  │
│  ┌──────────────────────────┐  │
│  │    Burst Aggregator      │  │
│  │  (TimeWindow/Weighted)   │  │
│  └──────────────────────────┘  │
└────────────────────────────────┘
```

## Usage Examples

### Basic Setup

```go
package main

import (
    "context"
    "time"
    
    "github.com/Fedosin/libkpa/api"
    "github.com/Fedosin/libkpa/manager"
)

func main() {
    // Configure autoscaler
    config := api.AutoscalerConfig{
        StableWindow:          60 * time.Second,
        BurstWindowPercentage: 10.0,
        TargetValue:           100.0,      // Target per pod
        BurstThreshold:        2.0,        // 200% threshold
        MaxScaleUpRate:        1000.0,     // Unlimited scale up
        MaxScaleDownRate:      2.0,        // Max halve pods
    }
    
    // Create scalers for different metrics
    cpuScaler, _ := manager.NewScaler("cpu", config, "linear")
    memoryScaler, _ := manager.NewScaler("memory", config, "weighted")
    
    // Create manager with bounds
    mgr := manager.NewManager(
        2,   // minReplicas
        100, // maxReplicas
        cpuScaler,
        memoryScaler,
    )
    
    // Record metrics and scale
    mgr.Record("cpu", 250.0, time.Now())
    mgr.Record("memory", 180.0, time.Now())
    
    replicas, _ := mgr.Scale(context.Background(), time.Now())
    fmt.Printf("Desired replicas: %d\n", replicas)
}
```

### Dynamic Scaler Management

```go
// Register a new scaler at runtime
networkScaler, _ := manager.NewScaler("network", config, "linear")
mgr.Register(networkScaler)

// Unregister a scaler
mgr.Unregister("cpu")

// Change aggregation algorithm
err := mgr.ChangeAggregationAlgorithm("memory", "linear")
```

### Adjusting Bounds

```go
// Adjust replica bounds based on time of day
if isBusinessHours() {
    mgr.SetMinScale(5)   // Higher minimum during business hours
    mgr.SetMaxScale(200) // Higher maximum
} else {
    mgr.SetMinScale(1)   // Allow scale to near-zero
    mgr.SetMaxScale(50)  // Lower maximum
}
```

### Continuous Scaling Loop

```go
func runAutoscaler(mgr *manager.Manager) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        ctx := context.Background()
        now := time.Now()
        
        // Collect and record metrics
        cpuUsage := collectCPUMetric()
        memUsage := collectMemoryMetric()
        
        mgr.Record("cpu", cpuUsage, now)
        mgr.Record("memory", memUsage, now)
        
        // Calculate desired scale
        desiredReplicas, err := mgr.Scale(ctx, now)
        if err != nil {
            log.Printf("Scaling error: %v", err)
            continue
        }
        
        // Apply the scale (integrate with Kubernetes)
        applyScale(desiredReplicas)
    }
}
```

## API Reference

### Scaler

```go
// NewScaler creates a new scaler instance
func NewScaler(
    name string,
    cfg api.AutoscalerConfig,
    algoType string, // "linear" or "weighted"
) (*Scaler, error)

// Methods
func (s *Scaler) Name() string
func (s *Scaler) Record(value float64, t time.Time)
func (s *Scaler) Scale(readyPods int32, now time.Time) api.ScaleRecommendation
func (s *Scaler) Config() api.AutoscalerConfig
func (s *Scaler) Update(config api.AutoscalerConfig) error
func (s *Scaler) ChangeAggregationAlgorithm(algoType string) error
```

### Manager

```go
// NewManager creates a new manager instance
func NewManager(
    minReplicas, maxReplicas int32,
    initialScalers ...*Scaler,
) *Manager

// Methods
func (m *Manager) Register(s *Scaler)
func (m *Manager) Unregister(name string)
func (m *Manager) GetMinScale() int32
func (m *Manager) GetMaxScale() int32
func (m *Manager) SetMinScale(min int32)
func (m *Manager) SetMaxScale(max int32)
func (m *Manager) ChangeAggregationAlgorithm(name, algoType string) error
func (m *Manager) Record(name string, value float64, t time.Time) error
func (m *Manager) Scale(ctx context.Context, now time.Time) (int32, error)
```

## Aggregation Algorithms

### Linear (TimeWindow)

The linear algorithm computes a simple average over the time window:

```
Average = Sum(values) / Count(values)
```

**Use when:**
- Metrics have consistent importance over time
- You want predictable, stable scaling behavior
- Historical values are as important as recent ones

### Weighted (WeightedTimeWindow)

The weighted algorithm gives more importance to recent values:

```
WeightedAverage = Σ(value[i] * weight[i]) / Σ(weight[i])
where weight decreases exponentially for older values
```

**Use when:**
- Recent metrics are more indicative of current load
- You need faster response to sudden changes
- Traffic patterns are bursty or unpredictable

### Choosing an Algorithm

| Metric Type | Recommended Algorithm | Reason |
|-------------|----------------------|---------|
| CPU Usage | Linear | CPU tends to be stable |
| Memory Usage | Linear | Memory changes gradually |
| Request Rate | Weighted | Can spike suddenly |
| Queue Depth | Weighted | Recent values more important |
| Response Time | Weighted | Indicates current stress |

## Best Practices

### 1. Metric Selection

Choose complementary metrics that capture different aspects of load:

```go
// Good: Different resource dimensions
cpuScaler := manager.NewScaler("cpu", config, "linear")
memoryScaler := manager.NewScaler("memory", config, "linear")
requestScaler := manager.NewScaler("requests", config, "weighted")

// Avoid: Redundant metrics
requestsPerSecond := manager.NewScaler("rps", config, "weighted")
requestsPerMinute := manager.NewScaler("rpm", config, "weighted") // Redundant
```

### 2. Target Value Configuration

Set target values based on actual capacity testing:

```go
config := api.AutoscalerConfig{
    // CPU: 80 mCPU utilization target
    TargetValue: 80.0,
    
    // For request-based scaling
    // TargetValue: 1000.0,  // 1000 requests per second per pod
}
```

### 3. Window Sizing

- **Stable Window**: 60-300 seconds for most workloads
- **Burst Window**: 5-10% of stable window
- Shorter windows = faster response but more noise
- Longer windows = stability but slower response

### 4. Bounds Management

```go
// Development environment
devManager := manager.NewManager(
    1,   // Allow scale to 1 for cost savings
    10,  // Limited max scale
)

// Production environment
prodManager := manager.NewManager(
    3,    // Minimum 3 for high availability
    500,  // High max for traffic spikes
)
```

### 5. Metric Recording

Record metrics at consistent intervals:

```go
// Good: Regular intervals
ticker := time.NewTicker(1 * time.Second)
for range ticker.C {
    mgr.Record("cpu", getCPU(), time.Now())
}

// Avoid: Irregular recording
// This can cause window calculation issues
go func() {
    mgr.Record("cpu", getCPU(), time.Now())
    time.Sleep(randomDuration()) // Don't do this
}()
```

## Advanced Topics

### Custom Metrics

You can scale on any metric by recording custom values:

```go
// Scale based on queue depth
queueDepth := float64(messageQueue.Length())
mgr.Record("queue", queueDepth, time.Now())

// Scale based on custom business metrics
activeUsers := float64(getActiveUserCount())
mgr.Record("users", activeUsers, time.Now())

// Scale based on error rate
errorRate := float64(errors) / float64(requests) * 100
mgr.Record("errors", errorRate, time.Now())
```

### Coordinating Multiple Managers

For complex scenarios, you might use multiple managers:

```go
// Frontend tier
frontendMgr := manager.NewManager(2, 50)
frontendCPU, _ := manager.NewScaler("cpu", feConfig, "linear")
frontendReqs, _ := manager.NewScaler("requests", feConfig, "weighted")
frontendMgr.Register(frontendCPU)
frontendMgr.Register(frontendReqs)

// Backend tier
backendMgr := manager.NewManager(3, 100)
backendCPU, _ := manager.NewScaler("cpu", beConfig, "linear")
backendQueue, _ := manager.NewScaler("queue", beConfig, "weighted")
backendMgr.Register(backendCPU)
backendMgr.Register(backendQueue)

// Scale both tiers
fScale, _ := frontendMgr.Scale(ctx, now)
bScale, _ := backendMgr.Scale(ctx, now)
```

### Integration with Kubernetes

Example integration with Kubernetes HPA:

```go
func updateHPA(mgr *manager.Manager, hpaClient kubernetes.Interface) {
    replicas, err := mgr.Scale(context.Background(), time.Now())
    if err != nil {
        log.Printf("Scale calculation failed: %v", err)
        return
    }
    
    // Update HPA or Deployment
    deployment, _ := hpaClient.AppsV1().Deployments("default").Get(
        context.Background(), 
        "my-app", 
        metav1.GetOptions{},
    )
    
    deployment.Spec.Replicas = &replicas
    hpaClient.AppsV1().Deployments("default").Update(
        context.Background(),
        deployment,
        metav1.UpdateOptions{},
    )
}
```

### Monitoring and Observability

Add metrics to monitor the autoscaler itself:

```go
func instrumentedScale(mgr *manager.Manager) {
    start := time.Now()
    replicas, err := mgr.Scale(context.Background(), time.Now())
    
    // Record metrics
    scaleLatency.Observe(time.Since(start).Seconds())
    if err != nil {
        scaleErrors.Inc()
    } else {
        desiredReplicas.Set(float64(replicas))
    }
    
    // Log scaling decisions
    log.Printf("Scaling decision: desired=%d min=%d max=%d",
        replicas, mgr.GetMinScale(), mgr.GetMaxScale())
}
```

## Troubleshooting

### Common Issues

1. **Scale oscillation**: Increase stable window or add scale-down delay
2. **Slow response**: Decrease stable window or use weighted algorithm  
3. **Over-scaling**: Check target values match actual capacity
4. **Under-scaling**: Ensure metrics are recorded frequently enough

### Debug Logging

```go
// Add debug logging to understand scaling decisions
type debugManager struct {
    *manager.Manager
}

func (d *debugManager) Scale(ctx context.Context, now time.Time) (int32, error) {
    replicas, err := d.Manager.Scale(ctx, now)
    log.Printf("[DEBUG] Scale decision: %d replicas (err: %v)", replicas, err)
    return replicas, err
}
```
