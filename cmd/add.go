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
	branchFlag      string
	enableRewrite   bool
	forceProgress   bool
	parallelCoW     bool
	forceParallel   bool
	parallelDepth   int
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

If CoW is not supported, it will fall back to traditional git worktree.

Performance note: By default, absolute path rewriting is disabled for speed.
Use --rewrite-paths if you need build artifacts (venv, node_modules) to work correctly.`,
	Args: cobra.MinimumNArgs(1),
	RunE: addWorktree,
}

func addWorktree(cmd *cobra.Command, args []string) error {
	if err := checkGitRepo(); err != nil {
		return err
	}

	// Parse arguments like git worktree add
	worktreePath := args[0]
	
	// Canonicalize and absolutize the worktree path immediately
	absWorktreePath, err := filepath.Abs(worktreePath)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path for %s: %w", worktreePath, err)
	}
	
	// Also canonicalize to resolve any symlinks in the path
	// Note: EvalSymlinks will fail if the path doesn't exist yet, so we need to handle parent directories
	canonicalWorktreePath, err := canonicalizePath(absWorktreePath)
	if err != nil {
		return fmt.Errorf("failed to canonicalize worktree path %s: %w", absWorktreePath, err)
	}
	worktreePath = canonicalWorktreePath
	
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

	// Get current working directory as repo path and canonicalize it
	repoPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}
	
	// Canonicalize the repo path to resolve any symlinks
	canonicalRepoPath, err := filepath.EvalSymlinks(repoPath)
	if err != nil {
		return fmt.Errorf("failed to canonicalize repo path %s: %w", repoPath, err)
	}
	repoPath = canonicalRepoPath

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

	// Create worktree instance (invert the logic - disable rewrite by default)
	worktree := cowgit.NewWorktreeWithAllOptions(repoPath, worktreePath, branchName, !enableRewrite, parallelCoW, forceParallel, parallelDepth)

	// Create progress tracker for TTY output (shows in interactive mode or when forced)
	progress := cowgit.NewProgressTracker(forceProgress)

	// Try CoW first, fall back to regular if not supported or disabled
	isCoW := false
	if !noCow {
		if supported, err := cowgit.IsCoWSupported(repoPath); err == nil && supported {
			if err := worktree.CreateCoWWorktreeWithProgress(progress); err == nil {
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


// canonicalizePath resolves symlinks in a path, handling the case where the final component doesn't exist yet
func canonicalizePath(path string) (string, error) {
	// Try to canonicalize the full path first
	if canonical, err := filepath.EvalSymlinks(path); err == nil {
		return canonical, nil
	}
	
	// If that fails (path doesn't exist), canonicalize the parent and append the final component
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	
	// Canonicalize the parent directory
	canonicalDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		// If parent doesn't exist either, just return the original absolute path
		return path, nil
	}
	
	return filepath.Join(canonicalDir, base), nil
}

func init() {
	rootCmd.AddCommand(addCmd)

	addCmd.Flags().StringVarP(&branchFlag, "branch", "b", "", "create a new branch")
	addCmd.Flags().BoolVar(&enableRewrite, "rewrite-paths", false, "enable absolute path rewriting in gitignored files (slow but fixes build artifacts)")
	addCmd.Flags().BoolVar(&forceProgress, "progress", false, "show progress indicators even in non-interactive mode")
	addCmd.Flags().BoolVar(&parallelCoW, "parallel-cow", false, "experimental: use parallel file-level CoW instead of atomic directory clone")
	addCmd.Flags().BoolVar(&forceParallel, "force-parallel", false, "force parallel CoW even when atomic clone would work (for testing)")
	addCmd.Flags().IntVar(&parallelDepth, "parallel-depth", 0, "recurse to depth N and clone each subdirectory atomically in parallel (0=disabled)")
}