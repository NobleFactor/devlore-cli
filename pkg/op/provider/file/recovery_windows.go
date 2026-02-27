//go:build windows

package file

import (
	"os"
	"path/filepath"
	"strings"
)

// getRecoveryBase uses the Volume Name (Drive Letter) to anchor the recovery site.
func (p *Provider) getRecoveryBase(absPath string) string {
	const folderName = ".devlore_recovery"

	vol := filepath.VolumeName(absPath)

	// If it's a standard drive letter (e.g., "C:"), ensure we have a backslash.
	// If it's a UNC path (e.g., "\\server\share"), VolumeName handles it.
	base := vol + string(filepath.Separator)

	// Check if we can use a more "hidden" user-level path on the same drive.
	if cacheDir, err := os.UserCacheDir(); err == nil {
		if strings.HasPrefix(strings.ToUpper(cacheDir), strings.ToUpper(vol)) {
			return filepath.Join(cacheDir, "devlore", "recovery")
		}
	}

	return filepath.Join(base, folderName)
}

// isSameDevice on Windows simply checks if the Volume/Drive is identical.
func isSameDevice(path1, path2 string) bool {
	v1 := filepath.VolumeName(path1)
	v2 := filepath.VolumeName(path2)
	if v1 == "" || v2 == "" {
		return false
	}
	return strings.EqualFold(v1, v2)
}
