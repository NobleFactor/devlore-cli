//go:build unix

package file

import (
	"os"
	"path/filepath"
	"syscall"
)

// getRecoveryBase finds the mount point root to ensure a zero-copy Rename.

func (p *Provider) getRecoveryBase(absPath string) string {
	const folderName = ".devlore_recovery"
	mountPoint := findMountPoint(absPath)

	// If at system root, try to use UserCacheDir for less clutter,
	// but only if it's on the same device/partition.
	if mountPoint == "/" {
		if cacheDir, err := os.UserCacheDir(); err == nil {
			cachePath := filepath.Join(cacheDir, "devlore", "recovery")
			if isSameDevice(absPath, cachePath) {
				return cachePath
			}
		}
	}

	return filepath.Join(mountPoint, folderName)
}

func findMountPoint(path string) string {
	curr := path
	stat, err := os.Stat(curr)
	if err != nil {
		return "/"
	}
	dev := stat.Sys().(*syscall.Stat_t).Dev

	for {
		parent := filepath.Dir(curr)
		if parent == curr {
			return curr
		}
		pStat, err := os.Stat(parent)
		if err != nil || pStat.Sys().(*syscall.Stat_t).Dev != dev {
			return curr
		}
		curr = parent
	}
}

func isSameDevice(path1, path2 string) bool {
	s1, err1 := os.Stat(path1)
	s2, err2 := os.Stat(path2)
	if err1 != nil || err2 != nil {
		return false
	}
	return s1.Sys().(*syscall.Stat_t).Dev == s2.Sys().(*syscall.Stat_t).Dev
}
