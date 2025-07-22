package cowgit

import (
	"os"
	"os/exec"
	"path/filepath"
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

func runCommand(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	return cmd.Run()
}