// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gitignore"
)

var _ op.Provider = (*Provider)(nil) // Interface Guard: ensures *Provider implements op.Provider.

// Provider provides file system actions.
//
// Compensable forward methods return (T, Tombstone, error): the result, the compensation tombstone, and an error.
// The tombstone is opaque to the executor, meaningful only to the corresponding "Compensate*" backward method.
//
// +devlore:access=both
// +devlore:bind Root=WorkDir
type Provider struct {
	op.ProviderBase
	Root Resource
}

// Actor returns a Reducer that calls the given function for each file or directory in a WalkTree operation.
//
// This is a convenience function for creating Reducers from simple functions that don't accumulate results or need to
// access the recovery stack.
func Actor(fn func(resource Resource, relativePath string) error) Reducer {
	return func(result any, resource Resource, relativePath string, stack *op.RecoveryStack) (any, error) {
		var zero any
		return zero, fn(resource, relativePath)
	}
}

// Reducer is a function called for each file or directory in a WalkTree operation.
// +devlore:callable swallow=stack
type Reducer func(initial any, resource Resource, relativePath string, stack *op.RecoveryStack) (result any, err error)

var (
	// SkipDir indicates that the current directory should be skipped.
	SkipDir = fs.SkipDir

	// SkipAll signals the walker to terminate immediately (success).
	SkipAll = fs.SkipAll
)

// ── Compensable Pairs ────────────────────────────────────────────────────────────────────────────────────────────────

// Backup moves the file at "path" to a timestamped backup location.
//
// Parameters:
//   - path: Absolute path to the file to back up
//   - backupSuffix: Suffix appended before the timestamp (default: .devlore-backup)
//
// Returns:
//   - result: Resource at the backup location
//   - undo: Tombstone for restoring the original
//   - err: any error
func (p *Provider) Backup(path Resource, backupSuffix string) (result Resource, undo Tombstone, err error) {

	if backupSuffix == "" {
		backupSuffix = ".devlore-backup"
	}

	timestamp := time.Now().Format("20060102-150405")
	originalPath := path.SourcePath
	backupPath := originalPath + backupSuffix + "." + timestamp

	if err := os.Rename(originalPath, backupPath); err != nil {
		return Resource{}, Tombstone{}, err
	}

	result = path
	result.SourcePath = backupPath

	// Undo resource reflects where the data IS (backup path).
	// OriginalPath records where it WAS (restoration target).
	undoResource := path
	undoResource.SourcePath = backupPath
	undo = Tombstone{
		TombstoneBase: op.NewTombstoneBase(&undoResource),
		OriginalPath:  originalPath,
	}

	return result, undo, nil
}

// CompensateBackup undoes a Backup by moving the backup back to the original path.
//
// The resource's checksum is verified before restoring; a mismatch indicates external modification.
func (p *Provider) CompensateBackup(undo Tombstone) error {
	if undo.Resource() == nil {
		return nil
	}

	resource := undo.Resource().(*Resource)
	recoveryPath := resource.SourcePath
	if resource.Checksum != "" {
		actual := checksumFile(recoveryPath)
		if actual == "" {
			return fmt.Errorf("cannot read %s for verification", recoveryPath)
		}
		if actual != resource.Checksum {
			return fmt.Errorf("%s has been modified (checksum mismatch)", recoveryPath)
		}
	}

	return p.restoreFromRecovery(undo)
}

// Copy copies a blob to the file at "destination" with the given mode.
//
// If the destination already exists, it is moved to a recovery site before writing.
//
// Parameters:
//   - sourceFile: Resource wrapping the source file path
//   - destinationFilename: Resource for the destination path
//   - destinationFileMode: The file mode to use (default: 0644)
//
// Returns:
//   - result: Resource for the written file
//   - undo: Tombstone for restoring the original state
//   - err: any error that occurred during the copy
func (p *Provider) Copy(sourceFile Resource, destinationFilename Resource, destinationFileMode os.FileMode) (result Resource, undo Tombstone, err error) {

	result, undo, err = p.prepareWrite(destinationFilename)
	if err != nil {
		return Resource{}, Tombstone{}, err
	}

	if destinationFileMode == 0 {
		destinationFileMode = 0o644
	}

	f, err := os.OpenFile(result.SourcePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, destinationFileMode)
	if err != nil {
		return result, undo, err
	}
	defer f.Close()

	if _, err := sourceFile.WriteTo(f); err != nil {
		return result, undo, err
	}

	return result, undo, nil
}

// CompensateCopy undoes a Copy by restoring the original file from recovery.
func (p *Provider) CompensateCopy(undo Tombstone) error {
	return p.compensateWrite(undo)
}

// Link creates a symlink at a path pointing to a source file.
//
// Idempotent: if the symlink already points correctly, it's a no-op.
//
// If something exists at the path, it is moved to recovery before creating the symlink.
//
// Parameters:
//   - source: Resource for the symlink target
//   - path: Resource for the symlink location
//
// Returns:
//   - result: Resource for the created symlink
//   - undo: Tombstone for restoring the previous state
//   - err: any error
func (p *Provider) Link(source, path Resource) (result Resource, undo Tombstone, err error) {

	if info, err := os.Lstat(path.SourcePath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			existing, readErr := os.Readlink(path.SourcePath)
			if readErr == nil && existing == source.SourcePath {
				return path, Tombstone{}, nil // Already correct — no change
			}
		}

		// Something exists at the path — move it to recovery before creating the symlink.
		undo, err = p.moveToRecovery(path, false, "")
		if err != nil {
			return Resource{}, Tombstone{}, err
		}
	} else {
		// Nothing exists — tombstone records the path for removal on compensation.
		undo = Tombstone{
			TombstoneBase: op.NewTombstoneBase(&path),
		}
	}

	if err := os.MkdirAll(filepath.Dir(path.SourcePath), 0o750); err != nil {
		return Resource{}, Tombstone{}, err
	}

	if err := os.Symlink(source.SourcePath, path.SourcePath); err != nil {
		return Resource{}, Tombstone{}, err
	}

	result, err = NewResource(path.SourcePath)
	if err != nil {
		return Resource{}, undo, err
	}
	return result, undo, nil
}

// CompensateLink undoes a Link by removing the symlink and restoring whatever was there before.
func (p *Provider) CompensateLink(undo Tombstone) error {
	return p.compensateWrite(undo)
}

// Move moves a file from source to destination using "os.Rename".
//
// Parameters:
//   - source: Resource at the source location
//   - destination: Resource for the destination location
//
// Returns:
//   - result: Resource at the destination
//   - undo: Tombstone for moving the file back
//   - err: any error
func (p *Provider) Move(source, destination Resource) (result Resource, undo Tombstone, err error) {

	if _, err := os.Stat(source.SourcePath); err != nil {
		return Resource{}, Tombstone{}, err
	}

	if err := os.MkdirAll(filepath.Dir(destination.SourcePath), 0o750); err != nil {
		return Resource{}, Tombstone{}, err
	}

	if err := os.Rename(source.SourcePath, destination.SourcePath); err != nil {
		return Resource{}, Tombstone{}, err
	}

	result = source
	result.SourcePath = destination.SourcePath

	// Undo resource reflects where the data IS (destination).
	// OriginalPath records where it WAS (source).
	undoResource := source
	undoResource.SourcePath = destination.SourcePath
	undo = Tombstone{
		TombstoneBase: op.NewTombstoneBase(&undoResource),
		OriginalPath:  source.SourcePath,
	}

	return result, undo, nil
}

// CompensateMove undoes a Move by moving the file back to its original location.
//
// The resource's checksum is verified before restoring; a mismatch indicates external modification.
func (p *Provider) CompensateMove(undo Tombstone) error {
	if undo.Resource() == nil {
		return nil
	}

	resource := undo.Resource().(*Resource)
	recoveryPath := resource.SourcePath
	if resource.Checksum != "" {
		actual := checksumFile(recoveryPath)
		if actual == "" {
			return fmt.Errorf("cannot read %s for verification", recoveryPath)
		}
		if actual != resource.Checksum {
			return fmt.Errorf("%s has been modified (checksum mismatch)", recoveryPath)
		}
	}

	return p.restoreFromRecovery(undo)
}

// Remove deletes the file at "path".
//
// If prune is true and pruneBoundary is set, empty parent directories are removed up to the boundary.
//
// Parameters:
//   - path: Resource for the file to delete
//   - prune: If true, remove empty parent directories after deletion
//   - pruneBoundary: Stop pruning at this directory (prevents removing too much)
//
// Returns:
//   - result: Tombstone for restoring the deleted file
//   - err: any error
func (p *Provider) Remove(path Resource, prune bool, pruneBoundary Resource) (result Tombstone, undo Tombstone, err error) {

	nonEmptyDirectory, err := isDirAndNotEmpty(path.SourcePath)

	if err != nil {
		if os.IsNotExist(err) {
			return Tombstone{}, Tombstone{}, nil
		}
		return Tombstone{}, Tombstone{}, err
	}

	if nonEmptyDirectory {
		return Tombstone{}, Tombstone{}, fmt.Errorf("directory %s is not empty", path.SourcePath)
	}

	tombstone, err := p.moveToRecovery(path, prune, pruneBoundary.SourcePath)
	return tombstone, tombstone, err
}

// CompensateRemove undoes a Remove by restoring the file from recovery.
func (p *Provider) CompensateRemove(undo Tombstone) error {
	return p.restoreFromRecovery(undo)
}

// RemoveAll removes the file at "path" and any children it contains.
//
// Parameters:
//   - path: Resource for the file or directory to remove
//   - prune: If true, remove empty parent directories after deletion
//   - pruneBoundary: Stop pruning at this directory (prevents removing too much)
//
// Returns:
//   - result: Tombstone for restoring the deleted tree
//   - err: any error
func (p *Provider) RemoveAll(path Resource, prune bool, pruneBoundary Resource) (result Tombstone, undo Tombstone, err error) {
	tombstone, err := p.moveToRecovery(path, prune, pruneBoundary.SourcePath)
	return tombstone, tombstone, err
}

// CompensateRemoveAll undoes a RemoveAll by restoring from recovery.
func (p *Provider) CompensateRemoveAll(undo Tombstone) error {
	return p.restoreFromRecovery(undo)
}

// Unlink removes the symlink at "path".
//
// If prune is true and pruneBoundary is set, empty parent directories are removed up to the boundary.
//
// Parameters:
//   - path: Resource for the symlink to remove
//   - prune: If true, remove empty parent directories after unlinking
//   - pruneBoundary: Stop pruning at this directory (prevents removing too much)
//
// Returns:
//   - result: Tombstone for restoring the deleted symlink
//   - err: any error
func (p *Provider) Unlink(path Resource, prune bool, pruneBoundary Resource) (result Tombstone, undo Tombstone, err error) {

	info, err := os.Lstat(path.SourcePath)

	if os.IsNotExist(err) {
		return Tombstone{}, Tombstone{}, nil // Already gone — no change
	}

	if err != nil {
		return Tombstone{}, Tombstone{}, err
	}

	if info.Mode()&os.ModeSymlink == 0 {
		return Tombstone{}, Tombstone{}, fmt.Errorf("%s is not a symlink", path.SourcePath)
	}

	tombstone, err := p.moveToRecovery(path, prune, pruneBoundary.SourcePath)
	return tombstone, tombstone, err
}

// CompensateUnlink undoes an Unlink by restoring the symlink from recovery.
func (p *Provider) CompensateUnlink(undo Tombstone) error {
	return p.restoreFromRecovery(undo)
}

// WalkTree performs a depth-first traversal with an accumulator and a RecoveryStack for compensable operations.
//
// The visitor can push compensable operations onto the stack during traversal. On error mid-walk, the stack
// is unwound automatically and errors are joined. On success, the accumulated result and the stack are returned--the
// stack serves as the undo receipt.
//
// +devlore:defaults root="",honorGitignore=true
//
// Parameters:
//   - root: Root directory to start traversal from
//   - fn: Reducer function to call for each file or directory
//   - gitignore: If true, filter results using gitignore rules
//
// Returns:
//   - result: The accumulated result from the visitor function
//   - stack: The compensable operations stack
//   - err: The first error encountered during traversal, if any
func (p *Provider) WalkTree(root Resource, fn Reducer, honorGitignore bool) (result any, stack *op.RecoveryStack, err error) {

	stack = op.NewRecoveryStack()

	var tracker *gitignore.Tracker

	if honorGitignore {
		value, err := gitignore.NewTracker(root.SourcePath)
		if err != nil {
			return nil, nil, err
		}
		tracker = value
	}

	absoluteRoot, err := filepath.Abs(root.SourcePath)
	if err != nil {
		return nil, nil, err
	}

	if _, err := os.Stat(absoluteRoot); err != nil {
		return nil, nil, err
	}

	walkErr := filepath.WalkDir(absoluteRoot, func(path string, d fs.DirEntry, walkDirErr error) error {

		if walkDirErr != nil {
			return walkDirErr
		}

		relativePath, relativeErr := filepath.Rel(absoluteRoot, path)

		if relativeErr != nil {
			return relativeErr
		}

		if relativePath == "." {
			return nil
		}

		isDir := d.IsDir()

		if isDir && d.Name() == ".git" {
			return SkipDir
		}

		if tracker != nil {
			if isDir {
				tracker.Push(relativePath)
			}
			ignored, _ := tracker.IsIgnored(relativePath, isDir)
			if ignored && isDir {
				return SkipDir
			}
			if ignored {
				return nil
			}
		}

		resource, resErr := NewResource(path)
		if resErr != nil {
			return resErr
		}

		result, err = fn(result, resource, relativePath, stack)
		return err
	})

	if walkErr != nil {
		return nil, stack, walkErr
	}

	return result, stack, nil
}

// CompensateWalkTree unwinds the RecoveryStack returned by WalkTree in LIFO order.
//
// Best-effort: all entries are attempted, errors are joined.
//
// Parameters:
//   - stack: The stack returned by WalkTree
//
// Returns:
//   - err: The first error encountered during compensation, if any
func (p *Provider) CompensateWalkTree(stack *op.RecoveryStack) error {
	if stack == nil {
		return nil
	}
	return stack.Unwind()
}

// WriteBytes writes inline content to the file at "path" with the given mode.
//
// Parameters:
//   - destination: Resource for the file to write
//   - content: String content to write to the file
//   - mode: File permission bits (e.g., 0o644)
//
// Returns:
//   - result: Resource for the written file
//   - undo: Tombstone for restoring the previous state
//   - err: any error that occurred while writing
func (p *Provider) WriteBytes(destination Resource, content string, mode os.FileMode) (result Resource, undo Tombstone, err error) {
	return p.write(destination, []byte(content), mode)
}

// CompensateWriteBytes undoes a WriteBytes by restoring the original file.
func (p *Provider) CompensateWriteBytes(undo Tombstone) error {
	return p.compensateWrite(undo)
}

// WriteText writes inline content to the file at "path" with the given mode.
//
// Parameters:
//   - destination: Resource for the file to write
//   - content: String content to write to the file
//   - mode: File permission bits (e.g., 0o644)
//
// Returns:
//   - result: Resource for the written file
//   - undo: Tombstone for restoring the previous state
//   - err: any error that occurred while writing
func (p *Provider) WriteText(destination Resource, content string, mode os.FileMode) (result Resource, undo Tombstone, err error) {
	return p.write(destination, []byte(content), mode)
}

// CompensateWriteText undoes a WriteText by restoring the original file.
func (p *Provider) CompensateWriteText(undo Tombstone) error {
	return p.compensateWrite(undo)
}

// ── Non-compensable Methods (pure functions) ─────────────────────────────────────────────────────────────────────────

// Exists returns true if the file at "path" exists.
//
// Parameters:
//   - path: Absolute path to check
//
// Returns:
//   - bool: true if the file at "path" exists, false otherwise
func (p *Provider) Exists(resource Resource) bool {
	_, err := os.Lstat(resource.SourcePath)
	return err == nil
}

// Glob returns file paths matching a pattern relative to Root.
//
// Parameters:
//   - pattern: Glob pattern (e.g., "*.go", "**/*.yaml")
//   - honorGitignore: If true, filter results using gitignore rules
//
// Returns:
//   - []string: List of matching file paths
func (p *Provider) Glob(pattern string, honorGitignore bool) ([]string, error) {

	matches, err := filepath.Glob(pattern)

	if err != nil {
		return nil, err
	}

	if !honorGitignore || p.Root.SourcePath == "" {
		return matches, nil
	}

	tracker, trackerErr := gitignore.NewTracker(p.Root.SourcePath)

	if trackerErr != nil {
		return matches, nil //nolint:nilerr // graceful degradation: return unfiltered if gitignore unavailable
	}

	var filtered []string
	for _, m := range matches {
		relPath, relErr := filepath.Rel(tracker.Root(), m)
		if relErr != nil {
			filtered = append(filtered, m)
			continue
		}
		info, statErr := os.Stat(m)
		isDir := statErr == nil && info.IsDir()
		ignored, _ := tracker.IsIgnored(relPath, isDir)

		if !ignored {
			filtered = append(filtered, m)
		}
	}
	return filtered, nil
}

// IsDir returns true if the file at "path" exists and is a directory.
//
// Parameters:
//   - path: Absolute path to check
//
// Returns:
//   - bool: true if the file at "path "is a directory, false otherwise
func (p *Provider) IsDir(resource Resource) bool {
	info, err := os.Stat(resource.SourcePath)
	return err == nil && info.IsDir()
}

// IsFile returns true if the file at "path" exists and is a regular file.
//
// Parameters:
//   - path: Absolute path to check
//
// Returns:
//   - bool: true if the file at "path" is a regular file, false otherwise
func (p *Provider) IsFile(resource Resource) bool {
	info, err := os.Stat(resource.SourcePath)
	return err == nil && info.Mode().IsRegular()
}

// Join joins path components using the OS path separator.
//
// Parameters:
//   - parts: Path components to join
//
// Returns:
//   - string: The joined path or an empty string, if no parts are provided or all parts are empty
func (p *Provider) Join(parts ...string) string {
	return filepath.Join(parts...)
}

// Mkdir creates a directory (and parents) with the given mode.
//
// Parameters:
//   - path: Absolute path of the directory to create
//   - mode: Directory permission bits (e.g., 0o755)
//
// Returns:
//   - string: The absolute path of the created directory
func (p *Provider) Mkdir(resource Resource, mode os.FileMode) (Resource, error) {
	return resource, os.MkdirAll(resource.SourcePath, mode)
}

// Name returns the last element of "path" (a file or directory name).
//
// Parameters:
//   - path: Path to extract the name from
//
// Returns:
//   - string: The name of the file or directory
func (p *Provider) Name(path string) string {
	return filepath.Base(path)
}

// Parent returns the directory containing the file at "path".
//
// Parameters:
//   - path: Path to a file
//
// Returns:
//   - string: The parent directory of the file
func (p *Provider) Parent(path string) string {
	return filepath.Dir(path)
}

// Read creates a Resource from the file at "path" for reading the contents of the file at "path".
//
// Parameters:
//   - path: Absolute path to the file to read
//
// Returns:
//   - result: the contents of the file
func (p *Provider) Read(path Resource) (result Resource, err error) {
	return NewResource(path.SourcePath)
}

// region Internal

// compensateWrite reverts a write or link operation by removing the new file and restoring the original from recovery.
//
// When OriginalPath is empty, no file existed before — the new file at
// Resource.SourcePath is simply removed. When OriginalPath is set, the
// new file at OriginalPath is removed and the old file is restored from
// Resource.SourcePath (recovery) back to OriginalPath.
func (p *Provider) compensateWrite(undo Tombstone) error {
	if undo.Resource() == nil {
		return nil
	}

	if undo.OriginalPath == "" {
		// Nothing existed before — just remove the new file.
		recoveryPath := undo.Resource().(*Resource).SourcePath
		if err := os.Remove(recoveryPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	// Something existed before — remove the new file, restore the old.
	if err := os.Remove(undo.OriginalPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return p.restoreFromRecovery(undo)
}

// prepareWrite handles pre-write backup for destructive operations.
// If the destination exists, it is moved to a recovery site before the
// write proceeds. If the destination does not exist, the parent directory
// is created and a tombstone with no OriginalPath is returned (compensation
// will simply remove the newly created file).
func (p *Provider) prepareWrite(resource Resource) (result Resource, undo Tombstone, err error) {

	result, err = NewResource(resource.SourcePath)
	if err != nil {
		return Resource{}, Tombstone{}, err
	}

	if !result.Exists() {
		err = os.MkdirAll(filepath.Dir(result.SourcePath), 0o750)
		if err != nil {
			return Resource{}, Tombstone{}, errors.Join(os.ErrNotExist, err)
		}

		// Nothing existed before. Resource.SourcePath = the destination
		// (where the new file will be). OriginalPath is empty — compensation
		// will just remove the new file.
		undo = Tombstone{
			TombstoneBase: op.NewTombstoneBase(&result),
		}
		return result, undo, nil
	}

	tombstone, _, err := p.Remove(result, false, Resource{})
	if err != nil {
		return Resource{}, Tombstone{}, fmt.Errorf("failed to backup existing file: %w", err)
	}

	return result, tombstone, nil
}

// write writes data to the specified path after preparing the write operation.
func (p *Provider) write(resource Resource, data []byte, mode os.FileMode) (result Resource, undo Tombstone, err error) {

	result, undo, err = p.prepareWrite(resource)
	if err != nil {
		return Resource{}, Tombstone{}, err
	}

	if mode == 0 {
		mode = 0o644
	}

	f, err := os.OpenFile(result.SourcePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return result, undo, err
	}
	defer f.Close()

	hasher := sha256.New()
	mw := io.MultiWriter(f, hasher)

	var size int
	size, err = mw.Write(data)
	if err != nil {
		return result, undo, err
	}

	if err = f.Sync(); err != nil {
		return result, undo, err
	}

	err = result.RefreshMetadataWith(hex.EncodeToString(hasher.Sum(nil)), int64(size))
	if err != nil {
		return result, undo, err
	}

	return result, undo, nil
}

// endregion
