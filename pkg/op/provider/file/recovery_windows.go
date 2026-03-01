//go:build windows

package file

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// getRecoveryBase finds a same-volume directory for zero-copy recovery via Rename.
//
// Parameters:
//   - absolutePath: Absolute path of the file to recover
//
// Returns:
//   - string: The recovery base path
//   - error: Error if any occurred during recovery base calculation
func (p *Provider) getRecoveryBase(absolutePath string) (string, error) {

	const folderName = ".devlore_recovery"

	vol := filepath.VolumeName(absolutePath)
	if vol == "" {
		return "", fmt.Errorf("unable to determine volume for %q", absolutePath)
	}

	if cacheDir, err := os.UserCacheDir(); err == nil {
		if strings.HasPrefix(strings.ToUpper(cacheDir), strings.ToUpper(vol)) {
			return filepath.Join(cacheDir, "devlore", "recovery"), nil
		}
	}

	// Fall back to volume root
	base := vol + string(filepath.Separator)
	return filepath.Join(base, folderName), nil
}
