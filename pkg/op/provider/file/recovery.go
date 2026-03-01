package file

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

func (p *Provider) moveToRecovery(path string, prune bool, pruneBoundary string) (result Tombstone, undoState map[string]any, err error) {

	// Get the absolute path of the file to remove as well as the recovery base directory (on the same partition)

	absolutePath, err := filepath.Abs(path)

	if err != nil {
		return Tombstone{}, nil, err
	}

	recoveryBase, err := p.getRecoveryBase(absolutePath)

	if err != nil {
		return Tombstone{}, nil, err
	}

	// Create a unique ID for this specific removal operation

	id := uuid.New().String()
	recoveryPath := filepath.Join(recoveryBase, id)

	// Ensure the recovery container exists

	if err := os.MkdirAll(recoveryBase, 0700); err != nil {
		return Tombstone{}, nil, fmt.Errorf("failed to create recovery site: %w", err)
	}

	// Perform the removal in O(1) time with no data movement because it's on the same partition

	if err := os.Rename(absolutePath, recoveryPath); err != nil {
		return Tombstone{}, nil, err
	}

	pruneEmptyParents(path, prune, pruneBoundary)

	// Formulate return values

	result = Tombstone{
		OriginalPath: absolutePath,
		RecoveryPath: recoveryPath,
	}

	undoState = map[string]any{
		"tombstone": result,
	}

	return result, undoState, nil
}

// restoreFromRecovery is the compensating action (undo) for any removal operation.
//
// It moves the entity back to its original location from the tombstone site.
//
// Parameters:
//   - t: The tombstone returned by RemoveAll
//
// Returns:
//   - err: any error that occurred during the compensation
func (p *Provider) restoreFromRecovery(tombstone Tombstone) error {

	// Validate the tombstone

	if tombstone.RecoveryPath == "" || tombstone.OriginalPath == "" {
		return fmt.Errorf("invalid tombstone: missing path metadata")
	}

	// Verify the recovery site still exists

	if _, err := os.Stat(tombstone.RecoveryPath); errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("recovery source not found: %s (perhaps it was purged?)", tombstone.RecoveryPath)
	}

	// Ensure the original destination's parent exists

	parentDir := filepath.Dir(tombstone.OriginalPath)

	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to recreate original parent directory: %w", err)
	}

	// Relocate with no data movement since we guaranteed we are on the same partition as the original file

	if err := os.Rename(tombstone.RecoveryPath, tombstone.OriginalPath); err != nil {
		return fmt.Errorf("failed to restore from tombstone: %w", err)
	}

	return nil
}
