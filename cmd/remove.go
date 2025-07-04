package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"cowtree/pkg/cowgit"
	"github.com/spf13/cobra"
)

var (
	keepBranch bool
	force      bool
)

// removeCmd represents the remove command
var removeCmd = &cobra.Command{
	Use:   "remove <branch-name>",
	Short: "Remove a CoW worktree",
	Long: `Remove a copy-on-write worktree and optionally its associated branch.

This command will:
1. Remove the worktree directory
2. Clean up git worktree references
3. Optionally remove the associated branch (unless --keep-branch is specified)`,
	Args: cobra.ExactArgs(1),
	RunE: removeWorktree,
}

func removeWorktree(cmd *cobra.Command, args []string) error {
	if err := checkGitRepo(); err != nil {
		return err
	}

	branchName := args[0]

	// Get current working directory as repo path
	repoPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Try to find the worktree
	worktrees, err := cowgit.ListWorktrees(repoPath)
	if err != nil {
		return fmt.Errorf("failed to list worktrees: %w", err)
	}

	var worktreePath string
	for _, wt := range worktrees {
		if wt.Branch == branchName {
			worktreePath = wt.Path
			break
		}
	}

	if worktreePath == "" {
		// Try default path
		worktreePath = filepath.Join(repoPath, ".cow-worktrees", branchName)
	}

	if verbose {
		fmt.Printf("Removing worktree: %s\n", worktreePath)
		fmt.Printf("Branch: %s\n", branchName)
		fmt.Printf("Keep branch: %t\n", keepBranch)
	}

	if dryRun {
		fmt.Printf("Would remove worktree at: %s\n", worktreePath)
		if !keepBranch {
			fmt.Printf("Would remove branch: %s\n", branchName)
		}
		return nil
	}

	// Check if worktree exists
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		if !force {
			return fmt.Errorf("worktree does not exist: %s", worktreePath)
		}
		fmt.Printf("Worktree does not exist, continuing with branch cleanup\n")
	}

	// Create worktree instance
	worktree := cowgit.NewWorktree(repoPath, worktreePath, branchName)

	// Remove the worktree
	if keepBranch {
		if err := worktree.Remove(); err != nil {
			if !force {
				return fmt.Errorf("failed to remove worktree: %w", err)
			}
			fmt.Printf("Warning: Failed to remove worktree: %v\n", err)
		}
		fmt.Printf("Removed worktree (kept branch): %s\n", worktreePath)
	} else {
		if err := worktree.RemoveWithBranch(); err != nil {
			if !force {
				return fmt.Errorf("failed to remove worktree and branch: %w", err)
			}
			fmt.Printf("Warning: Failed to remove worktree and branch: %v\n", err)
		}
		fmt.Printf("Removed worktree and branch: %s\n", worktreePath)
	}

	return nil
}

func init() {
	rootCmd.AddCommand(removeCmd)

	removeCmd.Flags().BoolVar(&keepBranch, "keep-branch", false, "don't delete the git branch")
	removeCmd.Flags().BoolVar(&force, "force", false, "remove even if worktree has uncommitted changes")
}