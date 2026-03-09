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

var _ op.ContextProvider = (*Provider)(nil) // Interface Guard: ensures *Provider implements op.ContextProvider.

var (
	// SkipDir indicates that the current directory should be skipped.
	SkipDir = fs.SkipDir

	// SkipAll signals the walker to terminate immediately (success).
	SkipAll = fs.SkipAll
)

// Provider provides file system actions.
//
// Compensable forward methods return (T, Tombstone, error): the result, the compensation tombstone, and an error.
// The tombstone is opaque to the executor, meaningful only to the corresponding "Compensate*" backward method.
//
// +devlore:access=both
type Provider struct {
	op.ProviderBase
}

// Reducer is a function called for each file or directory in a [#Provider.WalkTree] operation.
type Reducer func(initial any, resource Resource, relativePath string, stack *op.RecoveryStack) (result any, err error)

// region EXPORTED METHODS

// region State management

func (p *Provider) Root() string {
	if p.Context().Root == nil {
		return ""
	}
	return p.Context().Root.Name()
}

// endregion

// region Behaviors

// Compensable actions

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
	backupPath := path.SourcePath.Abs + backupSuffix + "." + timestamp

	if err := p.rename(path.SourcePath.Abs, backupPath); err != nil {
		return Resource{}, Tombstone{}, err
	}

	result = NewResource(backupPath)
	if err := result.Resolve(p.Context().Root); err != nil {
		return Resource{}, Tombstone{}, err
	}

	// Tombstone preserves the resource's true identity. RecoveryPath records where the data was moved to.
	undo = Tombstone{
		TombstoneBase: op.NewTombstoneBase(&path),
		RecoveryPath:  backupPath,
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
	recoveryPath := undo.RecoveryPath

	if resource.Checksum != "" {
		actual := checksumFile(recoveryPath)
		if actual == "" {
			return fmt.Errorf("cannot read %s for verification", recoveryPath)
		}
		if actual != resource.Checksum {
			return fmt.Errorf("%s has been modified (checksum mismatch)", recoveryPath)
		}
	}

	return p.Context().RecoverySite.RestoreFile(resource.SourcePath.Abs, recoveryPath)
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

	src, err := p.open(sourceFile.SourcePath.Abs)
	if err != nil {
		return result, undo, err
	}
	defer src.Close()

	dst, err := p.openFile(result.SourcePath.Abs, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, destinationFileMode)
	if err != nil {
		return result, undo, err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
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
// Idempotent: if the symlink already points correctly, it's a no-op. If something exists at the path, it is moved to
// recovery before creating the symlink.
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

	if info, err := p.lstat(path.SourcePath.Abs); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			existing, readErr := p.readlink(path.SourcePath.Abs)
			if readErr == nil && existing == source.SourcePath.Abs {
				return path, Tombstone{}, nil // Already correct — no change
			}
		}

		// Something exists at the path — archive it before creating the symlink.
		recoveryPath, archiveErr := p.Context().RecoverySite.ArchiveFile(path.SourcePath.Abs)
		if archiveErr != nil {
			return Resource{}, Tombstone{}, archiveErr
		}
		undo = Tombstone{
			TombstoneBase: op.NewTombstoneBase(&path),
			RecoveryPath:  recoveryPath,
		}
	} else {
		// Nothing exists — tombstone records the path for removal on compensation.
		undo = Tombstone{
			TombstoneBase: op.NewTombstoneBase(&path),
		}
	}

	if err := p.mkdirAll(filepath.Dir(path.SourcePath.Abs), 0o750); err != nil {
		return Resource{}, Tombstone{}, err
	}

	if err := p.symlink(source.SourcePath.Abs, path.SourcePath.Abs); err != nil {
		return Resource{}, Tombstone{}, err
	}

	result = NewResource(path.SourcePath.Abs)
	if err = result.Resolve(p.Context().Root); err != nil {
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

	if _, err := p.stat(source.SourcePath.Abs); err != nil {
		return Resource{}, Tombstone{}, err
	}

	if err := p.mkdirAll(filepath.Dir(destination.SourcePath.Abs), 0o750); err != nil {
		return Resource{}, Tombstone{}, err
	}

	if err := p.rename(source.SourcePath.Abs, destination.SourcePath.Abs); err != nil {
		return Resource{}, Tombstone{}, err
	}

	result = NewResource(destination.SourcePath.Abs)
	if err := result.Resolve(p.Context().Root); err != nil {
		return Resource{}, Tombstone{}, err
	}

	// Tombstone preserves the source's true identity. RecoveryPath records where the data was moved to.
	undo = Tombstone{
		TombstoneBase: op.NewTombstoneBase(&source),
		RecoveryPath:  destination.SourcePath.Abs,
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
	recoveryPath := undo.RecoveryPath

	if resource.Checksum != "" {

		actual := checksumFile(recoveryPath)

		if actual == "" {
			return fmt.Errorf("cannot read %s for verification", recoveryPath)
		}

		if actual != resource.Checksum {
			return fmt.Errorf("%s has been modified (checksum mismatch)", recoveryPath)
		}
	}

	return p.Context().RecoverySite.RestoreFile(resource.SourcePath.Abs, recoveryPath)
}

// Remove deletes the file at "path".
//
// If prune is true and boundary is set, empty parent directories are removed up to the boundary.
//
// Parameters:
//   - path: Resource for the file to delete
//   - prune: If true, remove empty parent directories after deletion
//   - boundary: Stop pruning at this directory (prevents removing too much)
//
// Returns:
//   - result: Tombstone for restoring the deleted file
//   - err: any error
func (p *Provider) Remove(path Resource, prune bool, boundary Resource) (result Tombstone, undo Tombstone, err error) {

	nonEmptyDirectory, err := isDirAndNotEmpty(path.SourcePath.Abs, p.Context().Root)
	if err != nil {
		if os.IsNotExist(err) {
			return Tombstone{}, Tombstone{}, nil
		}
		return Tombstone{}, Tombstone{}, err
	}

	if nonEmptyDirectory {
		return Tombstone{}, Tombstone{}, fmt.Errorf("directory %s is not empty", path.SourcePath.Abs)
	}

	recoveryPath, err := p.Context().RecoverySite.ArchiveFile(path.SourcePath.Abs)
	if err != nil {
		return Tombstone{}, Tombstone{}, err
	}

	p.pruneEmptyParents(path.SourcePath.Abs, prune, boundary.SourcePath.Abs)

	tombstone := Tombstone{
		TombstoneBase: op.NewTombstoneBase(&path),
		RecoveryPath:  recoveryPath,
	}
	return tombstone, tombstone, nil
}

// CompensateRemove undoes a Remove by restoring the file from recovery.
func (p *Provider) CompensateRemove(undo Tombstone) error {
	if undo.Resource() == nil {
		return nil
	}
	return p.Context().RecoverySite.RestoreFile(undo.Resource().(*Resource).SourcePath.Abs, undo.RecoveryPath)
}

// RemoveAll removes the file at "path" and any children it contains.
//
// Parameters:
//   - path: Resource for the file or directory to remove
//   - prune: If true, remove empty parent directories after deletion
//   - boundary: Stop pruning at this directory (prevents removing too much)
//
// Returns:
//   - result: Tombstone for restoring the deleted tree
//   - err: any error
func (p *Provider) RemoveAll(path Resource, prune bool, boundary Resource) (result Tombstone, undo Tombstone, err error) {

	recoveryPath, err := p.Context().RecoverySite.ArchiveFile(path.SourcePath.Abs)
	if err != nil {
		return Tombstone{}, Tombstone{}, err
	}

	p.pruneEmptyParents(path.SourcePath.Abs, prune, boundary.SourcePath.Abs)

	tombstone := Tombstone{
		TombstoneBase: op.NewTombstoneBase(&path),
		RecoveryPath:  recoveryPath,
	}
	return tombstone, tombstone, nil
}

// CompensateRemoveAll undoes a RemoveAll by restoring from recovery.
func (p *Provider) CompensateRemoveAll(undo Tombstone) error {
	if undo.Resource() == nil {
		return nil
	}
	return p.Context().RecoverySite.RestoreFile(undo.Resource().(*Resource).SourcePath.Abs, undo.RecoveryPath)
}

// Unlink removes the symlink at "path".
//
// If prune is true and boundary is set, empty parent directories are removed up to the boundary.
//
// Parameters:
//   - path: Resource for the symlink to remove
//   - prune: If true, remove empty parent directories after unlinking
//   - boundary: Stop pruning at this directory (prevents removing too much)
//
// Returns:
//   - result: Tombstone for restoring the deleted symlink
//   - err: any error
func (p *Provider) Unlink(path Resource, prune bool, boundary Resource) (result Tombstone, undo Tombstone, err error) {

	info, err := p.lstat(path.SourcePath.Abs)
	if os.IsNotExist(err) {
		return Tombstone{}, Tombstone{}, nil // Already gone — no change
	}

	if err != nil {
		return Tombstone{}, Tombstone{}, err
	}

	if info.Mode()&os.ModeSymlink == 0 {
		return Tombstone{}, Tombstone{}, fmt.Errorf("%s is not a symlink", path.SourcePath.Abs)
	}

	recoveryPath, err := p.Context().RecoverySite.ArchiveFile(path.SourcePath.Abs)
	if err != nil {
		return Tombstone{}, Tombstone{}, err
	}

	p.pruneEmptyParents(path.SourcePath.Abs, prune, boundary.SourcePath.Abs)

	tombstone := Tombstone{
		TombstoneBase: op.NewTombstoneBase(&path),
		RecoveryPath:  recoveryPath,
	}
	return tombstone, tombstone, nil
}

// CompensateUnlink undoes an Unlink by restoring the symlink from recovery.
func (p *Provider) CompensateUnlink(undo Tombstone) error {
	if undo.Resource() == nil {
		return nil
	}
	return p.Context().RecoverySite.RestoreFile(undo.Resource().(*Resource).SourcePath.Abs, undo.RecoveryPath)
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
		value, err := gitignore.NewTracker(root.SourcePath.Abs)
		if err != nil {
			return nil, nil, err
		}
		tracker = value
	}

	absoluteRoot, err := filepath.Abs(root.SourcePath.Abs)
	if err != nil {
		return nil, nil, err
	}

	if _, err := p.stat(absoluteRoot); err != nil {
		return nil, nil, err
	}

	osRoot := p.Context().Root

	walkFn := func(entryAbs string, d fs.DirEntry, walkDirErr error) error {

		if walkDirErr != nil {
			return walkDirErr
		}

		relativePath, relativeErr := filepath.Rel(absoluteRoot, entryAbs)
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

		resource := NewResource(entryAbs)
		if resErr := resource.Resolve(osRoot); resErr != nil {
			return resErr
		}

		result, err = fn(result, resource, relativePath, stack)
		return err
	}

	var walkErr error
	if osRoot != nil {
		if relRoot := rootRel(osRoot, absoluteRoot); relRoot != "" {
			walkErr = fs.WalkDir(osRoot.FS(), relRoot, func(relPath string, d fs.DirEntry, walkDirErr error) error {
				return walkFn(filepath.Join(osRoot.Name(), relPath), d, walkDirErr)
			})
		} else {
			walkErr = filepath.WalkDir(absoluteRoot, walkFn)
		}
	} else {
		walkErr = filepath.WalkDir(absoluteRoot, walkFn)
	}

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

// Fallible actions

// Exists returns true if the file at "path" exists.
//
// Parameters:
//   - resource: Resource to check
//
// Returns:
//   - bool: true if the resource exists, false otherwise
//   - error: permission or other I/O errors (not-exist is not an error)
func (p *Provider) Exists(resource Resource) (bool, error) {
	_, err := p.lstat(resource.SourcePath.Abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
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

	if !honorGitignore || p.Root() == "" {
		return matches, nil
	}

	tracker, trackerErr := gitignore.NewTracker(p.Root())

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
		info, statErr := p.stat(m)
		isDir := statErr == nil && info.IsDir()
		ignored, _ := tracker.IsIgnored(relPath, isDir)

		if !ignored {
			filtered = append(filtered, m)
		}
	}
	return filtered, nil
}

// IsDir returns true if the resource exists and is a directory.
//
// Parameters:
//   - resource: Resource to check
//
// Returns:
//   - bool: true if the resource is a directory, false otherwise
//   - error: permission or other I/O errors (not-exist is not an error)
func (p *Provider) IsDir(resource Resource) (bool, error) {
	info, err := p.stat(resource.SourcePath.Abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

// IsFile returns true if the resource exists and is a regular file.
//
// Parameters:
//   - resource: Resource to check
//
// Returns:
//   - bool: true if the resource is a regular file, false otherwise
//   - error: permission or other I/O errors (not-exist is not an error)
func (p *Provider) IsFile(resource Resource) (bool, error) {
	info, err := p.stat(resource.SourcePath.Abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return info.Mode().IsRegular(), nil
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
	return resource, p.mkdirAll(resource.SourcePath.Abs, mode)
}

// Read creates a Resource from the file at "path" for reading the contents of the file at "path".
//
// Parameters:
//   - path: Absolute path to the file to read
//
// Returns:
//   - result: the contents of the file
func (p *Provider) Read(path Resource) (result Resource, err error) {
	r := NewResource(path.SourcePath.Abs)
	if err := r.Resolve(p.Context().Root); err != nil {
		return Resource{}, err
	}
	return r, nil
}

// Actions

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

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// compensateWrite reverts a write or link operation by removing the new file and restoring the original from recovery.
//
// The resource's SourcePath.Abs is the file's true home — where the new file was written. When RecoveryPath is empty,
// no file existed before — the new file is simply removed. When RecoveryPath is set, the new file is removed and the
// old data is restored from RecoveryPath back to SourcePath.Abs.
//
// Parameters:
//   - undo: Tombstone from the forward write or link operation
//
// Returns:
//   - error: any error from removing the new file or restoring the original
func (p *Provider) compensateWrite(undo Tombstone) error {

	if undo.Resource() == nil {
		return nil
	}

	homePath := undo.Resource().(*Resource).SourcePath.Abs
	if err := p.remove(homePath); err != nil && !os.IsNotExist(err) {
		return err
	}

	if undo.RecoveryPath == "" {
		return nil
	}

	return p.Context().RecoverySite.RestoreFile(homePath, undo.RecoveryPath)
}

// lstat returns file info without following symlinks. Dispatches to root.Lstat when root is available, falls back to
// os.Lstat otherwise.
//
// Parameters:
//   - abs: Absolute path to stat
//
// Returns:
//   - os.FileInfo: file metadata
//   - error: any stat error
func (p *Provider) lstat(abs string) (os.FileInfo, error) {

	root := p.Context().Root
	if rel := rootRel(root, abs); rel != "" {
		return root.Lstat(rel)
	}

	return os.Lstat(abs)
}

// mkdirAll creates a directory and all parents. Dispatches to root.MkdirAll when root is available, falls back to
// os.MkdirAll otherwise.
//
// Parameters:
//   - abs: Absolute path to create
//   - perm: Directory permission bits
//
// Returns:
//   - error: any error from creating the directory
func (p *Provider) mkdirAll(abs string, perm os.FileMode) error {

	root := p.Context().Root
	if rel := rootRel(root, abs); rel != "" {
		return root.MkdirAll(rel, perm)
	}

	return os.MkdirAll(abs, perm)
}

// open opens a file for reading. Dispatches to root.Open when root is available, falls back to os.Open otherwise.
//
// Parameters:
//   - abs: Absolute path to the file
//
// Returns:
//   - *os.File: open file handle
//   - error: any error from opening the file
func (p *Provider) open(abs string) (*os.File, error) {

	root := p.Context().Root
	if rel := rootRel(root, abs); rel != "" {
		return root.Open(rel)
	}

	return os.Open(abs)
}

// openFile opens a file with the given flags and permissions. Dispatches to root.OpenFile when root is available, falls
// back to os.OpenFile otherwise.
//
// Parameters:
//   - abs: Absolute path to the file
//   - flag: File open flags (e.g., os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
//   - perm: File permission bits
//
// Returns:
//   - *os.File: open file handle
//   - error: any error from opening the file
func (p *Provider) openFile(abs string, flag int, perm os.FileMode) (*os.File, error) {

	root := p.Context().Root
	if rel := rootRel(root, abs); rel != "" {
		return root.OpenFile(rel, flag, perm)
	}

	return os.OpenFile(abs, flag, perm)
}

// prepareWrite handles pre-write backup for destructive operations. If the destination exists, it is moved to a
// recovery site before the write proceeds. If the destination does not exist, the parent directory is created and a
// tombstone with no RecoveryPath is returned (compensation will simply remove the newly created file).
//
// Parameters:
//   - resource: Resource for the destination file
//
// Returns:
//   - Resource: resolved destination resource
//   - Tombstone: compensation state for undoing the write
//   - error: any error from backup or directory creation
func (p *Provider) prepareWrite(resource Resource) (result Resource, undo Tombstone, err error) {

	result = NewResource(resource.SourcePath.Abs)
	if err = result.Resolve(p.Context().Root); err != nil {
		return Resource{}, Tombstone{}, err
	}

	if !result.Exists() {
		err = p.mkdirAll(filepath.Dir(result.SourcePath.Abs), 0o750)
		if err != nil {
			return Resource{}, Tombstone{}, errors.Join(os.ErrNotExist, err)
		}

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

// pruneEmptyParents removes empty parent directories up to the boundary.
//
// If prune is false, this function does nothing. Errors are ignored because pruning is merely hygiene.
//
// Parameters:
//   - path: The path to remove empty parent directories from
//   - prune: If true, remove empty parent directories
//   - boundary: Stop pruning at this directory (prevents removing too much). Default: Root().
func (p *Provider) pruneEmptyParents(path string, prune bool, boundary string) {

	if !prune {
		return
	}

	if boundary == "" {
		boundary = p.Root()
	}

	dir := filepath.Dir(path)

	for dir != boundary && dir != "." && dir != "/" {
		if err := p.remove(dir); err != nil {
			return // not empty or permission error
		}
		dir = filepath.Dir(dir)
	}
}

// readlink reads the destination of a symlink. Dispatches to root.Readlink when root is available, falls back to
// os.Readlink otherwise. Always returns an absolute path.
//
// Parameters:
//   - abs: Absolute path to the symlink
//
// Returns:
//   - string: absolute path the symlink points to
//   - error: any error from reading the link
func (p *Provider) readlink(abs string) (string, error) {

	root := p.Context().Root
	if rel := rootRel(root, abs); rel != "" {
		target, err := root.Readlink(rel)
		if err != nil {
			return "", err
		}

		// Root.Readlink returns the symlink target as stored (relative). Resolve to absolute for comparison.
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(abs), target)
		}

		return filepath.Clean(target), nil
	}

	return os.Readlink(abs)
}

// remove removes a file or empty directory. Dispatches to root.Remove when root is available, falls back to os.Remove
// otherwise.
//
// Parameters:
//   - abs: Absolute path to remove
//
// Returns:
//   - error: any error from removing the file or directory
func (p *Provider) remove(abs string) error {

	root := p.Context().Root
	if rel := rootRel(root, abs); rel != "" {
		return root.Remove(rel)
	}

	return os.Remove(abs)
}

// rename moves a file from oldAbs to newAbs. Dispatches to root.Rename when root is available, falls back to
// os.Rename otherwise.
//
// Parameters:
//   - oldAbs: Absolute path of the file to move
//   - newAbs: Absolute path of the destination
//
// Returns:
//   - error: any error from the rename operation
func (p *Provider) rename(oldAbs, newAbs string) error {

	root := p.Context().Root
	if rel := rootRel(root, oldAbs); rel != "" {
		return root.Rename(rel, rootRel(root, newAbs))
	}

	return os.Rename(oldAbs, newAbs)
}

// stat returns file info following symlinks. Dispatches to root.Stat when root is available, falls back to os.Stat
// otherwise.
//
// Parameters:
//   - abs: Absolute path to stat
//
// Returns:
//   - os.FileInfo: file metadata
//   - error: any stat error
func (p *Provider) stat(abs string) (os.FileInfo, error) {

	root := p.Context().Root
	if rel := rootRel(root, abs); rel != "" {
		return root.Stat(rel)
	}

	return os.Stat(abs)
}

// symlink creates a symbolic link. When root is available, the target is stored as a relative path (os.Root requires
// non-absolute symlink targets). Falls back to os.Symlink with absolute target otherwise.
//
// Parameters:
//   - targetAbs: Absolute path that the symlink should point to
//   - linkAbs: Absolute path where the symlink should be created
//
// Returns:
//   - error: any error from creating the symlink
func (p *Provider) symlink(targetAbs, linkAbs string) error {

	root := p.Context().Root
	if rel := rootRel(root, linkAbs); rel != "" {
		relTarget, err := filepath.Rel(filepath.Dir(linkAbs), targetAbs)
		if err != nil {
			return err
		}
		return root.Symlink(relTarget, rel)
	}

	return os.Symlink(targetAbs, linkAbs)
}

// write writes data to the specified path after preparing the write operation.
//
// Parameters:
//   - resource: Resource for the destination file
//   - data: Content bytes to write
//   - mode: File permission bits (default: 0o644)
//
// Returns:
//   - Resource: resolved resource for the written file
//   - Tombstone: compensation state for undoing the write
//   - error: any error from writing
func (p *Provider) write(resource Resource, data []byte, mode os.FileMode) (result Resource, undo Tombstone, err error) {

	result, undo, err = p.prepareWrite(resource)
	if err != nil {
		return Resource{}, Tombstone{}, err
	}

	if mode == 0 {
		mode = 0o644
	}

	f, err := p.openFile(result.SourcePath.Abs, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
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

	err = result.RefreshWith(p.Context().Root, hex.EncodeToString(hasher.Sum(nil)), int64(size))
	if err != nil {
		return result, undo, err
	}

	return result, undo, nil
}

// endregion

// endregion
