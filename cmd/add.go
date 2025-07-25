package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"coworktree/pkg/cowgit"
	"github.com/spf13/cobra"
)

var (
	branchFlag string
	noRewrite  bool
)

// addCmd represents the add command
var addCmd = &cobra.Command{
	Use:   "add <path> [<commit-ish>]",
	Short: "Add a new CoW worktree",
	Long: `Add a new copy-on-write worktree. Compatible with git worktree add syntax.

This command will:
1. Create a CoW clone of the current repository (if supported)
2. Create a new git branch in the worktree
3. Register the worktree with git

If CoW is not supported, it will fall back to traditional git worktree.`,
	Args: cobra.MinimumNArgs(1),
	RunE: addWorktree,
}

func addWorktree(cmd *cobra.Command, args []string) error {
	if err := checkGitRepo(); err != nil {
		return err
	}

	// Parse arguments like git worktree add
	worktreePath := args[0]
	
	// Use branch flag if provided, otherwise auto-generate from path
	branchName := branchFlag
	if branchName == "" {
		branchName = filepath.Base(worktreePath)
	}
	
	// If commit-ish is provided, use it
	var commitish string
	if len(args) > 1 {
		commitish = args[1]
	}

	// Get current working directory as repo path
	repoPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
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

	// Create worktree instance
	worktree := cowgit.NewWorktreeWithOptions(repoPath, worktreePath, branchName, noRewrite)

	// Try CoW first, fall back to regular if not supported or disabled
	isCoW := false
	if !noCow {
		if supported, err := cowgit.IsCoWSupported(repoPath); err == nil && supported {
			if err := worktree.CreateCoWWorktree(); err == nil {
				isCoW = true
			}
		}
	}

	// Fall back to regular worktree if CoW failed or was disabled
	if !isCoW {
		// Build git worktree add command
		gitArgs := []string{"worktree", "add"}
		if branchFlag != "" {
			gitArgs = append(gitArgs, "-b", branchName)
		}
		gitArgs = append(gitArgs, worktreePath)
		if commitish != "" {
			gitArgs = append(gitArgs, commitish)
		}
		
		cmd := exec.Command("git", gitArgs...)
		cmd.Dir = repoPath
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to create regular worktree: %w", err)
		}
	}

	if isCoW {
		fmt.Printf("Created CoW worktree at: %s\n", worktreePath)
	} else {
		fmt.Printf("Created regular worktree at: %s\n", worktreePath)
	}

	return nil
}


func init() {
	rootCmd.AddCommand(addCmd)

	addCmd.Flags().StringVarP(&branchFlag, "branch", "b", "", "create a new branch")
	addCmd.Flags().BoolVar(&noRewrite, "no-rewrite", false, "skip absolute path rewriting in gitignored files")
}