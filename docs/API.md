# API Reference

This document describes the primary API types and interfaces provided by the libkpa library.

## Core Types

### AutoscalerSpec

The `AutoscalerSpec` type defines the parameters for autoscaling behavior:

```go
type AutoscalerSpec struct {
    MaxScaleUpRate        float64       // Max rate to scale up (e.g., 2.0 = double pods)
    MaxScaleDownRate      float64       // Max rate to scale down (e.g., 2.0 = halve pods)
    ScalingMetric         ScalingMetric // Metric type: "concurrency" or "rps"
    TargetValue           float64       // Target metric value per pod
    TotalValue            float64       // Total capacity per pod
    TargetBurstCapacity   float64       // Burst capacity without queuing
    PanicThreshold        float64       // Threshold to enter panic mode (as ratio)
    PanicWindowPercentage float64       // Panic window as % of stable window
    StableWindow          time.Duration // Time window for stable metrics
    ScaleDownDelay        time.Duration // Delay before scaling down
    InitialScale          int32         // Initial pod count
    MinScale              int32         // Minimum pod count
    MaxScale              int32         // Maximum pod count (0 = unlimited)
    ActivationScale       int32         // Minimum scale when activating from zero
    Reachable             bool          // Whether service is reachable
}
```

### PodMetrics

Represents metrics collected from a single pod:

```go
type PodMetrics struct {
    PodName            string
    Timestamp          time.Time
    ConcurrentRequests float64
    RequestsPerSecond  float64
    ProcessUptime      time.Duration
}
```

### ScaleRecommendation

The autoscaler's scaling recommendation:

```go
type ScaleRecommendation struct {
    DesiredPodCount     int32   // Recommended number of pods
    ExcessBurstCapacity int32   // Excess capacity (negative = insufficient)
    ScaleValid          bool    // Whether recommendation is valid
    InPanicMode         bool    // Whether in panic mode
    ObservedStableValue float64 // Observed stable window value
    ObservedPanicValue  float64 // Observed panic window value
    CurrentPodCount     int32   // Current ready pod count
}
```

## Interfaces

### Autoscaler

The main interface for the autoscaler:

```go
type Autoscaler interface {
    // Calculate desired scale based on metrics
    Scale(ctx context.Context, metrics MetricSnapshot, now time.Time) ScaleRecommendation
    
    // Update autoscaler configuration
    Update(spec *AutoscalerSpec) error
    
    // Get current configuration
    GetSpec() AutoscalerSpec
}
```

### MetricSnapshot

Point-in-time view of metrics:

```go
type MetricSnapshot interface {
    StableValue() float64    // Metric averaged over stable window
    PanicValue() float64     // Metric averaged over panic window
    ReadyPodCount() int      // Number of ready pods
    Timestamp() time.Time    // When snapshot was taken
}
```

### MetricAggregator

For aggregating metrics over time windows:

```go
type MetricAggregator interface {
    Record(time time.Time, value float64)
    WindowAverage(now time.Time) float64
    IsEmpty(now time.Time) bool
}
```

## Example Usage

### Creating an Autoscaler

```go
// Define autoscaler configuration
spec := &api.AutoscalerSpec{
    MaxScaleUpRate:        10.0,
    MaxScaleDownRate:      2.0,
    ScalingMetric:         api.Concurrency,
    TargetValue:           100.0,
    TotalValue:            1000.0,
    TargetBurstCapacity:   200.0,
    PanicThreshold:        2.0,
    PanicWindowPercentage: 10.0,
    StableWindow:          60 * time.Second,
    ScaleDownDelay:        0,
    InitialScale:          1,
    MinScale:              0,
    MaxScale:              10,
    ActivationScale:       1,
    Reachable:             true,
}

// Create the autoscaler
autoscaler := algorithm.NewSlidingWindowAutoscaler(spec)
```

### Creating a Metric Snapshot

```go
// In a real implementation, these values would come from pod metrics
stableValue := 250.0  // e.g., total concurrent requests
panicValue := 300.0   // recent spike in requests
readyPods := 2

snapshot := metrics.NewMetricSnapshot(
    stableValue,
    panicValue,
    readyPods,
    time.Now(),
)
```

### Getting a Scale Recommendation

```go
ctx := context.Background()
recommendation := autoscaler.Scale(ctx, snapshot, time.Now())

if recommendation.ScaleValid {
    fmt.Printf("Current pods: %d\n", recommendation.CurrentPodCount)
    fmt.Printf("Desired pods: %d\n", recommendation.DesiredPodCount)
    fmt.Printf("In panic mode: %v\n", recommendation.InPanicMode)
    
    // Apply the recommendation to your deployment
    // deployment.Spec.Replicas = &recommendation.DesiredPodCount
}
```

### Updating Configuration

```go
// Update configuration at runtime
newSpec := autoscaler.GetSpec()
newSpec.TargetValue = 150.0
err := autoscaler.Update(&newSpec)
if err != nil {
    log.Printf("Failed to update autoscaler: %v", err)
}
```

## Integration with Kubernetes

To integrate libkpa with a Kubernetes controller:

1. **Implement PodCounter**: Create a type that can count ready pods
2. **Implement MetricCollector**: Collect metrics from your pods
3. **Create MetricSnapshots**: Aggregate metrics into snapshots
4. **Use Autoscaler**: Feed snapshots to get scaling recommendations
5. **Apply Recommendations**: Update deployment replicas

Example integration pattern:

```go
type Controller struct {
    autoscaler api.Autoscaler
    client     kubernetes.Interface
}

func (c *Controller) reconcile(deployment *appsv1.Deployment) error {
    // Collect metrics from pods
    metrics := c.collectPodMetrics(deployment)
    
    // Create snapshot
    snapshot := c.createSnapshot(metrics)
    
    // Get recommendation
    recommendation := c.autoscaler.Scale(ctx, snapshot, time.Now())
    
    if recommendation.ScaleValid {
        // Update deployment
        deployment.Spec.Replicas = &recommendation.DesiredPodCount
        _, err := c.client.AppsV1().Deployments(deployment.Namespace).
            Update(ctx, deployment, metav1.UpdateOptions{})
        return err
    }
    
    return nil
}
``` 