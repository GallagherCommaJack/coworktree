package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"cowtree/pkg/cowgit"
	"github.com/spf13/cobra"
)

var (
	createFrom   string
	createPrefix string
	createPath   string
	noRewrite    bool
)

// createCmd represents the create command
var createCmd = &cobra.Command{
	Use:   "create <branch-name>",
	Short: "Create a new CoW worktree",
	Long: `Create a new copy-on-write worktree with the specified branch name.

This command will:
1. Create a CoW clone of the current repository (if supported)
2. Create a new git branch in the worktree
3. Register the worktree with git

If CoW is not supported, it will fall back to traditional git worktree.`,
	Args: cobra.ExactArgs(1),
	RunE: createWorktree,
}

func createWorktree(cmd *cobra.Command, args []string) error {
	if err := checkGitRepo(); err != nil {
		return err
	}

	branchName := args[0]
	if createPrefix != "" {
		branchName = createPrefix + branchName
	}

	// Get current working directory as repo path
	repoPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Determine worktree path
	var worktreePath string
	if createPath != "" {
		worktreePath = createPath
	} else {
		worktreePath = filepath.Join(repoPath, ".cow-worktrees", branchName)
	}

	if verbose {
		fmt.Printf("Creating worktree: %s\n", worktreePath)
		fmt.Printf("Branch: %s\n", branchName)
		fmt.Printf("CoW enabled: %t\n", !noCow)
	}

	if dryRun {
		fmt.Printf("Would create worktree at: %s\n", worktreePath)
		fmt.Printf("Would create branch: %s\n", branchName)
		return nil
	}

	// Create the worktree
	worktree := cowgit.NewWorktreeWithOptions(repoPath, worktreePath, branchName, noRewrite)

	// Check if CoW is supported and enabled
	if !noCow {
		if supported, err := cowgit.IsCoWSupported(repoPath); err != nil {
			if verbose {
				fmt.Printf("Warning: Failed to check CoW support: %v\n", err)
			}
		} else if supported {
			if verbose {
				fmt.Printf("CoW supported, creating CoW worktree\n")
			}
			if err := worktree.CreateCoWWorktree(); err != nil {
				if verbose {
					fmt.Printf("CoW failed, falling back to regular worktree: %v\n", err)
				}
				return createRegularWorktree(worktree)
			}
			fmt.Printf("Created CoW worktree at: %s\n", worktreePath)
			return nil
		}
	}

	// Fall back to regular worktree
	if verbose {
		fmt.Printf("Creating regular worktree\n")
	}
	return createRegularWorktree(worktree)
}

func createRegularWorktree(worktree *cowgit.Worktree) error {
	// For regular worktree, we need to use git commands directly
	// since our CoW implementation expects to clone the whole repo
	if err := os.MkdirAll(filepath.Dir(worktree.WorktreePath), 0755); err != nil {
		return fmt.Errorf("failed to create worktree directory: %w", err)
	}

	// Use git worktree add directly
	if err := runGitCommand(worktree.RepoPath, "worktree", "add", "-b", worktree.BranchName, worktree.WorktreePath); err != nil {
		return fmt.Errorf("failed to create regular worktree: %w", err)
	}

	fmt.Printf("Created regular worktree at: %s\n", worktree.WorktreePath)
	return nil
}

func init() {
	rootCmd.AddCommand(createCmd)

	createCmd.Flags().StringVar(&createFrom, "from", "HEAD", "create from specific commit/branch")
	createCmd.Flags().StringVar(&createPrefix, "prefix", "", "branch name prefix")
	createCmd.Flags().StringVar(&createPath, "path", "", "custom worktree path")
	createCmd.Flags().BoolVar(&noRewrite, "no-rewrite", false, "skip absolute path rewriting in gitignored files")
}