package cowgit

import (
	"testing"
)

func TestIsAPFS(t *testing.T) {
	// Test with current directory (should work on macOS)
	result, err := isAPFS(".")
	if err != nil {
		t.Fatalf("Failed to check filesystem: %v", err)
	}
	
	// On macOS, this should typically be true
	t.Logf("Current directory is on APFS: %v", result)
	
	// Test with non-existent path
	_, err = isAPFS("/nonexistent/path")
	if err == nil {
		t.Error("Expected error for non-existent path")
	}
}

func TestCloneDirectory(t *testing.T) {
	// This test will only work on APFS
	result, err := isAPFS(".")
	if err != nil {
		t.Fatalf("Failed to check filesystem: %v", err)
	}
	
	if !result {
		t.Skip("Skipping CoW test - not on APFS filesystem")
	}
	
	// Test with non-existent source
	err = CloneDirectory("/nonexistent/source", "/tmp/test-dest")
	if err == nil {
		t.Error("Expected error for non-existent source")
	}
	
	t.Logf("CoW clone correctly failed with non-existent source: %v", err)
}

func TestIsCoWSupported(t *testing.T) {
	// Test with current directory
	result, err := IsCoWSupported(".")
	if err != nil {
		t.Fatalf("Failed to check CoW support: %v", err)
	}
	
	t.Logf("CoW supported for current directory: %v", result)
	
	// Test with non-existent path
	_, err = IsCoWSupported("/nonexistent/path")
	if err == nil {
		t.Error("Expected error for non-existent path")
	}
}