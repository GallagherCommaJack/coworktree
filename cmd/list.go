package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"cowtree/pkg/cowgit"
	"github.com/spf13/cobra"
)

var (
	listFormat   string
	showStats    bool
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all CoW worktrees",
	Long: `List all copy-on-write worktrees in the current repository.

Shows information about each worktree including:
- Worktree path
- Branch name
- HEAD commit
- Optionally disk usage statistics (with --show-stats)`,
	RunE: listWorktrees,
}

func listWorktrees(cmd *cobra.Command, args []string) error {
	if err := checkGitRepo(); err != nil {
		return err
	}

	// Get current working directory as repo path
	repoPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Get list of worktrees
	worktrees, err := cowgit.ListWorktrees(repoPath)
	if err != nil {
		return fmt.Errorf("failed to list worktrees: %w", err)
	}

	// Filter to only show CoW worktrees (those in .cow-worktrees directory)
	var cowWorktrees []cowgit.WorktreeInfo
	for _, wt := range worktrees {
		if strings.Contains(wt.Path, ".cow-worktrees") || wt.Path != repoPath {
			cowWorktrees = append(cowWorktrees, wt)
		}
	}

	switch listFormat {
	case "json":
		return outputJSON(cowWorktrees)
	case "compact":
		return outputCompact(cowWorktrees)
	default:
		return outputTable(cowWorktrees, repoPath)
	}
}

func outputTable(worktrees []cowgit.WorktreeInfo, repoPath string) error {
	if len(worktrees) == 0 {
		fmt.Println("No CoW worktrees found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()

	// Header
	if showStats {
		fmt.Fprintln(w, "BRANCH\tPATH\tHEAD\tSIZE")
	} else {
		fmt.Fprintln(w, "BRANCH\tPATH\tHEAD")
	}

	// Data rows
	for _, wt := range worktrees {
		relativePath, _ := filepath.Rel(repoPath, wt.Path)
		shortHEAD := wt.HEAD
		if len(shortHEAD) > 8 {
			shortHEAD = shortHEAD[:8]
		}

		if showStats {
			size := getDirSize(wt.Path)
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", wt.Branch, relativePath, shortHEAD, formatSize(size))
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\n", wt.Branch, relativePath, shortHEAD)
		}
	}

	return nil
}

func outputJSON(worktrees []cowgit.WorktreeInfo) error {
	type jsonWorktree struct {
		Branch string `json:"branch"`
		Path   string `json:"path"`
		HEAD   string `json:"head"`
		Size   int64  `json:"size,omitempty"`
	}

	var jsonWorktrees []jsonWorktree
	for _, wt := range worktrees {
		jw := jsonWorktree{
			Branch: wt.Branch,
			Path:   wt.Path,
			HEAD:   wt.HEAD,
		}
		if showStats {
			jw.Size = getDirSize(wt.Path)
		}
		jsonWorktrees = append(jsonWorktrees, jw)
	}

	output := map[string]interface{}{
		"worktrees": jsonWorktrees,
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func outputCompact(worktrees []cowgit.WorktreeInfo) error {
	if len(worktrees) == 0 {
		fmt.Println("No CoW worktrees found")
		return nil
	}

	for _, wt := range worktrees {
		shortHEAD := wt.HEAD
		if len(shortHEAD) > 8 {
			shortHEAD = shortHEAD[:8]
		}
		fmt.Printf("%s (%s)\n", wt.Branch, shortHEAD)
	}

	return nil
}

func getDirSize(path string) int64 {
	var size int64
	filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func init() {
	rootCmd.AddCommand(listCmd)

	listCmd.Flags().StringVar(&listFormat, "format", "table", "output format: table, json, compact")
	listCmd.Flags().BoolVar(&showStats, "show-stats", false, "include disk usage statistics")
}