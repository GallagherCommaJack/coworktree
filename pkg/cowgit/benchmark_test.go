package cowgit

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

// BenchmarkConfig defines a test scenario
type BenchmarkConfig struct {
	Name         string
	FileCount    int
	FileSizeKB   int
	DirDepth     int
	FilesPerDir  int
	BinaryFiles  bool // Whether to include binary files
}

// Standard benchmark configurations - ordered from fastest to slowest for immediate feedback
var benchmarkConfigs = []BenchmarkConfig{
	// Ultra-quick tests first - empty files for instant setup
	{Name: "Instant_5files_0B", FileCount: 5, FileSizeKB: 0, DirDepth: 1, FilesPerDir: 5, BinaryFiles: false},
	{Name: "Instant_10files_0B", FileCount: 10, FileSizeKB: 0, DirDepth: 1, FilesPerDir: 10, BinaryFiles: false},
	{Name: "Instant_50files_0B", FileCount: 50, FileSizeKB: 0, DirDepth: 2, FilesPerDir: 25, BinaryFiles: false},
	
	// Quick tests with minimal content - these should complete in <1 second each
	{Name: "Tiny_5files_1KB", FileCount: 5, FileSizeKB: 1, DirDepth: 1, FilesPerDir: 5, BinaryFiles: false},
	{Name: "Small_10files_1KB", FileCount: 10, FileSizeKB: 1, DirDepth: 1, FilesPerDir: 10, BinaryFiles: false},
	{Name: "Quick_50files_1KB", FileCount: 50, FileSizeKB: 1, DirDepth: 2, FilesPerDir: 25, BinaryFiles: false},
	
	// Medium tests - should complete in a few seconds
	{Name: "Medium_100files_10KB", FileCount: 100, FileSizeKB: 10, DirDepth: 2, FilesPerDir: 50, BinaryFiles: false},
	{Name: "Medium_500files_5KB", FileCount: 500, FileSizeKB: 5, DirDepth: 3, FilesPerDir: 50, BinaryFiles: false},
	
	// Structure variation tests - moderate size
	{Name: "Shallow_200files_1KB", FileCount: 200, FileSizeKB: 1, DirDepth: 1, FilesPerDir: 200, BinaryFiles: false},
	{Name: "Deep_200files_1KB", FileCount: 200, FileSizeKB: 1, DirDepth: 8, FilesPerDir: 10, BinaryFiles: false},
	
	// Larger tests - may take 10+ seconds each
	{Name: "Large_1000files_100KB", FileCount: 1000, FileSizeKB: 100, DirDepth: 3, FilesPerDir: 100, BinaryFiles: false},
	{Name: "Binary_100files_1MB", FileCount: 100, FileSizeKB: 1024, DirDepth: 2, FilesPerDir: 50, BinaryFiles: true},
	
	// Stress tests - these are slow and optional
	{Name: "XLarge_5000files_100KB", FileCount: 5000, FileSizeKB: 100, DirDepth: 4, FilesPerDir: 100, BinaryFiles: true},
	{Name: "NodeModules_10000files_mix", FileCount: 10000, FileSizeKB: 10, DirDepth: 6, FilesPerDir: 200, BinaryFiles: true},
}

// Global cleanup registry for signal handling
var (
	cleanupMutex sync.Mutex
	cleanupDirs  []string
	signalSetup  sync.Once
)

func setupSignalHandler() {
	signalSetup.Do(func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-c
			cleanupMutex.Lock()
			for _, dir := range cleanupDirs {
				os.RemoveAll(dir)
			}
			cleanupMutex.Unlock()
			os.Exit(1)
		}()
	})
}

func registerCleanupDir(dir string) {
	cleanupMutex.Lock()
	cleanupDirs = append(cleanupDirs, dir)
	cleanupMutex.Unlock()
}

func unregisterCleanupDir(dir string) {
	cleanupMutex.Lock()
	for i, d := range cleanupDirs {
		if d == dir {
			cleanupDirs = append(cleanupDirs[:i], cleanupDirs[i+1:]...)
			break
		}
	}
	cleanupMutex.Unlock()
}

func BenchmarkCoWPerformance(b *testing.B) {
	// Skip if not on APFS
	if supported, err := IsCoWSupported("."); err != nil || !supported {
		b.Skip("CoW not supported on this filesystem")
	}

	for _, config := range benchmarkConfigs {
		b.Run(config.Name, func(b *testing.B) {
			benchmarkCoWClone(b, config)
		})
	}
}

func benchmarkCoWClone(b *testing.B, config BenchmarkConfig) {
	// Setup signal handler for cleanup
	setupSignalHandler()
	
	// Create test directory structure
	tempDir, err := ioutil.TempDir("", "cow_benchmark_")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	
	// Register for cleanup on interrupt
	registerCleanupDir(tempDir)
	defer func() {
		unregisterCleanupDir(tempDir)
		os.RemoveAll(tempDir)
	}()

	srcDir := filepath.Join(tempDir, "source")
	if err := createTestDirectory(srcDir, config); err != nil {
		b.Fatalf("Failed to create test directory: %v", err)
	}

	b.ResetTimer()
	
	// Measure CoW clone performance
	for i := 0; i < b.N; i++ {
		dstDir := filepath.Join(tempDir, fmt.Sprintf("clone_%d", i))
		
		start := time.Now()
		err := CloneDirectory(srcDir, dstDir)
		duration := time.Since(start)
		
		if err != nil {
			b.Fatalf("CoW clone failed: %v", err)
		}
		
		// Custom metric reporting
		b.ReportMetric(float64(duration.Nanoseconds()), "ns/op")
		b.ReportMetric(float64(config.FileCount), "files")
		b.ReportMetric(float64(config.FileSizeKB*config.FileCount), "total_kb")
		
		// Clean up for next iteration
		os.RemoveAll(dstDir)
	}
}

// Comparative benchmark: CoW vs traditional copy
func BenchmarkCoWVsTraditionalCopy(b *testing.B) {
	if supported, err := IsCoWSupported("."); err != nil || !supported {
		b.Skip("CoW not supported on this filesystem")
	}

	// Setup signal handler for cleanup
	setupSignalHandler()

	// Test with a smaller configuration - traditional copy is very slow
	config := BenchmarkConfig{
		Name: "Comparison_100files_10KB", 
		FileCount: 100, 
		FileSizeKB: 10, 
		DirDepth: 2, 
		FilesPerDir: 50, 
		BinaryFiles: false,
	}

	tempDir, err := ioutil.TempDir("", "cow_comparison_")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	
	// Register for cleanup on interrupt
	registerCleanupDir(tempDir)
	defer func() {
		unregisterCleanupDir(tempDir)
		os.RemoveAll(tempDir)
	}()

	srcDir := filepath.Join(tempDir, "source")
	if err := createTestDirectory(srcDir, config); err != nil {
		b.Fatalf("Failed to create test directory: %v", err)
	}

	b.Run("CoW_Clone", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			dstDir := filepath.Join(tempDir, fmt.Sprintf("cow_clone_%d", i))
			
			start := time.Now()
			err := CloneDirectory(srcDir, dstDir)
			duration := time.Since(start)
			
			if err != nil {
				b.Fatalf("CoW clone failed: %v", err)
			}
			
			b.ReportMetric(float64(duration.Nanoseconds()), "ns/op")
			os.RemoveAll(dstDir)
		}
	})

	b.Run("Traditional_Copy", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			dstDir := filepath.Join(tempDir, fmt.Sprintf("trad_copy_%d", i))
			
			start := time.Now()
			err := traditionalCopy(srcDir, dstDir)
			duration := time.Since(start)
			
			if err != nil {
				b.Fatalf("Traditional copy failed: %v", err)
			}
			
			b.ReportMetric(float64(duration.Nanoseconds()), "ns/op")
			os.RemoveAll(dstDir)
		}
	})
}

// Performance scaling benchmark
func BenchmarkCoWScaling(b *testing.B) {
	if supported, err := IsCoWSupported("."); err != nil || !supported {
		b.Skip("CoW not supported on this filesystem")
	}

	// Start with very small counts for immediate feedback, then scale up
	fileCounts := []int{5, 10, 25, 50, 100, 250, 500, 1000}
	
	for _, fileCount := range fileCounts {
		config := BenchmarkConfig{
			Name:        fmt.Sprintf("Scaling_%dfiles", fileCount),
			FileCount:   fileCount,
			FileSizeKB:  10,
			DirDepth:    3,
			FilesPerDir: 100,
			BinaryFiles: false,
		}
		
		b.Run(config.Name, func(b *testing.B) {
			benchmarkCoWClone(b, config)
		})
	}
}

// createTestDirectory creates a directory structure based on the given configuration
func createTestDirectory(rootDir string, config BenchmarkConfig) error {
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return err
	}

	filesCreated := 0
	rand.Seed(time.Now().UnixNano())

	// Calculate directory structure
	totalDirs := calculateDirCount(config.DirDepth, config.FilesPerDir, config.FileCount)
	dirsCreated := 0

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Create directory structure and files
	err = createDirStructure(rootDir, "", 0, config, &filesCreated, &dirsCreated, totalDirs)
	return err
}

func createDirStructure(rootDir, currentPath string, depth int, config BenchmarkConfig, filesCreated, dirsCreated *int, maxDirs int) error {
	if *filesCreated >= config.FileCount || depth > config.DirDepth {
		return nil
	}

	fullPath := filepath.Join(rootDir, currentPath)
	if err := os.MkdirAll(fullPath, 0755); err != nil {
		return err
	}

	// Create files in current directory
	filesInThisDir := min(config.FilesPerDir, config.FileCount-*filesCreated)
	for i := 0; i < filesInThisDir && *filesCreated < config.FileCount; i++ {
		fileName := fmt.Sprintf("file_%d", *filesCreated)
		if config.BinaryFiles && rand.Float32() < 0.3 {
			fileName += ".bin"
		} else {
			fileName += ".txt"
		}
		
		filePath := filepath.Join(fullPath, fileName)
		if err := createTestFile(filePath, config.FileSizeKB, config.BinaryFiles && filepath.Ext(fileName) == ".bin"); err != nil {
			return err
		}
		*filesCreated++
	}

	// Create subdirectories
	if depth < config.DirDepth && *filesCreated < config.FileCount && *dirsCreated < maxDirs {
		numSubdirs := min(3, (config.FileCount-*filesCreated)/config.FilesPerDir+1)
		for i := 0; i < numSubdirs && *filesCreated < config.FileCount; i++ {
			subDirName := fmt.Sprintf("dir_%d_%d", depth, i)
			subPath := filepath.Join(currentPath, subDirName)
			*dirsCreated++
			
			if err := createDirStructure(rootDir, subPath, depth+1, config, filesCreated, dirsCreated, maxDirs); err != nil {
				return err
			}
		}
	}

	return nil
}

func createTestFile(path string, sizeKB int, binary bool) error {
	// For 0-byte files, just create empty files (much faster)
	if sizeKB == 0 {
		file, err := os.Create(path)
		if err != nil {
			return err
		}
		return file.Close()
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	sizeBytes := sizeKB * 1024
	if binary {
		// Create binary content
		data := make([]byte, sizeBytes)
		rand.Read(data)
		_, err = file.Write(data)
	} else {
		// Create text content more efficiently
		text := "This is test content for benchmarking CoW performance. "
		textBytes := []byte(text)
		
		// Write efficiently in chunks
		written := 0
		for written < sizeBytes {
			remaining := sizeBytes - written
			if remaining < len(textBytes) {
				_, err = file.Write(textBytes[:remaining])
				written += remaining
			} else {
				_, err = file.Write(textBytes)
				written += len(textBytes)
			}
			if err != nil {
				break
			}
		}
	}

	return err
}

// Traditional copy implementation for comparison
func traditionalCopy(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return err
		}

		dstFile, err := os.Create(dstPath)
		if err != nil {
			return err
		}
		defer dstFile.Close()

		_, err = dstFile.ReadFrom(srcFile)
		if err != nil {
			return err
		}

		return os.Chmod(dstPath, info.Mode())
	})
}

// Utility functions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func calculateDirCount(depth, filesPerDir, totalFiles int) int {
	if depth <= 1 {
		return 1
	}
	// Rough estimation for directory count
	return (totalFiles / filesPerDir) + depth*2
}