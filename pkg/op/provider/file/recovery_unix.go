//go:build unix

package file

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// getRecoveryBase finds the mount point root to ensure a zero-copy Rename.
//
// Parameters:
//   - absolutePath: Absolute path of the file to recover
//
// Returns:
//   - string: The recovery base path
//   - error: Error if any occurred during recovery base calculation
func (p *Provider) getRecoveryBase(absolutePath string) (string, error) {

	const folderName = ".devlore_recovery"

	sourcePath, sourceInfo, err := getFirstExistingAncestor(absolutePath)

	if err != nil {
		return "", err
	}

	if cacheDir, err := os.UserCacheDir(); err == nil {

		recoveryDirectoryPath := filepath.Join(cacheDir, "devlore", "recovery")
		_, targetInfo, err := getFirstExistingAncestor(recoveryDirectoryPath)

		if err == nil && targetInfo.IsDir() {

			sameDevice, err := isSameDevice(sourceInfo, targetInfo)
			if err != nil {
				return "", err
			}
			if sameDevice {
				return recoveryDirectoryPath, nil
			}

		}
	}

	mountPoint, err := findMountPoint(sourcePath, sourceInfo)

	if err != nil {
		return "", err
	}

	return filepath.Join(mountPoint, folderName), nil
}

// findMountPoint returns the mount point of the filesystem containing the given path by traversing upward until the
// device ID changes or the root is reached.
//
// Parameters:
//   - path: Path to start traversing from
//   - fileInfo: File info of the path
//
// Returns:
//   - string: The mount point path
//   - err: Error if any occurred during the mount point search
func findMountPoint(path string, fileInfo os.FileInfo) (string, error) {

	stat, ok := fileInfo.Sys().(*syscall.Stat_t)

	if !ok {
		return "", fmt.Errorf("unable to determine device for %q: expected *syscall.Stat_t, got %T: %w", fileInfo.Name(), fileInfo.Sys(), fs.ErrInvalid)
	}

	device := stat.Dev

	for {

		parent := filepath.Dir(path)

		if parent == path {
			return path, nil
		}

		parentInfo, err := os.Stat(parent)

		if err != nil {
			return path, nil
		}

		stat, ok := parentInfo.Sys().(*syscall.Stat_t)

		if !ok {
			return path, nil
		}

		if stat.Dev != device {
			return path, nil
		}

		path = parent
	}
}

// getFirstExistingAncestor walks up from the given path until it finds a directory that exists.
//
// Parameters:
//   - path: Path to start walking from
//
// Returns:
//   - string: The first existing ancestor path
//   - os.FileInfo: File info of the first existing ancestor
//   - error: Error if any occurred during the ancestor walk
func getFirstExistingAncestor(path string) (string, os.FileInfo, error) {

	for {

		info, err := os.Stat(path) // we must follow symlinks

		if err == nil {
			return path, info, nil
		}

		parent := filepath.Dir(path)

		if parent == path {
			return "", nil, err
		}

		path = parent
	}
}

// isSameDevice returns true if the given files are on the same device.
//
// Parameters:
//   - fileInfo1: First file info to compare
//   - fileInfo2: Second file info to compare
//
// Returns:
//   - bool: true if the files are on the same device, false otherwise
//   - error: Error if any occurred during the device comparison
func isSameDevice(a, b os.FileInfo) (bool, error) {

	statA, ok := a.Sys().(*syscall.Stat_t)

	if !ok {
		return false, fmt.Errorf("unable to determine device for %q: expected *syscall.Stat_t, got %T: %w", a.Name(), a.Sys(), fs.ErrInvalid)
	}

	statB, ok := b.Sys().(*syscall.Stat_t)

	if !ok {
		return false, fmt.Errorf("unable to determine device for %q: expected *syscall.Stat_t, got %T: %w", b.Name(), b.Sys(), fs.ErrInvalid)
	}

	return statA.Dev == statB.Dev, nil
}
