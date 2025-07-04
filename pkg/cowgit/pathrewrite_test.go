package cowgit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitIgnoreParsing(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "gitignore-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create .gitignore file
	gitignoreContent := `# Comments should be ignored
node_modules/
*.log
build
dist/
.env
*.tmp

# More patterns
venv/
__pycache__/
*.pyc
`
	gitignorePath := filepath.Join(tempDir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte(gitignoreContent), 0644); err != nil {
		t.Fatalf("Failed to create .gitignore: %v", err)
	}

	// Parse gitignore
	gitignore := parseGitignore(tempDir)

	// Test cases
	testCases := []struct {
		path     string
		expected bool
	}{
		{"node_modules/react/package.json", true},
		{"build", true},
		{"dist/app.js", true},
		{"app.log", true},
		{"debug.tmp", true},
		{"venv/bin/python", true},
		{"__pycache__/module.pyc", true},
		{"src/app.js", false},
		{"README.md", false},
		{"package.json", false},
		{".env", true},
		{"config.env", false},
	}

	for _, tc := range testCases {
		result := gitignore.Match(tc.path)
		if result != tc.expected {
			t.Errorf("gitignore.Match(%q) = %v, want %v", tc.path, result, tc.expected)
		}
	}
}

func TestPathRewriting(t *testing.T) {
	// Skip if not on APFS (since we need CoW for full test)
	result, err := isAPFS(".")
	if err != nil {
		t.Fatalf("Failed to check filesystem: %v", err)
	}
	if !result {
		t.Skip("Skipping path rewriting test - not on APFS filesystem")
	}

	// Create temporary directories
	tempDir, err := os.MkdirTemp("", "pathrewrite-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	srcDir := filepath.Join(tempDir, "source")
	dstDir := filepath.Join(tempDir, "destination")

	// Create source directory structure
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("Failed to create source dir: %v", err)
	}

	// Create .gitignore
	gitignoreContent := `venv/
node_modules/
*.log
build/
`
	if err := os.WriteFile(filepath.Join(srcDir, ".gitignore"), []byte(gitignoreContent), 0644); err != nil {
		t.Fatalf("Failed to create .gitignore: %v", err)
	}

	// Create venv directory (gitignored)
	venvDir := filepath.Join(srcDir, "venv")
	if err := os.MkdirAll(venvDir, 0755); err != nil {
		t.Fatalf("Failed to create venv dir: %v", err)
	}

	// Create pyvenv.cfg with absolute path
	pyvenvContent := `home = /usr/bin
include-system-site-packages = false
version = 3.9.0
executable = ` + srcDir + `/bin/python
command = ` + srcDir + `/bin/python -m venv ` + srcDir + `/venv
`
	if err := os.WriteFile(filepath.Join(venvDir, "pyvenv.cfg"), []byte(pyvenvContent), 0644); err != nil {
		t.Fatalf("Failed to create pyvenv.cfg: %v", err)
	}

	// Create activation script with absolute path
	activateContent := `#!/bin/bash
VIRTUAL_ENV="` + srcDir + `/venv"
export VIRTUAL_ENV
export PATH="$VIRTUAL_ENV/bin:$PATH"
`
	binDir := filepath.Join(venvDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("Failed to create bin dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "activate"), []byte(activateContent), 0755); err != nil {
		t.Fatalf("Failed to create activate script: %v", err)
	}

	// Create source file (not gitignored) - should not be rewritten
	srcFile := filepath.Join(srcDir, "main.py")
	srcFileContent := `#!/usr/bin/env python3
# This file should NOT be rewritten because it's not gitignored
import sys
print("Project path: ` + srcDir + `")
`
	if err := os.WriteFile(srcFile, []byte(srcFileContent), 0644); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	// Create binary file in gitignored directory (should be skipped)
	binaryContent := []byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE}
	if err := os.WriteFile(filepath.Join(venvDir, "binary.dat"), binaryContent, 0644); err != nil {
		t.Fatalf("Failed to create binary file: %v", err)
	}

	// Perform CoW clone
	if err := CloneDirectory(srcDir, dstDir); err != nil {
		t.Fatalf("CoW clone failed: %v", err)
	}

	// Run path rewriting
	if err := rewriteAbsolutePathsAsync(srcDir, dstDir); err != nil {
		t.Fatalf("Path rewriting failed: %v", err)
	}

	// Verify pyvenv.cfg was rewritten
	rewrittenPyvenv, err := os.ReadFile(filepath.Join(dstDir, "venv", "pyvenv.cfg"))
	if err != nil {
		t.Fatalf("Failed to read rewritten pyvenv.cfg: %v", err)
	}
	rewrittenContent := string(rewrittenPyvenv)
	if strings.Contains(rewrittenContent, srcDir) {
		t.Errorf("pyvenv.cfg still contains source path: %s", rewrittenContent)
	}
	if !strings.Contains(rewrittenContent, dstDir) {
		t.Errorf("pyvenv.cfg doesn't contain destination path: %s", rewrittenContent)
	}

	// Verify activate script was rewritten
	rewrittenActivate, err := os.ReadFile(filepath.Join(dstDir, "venv", "bin", "activate"))
	if err != nil {
		t.Fatalf("Failed to read rewritten activate script: %v", err)
	}
	activateContent = string(rewrittenActivate)
	if strings.Contains(activateContent, srcDir) {
		t.Errorf("activate script still contains source path: %s", activateContent)
	}
	if !strings.Contains(activateContent, dstDir) {
		t.Errorf("activate script doesn't contain destination path: %s", activateContent)
	}

	// Verify source file was NOT rewritten (not gitignored)
	unchangedSrc, err := os.ReadFile(filepath.Join(dstDir, "main.py"))
	if err != nil {
		t.Fatalf("Failed to read source file: %v", err)
	}
	unchangedContent := string(unchangedSrc)
	if !strings.Contains(unchangedContent, srcDir) {
		t.Errorf("Source file was incorrectly rewritten: %s", unchangedContent)
	}

	// Verify binary file was not modified
	unchangedBinary, err := os.ReadFile(filepath.Join(dstDir, "venv", "binary.dat"))
	if err != nil {
		t.Fatalf("Failed to read binary file: %v", err)
	}
	if len(unchangedBinary) != len(binaryContent) {
		t.Errorf("Binary file was modified")
	}

	t.Logf("Path rewriting test passed - gitignored text files rewritten, others preserved")
}

func TestIsValidText(t *testing.T) {
	testCases := []struct {
		name     string
		content  []byte
		expected bool
	}{
		{
			name:     "valid text",
			content:  []byte("Hello, world!\nThis is text."),
			expected: true,
		},
		{
			name:     "empty file",
			content:  []byte(""),
			expected: true,
		},
		{
			name:     "with null bytes",
			content:  []byte("Hello\x00world"),
			expected: false,
		},
		{
			name:     "invalid utf8",
			content:  []byte{0xFF, 0xFE, 0xFD},
			expected: false,
		},
		{
			name:     "mostly binary",
			content:  []byte{0x01, 0x02, 0x03, 0x04, 0x05, 'h', 'i'},
			expected: false,
		},
		{
			name:     "json config",
			content:  []byte(`{"name": "test", "version": "1.0.0"}`),
			expected: true,
		},
		{
			name:     "python code",
			content:  []byte("#!/usr/bin/env python\nprint('hello')"),
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isValidText(tc.content)
			if result != tc.expected {
				t.Errorf("isValidText() = %v, want %v", result, tc.expected)
			}
		})
	}
}