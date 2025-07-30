# CoW Benchmark Suite

This benchmark suite tests the performance of Copy-on-Write (CoW) operations across different folder configurations with varying numbers and sizes of files.

## Quick Start

```bash
# Run standard benchmarks
./benchmark.sh

# Run with CoW vs traditional copy comparison
./benchmark.sh --compare --verbose

# Quick benchmarks only
./benchmark.sh --quick
```

## Benchmark Scenarios

The suite tests these configurations:

### File Count Scaling
- **Small**: 10 files, 1KB each
- **Medium**: 100 files, 10KB each  
- **Large**: 1,000 files, 100KB each
- **XLarge**: 10,000 files, 1MB each
- **NodeModules**: 50,000 files, mixed sizes (simulates node_modules)

### Directory Structure Variations
- **Shallow**: Many files in few directories
- **Deep**: Few files in many nested directories
- **Binary**: Mix of text and binary files

### Performance Metrics Measured
- **Clone Duration**: Time to create CoW copy
- **Throughput**: MB/s transfer rate
- **Files/sec**: File processing rate
- **Memory Usage**: Peak memory consumption
- **Speedup Ratio**: CoW vs traditional copy

## Usage Examples

### Basic Benchmarking
```bash
# Standard benchmark run
./benchmark.sh

# Custom output directory
./benchmark.sh --output my_results

# More iterations for accuracy
./benchmark.sh --count 10 --time 30s
```

### Comparison Testing
```bash
# Compare CoW vs traditional copy
./benchmark.sh --compare

# Verbose output with comparison
./benchmark.sh --compare --verbose
```

### Using the CLI Command
```bash
# Build the project first
go build -o coworktree

# Run specific benchmarks
./coworktree benchmark --help
./coworktree benchmark --compare --output results.json
```

### Running Go Benchmarks Directly
```bash
# Run all CoW benchmarks
go test ./pkg/cowgit -bench=BenchmarkCoW -benchtime=10s -count=3

# Run scaling tests only
go test ./pkg/cowgit -bench=BenchmarkCoWScaling -v

# Run comparison tests
go test ./pkg/cowgit -bench=BenchmarkCoWVsTraditional -benchmem
```

## Output Files

The benchmark generates several output formats:

### JSON Results (`benchmark_TIMESTAMP.json`)
```json
{
  "results": [
    {
      "name": "Small_10files_1KB",
      "iterations": 100,
      "ns_per_op": 1000000,
      "file_count": 10,
      "total_size_kb": 10,
      "speedup_ratio": 15.2
    }
  ],
  "timestamp": "2024-01-01T12:00:00Z",
  "platform": "Darwin ...",
  "go_version": "go version go1.21.0 ..."
}
```

### HTML Report (`benchmark_TIMESTAMP.html`)
- Visual summary of results
- System information
- Easy-to-read tables and charts

### CSV Export (`benchmark_TIMESTAMP.csv`)
- Machine-readable format
- Easy import into spreadsheets
- Suitable for further analysis

## Interpreting Results

### Key Metrics to Watch

1. **Duration Trends**: How performance scales with file count/size
2. **Speedup Ratios**: CoW vs traditional copy performance gains
3. **Throughput**: MB/s sustained transfer rates
4. **Memory Efficiency**: Peak memory usage patterns

### Expected Performance Characteristics

On APFS (macOS):
- **Small files**: CoW should be 10-50x faster than traditional copy
- **Large files**: CoW advantage diminishes but still significant
- **Many files**: CoW excels with large file counts
- **Memory**: CoW uses minimal additional memory

### Troubleshooting

#### CoW Not Supported
```
Error: copy-on-write not supported on this filesystem
```
- Ensure you're on APFS (macOS) or supported Linux filesystem
- Check with: `diskutil info .` (macOS)

#### Performance Issues
- Ensure SSD storage for best results
- Close other applications during benchmarking
- Run multiple iterations (`--count 5`) for statistical significance

#### Build Failures
```bash
# Clean and rebuild
go clean -cache
go mod tidy
go build -o coworktree
```

## Customizing Benchmarks

### Adding New Scenarios

Edit `pkg/cowgit/benchmark_test.go`:

```go
var customConfigs = []BenchmarkConfig{
    {
        Name: "Custom_5000files_500KB",
        FileCount: 5000,
        FileSizeKB: 500,
        DirDepth: 5,
        FilesPerDir: 200,
        BinaryFiles: true,
    },
}
```

### Modifying Test Parameters

```go
// In benchmark_test.go
func BenchmarkCustomScenario(b *testing.B) {
    config := BenchmarkConfig{
        // Your custom configuration
    }
    benchmarkCoWClone(b, config)
}
```

## Performance Baselines

Typical results on M1 MacBook Pro with APFS:

| Scenario | Traditional Copy | CoW Clone | Speedup |
|----------|------------------|-----------|---------|
| 100 files, 10KB | 50ms | 2ms | 25x |
| 1,000 files, 100KB | 500ms | 15ms | 33x |
| 10,000 files, 1MB | 5s | 150ms | 33x |

Your results may vary based on:
- Hardware (SSD speed, CPU)
- Filesystem (APFS version)
- System load
- File content (text vs binary)

## Contributing

To add new benchmark scenarios:

1. Add configuration to `benchmarkConfigs` in `benchmark_test.go`
2. Test with `go test ./pkg/cowgit -bench=YourBenchmark -v`
3. Update this documentation
4. Submit PR with benchmark results

## Troubleshooting

### Common Issues

**"not in a git repository"**
- Run from project root directory
- Ensure `.git` directory exists

**"clonefile not supported"**
- Not on APFS filesystem
- Filesystem doesn't support CoW
- Will fallback to traditional copy

**Benchmark timeouts**
- Reduce `--time` parameter
- Reduce file counts in large scenarios
- Check available disk space