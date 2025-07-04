package cowgit

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWorktreeOperations(t *testing.T) {
	// Create a temporary git repository for testing
	tempDir, err := os.MkdirTemp("", "cowtree-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Initialize git repo
	if err := runCommand(tempDir, "git", "init"); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure git
	if err := runCommand(tempDir, "git", "config", "user.email", "test@example.com"); err != nil {
		t.Fatalf("Failed to config git email: %v", err)
	}
	if err := runCommand(tempDir, "git", "config", "user.name", "Test User"); err != nil {
		t.Fatalf("Failed to config git name: %v", err)
	}

	// Create initial commit
	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	if err := runCommand(tempDir, "git", "add", "."); err != nil {
		t.Fatalf("Failed to add files: %v", err)
	}

	if err := runCommand(tempDir, "git", "commit", "-m", "Initial commit"); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	// Test NewWorktree
	worktreePath := filepath.Join(tempDir, ".cow-worktrees", "test-branch")
	worktree := NewWorktree(tempDir, worktreePath, "test-branch")

	if worktree.RepoPath != tempDir {
		t.Errorf("Expected RepoPath %s, got %s", tempDir, worktree.RepoPath)
	}
	if worktree.BranchName != "test-branch" {
		t.Errorf("Expected BranchName test-branch, got %s", worktree.BranchName)
	}

	// Test traditional worktree creation (since CoW may not be available)
	if err := worktree.CreateCoWWorktree(); err != nil {
		t.Logf("CoW worktree creation failed (expected on non-APFS): %v", err)
		
		// Fall back to regular git worktree for testing
		if err := os.MkdirAll(filepath.Dir(worktreePath), 0755); err != nil {
			t.Fatalf("Failed to create worktree dir: %v", err)
		}
		
		if err := runCommand(tempDir, "git", "worktree", "add", "-b", "test-branch", worktreePath); err != nil {
			t.Fatalf("Failed to create regular worktree: %v", err)
		}
		
		t.Logf("Created regular worktree for testing")
	}

	// Verify worktree was created
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		t.Error("Worktree path does not exist")
	}

	// Test ListWorktrees
	worktrees, err := ListWorktrees(tempDir)
	if err != nil {
		t.Fatalf("Failed to list worktrees: %v", err)
	}

	found := false
	for _, wt := range worktrees {
		if wt.Branch == "test-branch" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Created worktree not found in list")
	}

	// Test Remove
	if err := worktree.Remove(); err != nil {
		t.Fatalf("Failed to remove worktree: %v", err)
	}

	// Verify worktree was removed
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Error("Worktree path still exists after removal")
	}
}

func runCommand(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	return cmd.Run()
}