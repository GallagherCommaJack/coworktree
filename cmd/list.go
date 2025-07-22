package cmd

import (
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all worktrees",
	Long:  `List all worktrees. This forwards directly to 'git worktree list'.`,
	RunE:  listWorktrees,
}

func listWorktrees(cmd *cobra.Command, args []string) error {
	if err := checkGitRepo(); err != nil {
		return err
	}

	// Forward to git worktree list with all arguments
	gitCmd := exec.Command("git", append([]string{"worktree", "list"}, args...)...)
	gitCmd.Stdout = os.Stdout
	gitCmd.Stderr = os.Stderr
	return gitCmd.Run()
}

func init() {
	rootCmd.AddCommand(listCmd)
}