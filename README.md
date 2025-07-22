# CoWorktree

A Copy-on-Write Git Worktree Manager that combines filesystem-level CoW features with git worktrees to create instant, fully-featured development environments.

## Features

- **Instant environment setup**: Create working copies of projects in ~1 second instead of 10-30+ seconds
- **Complete isolation**: Each worktree can modify dependencies without affecting others
- **Zero manual setup**: No need to run `npm install`, `pip install`, `go mod download`, etc.
- **Git integration**: Proper git worktree management with branch tracking
- **Full compatibility**: Drop-in replacement for `git worktree` commands
- **Cross-platform**: Support macOS (APFS) and Linux (overlayfs - coming soon)

## Installation

```bash
go build -o coworktree
```

## Usage

CoWorktree is fully compatible with `git worktree` commands - just replace `git worktree` with `coworktree`:

### Add a new CoW worktree

```bash
# Create worktree at path (auto-generates branch name)
coworktree add ../feature-work

# Create worktree with specific branch name
coworktree add -b my-feature ../feature-work

# Create from specific commit
coworktree add ../hotfix abc123

# Create in temp directory (if no path specified)
coworktree add -b experiment
```

This will:
1. Create a CoW clone of your entire project (including `node_modules`, build artifacts, etc.)
2. Create a new git branch in the worktree
3. Register the worktree with git
4. Preserve all untracked and gitignored files

### List all worktrees

```bash
coworktree list
# Forwards directly to: git worktree list
```

### Remove a worktree

```bash
coworktree remove ../feature-work
# Forwards directly to: git worktree remove ../feature-work
```

### Global flags

- `--verbose, -v`: Enable verbose logging
- `--dry-run`: Show what would be done without executing
- `--no-cow`: Force traditional git worktree (skip CoW)
- `--no-rewrite`: Skip absolute path rewriting in gitignored files

### As a Go Library

```go
package main

import (
    "fmt"
    "log"
    
    "coworktree/pkg/cowgit"
)

func main() {
    // Create a new CoW worktree directly
    repoPath := "."
    worktreePath := "/tmp/my-feature"
    branchName := "my-feature"
    
    worktree := cowgit.NewWorktree(repoPath, worktreePath, branchName)
    
    // Check if CoW is supported
    if supported, err := cowgit.IsCoWSupported(repoPath); err == nil && supported {
        // Create CoW worktree
        if err := worktree.CreateCoWWorktree(); err != nil {
            log.Fatal(err)
        }
        fmt.Printf("Created CoW worktree at: %s\n", worktree.WorktreePath)
    } else {
        // Fall back to regular worktree
        if err := worktree.CreateFromExistingBranch(); err != nil {
            log.Fatal(err)
        }
        fmt.Printf("Created regular worktree at: %s\n", worktree.WorktreePath)
    }
    
    // List all worktrees
    worktrees, err := cowgit.ListWorktrees(repoPath)
    if err != nil {
        log.Fatal(err)
    }
    
    for _, wt := range worktrees {
        fmt.Printf("Branch: %s, Path: %s\n", wt.Branch, wt.Path)
    }
}
```

## Platform Support

### macOS (APFS)
- Uses `clonefile()` syscall for true copy-on-write
- Instant cloning regardless of project size
- Requires APFS filesystem (default on modern macOS)

### Linux (overlayfs)
- Coming soon
- Will use kernel overlayfs for CoW functionality

### Fallback
- Automatically falls back to traditional `git worktree` on unsupported platforms
- Graceful degradation ensures compatibility everywhere

## How It Works

CoWorktree leverages filesystem-level copy-on-write features to create instant copies of your entire project directory, including:

- Source code
- Dependencies (`node_modules`, `venv`, `vendor`, etc.)
- Build artifacts
- IDE configuration
- Any other project files

The CoW clone shares storage with the original until files are modified, making it extremely space-efficient while providing complete isolation.

## Use Cases

- **Feature development**: Quickly spin up isolated environments for different features
- **Experimentation**: Test dependency updates without affecting main environment
- **Parallel work**: Multiple developers/agents working on same project simultaneously
- **Code review**: Quickly checkout PRs with full working environment
- **CI/CD**: Faster build environments with pre-installed dependencies

## Performance

On a typical Node.js project with 18k+ files in `node_modules`:
- Traditional `git worktree` + `npm install`: 30-60 seconds
- CoWorktree: <2 seconds

## Testing

```bash
go test ./pkg/cowgit -v
```

Tests cover:
- CoW functionality on APFS
- Git worktree integration
- Preservation of untracked and gitignored files
- Large project handling
- Cross-platform compatibility

## Contributing

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Ensure all tests pass
5. Submit a pull request

## License

MIT License