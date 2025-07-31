# Autoscaling Algorithms

This document explains the autoscaling algorithms implemented in libkpa, derived from Knative's Pod Autoscaler (KPA).

## Table of Contents

1. [Sliding Window Algorithm](#sliding-window-algorithm)
2. [Burst Mode](#burst-mode)
3. [Scale Rate Limiting](#scale-rate-limiting)
4. [Scale-Down Delay](#scale-down-delay)
5. [Mathematical Formulas](#mathematical-formulas)

## Sliding Window Algorithm

The sliding window algorithm is the core of libkpa's autoscaling logic. It aggregates metrics over configurable time windows to make stable scaling decisions.

### How It Works

1. **Metric Collection**: Metrics are collected from all pods.
2. **Time Bucketing**: Metrics are stored in time-based buckets (typically 1-second granularity)
3. **Window Aggregation**: Two windows are maintained:
   - **Stable Window**: Long-term average (default 60s)
   - **Burst Window**: Short-term average (default 6s, 10% of stable)
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

### Example Scenarios

**Per-Pod Target Example:**
Given:
- Target concurrency: 100 per pod (`TargetValue = 100`)
- Current pods: 3
- Stable window metrics: [280, 290, 300, 310, 320] (average: 300)

Calculation:
```
Desired pods = ceil(300 / 100) = 3 pods (no change needed)
```

**Total Target Example:**
Given:
- Total target concurrency: 1000 (`TotalTargetValue = 1000`)
- Current pods: 3
- Stable window metrics: [2800, 2900, 3000, 3100, 3200] (average: 3000)

Calculation:
```
Desired pods = ceil(3000 / 1000) = 3 pods (no change needed)
```

## Burst Mode

Burst mode provides rapid scale-up when the system is under extreme load, preventing request failures.

### Triggering Conditions

Burst mode is entered when:
```
(Burst Window Average / Current Capacity) >= Burst Threshold
```

Default burst threshold is 2.0 (200%), meaning burst triggers when desired pods are double the current count.

### Behavior in Burst Mode

1. **No Scale Down**: Pod count never decreases
2. **Aggressive Scale Up**: Uses burst window metrics for faster response
3. **High Water Mark**: Maintains the highest pod count reached during burst

### Exit Conditions

Burst mode exits when:
1. Load drops below burst threshold AND
2. A full stable window has passed since load dropped

### Example

```
Time 0s: Current=2 pods, Burst metric=500, Threshold=200%
         Desired = 500/100 = 5 pods
         5/2 = 250% > 200% → ENTER BURST MODE
         Scale to 5 pods

Time 30s: Current=5 pods, Burst metric=300
          Desired = 300/100 = 3 pods
          But in burst mode → maintain 5 pods

Time 90s: Burst metric=150, below threshold for 60s
          → EXIT BURST MODE
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

## Mathematical Formulas

### Basic Scaling Formula

There are two modes for calculating desired pods:

**Per-Pod Target Mode** (when `TargetValue` is set):
```
DesiredPods = ⌈ObservedMetric / TargetValuePerPod⌉
```

**Total Target Mode** (when `TotalTargetValue` is set):
```
DesiredPods = ⌈CurrentNumberOfPods * ObservedMetric / TotalTargetValue⌉
```

The per-pod mode scales based on maintaining a target value per pod (e.g., 100 concurrent requests per pod).
The total target mode scales based on maintaining a total value across all pods (e.g., 1000 total elements in the queue).

Only one of these modes can be active at a time.

### Burst Mode Detection
```
BurstRatio = DesiredPodsBurst / CurrentPods
InBurstMode = BurstRatio >= BurstThreshold
```

### Rate Limited Scaling
```
ScaleUpLimit = ⌈CurrentPods × MaxScaleUpRate⌉
ScaleDownLimit = ⌊CurrentPods / MaxScaleDownRate⌋
FinalDesired = max(ScaleDownLimit, min(DesiredPods, ScaleUpLimit))
```

### Window Percentile (Burst Window)
```
BurstWindowSize = StableWindowSize × (BurstWindowPercentage / 100)
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
2. Calculate stable and burst window averages
3. Determine desired pods: ceil(metric / target)
4. Check burst mode:
   - If should enter → enter burst mode
   - If in burst → use max(stable, burst) desired
   - If should exit → exit burst mode
5. Apply scale rate limits
6. Apply scale-down delay (if configured)
7. Apply min/max scale bounds
8. Return recommendation
```

## Tuning Guidelines

### For Stable Workloads
- Increase stable window (120s-300s)
- Enable scale-down delay (30s-60s)
- Lower burst threshold (150%)

### For Spiky Workloads
- Decrease stable window (30s-60s)
- Reduce burst window percentage (5%)
- Higher scale-up rate

### For Cost Optimization
- Enable scale-to-zero
- Increase scale-down rate
- Longer grace periods 