package cowgit

import (
	"fmt"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
)

// ProgressTracker manages progress indicators for CoW operations
type ProgressTracker struct {
	spinner     *spinner.Spinner
	startTime   time.Time
	showSpinner bool
	stage       string
}

// NewProgressTracker creates a new progress tracker
func NewProgressTracker(forceShow bool) *ProgressTracker {
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Color("cyan")
	
	return &ProgressTracker{
		spinner:     s,
		showSpinner: forceShow || isTerminal(),
	}
}

// StartStage begins a new stage with a spinner
func (p *ProgressTracker) StartStage(stage string) {
	p.stage = stage
	p.startTime = time.Now()
	
	if p.showSpinner {
		p.spinner.Suffix = fmt.Sprintf(" %s...", stage)
		p.spinner.Start()
	}
}

// UpdateStage updates the current stage with additional info
func (p *ProgressTracker) UpdateStage(info string) {
	if p.showSpinner {
		elapsed := time.Since(p.startTime)
		p.spinner.Suffix = fmt.Sprintf(" %s... %s (%v)", p.stage, info, elapsed.Truncate(10*time.Millisecond))
	}
}

// FinishStage completes the current stage and shows timing
func (p *ProgressTracker) FinishStage() {
	elapsed := time.Since(p.startTime)
	
	if p.showSpinner {
		p.spinner.Stop()
	}
	
	green := color.New(color.FgGreen).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()
	
	if p.showSpinner {
		fmt.Printf("✓ %s %s in %v\n", green(p.stage), cyan("completed"), elapsed.Truncate(time.Microsecond))
	}
}

// FinishStageWithInfo completes the stage with additional info
func (p *ProgressTracker) FinishStageWithInfo(info string) {
	elapsed := time.Since(p.startTime)
	
	if p.showSpinner {
		p.spinner.Stop()
	}
	
	green := color.New(color.FgGreen).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	
	if p.showSpinner {
		fmt.Printf("✓ %s %s %s in %v\n", green(p.stage), yellow(info), cyan("completed"), elapsed.Truncate(time.Microsecond))
	}
}

// Error shows an error message and stops the spinner
func (p *ProgressTracker) Error(err error) {
	if p.showSpinner {
		p.spinner.Stop()
	}
	
	red := color.New(color.FgRed).SprintFunc()
	fmt.Printf("✗ %s %s\n", red("Error:"), err.Error())
}

// SetQuiet disables spinner output
func (p *ProgressTracker) SetQuiet(quiet bool) {
	if quiet {
		p.showSpinner = false
	}
	// Don't re-enable if it was disabled - let the constructor logic handle the initial state
}

// isTerminal checks if we're running in a TTY
func isTerminal() bool {
	fileInfo, _ := os.Stdout.Stat()
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

// ProgressCallback is a function type for progress updates
type ProgressCallback func(current, total int, info string)

// FileWalker tracks file walking progress
type FileWalker struct {
	tracker     *ProgressTracker
	filesWalked int
	totalFiles  int
	lastUpdate  time.Time
}

// NewFileWalker creates a new file walker with progress tracking
func NewFileWalker(tracker *ProgressTracker) *FileWalker {
	return &FileWalker{
		tracker:    tracker,
		lastUpdate: time.Now(),
	}
}

// WalkFile is called for each file during walking
func (fw *FileWalker) WalkFile() {
	fw.filesWalked++
	
	// Update progress every 100ms to avoid too much output
	if time.Since(fw.lastUpdate) > 100*time.Millisecond {
		if fw.totalFiles > 0 {
			percent := float64(fw.filesWalked) / float64(fw.totalFiles) * 100
			fw.tracker.UpdateStage(fmt.Sprintf("%d/%d files (%.1f%%)", fw.filesWalked, fw.totalFiles, percent))
		} else {
			fw.tracker.UpdateStage(fmt.Sprintf("%d files", fw.filesWalked))
		}
		fw.lastUpdate = time.Now()
	}
}

// SetTotal sets the total number of files expected
func (fw *FileWalker) SetTotal(total int) {
	fw.totalFiles = total
}

// GetFilesWalked returns the number of files walked
func (fw *FileWalker) GetFilesWalked() int {
	return fw.filesWalked
}