# Configuration Guide

This document describes all configuration options available in libkpa. Configuration can be provided through environment variables (with `AUTOSCALER_` prefix) or programmatically via configuration maps.

## Environment Variables

All environment variables use the `AUTOSCALER_` prefix.

### Core Autoscaling Parameters

| Environment Variable | Type | Default | Description | Valid Range |
|---------------------|------|---------|-------------|-------------|
| `AUTOSCALER_TARGET_VALUE` | float | `100.0` | Target metric value per pod (mutually exclusive with TOTAL_TARGET_VALUE) | >= 0 |
| `AUTOSCALER_TOTAL_TARGET_VALUE` | float | `0.0` | Total target metric value across all pods (mutually exclusive with TARGET_VALUE) | >= 0 |
| `AUTOSCALER_MAX_SCALE_UP_RATE` | float | `1000.0` | Maximum rate to scale up pods | > 1.0 |
| `AUTOSCALER_MAX_SCALE_DOWN_RATE` | float | `2.0` | Maximum rate to scale down pods | > 1.0 |

**Note**: Either `TARGET_VALUE` or `TOTAL_TARGET_VALUE` must be set, but not both.

### Time Windows

| Environment Variable | Type | Default | Description | Valid Range |
|---------------------|------|---------|-------------|-------------|
| `AUTOSCALER_STABLE_WINDOW` | duration | `60s` | Time window for stable metric averaging | 5s - 600s |
| `AUTOSCALER_SCALE_DOWN_DELAY` | duration | `0s` | Delay before applying scale-down decisions | >= 0s |
| `AUTOSCALER_SCALE_TO_ZERO_GRACE_PERIOD` | duration | `30s` | Grace period before scaling to zero | > 0s |

### Burst Mode Configuration

| Environment Variable | Type | Default | Description | Valid Range |
|---------------------|------|---------|-------------|-------------|
| `AUTOSCALER_BURST_THRESHOLD_PERCENTAGE` | float | `200.0` | Percentage threshold to enter burst mode | > 100.0 |
| `AUTOSCALER_BURST_WINDOW_PERCENTAGE` | float | `10.0` | Burst window as percentage of stable window | 1.0 - 100.0 |

### Scale Bounds

| Environment Variable | Type | Default | Description | Valid Range |
|---------------------|------|---------|-------------|-------------|
| `AUTOSCALER_MIN_SCALE` | int | `0` | Minimum number of pods | >= 0 |
| `AUTOSCALER_MAX_SCALE` | int | `0` | Maximum number of pods (0 = unlimited) | >= 0 |
| `AUTOSCALER_ACTIVATION_SCALE` | int | `1` | Minimum pods when scaling from zero | >= 1 |


## Configuration Map Format

When using `config.LoadFromMap()`, use the following keys:

```go
configMap := map[string]string{
    "target-value":                              "100",   // Per-pod target (mutually exclusive with total-target-value)
    "total-target-value":                        "0",     // Total target across all pods (mutually exclusive with target-value)
    "max-scale-up-rate":                         "10.0",
    "max-scale-down-rate":                       "2.0",
    "stable-window":                             "60s",
    "scale-down-delay":                          "0s",
    "scale-to-zero-grace-period":                "30s",
    "burst-threshold-percentage":                "200",
    "burst-window-percentage":                   "10",
    "min-scale":                                 "0",
    "max-scale":                                 "10",
    "activation-scale":                          "1",
}

config, err := config.LoadFromMap(configMap)
```

## Configuration Examples

### High-Traffic Service

For services with high, variable traffic:

```bash
export AUTOSCALER_TARGET_VALUE=1000
export AUTOSCALER_MAX_SCALE_UP_RATE=5.0
export AUTOSCALER_STABLE_WINDOW=120s
export AUTOSCALER_MIN_SCALE=5
export AUTOSCALER_MAX_SCALE=100
```

### Batch Processing Service

For services that process batch jobs:

```bash
export AUTOSCALER_TARGET_VALUE=1
export AUTOSCALER_STABLE_WINDOW=300s
export AUTOSCALER_SCALE_DOWN_DELAY=60s
export AUTOSCALER_SCALE_TO_ZERO_GRACE_PERIOD=300s
```

### Latency-Sensitive Service

For services where response time is critical:

```bash
export AUTOSCALER_TARGET_VALUE=50
export AUTOSCALER_BURST_THRESHOLD_PERCENTAGE=150
export AUTOSCALER_BURST_WINDOW_PERCENTAGE=5
export AUTOSCALER_MIN_SCALE=2
```

## Validation Rules

The configuration validation enforces these rules:

1. **Scale bounds**: `min-scale` <= `max-scale` (when max-scale > 0)
2. **Time windows**: Must be specified with second precision (no sub-second values)
3. **Percentages**: 
   - `burst-window-percentage`: [1, 100]
4. **Scale rates**: Must be > 1.0
5. **Target values**: Must be >= 0.01
6. **Stable window**: Must be between 5s and 600s

## Best Practices

1. **Start Conservative**: Begin with default values and adjust based on observed behavior
2. **Monitor Metrics**: Use the transmitter interface to export metrics for monitoring
3. **Test Scale Down**: Always test scale-down behavior in staging before production
4. **Burst Mode**: Set burst thresholds based on your service's ability to handle spikes
5. **Window Sizes**: Larger windows provide more stability but slower response to changes 