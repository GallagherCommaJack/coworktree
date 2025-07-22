package cmd

import (
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// removeCmd represents the remove command
var removeCmd = &cobra.Command{
	Use:   "remove <worktree>",
	Short: "Remove a worktree",
	Long:  `Remove a worktree. This forwards directly to 'git worktree remove'.`,
	RunE:  removeWorktree,
}

func removeWorktree(cmd *cobra.Command, args []string) error {
	if err := checkGitRepo(); err != nil {
		return err
	}

	// Forward to git worktree remove with all arguments
	gitCmd := exec.Command("git", append([]string{"worktree", "remove"}, args...)...)
	gitCmd.Stdout = os.Stdout
	gitCmd.Stderr = os.Stderr
	return gitCmd.Run()
}

func init() {
	rootCmd.AddCommand(removeCmd)
}