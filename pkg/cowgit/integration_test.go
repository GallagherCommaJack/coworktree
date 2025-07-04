package cowgit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCoWIntegration(t *testing.T) {
	// Skip if not on APFS
	result, err := isAPFS(".")
	if err != nil {
		t.Fatalf("Failed to check filesystem: %v", err)
	}
	if !result {
		t.Skip("Skipping CoW integration test - not on APFS filesystem")
	}

	// Create temporary directories
	tempDir, err := os.MkdirTemp("", "cow-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	srcDir := filepath.Join(tempDir, "source")
	dstDir := filepath.Join(tempDir, "destination")

	// Create source directory with some content
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("Failed to create source dir: %v", err)
	}

	// Create some test files
	testFile := filepath.Join(srcDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create subdirectory
	subDir := filepath.Join(srcDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	subFile := filepath.Join(subDir, "subfile.txt")
	if err := os.WriteFile(subFile, []byte("sub content"), 0644); err != nil {
		t.Fatalf("Failed to create subfile: %v", err)
	}

	// Test CoW clone
	err = CloneDirectory(srcDir, dstDir)
	if err != nil {
		t.Fatalf("CoW clone failed: %v", err)
	}

	// Verify destination exists
	if _, err := os.Stat(dstDir); os.IsNotExist(err) {
		t.Error("Destination directory doesn't exist")
	}

	// Verify files were copied
	if _, err := os.Stat(filepath.Join(dstDir, "test.txt")); os.IsNotExist(err) {
		t.Error("Test file not copied")
	}

	if _, err := os.Stat(filepath.Join(dstDir, "subdir", "subfile.txt")); os.IsNotExist(err) {
		t.Error("Subdirectory file not copied")
	}

	// Verify content
	content, err := os.ReadFile(filepath.Join(dstDir, "test.txt"))
	if err != nil {
		t.Fatalf("Failed to read copied file: %v", err)
	}
	if string(content) != "test content" {
		t.Errorf("File content mismatch: got %q, want %q", string(content), "test content")
	}

	t.Logf("CoW integration test passed - successfully cloned directory structure")
}