package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"coworktree/pkg/cowgit"
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

	// Get current working directory as repo path
	repoPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Create manager
	manager, err := cowgit.NewManager(repoPath)
	if err != nil {
		return err
	}

	// Set up create options
	opts := cowgit.CreateOptions{
		BranchName:   branchName,
		WorktreePath: createPath,
		FromCommit:   createFrom,
		NoCoW:        noCow,
		NoRewrite:    noRewrite,
		Prefix:       createPrefix,
	}

	if verbose {
		worktreePath := opts.WorktreePath
		effectiveBranchName := branchName
		if createPrefix != "" {
			effectiveBranchName = createPrefix + branchName
		}
		if worktreePath == "" {
			worktreePath = filepath.Join(repoPath, ".cow-worktrees", effectiveBranchName)
		}
		fmt.Printf("Creating worktree: %s\n", worktreePath)
		fmt.Printf("Branch: %s\n", effectiveBranchName)
		fmt.Printf("CoW enabled: %t\n", !noCow)
	}

	if dryRun {
		worktreePath := opts.WorktreePath
		effectiveBranchName := branchName
		if createPrefix != "" {
			effectiveBranchName = createPrefix + branchName
		}
		if worktreePath == "" {
			worktreePath = filepath.Join(repoPath, ".cow-worktrees", effectiveBranchName)
		}
		fmt.Printf("Would create worktree at: %s\n", worktreePath)
		fmt.Printf("Would create branch: %s\n", effectiveBranchName)
		return nil
	}

	// Create the worktree
	worktree, err := manager.Create(opts)
	if err != nil {
		return err
	}

	// Determine if CoW was used
	isCoW := !noCow
	if supported, err := manager.IsCoWSupported(); err != nil || !supported {
		isCoW = false
	}

	if isCoW {
		fmt.Printf("Created CoW worktree at: %s\n", worktree.WorktreePath)
	} else {
		fmt.Printf("Created regular worktree at: %s\n", worktree.WorktreePath)
	}

	return nil
}


func init() {
	rootCmd.AddCommand(createCmd)

	createCmd.Flags().StringVar(&createFrom, "from", "HEAD", "create from specific commit/branch")
	createCmd.Flags().StringVar(&createPrefix, "prefix", "", "branch name prefix")
	createCmd.Flags().StringVar(&createPath, "path", "", "custom worktree path")
	createCmd.Flags().BoolVar(&noRewrite, "no-rewrite", false, "skip absolute path rewriting in gitignored files")
}