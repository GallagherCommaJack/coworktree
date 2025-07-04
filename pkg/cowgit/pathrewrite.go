package cowgit

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// GitIgnore represents a parsed .gitignore file
type GitIgnore struct {
	patterns []string
}

// parseGitignore reads and parses .gitignore file
func parseGitignore(repoPath string) *GitIgnore {
	gitignorePath := filepath.Join(repoPath, ".gitignore")
	file, err := os.Open(gitignorePath)
	if err != nil {
		// No .gitignore file, return empty gitignore
		return &GitIgnore{patterns: []string{}}
	}
	defer file.Close()

	var patterns []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line != "" && !strings.HasPrefix(line, "#") {
			patterns = append(patterns, line)
		}
	}

	return &GitIgnore{patterns: patterns}
}

// Match checks if a relative path matches any gitignore pattern
func (g *GitIgnore) Match(relPath string) bool {
	for _, pattern := range g.patterns {
		if matchPattern(pattern, relPath) {
			return true
		}
	}
	return false
}

// matchPattern implements basic gitignore pattern matching
func matchPattern(pattern, path string) bool {
	// Handle directory patterns (ending with /)
	if strings.HasSuffix(pattern, "/") {
		pattern = strings.TrimSuffix(pattern, "/")
		// Check if path is in this directory
		return strings.HasPrefix(path, pattern+"/") || path == pattern
	}

	// Handle wildcard patterns
	if strings.Contains(pattern, "*") {
		return matchWildcard(pattern, path)
	}

	// Exact match or directory match
	return path == pattern || strings.HasPrefix(path, pattern+"/")
}

// matchWildcard implements basic wildcard matching
func matchWildcard(pattern, path string) bool {
	// Simple wildcard implementation - can be enhanced
	if pattern == "*" {
		return true
	}
	
	// Handle *.extension patterns
	if strings.HasPrefix(pattern, "*.") {
		ext := pattern[1:]
		return strings.HasSuffix(path, ext)
	}
	
	// Handle prefix* patterns
	if strings.HasSuffix(pattern, "*") {
		prefix := pattern[:len(pattern)-1]
		return strings.HasPrefix(path, prefix)
	}
	
	// For more complex patterns, fall back to simple contains
	wildcardParts := strings.Split(pattern, "*")
	if len(wildcardParts) == 2 {
		return strings.HasPrefix(path, wildcardParts[0]) && strings.HasSuffix(path, wildcardParts[1])
	}
	
	return false
}

// rewriteAbsolutePathsAsync rewrites absolute paths in gitignored text files using adaptive worker pool
func rewriteAbsolutePathsAsync(srcDir, dstDir string) error {
	gitignore := parseGitignore(srcDir)
	
	// Create adaptive worker pool
	pool := NewWorkerPool(srcDir, dstDir, gitignore)
	controller := NewPoolController(pool)
	
	// Start pool and controller
	pool.Start()
	controller.Start()
	
	// Clean up when done
	defer func() {
		controller.Stop()
		pool.Stop()
	}()
	
	// Submit all files for processing
	walkErr := filepath.Walk(dstDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			pool.Submit(path)
		}
		return nil
	})
	
	if walkErr != nil {
		return walkErr
	}
	
	// Wait for any errors
	select {
	case err := <-pool.Error():
		if err != nil {
			return err
		}
	default:
		// No errors
	}
	
	return nil
}

// isValidText checks if content is valid UTF-8 text and not binary
func isValidText(content []byte) bool {
	// Check if valid UTF-8
	if !utf8.Valid(content) {
		return false
	}

	// Check for null bytes (common in binary files)
	if bytes.Contains(content, []byte{0}) {
		return false
	}

	// Additional heuristics for binary detection
	// Check percentage of printable characters
	printableCount := 0
	for _, b := range content {
		if b >= 32 && b <= 126 || b == '\t' || b == '\n' || b == '\r' {
			printableCount++
		}
	}

	// If less than 95% printable, consider it binary
	if len(content) > 0 && float64(printableCount)/float64(len(content)) < 0.95 {
		return false
	}

	return true
}