#!/bin/bash

# CoW Benchmark Runner Script
# This script runs comprehensive benchmarks and generates reports

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values
OUTPUT_DIR="benchmark_results"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
OUTPUT_FILE="${OUTPUT_DIR}/benchmark_${TIMESTAMP}.json"
VERBOSE=false
COMPARE=false
COUNT=3
TIME="10s"
QUICK_ONLY=false

usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  -o, --output DIR     Output directory for results (default: benchmark_results)"
    echo "  -v, --verbose        Enable verbose output"
    echo "  -c, --compare        Include CoW vs traditional copy comparison"
    echo "  -n, --count N        Number of benchmark runs (default: 3)"
    echo "  -t, --time TIME      Maximum time per benchmark (default: 10s)"
    echo "  -q, --quick          Run only the fastest benchmarks (5-50 files)"
    echo "  -h, --help           Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0                           # Run standard benchmarks"
    echo "  $0 -c -v                     # Run with comparison and verbose output"
    echo "  $0 -o results -n 5           # Custom output dir, 5 runs per benchmark"
    echo "  $0 -q                        # Quick benchmarks only"
}

log() {
    echo -e "${BLUE}[$(date '+%H:%M:%S')]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -o|--output)
            OUTPUT_DIR="$2"
            OUTPUT_FILE="${OUTPUT_DIR}/benchmark_${TIMESTAMP}.json"
            shift 2
            ;;
        -v|--verbose)
            VERBOSE=true
            shift
            ;;
        -c|--compare)
            COMPARE=true
            shift
            ;;
        -n|--count)
            COUNT="$2"
            shift 2
            ;;
        -t|--time)
            TIME="$2"
            shift 2
            ;;
        -q|--quick)
            QUICK_ONLY=true
            TIME="5s"
            COUNT=1
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            error "Unknown option: $1"
            usage
            exit 1
            ;;
    esac
done

# Check prerequisites
check_prerequisites() {
    log "Checking prerequisites..."
    
    # Check if we're in the right directory
    if [[ ! -f "go.mod" ]] || [[ ! -d "pkg/cowgit" ]]; then
        error "Must be run from the coworktree project root directory"
        exit 1
    fi
    
    # Check Go installation
    if ! command -v go &> /dev/null; then
        error "Go is not installed or not in PATH"
        exit 1
    fi
    
    # Check filesystem support
    if [[ "$OSTYPE" == "darwin"* ]]; then
        log "Detected macOS - checking APFS support..."
        if ! diskutil info . | grep -q "APFS"; then
            warning "Current directory is not on APFS filesystem"
            warning "CoW benchmarks may fail or fall back to traditional copy"
        fi
    else
        warning "Non-macOS system detected - CoW may not be supported"
    fi
    
    success "Prerequisites check completed"
}

# Build the project
build_project() {
    log "Building coworktree..."
    if go build -o coworktree; then
        success "Build completed successfully"
    else
        error "Build failed"
        exit 1
    fi
}

# Run unit tests to ensure everything works
run_tests() {
    log "Running unit tests..."
    if go test ./pkg/cowgit -v; then
        success "All tests passed"
    else
        error "Tests failed"
        exit 1
    fi
}

# Run benchmarks using the built-in benchmark command
run_benchmarks() {
    log "Starting benchmark suite..."
    
    # Prepare output directory
    mkdir -p "$OUTPUT_DIR"
    
    # Build benchmark arguments
    BENCH_ARGS=(-o "$OUTPUT_FILE" -c "$COUNT" -t "$TIME")
    
    if [[ "$VERBOSE" == "true" ]]; then
        BENCH_ARGS+=(-v)
    fi
    
    if [[ "$COMPARE" == "true" ]]; then
        BENCH_ARGS+=(--compare)
    fi
    
    # Add pattern filter for quick-only mode
    if [[ "$QUICK_ONLY" == "true" ]]; then
        log "Running quick benchmarks only (Instant, Tiny, Small, Quick patterns)..."
        BENCH_ARGS+=(-p "Instant|Tiny|Small|Quick")
    fi
    
    # Run the benchmark
    log "Running: ./coworktree benchmark ${BENCH_ARGS[*]}"
    if ./coworktree benchmark "${BENCH_ARGS[@]}"; then
        success "Benchmarks completed successfully"
    else
        error "Benchmarks failed"
        exit 1
    fi
}

# Generate additional reports
generate_reports() {
    log "Generating additional reports..."
    
    # Create a simple HTML report
    HTML_REPORT="${OUTPUT_DIR}/benchmark_${TIMESTAMP}.html"
    
    cat > "$HTML_REPORT" << EOF
<!DOCTYPE html>
<html>
<head>
    <title>CoW Benchmark Results - ${TIMESTAMP}</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .header { background: #f0f0f0; padding: 20px; border-radius: 5px; }
        .result { margin: 20px 0; }
        .metrics { background: #f9f9f9; padding: 15px; border-radius: 3px; }
        table { border-collapse: collapse; width: 100%; }
        th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
        th { background-color: #f2f2f2; }
        .success { color: #28a745; }
        .warning { color: #ffc107; }
        .error { color: #dc3545; }
    </style>
</head>
<body>
    <div class="header">
        <h1>CoW Benchmark Results</h1>
        <p><strong>Timestamp:</strong> $(date)</p>
        <p><strong>Platform:</strong> $(uname -a)</p>
        <p><strong>Go Version:</strong> $(go version)</p>
    </div>
    
    <div class="result">
        <h2>Benchmark Configuration</h2>
        <ul>
            <li><strong>Count:</strong> $COUNT runs per benchmark</li>
            <li><strong>Time Limit:</strong> $TIME per benchmark</li>
            <li><strong>Comparison:</strong> $(if [[ "$COMPARE" == "true" ]]; then echo "Enabled"; else echo "Disabled"; fi)</li>
            <li><strong>Verbose:</strong> $(if [[ "$VERBOSE" == "true" ]]; then echo "Enabled"; else echo "Disabled"; fi)</li>
        </ul>
    </div>
    
    <div class="result">
        <h2>Results</h2>
        <p>Detailed results are available in the JSON file: <code>$(basename "$OUTPUT_FILE")</code></p>
        <p>Run the following command to view detailed results:</p>
        <pre>./coworktree benchmark --help</pre>
    </div>
    
    <div class="result">
        <h2>Quick Analysis</h2>
        <p>To analyze these results:</p>
        <ol>
            <li>Check the JSON file for detailed metrics</li>
            <li>Compare CoW vs traditional copy performance</li>
            <li>Look for scaling patterns across different file counts</li>
            <li>Monitor memory usage and throughput</li>
        </ol>
    </div>
</body>
</html>
EOF
    
    success "HTML report generated: $HTML_REPORT"
    
    # Create a CSV summary for easy analysis
    CSV_REPORT="${OUTPUT_DIR}/benchmark_${TIMESTAMP}.csv"
    echo "Name,Duration(ns),FileCount,TotalSizeKB,Throughput(MB/s)" > "$CSV_REPORT"
    
    if [[ -f "$OUTPUT_FILE" ]]; then
        # Extract basic metrics from JSON (requires jq if available)
        if command -v jq &> /dev/null; then
            jq -r '.results[] | [.name, .ns_per_op, .file_count, .total_size_kb, (if .total_size_kb > 0 and .ns_per_op > 0 then (.total_size_kb / 1024.0) / (.ns_per_op / 1000000000.0) else "" end)] | @csv' "$OUTPUT_FILE" >> "$CSV_REPORT" 2>/dev/null || true
            success "CSV report generated: $CSV_REPORT"
        else
            warning "jq not found - CSV report will be basic"
        fi
    fi
}

# Main execution
main() {
    echo -e "${GREEN}"
    echo "======================================="
    echo "     CoW Benchmark Runner v1.0"
    echo "======================================="
    echo -e "${NC}"
    
    check_prerequisites
    build_project
    run_tests
    run_benchmarks
    generate_reports
    
    echo ""
    success "Benchmark suite completed successfully!"
    echo ""
    log "Results saved to:"
    echo "  📊 JSON:  $OUTPUT_FILE"
    echo "  📈 HTML:  ${OUTPUT_DIR}/benchmark_${TIMESTAMP}.html"
    echo "  📋 CSV:   ${OUTPUT_DIR}/benchmark_${TIMESTAMP}.csv"
    echo ""
    log "To run benchmarks again:"
    echo "  ./benchmark.sh --compare --verbose"
    echo ""
}

# Cleanup function
cleanup() {
    echo -e "\n${YELLOW}Benchmark interrupted - cleaning up...${NC}"
    
    # Kill any running go test processes
    pkill -f "go test.*benchmark" 2>/dev/null || true
    
    # Clean up any leftover temp directories
    rm -rf /tmp/cow_benchmark_* /tmp/cow_comparison_* 2>/dev/null || true
    
    # Remove any partial output files
    if [[ -n "$OUTPUT_FILE" ]] && [[ -f "$OUTPUT_FILE.tmp" ]]; then
        rm -f "$OUTPUT_FILE.tmp" 2>/dev/null || true
    fi
    
    echo -e "${RED}Benchmark suite interrupted and cleaned up${NC}"
    exit 1
}

# Trap to clean up on exit
trap cleanup INT TERM

# Run main function
main "$@"