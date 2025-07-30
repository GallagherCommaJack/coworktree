package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type BenchmarkResult struct {
	Name         string        `json:"name"`
	Iterations   int           `json:"iterations"`
	NsPerOp      float64       `json:"ns_per_op"`
	Duration     time.Duration `json:"duration"`
	FileCount    int           `json:"file_count,omitempty"`
	TotalSizeKB  int           `json:"total_size_kb,omitempty"`
	SpeedupRatio float64       `json:"speedup_ratio,omitempty"`
}

type BenchmarkSuite struct {
	Results   []BenchmarkResult `json:"results"`
	Timestamp time.Time         `json:"timestamp"`
	Platform  string            `json:"platform"`
	GoVersion string            `json:"go_version"`
}

var benchmarkCmd = &cobra.Command{
	Use:   "benchmark",
	Short: "Run CoW performance benchmarks",
	Long: `Run comprehensive benchmarks to test CoW performance across different
folder configurations with varying file counts and sizes.`,
	Run: runBenchmark,
}

var (
	benchmarkOutput   string
	benchmarkVerbose  bool
	benchmarkCount    int
	benchmarkTime     string
	benchmarkPattern  string
	benchmarkCompare  bool
)

func init() {
	benchmarkCmd.Flags().StringVarP(&benchmarkOutput, "output", "o", "", "Output file for benchmark results (JSON)")
	benchmarkCmd.Flags().BoolVarP(&benchmarkVerbose, "verbose", "v", false, "Verbose output")
	benchmarkCmd.Flags().IntVarP(&benchmarkCount, "count", "c", 3, "Number of benchmark runs")
	benchmarkCmd.Flags().StringVarP(&benchmarkTime, "time", "t", "10s", "Maximum time per benchmark")
	benchmarkCmd.Flags().StringVarP(&benchmarkPattern, "pattern", "p", ".", "Benchmark pattern to run")
	benchmarkCmd.Flags().BoolVar(&benchmarkCompare, "compare", false, "Include CoW vs traditional copy comparison")
}

func runBenchmark(cmd *cobra.Command, args []string) {
	fmt.Println("🚀 Starting CoW Performance Benchmarks")
	fmt.Println("=====================================")

	// Check if we're in the right directory
	if _, err := os.Stat("pkg/cowgit"); err != nil {
		fmt.Printf("Error: Must run from project root directory\n")
		os.Exit(1)
	}

	suite := BenchmarkSuite{
		Timestamp: time.Now(),
		Platform:  getPlatformInfo(),
		GoVersion: getGoVersion(),
		Results:   make([]BenchmarkResult, 0),
	}

	// Run the main benchmark suite
	fmt.Println("\n📊 Running CoW Performance Tests...")
	results, err := runGoBenchmark("BenchmarkCoWPerformance", benchmarkCount, benchmarkTime)
	if err != nil {
		fmt.Printf("Error running CoW benchmarks: %v\n", err)
		os.Exit(1)
	}
	suite.Results = append(suite.Results, results...)

	// Run scaling benchmarks
	fmt.Println("\n📈 Running Scaling Tests...")
	scalingResults, err := runGoBenchmark("BenchmarkCoWScaling", benchmarkCount, benchmarkTime)
	if err != nil {
		fmt.Printf("Error running scaling benchmarks: %v\n", err)
	} else {
		suite.Results = append(suite.Results, scalingResults...)
	}

	// Run comparison benchmarks if requested (warning: slow!)
	if benchmarkCompare {
		fmt.Println("\n⚖️  Running CoW vs Traditional Copy Comparison...")
		fmt.Println("⚠️  Warning: Traditional copy is slow - this may take several minutes")
		comparisonResults, err := runGoBenchmark("BenchmarkCoWVsTraditionalCopy", benchmarkCount, benchmarkTime)
		if err != nil {
			fmt.Printf("Error running comparison benchmarks: %v\n", err)
		} else {
			suite.Results = append(suite.Results, comparisonResults...)
			calculateSpeedups(&suite)
		}
	}

	// Print summary
	printBenchmarkSummary(suite)

	// Save results if output file specified
	if benchmarkOutput != "" {
		if err := saveBenchmarkResults(suite, benchmarkOutput); err != nil {
			fmt.Printf("Error saving results: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\n💾 Results saved to: %s\n", benchmarkOutput)
	}
}

func runGoBenchmark(benchmarkName string, count int, timeLimit string) ([]BenchmarkResult, error) {
	args := []string{
		"test", 
		"./pkg/cowgit", 
		"-bench=" + benchmarkName,
		"-benchmem",
		"-count=" + strconv.Itoa(count),
		"-benchtime=" + timeLimit,
	}

	if benchmarkVerbose {
		args = append(args, "-v")
	}

	cmd := exec.Command("go", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("benchmark failed: %w\nOutput: %s", err, string(output))
	}

	return parseBenchmarkOutput(string(output))
}

func parseBenchmarkOutput(output string) ([]BenchmarkResult, error) {
	var results []BenchmarkResult
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		if strings.HasPrefix(line, "Benchmark") && strings.Contains(line, "ns/op") {
			result, err := parseBenchmarkLine(line)
			if err != nil {
				if benchmarkVerbose {
					fmt.Printf("Warning: Could not parse line: %s\n", line)
				}
				continue
			}
			results = append(results, result)
		}
	}

	return results, nil
}

func parseBenchmarkLine(line string) (BenchmarkResult, error) {
	// Example line: BenchmarkCoWPerformance/Small_10files_1KB-8         	     100	  10000000 ns/op	      10 files	      10 total_kb
	parts := strings.Fields(line)
	if len(parts) < 4 {
		return BenchmarkResult{}, fmt.Errorf("invalid benchmark line format")
	}

	// Extract benchmark name
	nameParts := strings.Split(parts[0], "/")
	var name string
	if len(nameParts) > 1 {
		name = strings.Split(nameParts[1], "-")[0]
	} else {
		name = strings.Split(parts[0], "-")[0]
	}

	// Extract iterations
	iterations, err := strconv.Atoi(parts[1])
	if err != nil {
		return BenchmarkResult{}, fmt.Errorf("invalid iterations: %w", err)
	}

	// Extract ns/op
	nsPerOpStr := strings.TrimSpace(parts[2])
	nsPerOp, err := strconv.ParseFloat(nsPerOpStr, 64)
	if err != nil {
		return BenchmarkResult{}, fmt.Errorf("invalid ns/op: %w", err)
	}

	result := BenchmarkResult{
		Name:       name,
		Iterations: iterations,
		NsPerOp:    nsPerOp,
		Duration:   time.Duration(nsPerOp) * time.Nanosecond,
	}

	// Extract additional metrics if present
	for i := 4; i < len(parts); i += 2 {
		if i+1 < len(parts) {
			value, err := strconv.ParseFloat(parts[i], 64)
			if err != nil {
				continue
			}
			
			metric := parts[i+1]
			switch metric {
			case "files":
				result.FileCount = int(value)
			case "total_kb":
				result.TotalSizeKB = int(value)
			}
		}
	}

	return result, nil
}

func calculateSpeedups(suite *BenchmarkSuite) {
	// Create a map of traditional copy results for comparison
	traditionalResults := make(map[string]float64)
	
	for _, result := range suite.Results {
		if strings.Contains(result.Name, "Traditional_Copy") {
			baseName := strings.Replace(result.Name, "Traditional_Copy", "", 1)
			traditionalResults[baseName] = result.NsPerOp
		}
	}

	// Calculate speedup ratios for CoW results
	for i := range suite.Results {
		if strings.Contains(suite.Results[i].Name, "CoW_Clone") {
			baseName := strings.Replace(suite.Results[i].Name, "CoW_Clone", "", 1)
			if traditionalTime, exists := traditionalResults[baseName]; exists {
				suite.Results[i].SpeedupRatio = traditionalTime / suite.Results[i].NsPerOp
			}
		}
	}
}

func printBenchmarkSummary(suite BenchmarkSuite) {
	fmt.Println("\n📋 Benchmark Summary")
	fmt.Println("===================")
	fmt.Printf("Platform: %s\n", suite.Platform)
	fmt.Printf("Go Version: %s\n", suite.GoVersion)
	fmt.Printf("Timestamp: %s\n\n", suite.Timestamp.Format(time.RFC3339))

	// Group results by type
	cowResults := make([]BenchmarkResult, 0)
	scalingResults := make([]BenchmarkResult, 0)
	comparisonResults := make([]BenchmarkResult, 0)

	for _, result := range suite.Results {
		switch {
		case strings.Contains(result.Name, "Scaling_"):
			scalingResults = append(scalingResults, result)
		case strings.Contains(result.Name, "CoW_Clone") || strings.Contains(result.Name, "Traditional_Copy"):
			comparisonResults = append(comparisonResults, result)
		default:
			cowResults = append(cowResults, result)
		}
	}

	// Print CoW performance results
	if len(cowResults) > 0 {
		fmt.Println("🔥 CoW Performance Results:")
		fmt.Printf("%-30s %12s %10s %12s %12s\n", "Test", "Duration", "Files", "Size(KB)", "MB/s")
		fmt.Println(strings.Repeat("-", 80))
		
		for _, result := range cowResults {
			duration := formatDuration(result.Duration)
			throughput := ""
			if result.TotalSizeKB > 0 && result.Duration > 0 {
				mbps := float64(result.TotalSizeKB) / 1024.0 / result.Duration.Seconds()
				throughput = fmt.Sprintf("%.1f", mbps)
			}
			
			fmt.Printf("%-30s %12s %10d %12d %12s\n", 
				result.Name, duration, result.FileCount, result.TotalSizeKB, throughput)
		}
		fmt.Println()
	}

	// Print scaling results
	if len(scalingResults) > 0 {
		fmt.Println("📈 Scaling Results:")
		fmt.Printf("%-20s %12s %10s %15s\n", "File Count", "Duration", "Files", "Files/sec")
		fmt.Println(strings.Repeat("-", 60))
		
		for _, result := range scalingResults {
			duration := formatDuration(result.Duration)
			filesPerSec := ""
			if result.FileCount > 0 && result.Duration > 0 {
				fps := float64(result.FileCount) / result.Duration.Seconds()
				filesPerSec = fmt.Sprintf("%.0f", fps)
			}
			
			fmt.Printf("%-20s %12s %10d %15s\n", 
				result.Name, duration, result.FileCount, filesPerSec)
		}
		fmt.Println()
	}

	// Print comparison results
	if len(comparisonResults) > 0 {
		fmt.Println("⚖️  CoW vs Traditional Copy:")
		fmt.Printf("%-25s %12s %10s\n", "Method", "Duration", "Speedup")
		fmt.Println(strings.Repeat("-", 50))
		
		for _, result := range comparisonResults {
			duration := formatDuration(result.Duration)
			speedup := ""
			if result.SpeedupRatio > 0 {
				speedup = fmt.Sprintf("%.1fx", result.SpeedupRatio)
			}
			
			fmt.Printf("%-25s %12s %10s\n", 
				result.Name, duration, speedup)
		}
		fmt.Println()
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Microsecond {
		return fmt.Sprintf("%dns", d.Nanoseconds())
	} else if d < time.Millisecond {
		return fmt.Sprintf("%.1fμs", float64(d.Nanoseconds())/1000.0)
	} else if d < time.Second {
		return fmt.Sprintf("%.1fms", float64(d.Nanoseconds())/1000000.0)
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

func saveBenchmarkResults(suite BenchmarkSuite, filename string) error {
	// Create output directory if needed
	dir := filepath.Dir(filename)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	data, err := json.MarshalIndent(suite, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}

func getPlatformInfo() string {
	cmd := exec.Command("uname", "-a")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

func getGoVersion() string {
	cmd := exec.Command("go", "version")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}