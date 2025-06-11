# Profiling libkpa Autoscaler

This directory contains examples that demonstrate how to profile the libkpa autoscaler to understand its performance characteristics.

## Quick Start

### Basic Profiling (examples/main.go)

The basic example includes simple profiling that tracks:
- Algorithm execution time
- Memory consumption
- Window operation delays
- Metric collection overhead

Run it with:
```bash
go run examples/main.go
```

### Advanced Profiling (examples/profiling/main.go)

The advanced profiling example provides detailed performance analysis with Go's built-in profiling tools.

## Features

### 1. Timing Analysis
- **Scaling Decision Time**: Measures how long the `Scale()` method takes
- **Component Breakdown**: Tracks time spent in different parts of the algorithm
  - Window recording operations
  - Window averaging calculations
  - Metric snapshot creation
- **Statistical Analysis**: Min/max/average timing information

### 2. Memory Profiling
- **Heap Usage**: Tracks heap allocations over time
- **Peak Memory**: Records maximum memory usage
- **GC Statistics**: Number of garbage collection runs and total pause time
- **Allocation Tracking**: Total memory allocated during the simulation

### 3. Delay Tracking
- **Scale-down Delays**: Tracks when scale-down decisions are delayed
- **Delay Duration**: Measures the total time spent in delay states

## Usage Examples

### Basic Run
```bash
# Run with default settings (2 minute simulation)
go run examples/profiling/main.go
```

### CPU Profiling
```bash
# Generate CPU profile
go run examples/profiling/main.go -cpuprofile=cpu.prof

# Analyze the profile
go tool pprof cpu.prof
```

### Memory Profiling
```bash
# Generate memory profile
go run examples/profiling/main.go -memprofile=mem.prof

# Analyze the profile
go tool pprof mem.prof
```

### Execution Tracing
```bash
# Generate execution trace
go run examples/profiling/main.go -trace=trace.out

# View the trace
go tool trace trace.out
```

### Live Profiling with pprof
```bash
# Start with pprof server
go run examples/profiling/main.go -pprof=:6060

# In another terminal, access pprof endpoints:
# - http://localhost:6060/debug/pprof/
# - go tool pprof http://localhost:6060/debug/pprof/heap
# - go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30
```

### Custom Duration and Interval
```bash
# Run for 5 minutes with 100ms intervals
go run examples/profiling/main.go -duration=5m -interval=100ms
```

## Sample Output

```
=== Detailed Profiling Summary ===

Scaling Performance:
  Total Decisions: 240
  Average Time: 125.3µs
  Min Time: 89.2µs
  Max Time: 421.7µs

Component Breakdown:
  Window Record: 18.2ms (6.1%)
  Window Average: 42.7ms (14.2%)
  Snapshot Creation: 8.3ms (2.8%)

Memory Usage:
  Initial Heap: 0.52 MB
  Current Heap: 1.23 MB
  Peak Heap: 2.14 MB
  Total Allocations: 156.78 MB
  GC Runs: 42
  GC Pause: 4.21ms

Scale-Down Delays:
  Total Delays: 15
  Total Delay Time: 7m30s
  Average Delay: 30s
```

## Performance Tips

1. **Scaling Decision Performance**
   - The autoscaler typically makes decisions in microseconds
   - Performance scales with the number of metrics and window size

2. **Memory Usage**
   - Memory usage is proportional to the window size and granularity
   - Each window bucket stores one float64 value

3. **Optimization Opportunities**
   - Use smaller windows for faster decisions
   - Increase granularity (bucket size) to reduce memory usage
   - Consider using the panic window percentage to balance responsiveness

## Interpreting Results

### Timing Metrics
- **Average < 1ms**: Excellent performance for real-time scaling
- **Max > 10ms**: May indicate GC pauses or system load
- **High variance**: Could suggest inconsistent system performance

### Memory Metrics
- **Stable heap growth**: Normal behavior with bounded windows
- **High GC frequency**: May need to tune window parameters
- **Peak usage**: Important for container memory limits

### Component Breakdown
- **Window operations > 50%**: Consider optimizing window size
- **Snapshot creation high**: May indicate metric processing overhead 