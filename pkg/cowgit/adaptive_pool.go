package cowgit

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// WorkerPool manages file processing workers
type WorkerPool struct {
	fileChan      chan string
	errChan       chan error
	workerCount   int32
	activeWorkers map[int]chan struct{}
	nextWorkerID  int32
	
	// Rewriting context
	srcDirBytes []byte
	dstDirBytes []byte
	gitignore   *GitIgnore
	dstDir      string
	
	mu        sync.RWMutex
	wg        sync.WaitGroup
	
	// Metrics
	processedFiles    int64
	gitignoreMatches  int64  // Files that matched gitignore patterns
	textFiles         int64  // Text files that were candidates for rewriting
	modifiedFiles     int64  // Files that were actually modified
	skippedBinary     int64  // Binary files skipped
	skippedNoMatch    int64  // Files that didn't match gitignore
	startTime         time.Time
}

// PoolController manages worker pool scaling
type PoolController struct {
	pool       *WorkerPool
	done       chan struct{}
	lastAdjust time.Time
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(srcDir, dstDir string, gitignore *GitIgnore) *WorkerPool {
	return &WorkerPool{
		fileChan:      make(chan string, 1000),
		errChan:       make(chan error, 1),
		activeWorkers: make(map[int]chan struct{}),
		srcDirBytes:   []byte(srcDir),
		dstDirBytes:   []byte(dstDir),
		gitignore:     gitignore,
		dstDir:        dstDir,
		startTime:     time.Now(),
	}
}

// NewPoolController creates a controller for the worker pool
func NewPoolController(pool *WorkerPool) *PoolController {
	return &PoolController{
		pool: pool,
		done: make(chan struct{}),
	}
}

// Start initializes the worker pool with CPU count workers
func (p *WorkerPool) Start() {
	initialWorkers := runtime.NumCPU()
	for i := 0; i < initialWorkers; i++ {
		p.AddWorker()
	}
}

// Stop shuts down the worker pool
func (p *WorkerPool) Stop() {
	close(p.fileChan)
	p.wg.Wait()
	close(p.errChan)
}

// Submit adds a file to be processed
func (p *WorkerPool) Submit(filepath string) {
	p.fileChan <- filepath
}

// Error returns the error channel
func (p *WorkerPool) Error() <-chan error {
	return p.errChan
}

// AddWorker adds a new worker to the pool
func (p *WorkerPool) AddWorker() {
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
func (p *WorkerPool) RemoveWorker() bool {
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

// PathRewriteStats contains detailed statistics about path rewriting
type PathRewriteStats struct {
	Workers         int32
	ProcessedFiles  int64
	GitignoreMatches int64
	TextFiles       int64
	ModifiedFiles   int64
	SkippedBinary   int64
	SkippedNoMatch  int64
	QueueDepth      int
	ElapsedTime     time.Duration
}

// GetStats returns current pool statistics
func (p *WorkerPool) GetStats() (workers int32, processed int64, queueDepth int) {
	return atomic.LoadInt32(&p.workerCount),
		   atomic.LoadInt64(&p.processedFiles),
		   len(p.fileChan)
}

// GetDetailedStats returns comprehensive statistics
func (p *WorkerPool) GetDetailedStats() PathRewriteStats {
	return PathRewriteStats{
		Workers:          atomic.LoadInt32(&p.workerCount),
		ProcessedFiles:   atomic.LoadInt64(&p.processedFiles),
		GitignoreMatches: atomic.LoadInt64(&p.gitignoreMatches),
		TextFiles:        atomic.LoadInt64(&p.textFiles),
		ModifiedFiles:    atomic.LoadInt64(&p.modifiedFiles),
		SkippedBinary:    atomic.LoadInt64(&p.skippedBinary),
		SkippedNoMatch:   atomic.LoadInt64(&p.skippedNoMatch),
		QueueDepth:       len(p.fileChan),
		ElapsedTime:      time.Since(p.startTime),
	}
}

// worker processes files from the queue
func (p *WorkerPool) worker(id int, stop <-chan struct{}) {
	defer p.wg.Done()
	
	for {
		select {
		case path, ok := <-p.fileChan:
			if !ok {
				return // Channel closed
			}
			
			if err := p.processFile(path); err != nil {
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

// processFile handles the actual file processing
func (p *WorkerPool) processFile(path string) error {
	relPath, err := filepath.Rel(p.dstDir, path)
	if err != nil {
		return nil // Skip on error
	}

	// Filter: gitignored files only
	if !p.gitignore.Match(relPath) {
		atomic.AddInt64(&p.skippedNoMatch, 1)
		return nil
	}
	
	atomic.AddInt64(&p.gitignoreMatches, 1)

	// Read file and check if it's text
	content, err := os.ReadFile(path)
	if err != nil {
		return nil // Skip on error
	}

	// Skip binary files
	if !isValidText(content) {
		atomic.AddInt64(&p.skippedBinary, 1)
		return nil
	}
	
	atomic.AddInt64(&p.textFiles, 1)

	// Replace srcDir with dstDir
	if updated := bytes.ReplaceAll(content, p.srcDirBytes, p.dstDirBytes); !bytes.Equal(content, updated) {
		atomic.AddInt64(&p.modifiedFiles, 1)
		return os.WriteFile(path, updated, 0644)
	}
	
	return nil
}

// Start begins monitoring and adjusting the worker pool
func (c *PoolController) Start() {
	go c.controlLoop()
}

// Stop shuts down the controller
func (c *PoolController) Stop() {
	close(c.done)
}

// controlLoop is the main control logic that adjusts worker count
func (c *PoolController) controlLoop() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	
	var lastProcessed int64
	var lastCheck time.Time = time.Now()
	
	for {
		select {
		case <-ticker.C:
			workers, processed, queueLen := c.pool.GetStats()
			
			// Calculate processing rate
			now := time.Now()
			timeDelta := now.Sub(lastCheck).Seconds()
			if timeDelta > 0 {
				processingRate := float64(processed-lastProcessed) / timeDelta
				lastProcessed = processed
				lastCheck = now
				
				c.adjustWorkers(workers, queueLen, processingRate)
			}
			
		case <-c.done:
			return
		}
	}
}

// adjustWorkers implements the scaling logic
func (c *PoolController) adjustWorkers(workers int32, queueLen int, processingRate float64) {
	// Rate limit adjustments
	if time.Since(c.lastAdjust) < time.Second {
		return
	}
	
	maxWorkers := int32(runtime.NumCPU() * 4)
	
	// Queue backing up and processing? Add workers
	if queueLen > int(workers)*5 && workers < maxWorkers && processingRate > 0 {
		c.pool.AddWorker()
		c.lastAdjust = time.Now()
		return
	}
	
	// Queue empty and many workers? Remove workers
	if queueLen == 0 && processingRate < 1 {
		if c.pool.RemoveWorker() {
			c.lastAdjust = time.Now()
		}
		return
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}