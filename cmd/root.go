package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	verbose bool
	dryRun  bool
	noCow   bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "coworktree",
	Short: "A Copy-on-Write Git Worktree Manager",
	Long: `coworktree combines copy-on-write filesystem features with git worktrees 
to create instant, fully-featured development environments.

Features:
- Instant environment setup using CoW (APFS on macOS, overlayfs on Linux)
- Complete isolation with shared dependencies
- Proper git worktree integration
- Cross-platform support`,
	Version: "0.1.0",
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose logging")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "show what would be done without executing")
	rootCmd.PersistentFlags().BoolVar(&noCow, "no-cow", false, "force traditional git worktree (skip CoW)")
	
	// Add benchmark command
	rootCmd.AddCommand(benchmarkCmd)
}

// checkGitRepo verifies we're in a git repository
func checkGitRepo() error {
	if _, err := os.Stat(".git"); err != nil {
		return fmt.Errorf("not in a git repository (or any parent directories)")
	}
	return nil
}