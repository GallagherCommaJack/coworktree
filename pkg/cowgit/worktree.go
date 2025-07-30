package cowgit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// Worktree represents a git worktree with CoW capabilities
type Worktree struct {
	RepoPath     string
	WorktreePath string
	BranchName   string
	BaseCommit   string
	NoRewrite    bool
}

// NewWorktree creates a new Worktree instance
func NewWorktree(repoPath, worktreePath, branchName string) *Worktree {
	return &Worktree{
		RepoPath:     repoPath,
		WorktreePath: worktreePath,
		BranchName:   branchName,
	}
}

// NewWorktreeWithOptions creates a new Worktree instance with options
func NewWorktreeWithOptions(repoPath, worktreePath, branchName string, noRewrite bool) *Worktree {
	return &Worktree{
		RepoPath:     repoPath,
		WorktreePath: worktreePath,
		BranchName:   branchName,
		NoRewrite:    noRewrite,
	}
}

// CreateCoWWorktree creates a new worktree using copy-on-write
func (w *Worktree) CreateCoWWorktree() error {
	return w.CreateCoWWorktreeWithProgress(nil)
}

// CreateCoWWorktreeWithProgress creates a new worktree using copy-on-write with progress tracking
func (w *Worktree) CreateCoWWorktreeWithProgress(progress *ProgressTracker) error {
	// Clean up any existing worktree first
	w.runGitCommand(w.RepoPath, "worktree", "remove", "-f", w.WorktreePath) // Ignore error if worktree doesn't exist

	// Get HEAD commit
	output, err := w.runGitCommand(w.RepoPath, "rev-parse", "HEAD")
	if err != nil {
		if strings.Contains(err.Error(), "fatal: ambiguous argument 'HEAD'") ||
			strings.Contains(err.Error(), "fatal: not a valid object name") ||
			strings.Contains(err.Error(), "fatal: HEAD: not a valid object name") {
			return fmt.Errorf("this appears to be a brand new repository: please create an initial commit before creating a worktree")
		}
		return fmt.Errorf("failed to get HEAD commit hash: %w", err)
	}
	headCommit := strings.TrimSpace(string(output))
	w.BaseCommit = headCommit

	// Try copy-on-write first, fall back to regular worktree if it fails
	if err := w.setupWorktreeWithCoWProgress(progress); err != nil {
		return w.setupRegularWorktree(headCommit)
	}

	return nil
}

// CreateFromExistingBranch creates a worktree from an existing branch
func (w *Worktree) CreateFromExistingBranch() error {
	// Ensure worktrees directory exists
	worktreesDir := filepath.Dir(w.WorktreePath)
	if err := os.MkdirAll(worktreesDir, 0755); err != nil {
		return fmt.Errorf("failed to create worktrees directory: %w", err)
	}

	// Clean up any existing worktree first
	w.runGitCommand(w.RepoPath, "worktree", "remove", "-f", w.WorktreePath) // Ignore error if worktree doesn't exist

	// Create a new worktree from the existing branch
	if _, err := w.runGitCommand(w.RepoPath, "worktree", "add", w.WorktreePath, w.BranchName); err != nil {
		return fmt.Errorf("failed to create worktree from branch %s: %w", w.BranchName, err)
	}

	return nil
}

// setupWorktreeWithCoW creates a worktree using copy-on-write
func (w *Worktree) setupWorktreeWithCoW() error {
	return w.setupWorktreeWithCoWProgress(nil)
}

// setupWorktreeWithCoWProgress creates a worktree using copy-on-write with progress tracking
func (w *Worktree) setupWorktreeWithCoWProgress(progress *ProgressTracker) error {
	// Remove existing worktree path if it exists
	if err := os.RemoveAll(w.WorktreePath); err != nil {
		return fmt.Errorf("failed to remove existing worktree path: %w", err)
	}

	// Stage 1: Copy-on-write cloning
	if progress != nil {
		progress.StartStage("CoW cloning")
	}
	if err := CloneDirectory(w.RepoPath, w.WorktreePath); err != nil {
		if progress != nil {
			progress.Error(err)
		}
		return fmt.Errorf("failed to clone directory: %w", err)
	}
	if progress != nil {
		progress.FinishStage()
	}

	// Stage 2: Git branch setup
	if progress != nil {
		progress.StartStage("Setting up git branch")
	}
	
	// Create new branch without checking it out (to preserve untracked files)
	if _, err := w.runGitCommand(w.WorktreePath, "branch", w.BranchName); err != nil {
		// Clean up the clone if branch creation fails
		os.RemoveAll(w.WorktreePath)
		if progress != nil {
			progress.Error(err)
		}
		return fmt.Errorf("failed to create branch %s: %w", w.BranchName, err)
	}
	
	// Switch to the new branch without overwriting working directory
	if _, err := w.runGitCommand(w.WorktreePath, "symbolic-ref", "HEAD", "refs/heads/"+w.BranchName); err != nil {
		// Clean up the clone if symbolic-ref fails
		os.RemoveAll(w.WorktreePath)
		if progress != nil {
			progress.Error(err)
		}
		return fmt.Errorf("failed to switch to branch %s: %w", w.BranchName, err)
	}

	// Manually register the cloned directory as a proper git worktree
	if err := w.registerWorktreeManually(); err != nil {
		// Clean up the clone if worktree registration fails
		os.RemoveAll(w.WorktreePath)
		if progress != nil {
			progress.Error(err)
		}
		return fmt.Errorf("failed to register worktree: %w", err)
	}
	
	if progress != nil {
		progress.FinishStage()
	}

	// Stage 3: Path rewriting (if enabled)
	if !w.NoRewrite {
		if progress != nil {
			progress.StartStage("Fixing absolute paths")
		}
		
		if err := w.rewriteAbsolutePathsWithProgress(progress); err != nil {
			// Log warning but don't fail - path rewriting is best effort
			if progress != nil {
				progress.UpdateStage("(skipped due to error)")
			}
		}
		
		if progress != nil {
			progress.FinishStage()
		}
	}

	return nil
}

// rewriteAbsolutePathsWithProgress rewrites paths with progress tracking
func (w *Worktree) rewriteAbsolutePathsWithProgress(progress *ProgressTracker) error {
	if progress != nil {
		// Count files first to show progress
		walker := NewFileWalker(progress)
		if err := w.countFiles(walker); err == nil {
			walker.SetTotal(walker.GetFilesWalked())
		}
		walker.filesWalked = 0 // Reset for actual processing
	}
	
	return rewriteAbsolutePathsAsync(w.RepoPath, w.WorktreePath)
}

// countFiles walks the directory to count total files
func (w *Worktree) countFiles(walker *FileWalker) error {
	return filepath.Walk(w.WorktreePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			walker.WalkFile()
		}
		return nil
	})
}

// setupRegularWorktree creates a worktree using the traditional git worktree method
func (w *Worktree) setupRegularWorktree(headCommit string) error {
	if _, err := w.runGitCommand(w.RepoPath, "worktree", "add", "-b", w.BranchName, w.WorktreePath, headCommit); err != nil {
		return fmt.Errorf("failed to create worktree from commit %s: %w", headCommit, err)
	}
	return nil
}

// Remove removes the worktree but keeps the branch
func (w *Worktree) Remove() error {
	if _, err := w.runGitCommand(w.RepoPath, "worktree", "remove", "-f", w.WorktreePath); err != nil {
		return fmt.Errorf("failed to remove worktree: %w", err)
	}
	return nil
}

// RemoveWithBranch removes the worktree and associated branch
func (w *Worktree) RemoveWithBranch() error {
	var errs []error

	// Check if worktree path exists before attempting removal
	if _, err := os.Stat(w.WorktreePath); err == nil {
		// Remove the worktree using git command
		if _, err := w.runGitCommand(w.RepoPath, "worktree", "remove", "-f", w.WorktreePath); err != nil {
			errs = append(errs, err)
		}
	} else if !os.IsNotExist(err) {
		errs = append(errs, fmt.Errorf("failed to check worktree path: %w", err))
	}

	// Open the repository for branch cleanup
	repo, err := git.PlainOpen(w.RepoPath)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to open repository for cleanup: %w", err))
		return combineErrors(errs)
	}

	branchRef := plumbing.NewBranchReferenceName(w.BranchName)

	// Check if branch exists before attempting removal
	if _, err := repo.Reference(branchRef, false); err == nil {
		if err := repo.Storer.RemoveReference(branchRef); err != nil {
			errs = append(errs, fmt.Errorf("failed to remove branch %s: %w", w.BranchName, err))
		}
	} else if err != plumbing.ErrReferenceNotFound {
		errs = append(errs, fmt.Errorf("error checking branch %s existence: %w", w.BranchName, err))
	}

	// Prune the worktree to clean up any remaining references
	if err := w.Prune(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return combineErrors(errs)
	}

	return nil
}

// Prune removes all working tree administrative files and directories
func (w *Worktree) Prune() error {
	if _, err := w.runGitCommand(w.RepoPath, "worktree", "prune"); err != nil {
		return fmt.Errorf("failed to prune worktrees: %w", err)
	}
	return nil
}

// ListWorktrees returns a list of all worktrees in the repository
func ListWorktrees(repoPath string) ([]WorktreeInfo, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	var worktrees []WorktreeInfo
	var current WorktreeInfo
	lines := strings.Split(string(output), "\n")
	
	for _, line := range lines {
		if strings.HasPrefix(line, "worktree ") {
			if current.Path != "" {
				worktrees = append(worktrees, current)
			}
			current = WorktreeInfo{Path: strings.TrimPrefix(line, "worktree ")}
		} else if strings.HasPrefix(line, "branch ") {
			branchPath := strings.TrimPrefix(line, "branch ")
			current.Branch = strings.TrimPrefix(branchPath, "refs/heads/")
		} else if strings.HasPrefix(line, "HEAD ") {
			current.HEAD = strings.TrimPrefix(line, "HEAD ")
		}
	}
	
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}

	return worktrees, nil
}

// WorktreeInfo represents information about a git worktree
type WorktreeInfo struct {
	Path   string
	Branch string
	HEAD   string
}

// runGitCommand executes a git command in the specified directory
func (w *Worktree) runGitCommand(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.Output()
}

// registerWorktreeManually manually registers a CoW clone as a git worktree
func (w *Worktree) registerWorktreeManually() error {
	// Create worktree name (using the branch name)
	worktreeName := w.BranchName
	
	// Create the worktree metadata directory in main repo
	worktreeMetaDir := filepath.Join(w.RepoPath, ".git", "worktrees", worktreeName)
	if err := os.MkdirAll(worktreeMetaDir, 0755); err != nil {
		return fmt.Errorf("failed to create worktree metadata directory: %w", err)
	}
	
	// Create HEAD file in worktree metadata pointing to the branch
	headFile := filepath.Join(worktreeMetaDir, "HEAD")
	headRef := fmt.Sprintf("ref: refs/heads/%s\n", w.BranchName)
	if err := os.WriteFile(headFile, []byte(headRef), 0644); err != nil {
		return fmt.Errorf("failed to write HEAD file: %w", err)
	}
	
	// Create commondir file pointing to main repo's .git
	commondirFile := filepath.Join(worktreeMetaDir, "commondir")
	if err := os.WriteFile(commondirFile, []byte("../..\n"), 0644); err != nil {
		return fmt.Errorf("failed to write commondir file: %w", err)
	}
	
	// Create gitdir file pointing to worktree's .git file
	gitdirFile := filepath.Join(worktreeMetaDir, "gitdir")
	worktreeGitFile := filepath.Join(w.WorktreePath, ".git")
	if err := os.WriteFile(gitdirFile, []byte(worktreeGitFile+"\n"), 0644); err != nil {
		return fmt.Errorf("failed to write gitdir file: %w", err)
	}
	
	// Replace worktree's .git directory with .git file pointing to metadata
	worktreeGitDir := filepath.Join(w.WorktreePath, ".git")
	if err := os.RemoveAll(worktreeGitDir); err != nil {
		return fmt.Errorf("failed to remove .git directory: %w", err)
	}
	
	gitFileContent := fmt.Sprintf("gitdir: %s\n", worktreeMetaDir)
	if err := os.WriteFile(worktreeGitFile, []byte(gitFileContent), 0644); err != nil {
		return fmt.Errorf("failed to write .git file: %w", err)
	}
	
	return nil
}

// combineErrors combines multiple errors into a single error
func combineErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	
	var messages []string
	for _, err := range errs {
		messages = append(messages, err.Error())
	}
	
	return fmt.Errorf("multiple errors occurred: %s", strings.Join(messages, "; "))
}