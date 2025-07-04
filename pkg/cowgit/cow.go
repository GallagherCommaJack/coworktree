package cowgit

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

// CloneDirectory creates a copy-on-write clone of a directory using platform-specific methods
func CloneDirectory(src, dst string) error {
	switch runtime.GOOS {
	case "darwin":
		return cloneDirectoryAPFS(src, dst)
	case "linux":
		// TODO: Implement overlayfs for Linux
		return errors.New("linux CoW not yet implemented")
	default:
		return fmt.Errorf("copy-on-write not supported on %s", runtime.GOOS)
	}
}

// IsCoWSupported checks if copy-on-write is supported for the given path
func IsCoWSupported(path string) (bool, error) {
	switch runtime.GOOS {
	case "darwin":
		return isAPFS(path)
	case "linux":
		// TODO: Check overlayfs availability
		return false, errors.New("linux CoW not yet implemented")
	default:
		return false, nil
	}
}

// cloneDirectoryAPFS creates a CoW clone using APFS clonefile on macOS
func cloneDirectoryAPFS(src, dst string) error {
	// Check if we're on APFS
	if isAPFS, err := isAPFS(src); err != nil {
		return fmt.Errorf("failed to check filesystem: %w", err)
	} else if !isAPFS {
		return errors.New("copy-on-write requires APFS filesystem")
	}

	// Clone the directory
	if err := unix.Clonefile(src, dst, unix.CLONE_NOFOLLOW); err != nil {
		// Handle cases where clonefile isn't supported
		if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EXDEV) {
			return fmt.Errorf("clonefile not supported: %w", err)
		}
		return fmt.Errorf("clonefile failed: %w", err)
	}

	return nil
}

// isAPFS checks if the given path is on an APFS filesystem
func isAPFS(path string) (bool, error) {
	var stat unix.Statfs_t
	err := unix.Statfs(path, &stat)
	if err != nil {
		return false, err
	}

	// Convert filesystem name from C string
	fstype := unix.ByteSliceToString((*[256]byte)(unsafe.Pointer(&stat.Fstypename[0]))[:])
	return fstype == "apfs", nil
}