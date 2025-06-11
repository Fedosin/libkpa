# libkpa - Knative Pod Autoscaler Library

[![Go Reference](https://pkg.go.dev/badge/github.com/Fedosin/libkpa.svg)](https://pkg.go.dev/github.com/Fedosin/libkpa)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

`libkpa` is a standalone Go library extracted from Knative Serving's autoscaler (KPA - Knative Pod Autoscaler). It provides the core autoscaling algorithms and logic that can be integrated into any Kubernetes controller or operator that needs sophisticated pod autoscaling capabilities.

## Purpose

This library extracts the battle-tested autoscaling algorithms from Knative Serving, making them available for use in custom Kubernetes controllers without requiring the full Knative stack. It provides:

- **Sliding window metric aggregation** for stable scaling decisions
- **Panic mode** for handling traffic spikes
- **Configurable scale-up/down rates** to prevent flapping
- **Scale-to-zero capabilities** with grace periods
- **Support for multiple metrics** (concurrency and RPS)
- **Burst capacity calculations** for traffic absorption

## Installation

```bash
go get github.com/Fedosin/libkpa
```

## Quick Start

```go
package main

import (
    "context"
    "time"
    
    "github.com/Fedosin/libkpa/algorithm"
    "github.com/Fedosin/libkpa/api"
    "github.com/Fedosin/libkpa/config"
    "github.com/Fedosin/libkpa/metrics"
)

func main() {
    // Load configuration from environment or create custom config
    cfg, err := config.Load()
    if err != nil {
        panic(err)
    }
    
    // Create the autoscaler
    autoscaler := algorithm.NewSlidingWindowAutoscaler(cfg.AutoscalerSpec)
    
    // Create a metric snapshot (in real usage, collect from pods)
    snapshot := metrics.NewMetricSnapshot(
        150.0,  // stable value (e.g., total concurrent requests)
        200.0,  // panic value
        3,      // current ready pods
        time.Now(),
    )
    
    // Get scaling recommendation
    recommendation := autoscaler.Scale(context.Background(), snapshot, time.Now())
    
    if recommendation.ScaleValid {
        fmt.Printf("Desired pods: %d (current: %d)\n", 
            recommendation.DesiredPodCount, 
            recommendation.CurrentPodCount)
    }
}
```

## Package Overview

- **`api/`** - Core types, interfaces, and data structures for the autoscaler
- **`config/`** - Configuration loading and validation from environment variables or maps
- **`algorithm/`** - Autoscaling algorithm implementations (sliding window, panic mode)
- **`metrics/`** - Time-windowed metric collection and aggregation
- **`transmitter/`** - Metric reporting interfaces for monitoring integration
- **`delaywindow/`** - Delay window collection and aggregation

## Documentation

- [API Reference](docs/API.md) - Detailed API types and interfaces documentation
- [Configuration Guide](docs/CONFIGURATION.md) - All configuration options and environment variables
- [Algorithms Explained](docs/ALGORITHMS.md) - Deep dive into the autoscaling algorithms

## Features

### Sliding Window Algorithm
The core algorithm uses configurable time windows to aggregate metrics and make scaling decisions based on stable, averaged values rather than instantaneous spikes.

### Panic Mode
When load exceeds a configurable threshold, the autoscaler enters "panic mode" where it scales more aggressively and prevents scale-downs until the load stabilizes.

### Scale Bounds and Rates
Configure minimum/maximum pod counts and control how fast the autoscaler can scale up or down to prevent resource thrashing.

### Multiple Metrics Support
Scale based on either:
- **Concurrency**: Number of concurrent requests per pod
- **RPS**: Requests per second per pod

### Scale-to-Zero
Optionally scale deployments to zero pods when idle, with configurable grace periods before shutdown.

## Example Integration

See the [examples/](examples/) directory for a complete example of integrating libkpa into a Kubernetes controller.

## Performance and Profiling

The library includes comprehensive profiling support to help you understand the performance characteristics of the autoscaler:

- **Basic profiling** in the main example tracks execution time, memory usage, and component delays
- **Advanced profiling** with Go's built-in profiling tools (CPU, memory, execution trace)
- **Component-level timing** to identify bottlenecks in window operations, metric collection, etc.

Run the profiling example:
```bash
# Basic profiling with visual timing
go run examples/main.go

# CPU profiling
go run examples/profiling/main.go -cpuprofile=cpu.prof

# Memory profiling  
go run examples/profiling/main.go -memprofile=mem.prof

# Live profiling server
go run examples/profiling/main.go -pprof=:6060

# Custom duration and frequency
go run examples/profiling/main.go -duration=5m -interval=50ms
```

See [examples/profiling/README.md](examples/profiling/README.md) for detailed profiling documentation.

### Typical Performance

- **Scaling decisions**: < 200μs average (microseconds)
- **Memory usage**: Proportional to window size, typically < 5MB for standard configurations
- **CPU usage**: Minimal, mostly dependent on metric collection frequency

## Configuration

The library can be configured through environment variables (with `AUTOSCALER_` prefix) or programmatically. Key settings include:

- `AUTOSCALER_SCALING_METRIC`: Choose between "concurrency" or "rps"
- `AUTOSCALER_TARGET_VALUE`: Target metric value per pod
- `AUTOSCALER_STABLE_WINDOW`: Time window for metric averaging (default: 60s)
- `AUTOSCALER_PANIC_THRESHOLD_PERCENTAGE`: When to enter panic mode (default: 200%)

See [CONFIGURATION.md](docs/CONFIGURATION.md) for the complete list.

## Testing

Run the test suite:

```bash
go test ./...
```

Run with coverage:

```bash
go test -cover ./...
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Credits

This library is based on the autoscaler from the [Knative Serving](https://github.com/knative/serving) project. 