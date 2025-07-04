package cowgit

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

// rewriteAbsolutePathsAsync rewrites absolute paths in gitignored text files
func rewriteAbsolutePathsAsync(srcDir, dstDir string) error {
	numWorkers := runtime.NumCPU()
	fileChan := make(chan string, 1000)
	errChan := make(chan error, 1)
	var wg sync.WaitGroup

	gitignore := parseGitignore(srcDir)
	srcDirBytes := []byte(srcDir)
	dstDirBytes := []byte(dstDir)

	// File processing workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range fileChan {
				relPath, err := filepath.Rel(dstDir, path)
				if err != nil {
					continue
				}

				// Filter: gitignored files only
				if !gitignore.Match(relPath) {
					continue
				}

				// Read file and check if it's text
				content, err := os.ReadFile(path)
				if err != nil {
					continue
				}

				// Skip binary files
				if !isValidText(content) {
					continue
				}

				// Replace srcDir with dstDir
				if updated := bytes.ReplaceAll(content, srcDirBytes, dstDirBytes); !bytes.Equal(content, updated) {
					if err := os.WriteFile(path, updated, 0644); err != nil {
						select {
						case errChan <- err:
						default:
						}
						return
					}
				}
			}
		}()
	}

	// File discovery worker
	go func() {
		defer close(fileChan)
		filepath.Walk(dstDir, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				fileChan <- path
			}
			return nil
		})
	}()

	// Wait for completion
	go func() {
		wg.Wait()
		close(errChan)
	}()

	// Return first error if any
	for err := range errChan {
		if err != nil {
			return err
		}
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