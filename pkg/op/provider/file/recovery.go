// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/google/uuid"
)

func (p *Provider) moveToRecovery(resource Resource, prune bool, boundary string) (Tombstone, error) {

	// Normalize to absolute path for reliable recovery regardless of working directory at restore time.

	sourcePath, err := filepath.Abs(resource.SourcePath)
	if err != nil {
		return Tombstone{}, err
	}

	recoveryBase, err := p.getRecoveryBase(sourcePath)
	if err != nil {
		return Tombstone{}, err
	}

	// Create a unique ID for this specific removal operation

	id := uuid.New().String()
	recoveryPath := filepath.Join(recoveryBase, id)

	// Ensure the recovery container exists

	if err := os.MkdirAll(recoveryBase, 0700); err != nil {
		return Tombstone{}, fmt.Errorf("failed to create recovery site: %w", err)
	}

	// Perform the removal in O(1) time with no data movement because it's on the same partition

	if err := os.Rename(sourcePath, recoveryPath); err != nil {
		return Tombstone{}, err
	}

	p.pruneEmptyParents(sourcePath, prune, boundary)

	// Resource preserves its true identity (SourcePath = original location). RecoveryPath records where the data was
	// moved to.
	return Tombstone{
		TombstoneBase: op.NewTombstoneBase(&resource),
		RecoveryPath:  recoveryPath,
	}, nil
}

// restoreFromRecovery is the compensating action (undo) for any removal operation.
//
// It moves the entity back to its original location from the recovery site. The recovery path is tombstone.RecoveryPath
// (temporary excursion); the restoration target is Resource.SourcePath (the resource's true home).
func (p *Provider) restoreFromRecovery(tombstone Tombstone) error {

	originalPath := tombstone.Resource().(*Resource).SourcePath
	recoveryPath := tombstone.RecoveryPath

	// Validate the tombstone

	if recoveryPath == "" || originalPath == "" {
		return fmt.Errorf("invalid tombstone: missing path metadata")
	}

	// Verify the recovery site still exists

	if _, err := os.Lstat(recoveryPath); errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("recovery source not found: %s. Was it purged", recoveryPath)
	}

	// Ensure the original destination's parent exists

	parentDir := filepath.Dir(originalPath)

	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to recreate original parent directory: %w", err)
	}

	// Relocate with no data movement since we guaranteed we are on the same partition as the original file

	if err := os.Rename(recoveryPath, originalPath); err != nil {
		return fmt.Errorf("failed to restore from tombstone: %w", err)
	}

	return nil
}
