package cowgit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCoWDependencies(t *testing.T) {
	// Skip if not on APFS
	result, err := isAPFS(".")
	if err != nil {
		t.Fatalf("Failed to check filesystem: %v", err)
	}
	if !result {
		t.Skip("Skipping CoW dependency test - not on APFS filesystem")
	}

	// Create temporary directories
	tempDir, err := os.MkdirTemp("", "cow-deps-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	srcDir := filepath.Join(tempDir, "source")
	dstDir := filepath.Join(tempDir, "destination")

	// Create source directory structure that mimics a real project
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("Failed to create source dir: %v", err)
	}

	// Create package.json
	packageJson := `{
  "name": "test-project",
  "version": "1.0.0",
  "dependencies": {
    "lodash": "^4.17.21"
  }
}`
	if err := os.WriteFile(filepath.Join(srcDir, "package.json"), []byte(packageJson), 0644); err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	// Create mock node_modules with nested structure
	nodeModulesDir := filepath.Join(srcDir, "node_modules")
	if err := os.MkdirAll(nodeModulesDir, 0755); err != nil {
		t.Fatalf("Failed to create node_modules: %v", err)
	}

	// Create a mock lodash package
	lodashDir := filepath.Join(nodeModulesDir, "lodash")
	if err := os.MkdirAll(lodashDir, 0755); err != nil {
		t.Fatalf("Failed to create lodash dir: %v", err)
	}

	// Create lodash package.json
	lodashPackageJson := `{
  "name": "lodash",
  "version": "4.17.21",
  "description": "Lodash modular utilities."
}`
	if err := os.WriteFile(filepath.Join(lodashDir, "package.json"), []byte(lodashPackageJson), 0644); err != nil {
		t.Fatalf("Failed to create lodash package.json: %v", err)
	}

	// Create some JavaScript files
	if err := os.WriteFile(filepath.Join(lodashDir, "index.js"), []byte("module.exports = require('./lodash');"), 0644); err != nil {
		t.Fatalf("Failed to create lodash index.js: %v", err)
	}

	if err := os.WriteFile(filepath.Join(lodashDir, "lodash.js"), []byte("// Lodash library code here"), 0644); err != nil {
		t.Fatalf("Failed to create lodash.js: %v", err)
	}

	// Create nested dependency
	nestedDir := filepath.Join(nodeModulesDir, "nested-dep")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("Failed to create nested dep: %v", err)
	}

	if err := os.WriteFile(filepath.Join(nestedDir, "index.js"), []byte("// Nested dependency"), 0644); err != nil {
		t.Fatalf("Failed to create nested dep file: %v", err)
	}

	// Create some other common project files
	if err := os.WriteFile(filepath.Join(srcDir, "README.md"), []byte("# Test Project"), 0644); err != nil {
		t.Fatalf("Failed to create README.md: %v", err)
	}

	if err := os.WriteFile(filepath.Join(srcDir, "index.js"), []byte("const _ = require('lodash');"), 0644); err != nil {
		t.Fatalf("Failed to create index.js: %v", err)
	}

	// Create build directory
	buildDir := filepath.Join(srcDir, "build")
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		t.Fatalf("Failed to create build dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(buildDir, "app.js"), []byte("// Built application"), 0644); err != nil {
		t.Fatalf("Failed to create built app: %v", err)
	}

	t.Logf("Created source directory with %d files", countFiles(srcDir))

	// Test CoW clone
	err = CloneDirectory(srcDir, dstDir)
	if err != nil {
		t.Fatalf("CoW clone failed: %v", err)
	}

	// Verify all files were copied
	srcCount := countFiles(srcDir)
	dstCount := countFiles(dstDir)
	
	if srcCount != dstCount {
		t.Errorf("File count mismatch: source has %d files, destination has %d files", srcCount, dstCount)
	}

	// Verify specific important files
	testFiles := []string{
		"package.json",
		"node_modules/lodash/package.json",
		"node_modules/lodash/index.js",
		"node_modules/lodash/lodash.js",
		"node_modules/nested-dep/index.js",
		"README.md",
		"index.js",
		"build/app.js",
	}

	for _, file := range testFiles {
		srcFile := filepath.Join(srcDir, file)
		dstFile := filepath.Join(dstDir, file)

		// Check file exists
		if _, err := os.Stat(dstFile); os.IsNotExist(err) {
			t.Errorf("File %s not copied", file)
			continue
		}

		// Check content matches
		srcContent, err := os.ReadFile(srcFile)
		if err != nil {
			t.Errorf("Failed to read source file %s: %v", file, err)
			continue
		}

		dstContent, err := os.ReadFile(dstFile)
		if err != nil {
			t.Errorf("Failed to read destination file %s: %v", file, err)
			continue
		}

		if string(srcContent) != string(dstContent) {
			t.Errorf("Content mismatch for %s", file)
		}
	}

	t.Logf("CoW dependency test passed - all %d files copied correctly", srcCount)
}

func countFiles(dir string) int {
	count := 0
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			count++
		}
		return nil
	})
	return count
}