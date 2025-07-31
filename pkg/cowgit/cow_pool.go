package cowgit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

// CoWPool manages parallel file-level copy-on-write operations
type CoWPool struct {
	fileChan      chan CoWTask
	errChan       chan error
	workerCount   int32
	activeWorkers map[int]chan struct{}
	nextWorkerID  int32
	
	mu        sync.RWMutex
	wg        sync.WaitGroup
	
	// Metrics
	processedFiles int64
	copiedFiles    int64
	skippedFiles   int64
	directories    int64
	regularFiles   int64
	symlinks       int64
	startTime      time.Time
}

// CoWTask represents a file copy task
type CoWTask struct {
	SrcPath string
	DstPath string
	Info    os.FileInfo
}

// CoWStats contains statistics about the CoW pool operation
type CoWStats struct {
	Workers        int32
	ProcessedFiles int64
	CopiedFiles    int64
	SkippedFiles   int64
	Directories    int64
	RegularFiles   int64
	Symlinks       int64
	QueueDepth     int
	ElapsedTime    time.Duration
}

// CoWPoolController manages CoW pool scaling
type CoWPoolController struct {
	pool       *CoWPool
	done       chan struct{}
	lastAdjust time.Time
}

// NewCoWPool creates a new CoW worker pool
func NewCoWPool() *CoWPool {
	return &CoWPool{
		fileChan:      make(chan CoWTask, 1000),
		errChan:       make(chan error, 1),
		activeWorkers: make(map[int]chan struct{}),
		startTime:     time.Now(),
	}
}

// NewCoWPoolController creates a controller for the CoW pool
func NewCoWPoolController(pool *CoWPool) *CoWPoolController {
	return &CoWPoolController{
		pool: pool,
		done: make(chan struct{}),
	}
}

// Start initializes the CoW pool with CPU count workers
func (p *CoWPool) Start() {
	initialWorkers := runtime.NumCPU()
	for i := 0; i < initialWorkers; i++ {
		p.AddWorker()
	}
}

// Stop shuts down the CoW pool
func (p *CoWPool) Stop() {
	p.wg.Wait()
	close(p.errChan)
}

// Submit adds a file copy task to be processed
func (p *CoWPool) Submit(task CoWTask) {
	p.fileChan <- task
}

// Error returns the error channel
func (p *CoWPool) Error() <-chan error {
	return p.errChan
}

// AddWorker adds a new worker to the pool
func (p *CoWPool) AddWorker() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	workerID := int(atomic.AddInt32(&p.nextWorkerID, 1))
	stopChan := make(chan struct{})
	p.activeWorkers[workerID] = stopChan
	atomic.AddInt32(&p.workerCount, 1)
	
	p.wg.Add(1)
	go p.worker(workerID, stopChan)
}

// RemoveWorker removes a worker from the pool
func (p *CoWPool) RemoveWorker() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	minWorkers := max(runtime.NumCPU()/2, 1)
	if len(p.activeWorkers) <= minWorkers {
		return false
	}
	
	// Stop one worker
	for id, stopChan := range p.activeWorkers {
		close(stopChan)
		delete(p.activeWorkers, id)
		atomic.AddInt32(&p.workerCount, -1)
		return true
	}
	
	return false
}

// GetStats returns current pool statistics
func (p *CoWPool) GetStats() CoWStats {
	return CoWStats{
		Workers:        atomic.LoadInt32(&p.workerCount),
		ProcessedFiles: atomic.LoadInt64(&p.processedFiles),
		CopiedFiles:    atomic.LoadInt64(&p.copiedFiles),
		SkippedFiles:   atomic.LoadInt64(&p.skippedFiles),
		Directories:    atomic.LoadInt64(&p.directories),
		RegularFiles:   atomic.LoadInt64(&p.regularFiles),
		Symlinks:       atomic.LoadInt64(&p.symlinks),
		QueueDepth:     len(p.fileChan),
		ElapsedTime:    time.Since(p.startTime),
	}
}

// worker processes CoW tasks from the queue
func (p *CoWPool) worker(_ int, stop <-chan struct{}) {
	defer p.wg.Done()
	
	for {
		select {
		case task, ok := <-p.fileChan:
			if !ok {
				return // Channel closed
			}
			
			if err := p.processCoWTask(task); err != nil {
				select {
				case p.errChan <- err:
				default:
				}
				return
			}
			
			atomic.AddInt64(&p.processedFiles, 1)
			
		case <-stop:
			return // Worker stopped
		}
	}
}

// processCoWTask handles the actual file copy-on-write operation
func (p *CoWPool) processCoWTask(task CoWTask) error {
	// For directories, create them with proper permissions
	if task.Info.IsDir() {
		if err := os.MkdirAll(task.DstPath, task.Info.Mode()); err != nil {
			return err
		}
		atomic.AddInt64(&p.copiedFiles, 1)
		atomic.AddInt64(&p.directories, 1)
		return nil
	}
	
	// Ensure parent directory exists (this handles race conditions)
	dstDir := filepath.Dir(task.DstPath)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}
	
	// For regular files, try clonefile first
	if task.Info.Mode().IsRegular() {
		atomic.AddInt64(&p.regularFiles, 1)
		if err := unix.Clonefile(task.SrcPath, task.DstPath, unix.CLONE_NOFOLLOW); err != nil {
			// Fall back to regular copy if clonefile fails
			atomic.AddInt64(&p.skippedFiles, 1)
			return p.regularCopy(task.SrcPath, task.DstPath, task.Info)
		}
		atomic.AddInt64(&p.copiedFiles, 1)
		return nil
	}
	
	// For symlinks, copy the link
	if task.Info.Mode()&os.ModeSymlink != 0 {
		atomic.AddInt64(&p.symlinks, 1)
		target, err := os.Readlink(task.SrcPath)
		if err != nil {
			return err
		}
		atomic.AddInt64(&p.copiedFiles, 1)
		return os.Symlink(target, task.DstPath)
	}
	
	// For other special files, skip
	atomic.AddInt64(&p.skippedFiles, 1)
	return nil
}

// regularCopy performs a regular file copy as fallback
func (p *CoWPool) regularCopy(src, dst string, info os.FileInfo) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	
	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()
	
	// Use Go's efficient copy
	_, err = srcFile.WriteTo(dstFile)
	return err
}

// Start begins monitoring and adjusting the CoW pool
func (c *CoWPoolController) Start() {
	go c.controlLoop()
}

// Stop shuts down the controller
func (c *CoWPoolController) Stop() {
	close(c.done)
}

// controlLoop is the main control logic that adjusts worker count
func (c *CoWPoolController) controlLoop() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	
	var lastProcessed int64
	var lastCheck time.Time = time.Now()
	
	for {
		select {
		case <-ticker.C:
			stats := c.pool.GetStats()
			
			// Calculate processing rate
			now := time.Now()
			timeDelta := now.Sub(lastCheck).Seconds()
			if timeDelta > 0 {
				processingRate := float64(stats.ProcessedFiles-lastProcessed) / timeDelta
				lastProcessed = stats.ProcessedFiles
				lastCheck = now
				
				c.adjustWorkers(stats, processingRate)
			}
			
		case <-c.done:
			return
		}
	}
}

// adjustWorkers implements the scaling logic for CoW operations
func (c *CoWPoolController) adjustWorkers(stats CoWStats, processingRate float64) {
	// Rate limit adjustments
	if time.Since(c.lastAdjust) < time.Second {
		return
	}
	
	maxWorkers := int32(runtime.NumCPU() * 2) // Less aggressive than path rewriting
	
	// Queue backing up and processing? Add workers
	if stats.QueueDepth > int(stats.Workers)*10 && stats.Workers < maxWorkers && processingRate > 0 {
		c.pool.AddWorker()
		c.lastAdjust = time.Now()
		return
	}
	
	// Queue empty and many workers? Remove workers
	if stats.QueueDepth == 0 && processingRate < 5 {
		if c.pool.RemoveWorker() {
			c.lastAdjust = time.Now()
		}
		return
	}
}

// CloneDirectoryParallel creates a CoW clone using parallel file operations  
// It tries atomic cloning first, then falls back to file-by-file parallel cloning
func CloneDirectoryParallel(src, dst string, progress *ProgressTracker) error {
	// Check if we're on APFS
	if isAPFS, err := isAPFS(src); err != nil {
		return fmt.Errorf("failed to check filesystem: %w", err)
	} else if !isAPFS {
		return errors.New("copy-on-write requires APFS filesystem")
	}
	
	// Try atomic directory clone first - this is usually much faster
	if progress != nil {
		progress.UpdateStage("Trying atomic directory clone")
	}
	
	if err := unix.Clonefile(src, dst, unix.CLONE_NOFOLLOW); err == nil {
		// Atomic clone succeeded - we're done!
		if progress != nil {
			progress.UpdateStage("Atomic clone successful")
		}
		return nil
	} else if progress != nil {
		progress.UpdateStage("Atomic clone failed, using parallel approach")
	}
	
	// Atomic clone failed - fall back to parallel file-by-file approach
	return cloneDirectoryParallelFallback(src, dst, progress)
}

// cloneDirectoryParallelFallback handles the file-by-file parallel cloning
func cloneDirectoryParallelFallback(src, dst string, progress *ProgressTracker) error {
	// Create destination directory
	if err := os.MkdirAll(dst, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}
	
	// Create CoW pool
	pool := NewCoWPool()
	controller := NewCoWPoolController(pool)
	
	// Start pool and controller
	pool.Start()
	controller.Start()
	
	// Clean up when done
	defer func() {
		controller.Stop()
		pool.Stop()
	}()
	
	// Track progress with periodic updates
	done := make(chan bool)
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				if progress != nil {
					stats := pool.GetStats()
					throughput := float64(stats.ProcessedFiles) / stats.ElapsedTime.Seconds()
					
					info := fmt.Sprintf("%d items processed (%d dirs, %d files), %d cloned, %.0f items/sec", 
						stats.ProcessedFiles, stats.Directories, stats.RegularFiles, stats.CopiedFiles, throughput)
					progress.UpdateStage(info)
				}
			case <-done:
				return
			}
		}
	}()
	
	// Walk source tree and submit all tasks (files AND directories)
	walkErr := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// Calculate destination path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)
		
		// Skip the root directory itself
		if path == src {
			return nil
		}
		
		// Submit ALL paths for parallel processing (dirs and files)
		pool.Submit(CoWTask{
			SrcPath: path,
			DstPath: dstPath,
			Info:    info,
		})
		
		return nil
	})
	
	// Signal completion and wait for workers
	close(pool.fileChan)
	done <- true
	
	// Get final statistics
	if progress != nil {
		finalStats := pool.GetStats()
		
		if finalStats.CopiedFiles > 0 {
			info := fmt.Sprintf("%d of %d items cloned (%d dirs, %d files, %d symlinks, %d skipped)", 
				finalStats.CopiedFiles, finalStats.ProcessedFiles, 
				finalStats.Directories, finalStats.RegularFiles, finalStats.Symlinks, finalStats.SkippedFiles)
			progress.UpdateStage(info)
		} else {
			info := fmt.Sprintf("%d items processed, all used fallback copy", finalStats.ProcessedFiles)
			progress.UpdateStage(info)
		}
	}
	
	// Check for any errors
	select {
	case err := <-pool.Error():
		if err != nil {
			return err
		}
	default:
		// No errors
	}
	
	return walkErr
}

// CloneDirectoryParallelForced forces file-by-file parallel cloning (skips atomic)
func CloneDirectoryParallelForced(src, dst string, progress *ProgressTracker) error {
	// Check if we're on APFS
	if isAPFS, err := isAPFS(src); err != nil {
		return fmt.Errorf("failed to check filesystem: %w", err)
	} else if !isAPFS {
		return errors.New("copy-on-write requires APFS filesystem")
	}
	
	if progress != nil {
		progress.UpdateStage("Forcing parallel file-by-file CoW")
	}
	
	// Skip atomic attempt and go straight to parallel fallback
	return cloneDirectoryParallelFallback(src, dst, progress)
}

// CloneDirectoryParallelDepth recursively finds subdirectories at maxDepth and clones each atomically in parallel
func CloneDirectoryParallelDepth(src, dst string, maxDepth int, progress *ProgressTracker) error {
	// Check if we're on APFS
	if isAPFS, err := isAPFS(src); err != nil {
		return fmt.Errorf("failed to check filesystem: %w", err)
	} else if !isAPFS {
		return errors.New("copy-on-write requires APFS filesystem")
	}
	
	if progress != nil {
		progress.UpdateStage(fmt.Sprintf("Finding subdirectories at depth %d for parallel atomic cloning", maxDepth))
	}
	
	// Find all directories at the target depth
	targets, err := findDirectoriesAtDepth(src, maxDepth)
	if err != nil {
		return fmt.Errorf("failed to find directories at depth %d: %w", maxDepth, err)
	}
	
	if progress != nil {
		progress.UpdateStage(fmt.Sprintf("Found %d directories to clone in parallel", len(targets)))
	}
	
	// Create destination directory structure up to maxDepth-1
	if err := createDirectoryStructure(src, dst, maxDepth-1); err != nil {
		return fmt.Errorf("failed to create directory structure: %w", err)
	}
	
	// Create worker pool for atomic cloning of subdirectories
	pool := NewAtomicClonePool()
	controller := NewAtomicCloneController(pool)
	
	// Start pool and controller
	pool.Start()
	controller.Start()
	
	// Clean up when done
	defer func() {
		controller.Stop()
		pool.Stop()
	}()
	
	// Track progress
	done := make(chan bool)
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				if progress != nil {
					stats := pool.GetStats()
					throughput := float64(stats.ProcessedDirs) / stats.ElapsedTime.Seconds()
					
					info := fmt.Sprintf("%d of %d directories cloned, %.1f dirs/sec", 
						stats.CompletedDirs, len(targets), throughput)
					progress.UpdateStage(info)
				}
			case <-done:
				return
			}
		}
	}()
	
	// Submit all target directories for parallel atomic cloning
	for _, target := range targets {
		// Calculate destination path
		relPath, err := filepath.Rel(src, target.SrcPath)
		if err != nil {
			return fmt.Errorf("failed to calculate relative path: %w", err)
		}
		target.DstPath = filepath.Join(dst, relPath)
		pool.Submit(target)
	}
	
	// Signal completion and wait for workers
	close(pool.taskChan)
	done <- true
	
	// Get final statistics
	if progress != nil {
		finalStats := pool.GetStats()
		info := fmt.Sprintf("Completed %d directory clones in parallel", finalStats.CompletedDirs)
		progress.UpdateStage(info)
	}
	
	// Check for any errors
	select {
	case err := <-pool.Error():
		if err != nil {
			return err
		}
	default:
		// No errors
	}
	
	return nil
}

// AtomicCloneTask represents a directory to clone atomically
type AtomicCloneTask struct {
	SrcPath string
	DstPath string
}

// AtomicClonePool manages parallel atomic directory cloning
type AtomicClonePool struct {
	taskChan      chan AtomicCloneTask
	errChan       chan error
	workerCount   int32
	activeWorkers map[int]chan struct{}
	nextWorkerID  int32
	
	mu        sync.RWMutex
	wg        sync.WaitGroup
	
	// Metrics
	processedDirs int64
	completedDirs int64
	failedDirs    int64
	startTime     time.Time
}

// AtomicCloneStats contains statistics about atomic cloning
type AtomicCloneStats struct {
	Workers       int32
	ProcessedDirs int64
	CompletedDirs int64
	FailedDirs    int64
	QueueDepth    int
	ElapsedTime   time.Duration
}

// AtomicCloneController manages the atomic clone pool
type AtomicCloneController struct {
	pool       *AtomicClonePool
	done       chan struct{}
	lastAdjust time.Time
}

// NewAtomicClonePool creates a new atomic clone pool
func NewAtomicClonePool() *AtomicClonePool {
	return &AtomicClonePool{
		taskChan:      make(chan AtomicCloneTask, 100),
		errChan:       make(chan error, 1),
		activeWorkers: make(map[int]chan struct{}),
		startTime:     time.Now(),
	}
}

// NewAtomicCloneController creates a controller for the atomic clone pool
func NewAtomicCloneController(pool *AtomicClonePool) *AtomicCloneController {
	return &AtomicCloneController{
		pool: pool,
		done: make(chan struct{}),
	}
}

// Start initializes the atomic clone pool
func (p *AtomicClonePool) Start() {
	initialWorkers := runtime.NumCPU()
	for i := 0; i < initialWorkers; i++ {
		p.AddWorker()
	}
}

// Stop shuts down the atomic clone pool
func (p *AtomicClonePool) Stop() {
	p.wg.Wait()
	close(p.errChan)
}

// Submit adds a directory clone task
func (p *AtomicClonePool) Submit(task AtomicCloneTask) {
	p.taskChan <- task
}

// Error returns the error channel
func (p *AtomicClonePool) Error() <-chan error {
	return p.errChan
}

// AddWorker adds a new worker to the pool
func (p *AtomicClonePool) AddWorker() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	workerID := int(atomic.AddInt32(&p.nextWorkerID, 1))
	stopChan := make(chan struct{})
	p.activeWorkers[workerID] = stopChan
	atomic.AddInt32(&p.workerCount, 1)
	
	p.wg.Add(1)
	go p.worker(workerID, stopChan)
}

// RemoveWorker removes a worker from the pool
func (p *AtomicClonePool) RemoveWorker() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	minWorkers := max(runtime.NumCPU()/2, 1)
	if len(p.activeWorkers) <= minWorkers {
		return false
	}
	
	// Stop one worker
	for id, stopChan := range p.activeWorkers {
		close(stopChan)
		delete(p.activeWorkers, id)
		atomic.AddInt32(&p.workerCount, -1)
		return true
	}
	
	return false
}

// GetStats returns current pool statistics
func (p *AtomicClonePool) GetStats() AtomicCloneStats {
	return AtomicCloneStats{
		Workers:       atomic.LoadInt32(&p.workerCount),
		ProcessedDirs: atomic.LoadInt64(&p.processedDirs),
		CompletedDirs: atomic.LoadInt64(&p.completedDirs),
		FailedDirs:    atomic.LoadInt64(&p.failedDirs),
		QueueDepth:    len(p.taskChan),
		ElapsedTime:   time.Since(p.startTime),
	}
}

// worker processes atomic clone tasks
func (p *AtomicClonePool) worker(_ int, stop <-chan struct{}) {
	defer p.wg.Done()
	
	for {
		select {
		case task, ok := <-p.taskChan:
			if !ok {
				return // Channel closed
			}
			
			if err := p.processAtomicClone(task); err != nil {
				atomic.AddInt64(&p.failedDirs, 1)
				select {
				case p.errChan <- err:
				default:
				}
				return
			} else {
				atomic.AddInt64(&p.completedDirs, 1)
			}
			
			atomic.AddInt64(&p.processedDirs, 1)
			
		case <-stop:
			return // Worker stopped
		}
	}
}

// processAtomicClone performs atomic cloning of a directory
func (p *AtomicClonePool) processAtomicClone(task AtomicCloneTask) error {
	// Use atomic clonefile for the entire directory
	return unix.Clonefile(task.SrcPath, task.DstPath, unix.CLONE_NOFOLLOW)
}

// Start begins monitoring the atomic clone pool
func (c *AtomicCloneController) Start() {
	go c.controlLoop()
}

// Stop shuts down the controller
func (c *AtomicCloneController) Stop() {
	close(c.done)
}

// controlLoop manages worker pool scaling
func (c *AtomicCloneController) controlLoop() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	
	var lastProcessed int64
	var lastCheck time.Time = time.Now()
	
	for {
		select {
		case <-ticker.C:
			stats := c.pool.GetStats()
			
			// Calculate processing rate
			now := time.Now()
			timeDelta := now.Sub(lastCheck).Seconds()
			if timeDelta > 0 {
				processingRate := float64(stats.ProcessedDirs-lastProcessed) / timeDelta
				lastProcessed = stats.ProcessedDirs
				lastCheck = now
				
				c.adjustWorkers(stats, processingRate)
			}
			
		case <-c.done:
			return
		}
	}
}

// adjustWorkers implements scaling logic for atomic cloning
func (c *AtomicCloneController) adjustWorkers(stats AtomicCloneStats, processingRate float64) {
	// Rate limit adjustments
	if time.Since(c.lastAdjust) < time.Second {
		return
	}
	
	maxWorkers := int32(runtime.NumCPU() * 2)
	
	// Queue backing up? Add workers
	if stats.QueueDepth > int(stats.Workers)*5 && stats.Workers < maxWorkers && processingRate > 0 {
		c.pool.AddWorker()
		c.lastAdjust = time.Now()
		return
	}
	
	// Queue empty? Remove workers
	if stats.QueueDepth == 0 && processingRate < 1 {
		if c.pool.RemoveWorker() {
			c.lastAdjust = time.Now()
		}
		return
	}
}

// findDirectoriesAtDepth finds all directories at exactly the specified depth
func findDirectoriesAtDepth(root string, targetDepth int) ([]AtomicCloneTask, error) {
	var targets []AtomicCloneTask
	
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		if !info.IsDir() {
			return nil
		}
		
		// Calculate depth relative to root
		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		
		// Root directory is depth 0
		if relPath == "." {
			return nil
		}
		
		depth := strings.Count(relPath, string(filepath.Separator)) + 1
		
		if depth == targetDepth {
			// Found a directory at target depth
			targets = append(targets, AtomicCloneTask{
				SrcPath: path,
				DstPath: "", // Will be set later
			})
			// Don't recurse deeper into this directory
			return filepath.SkipDir
		} else if depth > targetDepth {
			// We've gone too deep, skip
			return filepath.SkipDir
		}
		
		return nil
	})
	
	return targets, err
}

// createDirectoryStructure creates the directory structure up to maxDepth-1
func createDirectoryStructure(src, dst string, maxDepth int) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		if !info.IsDir() {
			return nil
		}
		
		// Calculate depth and destination path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		
		if relPath == "." {
			// Create root destination directory
			return os.MkdirAll(dst, info.Mode())
		}
		
		depth := strings.Count(relPath, string(filepath.Separator)) + 1
		
		if depth <= maxDepth {
			// Create this directory
			dstPath := filepath.Join(dst, relPath)
			if err := os.MkdirAll(dstPath, info.Mode()); err != nil {
				return err
			}
		}
		
		if depth >= maxDepth {
			// Don't recurse deeper
			return filepath.SkipDir
		}
		
		return nil
	})
}