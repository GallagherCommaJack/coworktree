package cowgit

import (
	"fmt"
	"os"
	"path/filepath"
)

// Manager provides high-level operations for managing CoW worktrees
type Manager struct {
	RepoPath string
}

// NewManager creates a new Manager for the given repository path
func NewManager(repoPath string) (*Manager, error) {
	// Verify it's a git repository
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err != nil {
		return nil, fmt.Errorf("not a git repository: %s", repoPath)
	}
	
	return &Manager{RepoPath: repoPath}, nil
}

// CreateOptions holds options for creating a worktree
type CreateOptions struct {
	BranchName    string
	WorktreePath  string
	FromCommit    string
	NoCoW         bool
	NoRewrite     bool
	Prefix        string
}

// Create creates a new CoW worktree with the given options
func (m *Manager) Create(opts CreateOptions) (*Worktree, error) {
	branchName := opts.BranchName
	if opts.Prefix != "" {
		branchName = opts.Prefix + branchName
	}

	// Determine worktree path if not specified
	worktreePath := opts.WorktreePath
	if worktreePath == "" {
		worktreePath = filepath.Join(m.RepoPath, ".cow-worktrees", branchName)
	}

	// Create worktree instance
	worktree := NewWorktreeWithOptions(m.RepoPath, worktreePath, branchName, opts.NoRewrite)

	// Create the worktree
	if !opts.NoCoW {
		// Check if CoW is supported
		if supported, err := IsCoWSupported(m.RepoPath); err == nil && supported {
			if err := worktree.CreateCoWWorktree(); err == nil {
				return worktree, nil
			}
		}
	}

	// Fall back to regular worktree
	if err := m.createRegularWorktree(worktree); err != nil {
		return nil, err
	}

	return worktree, nil
}

// CreateFromBranch creates a worktree from an existing branch
func (m *Manager) CreateFromBranch(branchName, worktreePath string) (*Worktree, error) {
	if worktreePath == "" {
		worktreePath = filepath.Join(m.RepoPath, ".cow-worktrees", branchName)
	}

	worktree := NewWorktree(m.RepoPath, worktreePath, branchName)
	if err := worktree.CreateFromExistingBranch(); err != nil {
		return nil, err
	}

	return worktree, nil
}

// List returns all worktrees in the repository
func (m *Manager) List() ([]WorktreeInfo, error) {
	return ListWorktrees(m.RepoPath)
}

// ListCoW returns only CoW worktrees (those in .cow-worktrees)
func (m *Manager) ListCoW() ([]WorktreeInfo, error) {
	worktrees, err := ListWorktrees(m.RepoPath)
	if err != nil {
		return nil, err
	}

	var cowWorktrees []WorktreeInfo
	for _, wt := range worktrees {
		// Include worktrees that are in .cow-worktrees or not the main repo
		if filepath.Base(filepath.Dir(wt.Path)) == ".cow-worktrees" || wt.Path != m.RepoPath {
			cowWorktrees = append(cowWorktrees, wt)
		}
	}

	return cowWorktrees, nil
}

// Remove removes a worktree by branch name
func (m *Manager) Remove(branchName string, keepBranch bool) error {
	// Find the worktree
	worktrees, err := ListWorktrees(m.RepoPath)
	if err != nil {
		return fmt.Errorf("failed to list worktrees: %w", err)
	}

	var worktreePath string
	for _, wt := range worktrees {
		if wt.Branch == branchName {
			worktreePath = wt.Path
			break
		}
	}

	if worktreePath == "" {
		// Try default path
		worktreePath = filepath.Join(m.RepoPath, ".cow-worktrees", branchName)
	}

	worktree := NewWorktree(m.RepoPath, worktreePath, branchName)

	if keepBranch {
		return worktree.Remove()
	}

	return worktree.RemoveWithBranch()
}

// IsCoWSupported checks if CoW is supported for this repository
func (m *Manager) IsCoWSupported() (bool, error) {
	return IsCoWSupported(m.RepoPath)
}

// createRegularWorktree creates a regular git worktree
func (m *Manager) createRegularWorktree(worktree *Worktree) error {
	if err := os.MkdirAll(filepath.Dir(worktree.WorktreePath), 0755); err != nil {
		return fmt.Errorf("failed to create worktree directory: %w", err)
	}

	// Use git worktree add directly
	if _, err := worktree.runGitCommand(m.RepoPath, "worktree", "add", "-b", worktree.BranchName, worktree.WorktreePath); err != nil {
		return fmt.Errorf("failed to create regular worktree: %w", err)
	}

	return nil
}