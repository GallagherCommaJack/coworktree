#!/bin/bash

# Quick test script to verify CoW benchmarks work
set -e

echo "🚀 Quick CoW Test"
echo "================="

# Build first
echo "Building..."
go build -o coworktree

echo "Running single quick benchmark..."
# Run just one ultra-quick test
go test ./pkg/cowgit -bench=BenchmarkCoWPerformance/Instant_5files_0B -count=1 -benchtime=1s -v

echo "✅ Quick test completed!"