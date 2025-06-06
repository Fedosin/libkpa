# Autoscaling Algorithms

This document explains the autoscaling algorithms implemented in libkpa, derived from Knative's Pod Autoscaler (KPA).

## Table of Contents

1. [Sliding Window Algorithm](#sliding-window-algorithm)
2. [Panic Mode](#panic-mode)
3. [Scale Rate Limiting](#scale-rate-limiting)
4. [Scale-Down Delay](#scale-down-delay)
5. [Excess Burst Capacity](#excess-burst-capacity)
6. [Mathematical Formulas](#mathematical-formulas)

## Sliding Window Algorithm

The sliding window algorithm is the core of libkpa's autoscaling logic. It aggregates metrics over configurable time windows to make stable scaling decisions.

### How It Works

1. **Metric Collection**: Metrics (concurrency or RPS) are collected from all pods
2. **Time Bucketing**: Metrics are stored in time-based buckets (typically 1-second granularity)
3. **Window Aggregation**: Two windows are maintained:
   - **Stable Window**: Long-term average (default 60s)
   - **Panic Window**: Short-term average (default 6s, 10% of stable)
4. **Averaging**: Values are averaged over the window to smooth out spikes

### Implementation Details

```go
// Pseudocode for window average calculation
func WindowAverage(metrics []float64, windowSize time.Duration) float64 {
    validBuckets := getValidBuckets(metrics, windowSize)
    sum := 0.0
    for _, value := range validBuckets {
        sum += value
    }
    return sum / len(validBuckets)
}
```

### Example Scenario

Given:
- Target concurrency: 100 per pod
- Current pods: 3
- Stable window metrics: [280, 290, 300, 310, 320] (average: 300)

Calculation:
```
Desired pods = ceil(300 / 100) = 3 pods (no change needed)
```

## Panic Mode

Panic mode provides rapid scale-up when the system is under extreme load, preventing request failures.

### Triggering Conditions

Panic mode is entered when:
```
(Panic Window Average / Current Capacity) >= Panic Threshold
```

Default panic threshold is 2.0 (200%), meaning panic triggers when desired pods are double the current count.

### Behavior in Panic Mode

1. **No Scale Down**: Pod count never decreases
2. **Aggressive Scale Up**: Uses panic window metrics for faster response
3. **High Water Mark**: Maintains the highest pod count reached during panic

### Exit Conditions

Panic mode exits when:
1. Load drops below panic threshold AND
2. A full stable window has passed since load dropped

### Example

```
Time 0s: Current=2 pods, Panic metric=500, Threshold=200%
         Desired = 500/100 = 5 pods
         5/2 = 250% > 200% → ENTER PANIC MODE
         Scale to 5 pods

Time 30s: Current=5 pods, Panic metric=300
          Desired = 300/100 = 3 pods
          But in panic mode → maintain 5 pods

Time 90s: Panic metric=150, below threshold for 60s
          → EXIT PANIC MODE
          Can now scale down to 2 pods
```

## Scale Rate Limiting

Scale rate limiting prevents rapid fluctuations in pod count that could destabilize the system.

### Scale Up Rate

Maximum scale up is calculated as:
```
MaxScaleUp = ceil(CurrentPods * MaxScaleUpRate)
```

With default `MaxScaleUpRate=1000`, scale-up is effectively unlimited. Setting it to 2.0 would allow doubling at most.

### Scale Down Rate

Maximum scale down is calculated as:
```
MaxScaleDown = floor(CurrentPods / MaxScaleDownRate)
```

With default `MaxScaleDownRate=2.0`, pods can be halved at most in one step.

### Example

```
Current pods: 10
MaxScaleUpRate: 1.5
MaxScaleDownRate: 2.0

Calculated desired: 20 pods
MaxScaleUp = ceil(10 * 1.5) = 15 pods
→ Scale to 15 pods (not 20)

Next iteration, current: 15 pods
Calculated desired: 5 pods  
MaxScaleDown = floor(15 / 2.0) = 7 pods
→ Scale to 7 pods (not 5)
```

## Scale-Down Delay

Scale-down delay prevents premature scale-down during temporary load reductions.

### How It Works

1. **Delay Window**: Maintains a time window of desired pod counts
2. **Maximum Selection**: Always uses the maximum value in the window
3. **Gradual Decrease**: Only scales down after sustained low load

### Example

With 30s scale-down delay:
```
Time  0s: Load spike → desired=10 pods
Time 10s: Load drops → desired=3 pods (but keep 10)
Time 20s: Load still low → desired=3 pods (but keep 10)
Time 35s: Load still low → desired=3 pods (now scale to 3)
```

## Excess Burst Capacity

Excess burst capacity determines whether the Knative Activator should buffer requests.

### Calculation

```
Total Capacity = Ready Pods * Total Value per Pod
Excess = Total Capacity - Target Burst Capacity - Current Load
```

### Interpretation

- **Positive excess**: Pods can handle burst without activator
- **Negative excess**: Activator needed to buffer requests
- **Zero/disabled**: No burst protection needed

### Example

```
Ready pods: 5
Total capacity per pod: 1000
Target burst capacity: 2000
Current load (panic): 1500

Total capacity = 5 * 1000 = 5000
Excess = 5000 - 2000 - 1500 = 1500

Result: Positive excess, no activator needed
```

## Mathematical Formulas

### Basic Scaling Formula
```
DesiredPods = ⌈ObservedMetric / TargetValuePerPod⌉
```

### Panic Mode Detection
```
PanicRatio = DesiredPodsPanic / CurrentPods
InPanicMode = PanicRatio >= PanicThreshold
```

### Rate Limited Scaling
```
ScaleUpLimit = ⌈CurrentPods × MaxScaleUpRate⌉
ScaleDownLimit = ⌊CurrentPods / MaxScaleDownRate⌋
FinalDesired = max(ScaleDownLimit, min(DesiredPods, ScaleUpLimit))
```

### Window Percentile (Panic Window)
```
PanicWindowSize = StableWindowSize × (PanicWindowPercentage / 100)
```

### Activation Scale Application
```
if DesiredPods > 0 AND DesiredPods < ActivationScale:
    DesiredPods = ActivationScale
```

## Algorithm Flow

Here's the complete algorithm flow:

```
1. Collect metrics from all pods
2. Calculate stable and panic window averages
3. Determine desired pods: ceil(metric / target)
4. Check panic mode:
   - If should enter → enter panic mode
   - If in panic → use max(stable, panic) desired
   - If should exit → exit panic mode
5. Apply scale rate limits
6. Apply scale-down delay (if configured)
7. Apply min/max scale bounds
8. Calculate excess burst capacity
9. Return recommendation
```

## Tuning Guidelines

### For Stable Workloads
- Increase stable window (120s-300s)
- Enable scale-down delay (30s-60s)
- Lower panic threshold (150%)

### For Spiky Workloads
- Decrease stable window (30s-60s)
- Reduce panic window percentage (5%)
- Higher scale-up rate

### For Cost Optimization
- Enable scale-to-zero
- Increase scale-down rate
- Longer grace periods 