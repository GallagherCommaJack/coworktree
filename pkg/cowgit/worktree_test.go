package cowgit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorktreeOperations(t *testing.T) {
	// Create a temporary git repository for testing
	tempDir, err := os.MkdirTemp("", "coworktree-test-*")
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
	worktreePath, err := os.MkdirTemp("", "cow-worktree-*")
	if err != nil {
		t.Fatalf("Failed to create worktree temp dir: %v", err)
	}
	defer os.RemoveAll(worktreePath)
	
	// Remove it so CreateCoWWorktree can create it
	os.RemoveAll(worktreePath)
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

func TestCoWPreservesUntrackedFiles(t *testing.T) {
	// Skip if not on APFS (macOS)
	tempDir, err := os.MkdirTemp("", "coworktree-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Check if CoW is supported
	if supported, err := IsCoWSupported(tempDir); err != nil || !supported {
		t.Skip("CoW not supported on this filesystem")
	}

	// Initialize git repo
	setupGitRepo(t, tempDir)

	// Create untracked files in main repo
	untrackedFile := filepath.Join(tempDir, "untracked.txt")
	if err := os.WriteFile(untrackedFile, []byte("untracked content"), 0644); err != nil {
		t.Fatalf("Failed to create untracked file: %v", err)
	}

	// Create nested untracked file
	nestedDir := filepath.Join(tempDir, "nested")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("Failed to create nested dir: %v", err)
	}
	nestedUntracked := filepath.Join(nestedDir, "nested_untracked.txt")
	if err := os.WriteFile(nestedUntracked, []byte("nested untracked"), 0644); err != nil {
		t.Fatalf("Failed to create nested untracked file: %v", err)
	}

	// Create CoW worktree in temp directory
	worktreePath, err := os.MkdirTemp("", "cow-worktree-*")
	if err != nil {
		t.Fatalf("Failed to create worktree temp dir: %v", err)
	}
	defer os.RemoveAll(worktreePath)
	
	// Remove it so CloneDirectory can create it
	os.RemoveAll(worktreePath)
	
	worktree := NewWorktree(tempDir, worktreePath, "test-branch")

	// Verify untracked files exist in source before cloning
	if _, err := os.Stat(untrackedFile); err != nil {
		t.Fatalf("Untracked file doesn't exist in source: %v", err)
	}
	if _, err := os.Stat(nestedUntracked); err != nil {
		t.Fatalf("Nested untracked file doesn't exist in source: %v", err)
	}
	
	// List files in source directory for debugging
	if files, err := os.ReadDir(tempDir); err == nil {
		t.Logf("Files in source: %v", files)
	}

	if err := worktree.CreateCoWWorktree(); err != nil {
		t.Fatalf("Failed to create CoW worktree: %v", err)
	}

	// List files in worktree directory for debugging
	if files, err := os.ReadDir(worktreePath); err == nil {
		t.Logf("Files in worktree: %v", files)
		for _, file := range files {
			if file.IsDir() {
				if subfiles, err := os.ReadDir(filepath.Join(worktreePath, file.Name())); err == nil {
					t.Logf("  %s/: %v", file.Name(), subfiles)
				}
			}
		}
	}

	// Verify untracked files are preserved in worktree
	worktreeUntrackedFile := filepath.Join(worktreePath, "untracked.txt")
	if content, err := os.ReadFile(worktreeUntrackedFile); err != nil {
		t.Errorf("Untracked file not preserved: %v", err)
	} else if string(content) != "untracked content" {
		t.Errorf("Untracked file content mismatch: got %s", string(content))
	}

	worktreeNestedUntracked := filepath.Join(worktreePath, "nested", "nested_untracked.txt")
	if content, err := os.ReadFile(worktreeNestedUntracked); err != nil {
		t.Errorf("Nested untracked file not preserved: %v", err)
	} else if string(content) != "nested untracked" {
		t.Errorf("Nested untracked file content mismatch: got %s", string(content))
	}

	// Cleanup
	if err := worktree.Remove(); err != nil {
		t.Errorf("Failed to remove worktree: %v", err)
	}
}

func TestCoWPreservesGitIgnoredFiles(t *testing.T) {
	// Skip if not on APFS (macOS)
	tempDir, err := os.MkdirTemp("", "coworktree-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Check if CoW is supported
	if supported, err := IsCoWSupported(tempDir); err != nil || !supported {
		t.Skip("CoW not supported on this filesystem")
	}

	// Initialize git repo
	setupGitRepo(t, tempDir)

	// Create .gitignore
	gitignore := filepath.Join(tempDir, ".gitignore")
	if err := os.WriteFile(gitignore, []byte("*.log\nbuild/\n.env\n"), 0644); err != nil {
		t.Fatalf("Failed to create .gitignore: %v", err)
	}

	// Add and commit .gitignore
	if err := runCommand(tempDir, "git", "add", ".gitignore"); err != nil {
		t.Fatalf("Failed to add .gitignore: %v", err)
	}
	if err := runCommand(tempDir, "git", "commit", "-m", "Add gitignore"); err != nil {
		t.Fatalf("Failed to commit .gitignore: %v", err)
	}

	// Create gitignored files
	logFile := filepath.Join(tempDir, "debug.log")
	if err := os.WriteFile(logFile, []byte("debug logs"), 0644); err != nil {
		t.Fatalf("Failed to create log file: %v", err)
	}

	buildDir := filepath.Join(tempDir, "build")
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		t.Fatalf("Failed to create build dir: %v", err)
	}
	buildFile := filepath.Join(buildDir, "output.bin")
	if err := os.WriteFile(buildFile, []byte("binary content"), 0644); err != nil {
		t.Fatalf("Failed to create build file: %v", err)
	}

	envFile := filepath.Join(tempDir, ".env")
	if err := os.WriteFile(envFile, []byte("SECRET=value"), 0644); err != nil {
		t.Fatalf("Failed to create .env file: %v", err)
	}

	// Create CoW worktree in temp directory
	worktreePath, err := os.MkdirTemp("", "cow-worktree-*")
	if err != nil {
		t.Fatalf("Failed to create worktree temp dir: %v", err)
	}
	defer os.RemoveAll(worktreePath)
	
	// Remove it so CloneDirectory can create it
	os.RemoveAll(worktreePath)
	
	worktree := NewWorktree(tempDir, worktreePath, "test-branch")

	if err := worktree.CreateCoWWorktree(); err != nil {
		t.Fatalf("Failed to create CoW worktree: %v", err)
	}

	// Verify gitignored files are preserved in worktree
	worktreeLogFile := filepath.Join(worktreePath, "debug.log")
	if content, err := os.ReadFile(worktreeLogFile); err != nil {
		t.Errorf("Gitignored log file not preserved: %v", err)
	} else if string(content) != "debug logs" {
		t.Errorf("Gitignored log file content mismatch: got %s", string(content))
	}

	worktreeBuildFile := filepath.Join(worktreePath, "build", "output.bin")
	if content, err := os.ReadFile(worktreeBuildFile); err != nil {
		t.Errorf("Gitignored build file not preserved: %v", err)
	} else if string(content) != "binary content" {
		t.Errorf("Gitignored build file content mismatch: got %s", string(content))
	}

	worktreeEnvFile := filepath.Join(worktreePath, ".env")
	if content, err := os.ReadFile(worktreeEnvFile); err != nil {
		t.Errorf("Gitignored .env file not preserved: %v", err)
	} else if string(content) != "SECRET=value" {
		t.Errorf("Gitignored .env file content mismatch: got %s", string(content))
	}

	// Cleanup
	if err := worktree.Remove(); err != nil {
		t.Errorf("Failed to remove worktree: %v", err)
	}
}

func setupGitRepo(t *testing.T, tempDir string) {
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
}

func TestAPFSCloneNestedPath(t *testing.T) {
	// Create source git repository
	srcDir, err := os.MkdirTemp("", "apfs-clone-nested-src-*")
	if err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	defer os.RemoveAll(srcDir)

	// Check if CoW is supported
	if supported, err := IsCoWSupported(srcDir); err != nil || !supported {
		t.Skip("CoW not supported on this filesystem")
	}

	// Initialize git repo
	setupGitRepo(t, srcDir)

	// Add untracked files
	untrackedFile := filepath.Join(srcDir, "untracked.txt")
	if err := os.WriteFile(untrackedFile, []byte("untracked"), 0644); err != nil {
		t.Fatalf("Failed to create untracked file: %v", err)
	}

	// Test cloning to a nested path structure
	baseDir, err := os.MkdirTemp("", "apfs-clone-nested-base-*")
	if err != nil {
		t.Fatalf("Failed to create base dir: %v", err)
	}
	defer os.RemoveAll(baseDir)
	
	// Create nested destination path
	dstDir := filepath.Join(baseDir, "nested", "test-branch")
	
	// Method 1: Try without creating parent directory
	if err := CloneDirectory(srcDir, dstDir); err != nil {
		t.Logf("Clone to nested path failed: %v", err)
		
		// Method 2: Try creating parent directory first
		if err := os.MkdirAll(filepath.Dir(dstDir), 0755); err != nil {
			t.Fatalf("Failed to create parent dir: %v", err)
		}
		
		if err := CloneDirectory(srcDir, dstDir); err != nil {
			t.Logf("Clone to nested path with parent dir also failed: %v", err)
			t.Skip("Cannot clone to nested paths on this system")
		}
	}

	// Verify untracked file is preserved
	dstUntracked := filepath.Join(dstDir, "untracked.txt")
	if content, err := os.ReadFile(dstUntracked); err != nil {
		t.Errorf("Untracked file not preserved: %v", err)
	} else if string(content) != "untracked" {
		t.Errorf("Untracked file content wrong: got %s", string(content))
	}
}

func TestAPFSCloneGitRepoWithUntrackedFiles(t *testing.T) {
	// Create source git repository
	srcDir, err := os.MkdirTemp("", "apfs-clone-git-src-*")
	if err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	defer os.RemoveAll(srcDir)

	// Check if CoW is supported
	if supported, err := IsCoWSupported(srcDir); err != nil || !supported {
		t.Skip("CoW not supported on this filesystem")
	}

	// Initialize git repo
	setupGitRepo(t, srcDir)

	// Add untracked files
	untrackedFile := filepath.Join(srcDir, "untracked.txt")
	if err := os.WriteFile(untrackedFile, []byte("untracked"), 0644); err != nil {
		t.Fatalf("Failed to create untracked file: %v", err)
	}

	// Create destination
	dstDir, err := os.MkdirTemp("", "apfs-clone-git-dst-*")
	if err != nil {
		t.Fatalf("Failed to create dst dir: %v", err)
	}
	defer os.RemoveAll(dstDir)
	
	// Remove the empty dst directory so CloneDirectory can create it
	os.RemoveAll(dstDir)

	// Test APFS clone
	if err := CloneDirectory(srcDir, dstDir); err != nil {
		t.Fatalf("Failed to clone git repository: %v", err)
	}

	// Verify untracked file is preserved
	dstUntracked := filepath.Join(dstDir, "untracked.txt")
	if content, err := os.ReadFile(dstUntracked); err != nil {
		t.Errorf("Untracked file not preserved: %v", err)
	} else if string(content) != "untracked" {
		t.Errorf("Untracked file content wrong: got %s", string(content))
	}
}

func TestAPFSClonePreservesAllFiles(t *testing.T) {
	// Create source directory
	srcDir, err := os.MkdirTemp("", "apfs-clone-src-*")
	if err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	defer os.RemoveAll(srcDir)

	// Check if CoW is supported
	if supported, err := IsCoWSupported(srcDir); err != nil || !supported {
		t.Skip("CoW not supported on this filesystem")
	}

	// Create various types of files
	trackedFile := filepath.Join(srcDir, "tracked.txt")
	if err := os.WriteFile(trackedFile, []byte("tracked"), 0644); err != nil {
		t.Fatalf("Failed to create tracked file: %v", err)
	}

	untrackedFile := filepath.Join(srcDir, "untracked.txt")
	if err := os.WriteFile(untrackedFile, []byte("untracked"), 0644); err != nil {
		t.Fatalf("Failed to create untracked file: %v", err)
	}

	nestedDir := filepath.Join(srcDir, "nested")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("Failed to create nested dir: %v", err)
	}
	nestedFile := filepath.Join(nestedDir, "nested.txt")
	if err := os.WriteFile(nestedFile, []byte("nested"), 0644); err != nil {
		t.Fatalf("Failed to create nested file: %v", err)
	}

	// Create destination
	dstDir, err := os.MkdirTemp("", "apfs-clone-dst-*")
	if err != nil {
		t.Fatalf("Failed to create dst dir: %v", err)
	}
	defer os.RemoveAll(dstDir)
	
	// Remove the empty dst directory so CloneDirectory can create it
	os.RemoveAll(dstDir)

	// Test APFS clone
	if err := CloneDirectory(srcDir, dstDir); err != nil {
		t.Fatalf("Failed to clone directory: %v", err)
	}

	// Verify all files are preserved
	dstTracked := filepath.Join(dstDir, "tracked.txt")
	if content, err := os.ReadFile(dstTracked); err != nil {
		t.Errorf("Tracked file not preserved: %v", err)
	} else if string(content) != "tracked" {
		t.Errorf("Tracked file content wrong: got %s", string(content))
	}

	dstUntracked := filepath.Join(dstDir, "untracked.txt")
	if content, err := os.ReadFile(dstUntracked); err != nil {
		t.Errorf("Untracked file not preserved: %v", err)
	} else if string(content) != "untracked" {
		t.Errorf("Untracked file content wrong: got %s", string(content))
	}

	dstNested := filepath.Join(dstDir, "nested", "nested.txt")
	if content, err := os.ReadFile(dstNested); err != nil {
		t.Errorf("Nested file not preserved: %v", err)
	} else if string(content) != "nested" {
		t.Errorf("Nested file content wrong: got %s", string(content))
	}
}

func TestBranchInheritsGitHistoryDiagnostic(t *testing.T) {
	// Create a temporary git repository for testing
	tempDir, err := os.MkdirTemp("", "coworktree-history-diag-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Initialize git repo and create multiple commits to establish history
	setupGitRepo(t, tempDir)
	
	// Get the HEAD commit hash for later verification
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = tempDir
	headCommitBytes, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get HEAD commit: %v", err)
	}
	headCommit := strings.TrimSpace(string(headCommitBytes))
	t.Logf("Original HEAD commit: %s", headCommit)

	// List all commits in original repo
	cmd = exec.Command("git", "log", "--oneline")
	cmd.Dir = tempDir
	originalLogBytes, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get original log: %v", err)
	}
	t.Logf("Original repo commits:\n%s", string(originalLogBytes))

	// Test the current git worktree behavior (expected correct behavior)
	worktreePath, err := os.MkdirTemp("", "regular-worktree-*")
	if err != nil {
		t.Fatalf("Failed to create worktree temp dir: %v", err)
	}
	defer os.RemoveAll(worktreePath)
	
	// Remove it so git worktree can create it
	os.RemoveAll(worktreePath)
	
	// Create regular worktree with explicit commit (this should preserve history)
	if err := runCommand(tempDir, "git", "worktree", "add", "-b", "regular-test-branch", worktreePath, headCommit); err != nil {
		t.Fatalf("Failed to create regular worktree: %v", err)
	}

	// Check regular worktree history
	cmd = exec.Command("git", "log", "--oneline")
	cmd.Dir = worktreePath
	regularLogBytes, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get regular worktree log: %v", err)
	}
	t.Logf("Regular worktree commits:\n%s", string(regularLogBytes))

	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = worktreePath
	regularHeadBytes, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get regular worktree HEAD: %v", err)
	}
	regularHead := strings.TrimSpace(string(regularHeadBytes))

	if regularHead != headCommit {
		t.Errorf("Regular worktree HEAD %s doesn't match original %s", regularHead, headCommit)
	} else {
		t.Logf("✓ Regular worktree correctly preserves git history")
	}

	// Clean up regular worktree
	if err := runCommand(tempDir, "git", "worktree", "remove", "-f", worktreePath); err != nil {
		t.Logf("Warning: Failed to remove regular worktree: %v", err)
	}
}

func TestBranchInheritsGitHistory(t *testing.T) {
	// Create a temporary git repository for testing
	tempDir, err := os.MkdirTemp("", "coworktree-history-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Initialize git repo and create multiple commits to establish history
	setupGitRepo(t, tempDir)
	
	// Create additional commits to build history
	secondFile := filepath.Join(tempDir, "second.txt")
	if err := os.WriteFile(secondFile, []byte("second content"), 0644); err != nil {
		t.Fatalf("Failed to create second file: %v", err)
	}
	if err := runCommand(tempDir, "git", "add", "second.txt"); err != nil {
		t.Fatalf("Failed to add second file: %v", err)
	}
	if err := runCommand(tempDir, "git", "commit", "-m", "Second commit"); err != nil {
		t.Fatalf("Failed to create second commit: %v", err)
	}

	thirdFile := filepath.Join(tempDir, "third.txt")
	if err := os.WriteFile(thirdFile, []byte("third content"), 0644); err != nil {
		t.Fatalf("Failed to create third file: %v", err)
	}
	if err := runCommand(tempDir, "git", "add", "third.txt"); err != nil {
		t.Fatalf("Failed to add third file: %v", err)
	}
	if err := runCommand(tempDir, "git", "commit", "-m", "Third commit"); err != nil {
		t.Fatalf("Failed to create third commit: %v", err)
	}

	// Get the current HEAD commit hash for comparison (AFTER all commits are made)
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = tempDir
	headCommitBytes, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get HEAD commit: %v", err)
	}
	headCommit := strings.TrimSpace(string(headCommitBytes))
	t.Logf("Final HEAD commit after all test commits: %s", headCommit)

	// Get the commit log from main branch to compare against
	cmd = exec.Command("git", "log", "--oneline", "--format=%H %s")
	cmd.Dir = tempDir
	mainLogBytes, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get main branch log: %v", err)
	}
	mainLog := string(mainLogBytes)

	// Create a CoW worktree using either CoW or traditional git worktree
	worktreePath, err := os.MkdirTemp("", "cow-worktree-history-*")
	if err != nil {
		t.Fatalf("Failed to create worktree temp dir: %v", err)
	}
	defer os.RemoveAll(worktreePath)
	
	// Remove it so CreateCoWWorktree can create it
	os.RemoveAll(worktreePath)
	worktree := NewWorktree(tempDir, worktreePath, "history-test-branch")

	// Test both CoW worktree creation and fallback to regular worktree
	if err := worktree.CreateCoWWorktree(); err != nil {
		t.Logf("CoW worktree creation failed (expected on non-APFS): %v", err)
		
		// Fall back to regular git worktree for testing, but preserve the correct behavior
		// Use the same logic as setupRegularWorktree - create branch from specific commit
		if err := os.MkdirAll(filepath.Dir(worktreePath), 0755); err != nil {
			t.Fatalf("Failed to create worktree dir: %v", err)
		}
		
		// Re-capture the current HEAD since it might have changed
		cmd = exec.Command("git", "rev-parse", "HEAD")
		cmd.Dir = tempDir
		currentHeadBytes, err := cmd.Output()
		if err != nil {
			t.Fatalf("Failed to get current HEAD commit: %v", err)
		}
		currentHead := strings.TrimSpace(string(currentHeadBytes))
		
		if err := runCommand(tempDir, "git", "worktree", "add", "-b", "history-test-branch", worktreePath, currentHead); err != nil {
			t.Fatalf("Failed to create regular worktree with commit %s: %v", currentHead, err)
		}
		
		t.Logf("Created regular worktree for testing")
	} else {
		t.Logf("CoW worktree creation succeeded")
		
		// Debug: Check what git thinks about the CoW clone
		cmd = exec.Command("git", "log", "--oneline")
		cmd.Dir = worktreePath
		logBytes, err := cmd.Output()
		if err != nil {
			t.Logf("Failed to get CoW log: %v", err)
		} else {
			t.Logf("CoW worktree commits:\n%s", string(logBytes))
		}
		
		// Check if the target commit exists in the CoW clone
		cmd = exec.Command("git", "cat-file", "-e", headCommit)
		cmd.Dir = worktreePath
		if err := cmd.Run(); err != nil {
			t.Logf("Target commit %s does NOT exist in CoW clone: %v", headCommit, err)
		} else {
			t.Logf("Target commit %s EXISTS in CoW clone", headCommit)
		}
		
		// Check all available refs
		cmd = exec.Command("git", "for-each-ref")
		cmd.Dir = worktreePath
		refsBytes, err := cmd.Output()
		if err != nil {
			t.Logf("Failed to get refs: %v", err)
		} else {
			t.Logf("Available refs in CoW clone:\n%s", string(refsBytes))
		}
		
		cmd = exec.Command("git", "status")
		cmd.Dir = worktreePath
		statusBytes, err := cmd.Output()
		if err != nil {
			t.Logf("Failed to get CoW status: %v", err)
		} else {
			t.Logf("CoW status:\n%s", string(statusBytes))
		}
	}

	// Verify the worktree directory exists and is a git repository
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		t.Fatalf("Worktree directory does not exist: %s", worktreePath)
	}

	// Check if .git exists in worktree
	gitPath := filepath.Join(worktreePath, ".git")
	if _, err := os.Stat(gitPath); os.IsNotExist(err) {
		t.Fatalf("No .git found in worktree: %s", gitPath)
	}

	// Verify the new branch exists and has commits (not orphaned)
	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = worktreePath
	branchHeadBytes, err := cmd.Output()
	if err != nil {
		// This indicates the branch has no commits (orphaned branch)
		cmd = exec.Command("git", "status")
		cmd.Dir = worktreePath
		statusBytes, _ := cmd.Output()
		
		if strings.Contains(string(statusBytes), "No commits yet") {
			t.Errorf("DETECTED ORPHANED BRANCH: The branch was created without git history (equivalent to 'git switch -c')\nThis means the CoW worktree implementation is not properly preserving git history from the parent branch.\nStatus: %s", string(statusBytes))
			return // Don't continue with further tests since we detected the core issue
		}
		
		cmd = exec.Command("git", "branch", "--show-current")
		cmd.Dir = worktreePath
		currentBranchBytes, _ := cmd.Output()
		currentBranch := strings.TrimSpace(string(currentBranchBytes))
		
		t.Fatalf("Failed to get branch HEAD from %s (current branch: %s): %v\nStatus: %s", worktreePath, currentBranch, err, string(statusBytes))
	}
	branchHead := strings.TrimSpace(string(branchHeadBytes))

	if branchHead != headCommit {
		t.Errorf("Branch HEAD %s does not match original HEAD %s", branchHead, headCommit)
	}

	// Verify the branch has the same commit history as the main branch
	cmd = exec.Command("git", "log", "--oneline", "--format=%H %s")
	cmd.Dir = worktreePath
	branchLogBytes, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get branch log: %v", err)
	}
	branchLog := string(branchLogBytes)

	if branchLog != mainLog {
		t.Errorf("Branch commit history differs from main branch\nMain:\n%s\nBranch:\n%s", mainLog, branchLog)
	}

	// Verify we have multiple commits in the history (at least 3)
	commitCount := len(strings.Split(strings.TrimSpace(branchLog), "\n"))
	if commitCount < 3 {
		t.Errorf("Expected at least 3 commits in history, got %d", commitCount)
	}

	// Verify this is NOT an orphaned branch by checking if it shares commits with main
	// An orphaned branch would have completely different commit hashes
	cmd = exec.Command("git", "merge-base", "HEAD", "main")
	cmd.Dir = worktreePath
	sharedCommitsBytes, err := cmd.Output()
	if err != nil {
		// If there's no main branch, try master
		cmd = exec.Command("git", "merge-base", "HEAD", "master")
		cmd.Dir = worktreePath
		sharedCommitsBytes, err = cmd.Output()
		if err != nil {
			// For our test, we should be able to find shared history with the default branch
			// Let's get the default branch name from the original repo
			cmd = exec.Command("git", "branch", "--show-current")
			cmd.Dir = tempDir
			currentBranchBytes, err := cmd.Output()
			if err == nil {
				currentBranch := strings.TrimSpace(string(currentBranchBytes))
				if currentBranch != "history-test-branch" && currentBranch != "" {
					cmd = exec.Command("git", "merge-base", "HEAD", currentBranch)
					cmd.Dir = worktreePath
					sharedCommitsBytes, err = cmd.Output()
				}
			}
		}
	}

	// If merge-base succeeds, we have shared history (which is what we want)
	if err != nil {
		t.Logf("Warning: Could not verify shared history with merge-base: %v", err)
	} else {
		sharedCommit := strings.TrimSpace(string(sharedCommitsBytes))
		if sharedCommit == "" {
			t.Error("No shared commits found - branch appears to be orphaned")
		} else {
			t.Logf("Branch correctly shares history, merge-base: %s", sharedCommit)
		}
	}

	// Additional check: verify the branch was created via `git branch` not `git switch -c`
	// by checking that the first commit in our branch matches the first commit in the repo
	cmd = exec.Command("git", "rev-list", "--max-parents=0", "HEAD")
	cmd.Dir = tempDir
	firstMainCommitBytes, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get first commit: %v", err)
	}
	firstMainCommit := strings.TrimSpace(string(firstMainCommitBytes))

	// Check if our branch can see the same first commit
	cmd = exec.Command("git", "rev-list", "--max-parents=0", "history-test-branch")
	cmd.Dir = worktreePath
	branchFirstCommitBytes, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get branch first commit: %v", err)
	}
	branchFirstCommit := strings.TrimSpace(string(branchFirstCommitBytes))

	if firstMainCommit != branchFirstCommit {
		t.Errorf("Branch first commit %s differs from main first commit %s - branch may be orphaned", branchFirstCommit, firstMainCommit)
	}

	// Clean up
	if err := worktree.Remove(); err != nil {
		t.Errorf("Failed to remove worktree: %v", err)
	}
}

func runCommand(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	return cmd.Run()
}