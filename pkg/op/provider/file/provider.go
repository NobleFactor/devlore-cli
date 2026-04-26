// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package file provides file system actions for the operation graph.
package file

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gitignore"
)

var _ op.Provider = (*Provider)(nil) // Interface Guard: ensures *Provider implements op.Provider.

//goland:noinspection GoUnusedGlobalVariable
var (
	// SkipDir indicates that the current directory should be skipped.
	SkipDir = fs.SkipDir

	// SkipAll signals the walker to terminate immediately (success).
	SkipAll = fs.SkipAll

	// errSkipEntry is a sentinel error used by applyGitignore to signal that
	// a non-directory entry should be skipped. It is caught by the walkFn closure.
	errSkipEntry = errors.New("skip entry")
)

// Provider provides file system actions.
//
// Compensable forward methods return (T, Receipt, error): the result, the compensation receipt, and an error.
// The receipt is opaque to the executor, meaningful only to the corresponding "Compensate*" backward method.
//
// +devlore:access=both
type Provider struct {
	op.ProviderBase
}

// NewProvider creates a file provider bound to the given context.
func NewProvider(ctx *op.ExecutionContext) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(ctx)}
}

// Reducer is a function called for each file or directory in a [#Provider.WalkTree] operation.
type Reducer func(initial any, resource *Resource, relativePath string, stack *op.RecoveryStack) (result any, err error)

// region EXPORTED METHODS

// region State management

// Root returns the root path of the file system scope, or empty if no root is set.
func (p *Provider) Root() string {
	if p.ExecutionContext().Root == nil {
		return ""
	}
	return p.ExecutionContext().Root.Name()
}

// endregion

// region Behaviors

// Compensable actions

// Backup moves the file at "path" to a timestamped backup location.
//
// The backup destination is derived as path.SourcePath + backupSuffix + "." + timestamp, and the move itself
// delegates to [Provider.Move] so that Backup and Move share a single recovery behavior — same rename
// mechanics, same [Receipt] shape, same compensation path.
//
// Parameters:
//   - path: Absolute path to the file to back up.
//   - backupSuffix: Suffix appended before the timestamp (default: ".devlore-backup").
//
// Returns:
//   - *Resource: Resource at the backup location.
//   - Receipt: for restoring the original file (consumed by [Provider.CompensateBackup]).
//   - error: any error from the underlying Move.
func (p *Provider) Backup(source *Resource, backupSuffix string) (*Resource, Receipt, error) {

	if backupSuffix == "" {
		backupSuffix = ".devlore-backup"
	}

	timestamp := time.Now().Format("20060102-150405")
	backupPath := source.SourcePath.Abs() + backupSuffix + "." + timestamp

	return p.Move(source, backupPath)
}

// CompensateBackup undoes a Backup by delegating to [Provider.CompensateMove], reusing the Move-side
// recovery semantics (checksum verification, directory recreation, reverse rename).
//
// Parameters:
//   - receipt: [Receipt] returned by [Provider.Backup].
//
// Returns:
//   - error: any error from restoring the original file.
func (p *Provider) CompensateBackup(receipt Receipt) error {
	return p.CompensateMove(receipt)
}

// Copy copies source's contents to a new file at destinationPath with the given mode.
//
// Identity for the destination is constructed by [file.NewResource]. If the destination already exists, it is
// archived to the recovery site before writing. After the `write` succeeds, the destination resource's metadata is
// populated via [Resource.Resolve], which is what the executor's post-dispatch [op.ResourceCatalog.Transition]
// consumes to fill the pending entry in place.
//
// Parameters:
//   - source: [file.Resource] of the file to copy from.
//   - destinationPath: the path to write to.
//   - mode: the file mode for the new file. Zero defaults to 0o644.
//
// Returns:
//   - result: the destination [file.Resource] with populated metadata.
//   - undo: [file.Receipt] for restoring the original state at destination.
//   - err: any error from identity construction, backup, or the copy itself.
func (p *Provider) Copy(source *Resource, destinationPath string, mode os.FileMode) (product *Resource, receipt Receipt, err error) {

	product, err = NewResource(p.ExecutionContext(), destinationPath)
	if err != nil {
		return nil, Receipt{}, err
	}

	product, receipt, err = p.prepareWrite(product)
	if err != nil {
		return nil, Receipt{}, err
	}

	if mode == 0 {
		mode = 0o644
	}

	src, err := p.open(source.SourcePath.Abs())
	if err != nil {
		return product, receipt, err
	}
	defer iox.Close(&err, src)

	dst, err := p.openFile(product.SourcePath.Abs(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return product, receipt, err
	}
	defer iox.Close(&err, dst)

	if _, err := io.Copy(dst, src); err != nil {
		return product, receipt, err
	}

	if err := product.Resolve(); err != nil {
		return product, receipt, err
	}

	return product, receipt, nil
}

// CompensateCopy undoes a Copy by restoring the original file from recovery.
//
// Parameters:
//   - undo: [file.Receipt] returned by [Provider.Copy]
//
// Returns:
//   - error: any error from restoring the original file
func (p *Provider) CompensateCopy(receipt Receipt) error {
	return p.compensateWrite(receipt)
}

// Link creates a symbolic link at targetPath pointing to source.
//
// Idempotent: if targetPath already points to source, calling this function is a no-op. If something else exists
// at targetPath, it is moved to recovery before creating the symbolic link to source. Identity for the link
// target is constructed by [file.NewResource].
//
// Parameters:
//   - source: [file.Resource] that the symbolic link will point to.
//   - targetPath: the path where the symbolic link will be created.
//
// Returns:
//   - product: [file.Resource] for the created symbolic link.
//   - receipt: [file.Receipt] for restoring the previous state of targetPath.
//   - err: any error from creating the symbolic link.
func (p *Provider) Link(source *Resource, targetPath string) (product *Resource, undo Receipt, err error) {

	target, err := NewResource(p.ExecutionContext(), targetPath)
	if err != nil {
		return nil, Receipt{}, err
	}

	if info, err := p.lstat(target.SourcePath.Abs()); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			existing, readErr := p.readLink(target.SourcePath.Abs())
			if readErr == nil && existing == source.SourcePath.Abs() {
				return target, Receipt{}, nil // Already correct — no change
			}
		}

		// Something exists at the target — archive it before creating the symlink.
		if _, archiveErr := p.ExecutionContext().RecoverySite.ArchiveFile(target.SourcePath); archiveErr != nil {
			return nil, Receipt{}, archiveErr
		}

		undo = Receipt{
			ReceiptBase: op.NewReceiptBase(target),
		}
	} else {
		// Nothing exists — receipt records the target for removal on compensation.
		undo = Receipt{
			ReceiptBase: op.NewReceiptBase(target),
		}
	}

	if err := p.mkdirAll(filepath.Dir(target.SourcePath.Abs()), 0o750); err != nil {
		return nil, Receipt{}, err
	}

	if err := p.symlink(source.SourcePath.Abs(), target.SourcePath.Abs()); err != nil {
		return nil, Receipt{}, err
	}

	if err = target.Resolve(); err != nil {
		return nil, undo, err
	}
	return target, undo, nil
}

// CompensateLink undoes a Link by removing the symlink and restoring whatever was there before.
//
// Parameters:
//   - undo: [file.Receipt] returned by [Provider.Link]
//
// Returns:
//   - error: any error from restoring the previous state
func (p *Provider) CompensateLink(undo Receipt) error {
	return p.compensateWrite(undo)
}

// Mkdir creates a directory (and any missing parents) with the given mode.
//
// Idempotent: if the target directory already exists, calling this function is a no-op and the returned
// receipt is empty (nothing to compensate). Otherwise, the receipt's resource records the directory created
// — the boundary compensation walks up to (exclusive) when removing the created directories.
//
// Parameters:
//   - path: absolute path of the directory to create.
//   - mode: directory permission bits (e.g., 0o755). Defaults to 0755 when 0.
//
// Returns:
//   - product: the [file.Resource] created including populated metadata.
//   - receipt: [file.Receipt] whose Resource marks the directory created; empty when the target directory
//     already existed.
//   - err: any error from resource construction, directory creation, or metadata resolution.
//
// +devlore:defaults mode=0755
func (p *Provider) Mkdir(path string, mode os.FileMode) (product *Resource, receipt Receipt, err error) {

	product, err = NewResource(p.ExecutionContext(), path)
	if err != nil {
		return nil, Receipt{}, err
	}

	// Find nearest ancestor to the directory we've been asked to create.
	//
	// This enables us to completely undo the operation. The scoped root is always a valid stopping point its boundary.

	leaf := product.SourcePath.Abs()

	if info, statErr := p.stat(leaf); statErr == nil && info.IsDir() {
		return product, Receipt{}, nil
	}

	rootName := p.ExecutionContext().Root.Name()
	ancestor := leaf

	for ancestor != rootName {
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			break
		}
		if _, statErr := p.stat(parent); statErr == nil {
			ancestor = parent
			break
		}
		ancestor = parent
	}

	// Create the directory, if we must

	if err := p.mkdirAll(leaf, mode); err != nil {
		return nil, Receipt{}, err
	}

	if err := product.Resolve(); err != nil {
		return nil, Receipt{}, err
	}

	return product, NewReceipt(product), nil
}

// CompensateMkdir undoes a Mkdir by walking up from the directory created, removing it, and stopping at the
// nearest pre-existing ancestor.
//
// Not-exist errors are tolerated (something else already removed the entry). A non-empty directory halts the walk.
// We assume that kater operations adopted the directory and compensation leaves that state alone.
//
// Parameters:
//   - undo: [file.Receipt] returned by [Provider.Mkdir].
//
// Returns:
//   - error: any unexpected error from removing a directory.
func (p *Provider) CompensateMkdir(receipt Receipt) error {

	if receipt.Resource() == nil {
		return nil
	}

	resource, ok := receipt.Resource().(*Resource)
	if !ok {
		return fmt.Errorf("compensate mkdir: unexpected resource type %T", receipt.Resource())
	}

	current := resource.SourcePath.Abs()

	for current != receipt.TransactionID() {

		if err := p.remove(current); err != nil {
			if isDirNotEmpty(err) {
				return nil
			}
			if !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}

		parent := filepath.Dir(current)

		if parent == current {
			break
		}

		current = parent
	}

	return nil
}

// Move moves a file from source to destinationPath using "os.Rename".
//
// Identity for the destination is constructed by [file.NewResource].
//
// Parameters:
//   - source: [file.Resource] at the source location.
//   - destinationPath: the path to move to.
//
// Returns:
//   - result: the destination [file.Resource] with populated metadata.
//   - undo: [file.Receipt] for moving the file back.
//   - err: any error.
func (p *Provider) Move(source *Resource, destinationPath string) (product *Resource, receipt Receipt, err error) {

	product, err = NewResource(p.ExecutionContext(), destinationPath)
	if err != nil {
		return nil, Receipt{}, err
	}

	if _, err := p.stat(source.SourcePath.Abs()); err != nil {
		return nil, Receipt{}, err
	}

	if err := p.mkdirAll(filepath.Dir(product.SourcePath.Abs()), 0o750); err != nil {
		return nil, Receipt{}, err
	}

	if err := p.rename(source.SourcePath.Abs(), product.SourcePath.Abs()); err != nil {
		return nil, Receipt{}, err
	}

	if err := product.Resolve(); err != nil {
		return nil, Receipt{}, err
	}

	return product, NewReceipt(product), nil
}

// CompensateMove undoes a Move by moving the file back to its original location.
//
// Move uses a plain rename (not RecoverySite), so compensation renames back directly. The resource's checksum is
// verified before restoring; a mismatch indicates external modification.
//
// Parameters:
//   - undo: [file.Receipt] returned by [Provider.Move]
//
// Returns:
//   - error: any error from restoring the original file
func (p *Provider) CompensateMove(receipt Receipt) error {

	if receipt.Resource() == nil {
		return nil
	}

	resource, ok := receipt.Resource().(*Resource)

	if !ok {
		return fmt.Errorf("compensate move: unexpected resource type %T", receipt.Resource())
	}

	recoveryID := receipt.TransactionID()

	if resource.Checksum != "" {
		actual := checksumFile(p.ExecutionContext().Root, recoveryID)

		if actual == "" {
			return fmt.Errorf("cannot read %s for verification", recoveryID)
		}

		if actual != resource.Checksum {
			return fmt.Errorf("%s has been modified (checksum mismatch)", recoveryID)
		}
	}

	if err := p.mkdirAll(filepath.Dir(resource.SourcePath.Abs()), 0o755); err != nil {
		return err
	}

	return p.rename(recoveryID, resource.SourcePath.Abs())
}

// Remove deletes the file at "path".
//
// If prune is true and boundary is set, empty parent directories are removed up to the boundary.
//
// +devlore:defaults prune=false,boundary=""
//
// Parameters:
//   - resource: Resource representing the file to delete
//   - prune: If true, remove empty parent directories after deletion
//   - boundary: Stop pruning at this directory (prevents removing too much)
//
// Returns:
//   - result: Receipt for restoring the deleted file
//   - err: any error
func (p *Provider) Remove(resource *Resource, prune bool, boundary *Resource) (product *Resource, receipt Receipt, err error) {

	nonEmptyDirectory, err := p.isDirAndNotEmpty(resource.SourcePath.Abs())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, Receipt{}, nil
		}
		return nil, Receipt{}, err
	}

	if nonEmptyDirectory {
		return nil, Receipt{}, fmt.Errorf("directory %s is not empty", resource.SourcePath.Abs())
	}

	if _, err := p.ExecutionContext().RecoverySite.ArchiveFile(resource.SourcePath); err != nil {
		return nil, Receipt{}, err
	}

	var boundaryPath string

	if boundary != nil {
		boundaryPath = boundary.SourcePath.Abs()
	}

	p.pruneEmptyParents(resource.SourcePath.Abs(), prune, boundaryPath)

	return nil, NewReceipt(resource), nil
}

// CompensateRemove undoes a Remove by restoring the file from recovery.
//
// Parameters:
//   - undo: [file.Receipt] returned by [Provider.Remove]
//
// Returns:
//   - error: any error from restoring the removed file
func (p *Provider) CompensateRemove(receipt Receipt) error {

	if receipt.Resource() == nil {
		return nil
	}

	resource, ok := receipt.Resource().(*Resource)
	if !ok {
		return fmt.Errorf("compensate remove: unexpected resource type %T", receipt.Resource())
	}

	return p.ExecutionContext().RecoverySite.RestoreFile(resource.SourcePath, receipt.TransactionID())
}

// RemoveAll removes the file at "path" and any children it contains.
//
// +devlore:defaults prune=false,boundary=""
//
// Parameters:
//   - path: Resource for the file or directory to remove
//   - prune: If true, remove empty parent directories after deletion
//   - boundary: Stop pruning at this directory (prevents removing too much)
//
// Returns:
//   - result: Receipt for restoring the deleted tree
//   - err: any error
func (p *Provider) RemoveAll(resource *Resource, prune bool, boundary *Resource) (product *Resource, receipt Receipt, err error) {

	if _, err := p.ExecutionContext().RecoverySite.ArchiveFile(resource.SourcePath); err != nil {
		return nil, Receipt{}, err
	}

	p.pruneEmptyParents(resource.SourcePath.Abs(), prune, boundary.SourcePath.Abs())

	return nil, NewReceipt(resource), nil
}

// CompensateRemoveAll undoes a RemoveAll by restoring from recovery.
//
// Parameters:
//   - undo: [file.Receipt] returned by [Provider.RemoveAll]
//
// Returns:
//   - error: any error from restoring the removed files
func (p *Provider) CompensateRemoveAll(receipt Receipt) error {

	if receipt.Resource() == nil {
		return nil
	}

	resource, ok := receipt.Resource().(*Resource)
	if !ok {
		return fmt.Errorf("compensate remove_all: unexpected resource type %T", receipt.Resource())
	}

	return p.ExecutionContext().RecoverySite.RestoreFile(resource.SourcePath, receipt.TransactionID())
}

// Unlink removes a symlink.
//
// If prune is true and boundary is set, empty parent directories are removed up to the boundary.
//
// +devlore:defaults prune=false,boundary=""
//
// Parameters:
//   - resource: Resource for the symlink to remove
//   - prune: If true, remove empty parent directories after unlinking
//   - boundary: Stop pruning at this directory (prevents removing too much)
//
// Returns:
//   - result: Receipt for restoring the deleted symlink
//   - err: any error
func (p *Provider) Unlink(resource *Resource, prune bool, boundary *Resource) (product *Resource, receipt Receipt, err error) {

	info, err := p.lstat(resource.SourcePath.Abs())
	if os.IsNotExist(err) {
		return nil, Receipt{}, nil // Already gone — no change
	}

	if err != nil {
		return nil, Receipt{}, err
	}

	if info.Mode()&os.ModeSymlink == 0 {
		return nil, Receipt{}, fmt.Errorf("%s is not a symlink", resource.SourcePath.Abs())
	}

	if _, err := p.ExecutionContext().RecoverySite.ArchiveFile(resource.SourcePath); err != nil {
		return nil, Receipt{}, err
	}

	p.pruneEmptyParents(resource.SourcePath.Abs(), prune, boundary.SourcePath.Abs())

	return nil, NewReceipt(resource), nil
}

// CompensateUnlink undoes an Unlink by restoring the symlink from recovery.
//
// Parameters:
//   - undo: [file.Receipt] returned by [Provider.Unlink]
//
// Returns:
//   - error: any error from restoring the symlink
func (p *Provider) CompensateUnlink(receipt Receipt) error {

	if receipt.Resource() == nil {
		return nil
	}

	resource, ok := receipt.Resource().(*Resource)
	if !ok {
		return fmt.Errorf("compensate unlink: unexpected resource type %T", receipt.Resource())
	}

	return p.ExecutionContext().RecoverySite.RestoreFile(resource.SourcePath, receipt.TransactionID())
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
func (p *Provider) WalkTree(root *Resource, fn Reducer, honorGitignore bool) (result any, stack *op.RecoveryStack, err error) {
	stack = op.NewRecoveryStack()

	tracker, err := p.newTrackerIfEnabled(root.SourcePath.Abs(), honorGitignore)
	if err != nil {
		return nil, nil, err
	}

	absoluteRoot, err := filepath.Abs(root.SourcePath.Abs())
	if err != nil {
		return nil, nil, err
	}

	if _, err := p.stat(absoluteRoot); err != nil {
		return nil, nil, err
	}

	osRoot := p.ExecutionContext().Root

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

		if skip := p.applyGitignore(tracker, d, relativePath); skip != nil {
			if errors.Is(skip, errSkipEntry) {
				return nil
			}
			return skip
		}

		resource, resErr := NewResource(p.ExecutionContext(), entryAbs)
		if resErr != nil {
			return resErr
		}
		if resErr = resource.Resolve(); resErr != nil {
			return resErr
		}

		result, err = fn(result, resource, relativePath, stack)
		return err
	}

	walkErr := p.walkDir(osRoot, absoluteRoot, walkFn)
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

// WriteBytes writes inline byte content to a file at destinationPath with the given mode.
//
// Identity is constructed by [file.NewResource].
//
// +devlore:defaults mode=0
//
// Parameters:
//   - destinationPath: the path to write to.
//   - content: String content to write to the file.
//   - mode: File permission bits (e.g., 0o644). Defaults to 0644 when 0.
//
// Returns:
//   - result: the destination [file.Resource] with populated metadata.
//   - undo: [file.Receipt] for restoring the previous state.
//   - err: any error that occurred while writing.
func (p *Provider) WriteBytes(destinationPath string, content string, mode os.FileMode) (product *Resource, receipt Receipt, err error) {

	destination, err := NewResource(p.ExecutionContext(), destinationPath)
	if err != nil {
		return nil, Receipt{}, err
	}

	return p.write(destination, []byte(content), mode)
}

// CompensateWriteBytes undoes a WriteBytes by restoring the original file.
//
// Parameters:
//   - undo: [file.Receipt] returned by [Provider.WriteBytes]
//
// Returns:
//   - error: any error from restoring the original file
func (p *Provider) CompensateWriteBytes(receipt Receipt) error {
	return p.compensateWrite(receipt)
}

// WriteText writes inline content to a file at `destinationPath` with the given mode.
//
// Identity is constructed by [file.NewResource]. After the `write` succeeds, the destination resource's metadata is
// populated so the executor's post-dispatch [op.ResourceCatalog.Transition] can fill the pending catalog entry in
// place.
//
// Parameters:
//   - destinationPath: the path to write to.
//   - content: String content to write to the file.
//   - mode: File permission bits (e.g., 0o644). Defaults to 0644 when 0.
//
// Returns:
//   - result: the destination [file.Resource] with populated metadata.
//   - undo: [file.Receipt] for restoring the previous state.
//   - err: any error that occurred while writing.
//
// +devlore:defaults mode=0o755
func (p *Provider) WriteText(destinationPath string, content string, mode os.FileMode) (product *Resource, receipt Receipt, err error) {

	destination, err := NewResource(p.ExecutionContext(), destinationPath)
	if err != nil {
		return nil, Receipt{}, err
	}

	return p.write(destination, []byte(content), mode)
}

// CompensateWriteText undoes a WriteText by restoring the original file.
//
// Parameters:
//   - undo: [file.Receipt] returned by [Provider.WriteText]
//
// Returns:
//   - error: any error from restoring the original file
func (p *Provider) CompensateWriteText(receipt Receipt) error {
	return p.compensateWrite(receipt)
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
func (p *Provider) Exists(resource *Resource) (bool, error) {
	_, err := p.lstat(resource.SourcePath.Abs())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Find returns file paths matching a glob pattern with recursive ** support.
//
// Unlike [Glob], which uses Go's [filepath.Glob] (no ** support), Find walks the directory tree
// and matches each entry against the pattern. The ** wildcard matches zero or more directory levels.
//
// +devlore:defaults honorGitignore=true
//
// Parameters:
//   - pattern: Glob pattern with ** support (e.g., "**/*.go", "src/**/*.yaml")
//   - honorGitignore: If true, filter results using gitignore rules
//
// Returns:
//   - []string: List of matching file paths
//   - error: any error from walking the directory tree
func (p *Provider) Find(pattern string, honorGitignore bool) ([]string, error) {

	root, matchPattern := splitFindPattern(pattern)
	if root == "" {
		root = "."
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("find: resolve root %q: %w", root, err)
	}

	tracker, err := p.newTrackerIfEnabled(absRoot, honorGitignore)
	if err != nil {
		return nil, fmt.Errorf("find: gitignore tracker: %w", err)
	}

	var matches []string
	walkErr := filepath.WalkDir(absRoot, func(entryAbs string, d fs.DirEntry, walkDirErr error) error {
		if walkDirErr != nil {
			return walkDirErr
		}

		relPath, relErr := filepath.Rel(absRoot, entryAbs)
		if relErr != nil {
			return relErr
		}

		if relPath == "." {
			return nil
		}

		if skip := p.applyGitignore(tracker, d, relPath); skip != nil {
			if errors.Is(skip, errSkipEntry) {
				return nil
			}
			return skip
		}

		if d.IsDir() {
			return nil
		}

		if matchDoubleStar(matchPattern, relPath) {
			matches = append(matches, entryAbs)
		}

		return nil
	})

	if walkErr != nil {
		return nil, fmt.Errorf("find: walk %q: %w", absRoot, walkErr)
	}

	return matches, nil
}

// Glob returns [Resource] entries for filesystem paths matching the pattern.
//
// Pattern syntax follows [filepath.Glob] (no `**` recursion — see [Provider.Find] for that). When
// honorGitignore is true and the provider has a Root, results are filtered through a [gitignore.Tracker]
// loaded from Root. Tracker initialization failure is treated as graceful degradation — unfiltered results
// are returned with no error so glob continues to work in trees without gitignore data.
//
// +devlore:defaults honorGitignore=true
//
// Parameters:
//   - pattern: glob pattern (e.g., "*.go", "*.yaml") interpreted by [filepath.Glob].
//   - honorGitignore: when true, filter matches using gitignore rules loaded from Root.
//
// Returns:
//   - []*Resource: one entry per surviving match; per-path resource construction errors leave nil entries
//     (per [Provider.resources]).
//   - error: any error from [filepath.Glob] itself; nil when gitignore tracker initialization fails
//     (degrades to unfiltered output).
func (p *Provider) Glob(pattern string, honorGitignore bool) ([]*Resource, error) {

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	if !honorGitignore || p.Root() == "" {
		return p.resources(matches)
	}

	tracker, err := gitignore.NewTracker(p.Root())
	if err != nil {
		return p.resources(matches) //nolint:nilerr // graceful degradation: return unfiltered if gitignore unavailable
	}

	kept := matches[:0]
	for _, match := range matches {
		if !p.isIgnored(tracker, match) {
			kept = append(kept, match)
		}
	}

	return p.resources(kept)
}

// IsDir returns true if the resource exists and is a directory.
//
// Parameters:
//   - resource: Resource to check
//
// Returns:
//   - bool: true if the resource is a directory, false otherwise
//   - error: permission or other I/O errors (not-exist is not an error)
func (p *Provider) IsDir(resource *Resource) (bool, error) {
	info, err := p.stat(resource.SourcePath.Abs())
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
func (p *Provider) IsFile(resource *Resource) (bool, error) {
	info, err := p.stat(resource.SourcePath.Abs())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return info.Mode().IsRegular(), nil
}

// ReadBytes returns the contents of a file [Resource].
//
// Parameters:
//   - resource: the file resource.
//
// Returns:
//   - result: the contents of the file as an array of bytes
//   - err: I/O error from reading the file through the scoped root
func (p *Provider) ReadBytes(resource *Resource) (result []byte, err error) {
	buffer, err := p.read(resource)
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// ReadText returns the contents of a file [Resource].
//
// Parameters:
//   - resource: the file resource.
//
// Returns:
//   - result: the contents of the file as a string
//   - err: I/O error from reading the file through the scoped root
func (p *Provider) ReadText(resource *Resource) (result string, err error) {
	buffer, err := p.read(resource)
	if err != nil {
		return "", err
	}
	return buffer.String(), nil
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

// Fallible actions

// applyGitignore checks if a directory entry should be skipped based on gitignore rules.
//
// Returns SkipDir to skip directories, a sentinel error to skip files, or nil to proceed.
//
// Parameters:
//   - tracker: gitignore tracker (may be nil to skip gitignore enforcement).
//   - d: directory entry from the walker.
//   - relativePath: entry path relative to the walk root.
//
// Returns:
//   - error: SkipDir for ignored directories, errSkipEntry for ignored files, nil to proceed, or any tracker
//     push error.
func (p *Provider) applyGitignore(tracker *gitignore.Tracker, d fs.DirEntry, relativePath string) error {
	isDir := d.IsDir()

	if isDir && d.Name() == ".git" {
		return SkipDir
	}

	if tracker == nil {
		return nil
	}

	if isDir {
		if pushErr := tracker.Push(relativePath); pushErr != nil {
			return pushErr
		}
	}

	ignored, _ := tracker.IsIgnored(relativePath, isDir)
	if ignored && isDir {
		return SkipDir
	}
	if ignored {
		return errSkipEntry
	}

	return nil
}

// compensateWrite reverts a write or link operation by removing the new file and restoring the original from recovery.
//
// The resource's SourcePath is the file's true home — where the new file was written. When the receipt's
// TransactionID is empty, no file existed before — the new file is simply removed. When TransactionID is set,
// the new file is removed and the old data is restored via [op.RecoverySite.RestoreFile] back to SourcePath.
//
// Parameters:
//   - undo: Receipt from the forward write or link operation
//
// Returns:
//   - error: any error from removing the new file or restoring the original
func (p *Provider) compensateWrite(undo Receipt) error {
	if undo.Resource() == nil {
		return nil
	}

	resource, ok := undo.Resource().(*Resource)
	if !ok {
		return fmt.Errorf("compensate write: unexpected resource type %T", undo.Resource())
	}
	if err := p.remove(resource.SourcePath.Abs()); err != nil && !os.IsNotExist(err) {
		return err
	}

	if undo.TransactionID() == "" {
		return nil
	}

	return p.ExecutionContext().RecoverySite.RestoreFile(resource.SourcePath, undo.TransactionID())
}

// isDirAndNotEmpty checks if the path is a directory that contains at least one entry.
//
// Returns true if it is a directory with contents, false if it's a file, a symlink, or an empty directory.
// Check for existence on error return using errors.Is(err, os.ErrNotExist).
//
// Parameters:
//   - abs: absolute filesystem path.
//
// Returns:
//   - bool: true if the path is a non-empty directory.
//   - error: any error from opening the path or reading its entries.
func (p *Provider) isDirAndNotEmpty(abs string) (_ bool, err error) {
	f, err := p.open(abs)
	if err != nil {
		return false, err
	}
	defer iox.Close(&err, f)

	fileInfo, err := f.Stat()
	if err != nil {
		return false, err
	}

	if !fileInfo.IsDir() {
		return false, nil
	}

	_, err = f.Readdirnames(1)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// lstat returns file info without following symlinks.
//
// Parameters:
//   - abs: Absolute path to stat
//
// Returns:
//   - os.FileInfo: file metadata
//   - error: any stat error
func (p *Provider) lstat(abs string) (os.FileInfo, error) {
	root := p.ExecutionContext().Root
	return root.Lstat(root.NewPath(abs))
}

// mkdirAll creates a directory and all parents.
//
// Parameters:
//   - abs: Absolute path to create
//   - perm: Directory permission bits
//
// Returns:
//   - error: any error from creating the directory
func (p *Provider) mkdirAll(abs string, perm os.FileMode) error {
	root := p.ExecutionContext().Root
	return root.MkdirAll(root.NewPath(abs), perm)
}

// newTrackerIfEnabled creates a gitignore tracker if honorGitignore is true.
//
// Parameters:
//   - rootPath: absolute filesystem path the tracker should treat as the gitignore root.
//   - honorGitignore: when false, returns (nil, nil) without consulting gitignore data.
//
// Returns:
//   - *gitignore.Tracker: a constructed tracker, or nil when honorGitignore is false.
//   - error: any error from [gitignore.NewTracker].
func (p *Provider) newTrackerIfEnabled(rootPath string, honorGitignore bool) (*gitignore.Tracker, error) {
	if !honorGitignore {
		return nil, nil
	}
	return gitignore.NewTracker(rootPath)
}

// open opens a file for reading.
//
// Parameters:
//   - abs: Absolute path to the file
//
// Returns:
//   - *os.File: open file handle
//   - error: any error from opening the file
func (p *Provider) open(abs string) (*os.File, error) {
	root := p.ExecutionContext().Root
	return root.Open(root.NewPath(abs))
}

// openFile opens a file with the given flags and permissions.
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
	root := p.ExecutionContext().Root
	return root.OpenFile(root.NewPath(abs), flag, perm)
}

// prepareWrite handles pre-write backup for destructive operations. If the destination exists, it is moved to a
// recovery site before the write proceeds. If the destination does not exist, the parent directory is created
// and a receipt with an empty TransactionID is returned (compensation will simply remove the newly created file).
//
// Parameters:
//   - resource: Resource for the destination file
//
// Returns:
//   - Resource: resolved destination resource
//   - Receipt: compensation state for undoing the write
//   - error: any error from backup or directory creation
func (p *Provider) prepareWrite(resource *Resource) (result *Resource, undo Receipt, err error) {

	if result, err = NewResource(p.ExecutionContext(), resource.SourcePath.Abs()); err != nil {
		return nil, Receipt{}, err
	}

	if err = result.Resolve(); err != nil {
		return nil, Receipt{}, err
	}

	if !result.Exists() {
		err = p.mkdirAll(filepath.Dir(result.SourcePath.Abs()), 0o750)
		if err != nil {
			return nil, Receipt{}, errors.Join(os.ErrNotExist, err)
		}

		undo = Receipt{
			ReceiptBase: op.NewReceiptBase(result),
		}
		return result, undo, nil
	}

	_, receipt, err := p.Remove(result, false, nil)

	if err != nil {
		return nil, Receipt{}, fmt.Errorf("failed to backup existing file: %w", err)
	}

	return result, receipt, nil
}

// read reads the contents of a file [Resource]
//
// Parameters:
//   - resource: The file resource to read
//
// Returns:
//   - a pointer to a buffer with the contents of the file
//   - any error from reading the file
func (p *Provider) read(resource *Resource) (*bytes.Buffer, error) {
	root := p.ExecutionContext().Root
	data, err := root.ReadFile(root.NewPath(resource.SourcePath.Abs()))
	if err != nil {
		return nil, err
	}
	return bytes.NewBuffer(data), nil
}

// readLink reads the destination of a symlink. Always returns an absolute path.
//
// Parameters:
//   - abs: Absolute path to the symlink
//
// Returns:
//   - string: absolute path the symlink points to
//   - error: any error from reading the link
func (p *Provider) readLink(abs string) (string, error) {
	root := p.ExecutionContext().Root
	target, err := root.Readlink(root.NewPath(abs))
	if err != nil {
		return "", err
	}

	// Root.Readlink returns the symlink target as stored (relative). Resolve to absolute for comparison.
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(abs), target)
	}

	return filepath.Clean(target), nil
}

// remove removes a file or empty directory.
//
// Parameters:
//   - abs: Absolute path to remove
//
// Returns:
//   - error: any error from removing the file or directory
func (p *Provider) remove(abs string) error {
	root := p.ExecutionContext().Root
	return root.Remove(root.NewPath(abs))
}

// rename moves a file from oldAbs to newAbs.
//
// Parameters:
//   - oldAbs: Absolute path of the file to move
//   - newAbs: Absolute path of the destination
//
// Returns:
//   - error: any error from the rename operation
func (p *Provider) rename(oldAbs, newAbs string) error {
	root := p.ExecutionContext().Root
	return root.Rename(root.NewPath(oldAbs), root.NewPath(newAbs))
}

// resources constructs a [Resource] for each input path.
//
// Each entry is built via [NewResource] under the provider's [op.ExecutionContext].
//
// Parameters:
//   - paths: filesystem paths in the order callers want preserved on the output.
//
// Returns:
//   - []*Resource: one entry per input path or nil, if an error is encountered
//   - error: first error encountered or nil
func (p *Provider) resources(paths []string) (product []*Resource, err error) {

	resources := make([]*Resource, len(paths))

	for i, path := range paths {
		resources[i], err = NewResource(p.ExecutionContext(), path)
		if err != nil {
			return nil, err
		}
	}

	return resources, nil
}

// stat returns file info following symlinks.
//
// Parameters:
//   - abs: Absolute path to stat
//
// Returns:
//   - os.FileInfo: file metadata
//   - error: any stat error
func (p *Provider) stat(abs string) (os.FileInfo, error) {
	root := p.ExecutionContext().Root
	return root.Stat(root.NewPath(abs))
}

// symlink creates a symbolic link.
//
// The target is stored as a relative path (os.Root requires non-absolute symlink targets).
//
// Parameters:
//   - targetAbs: Absolute path that the symlink should point to
//   - linkAbs: Absolute path where the symlink should be created
//
// Returns:
//   - error: any error from creating the symlink
func (p *Provider) symlink(targetAbs, linkAbs string) error {
	root := p.ExecutionContext().Root
	relTarget, err := filepath.Rel(filepath.Dir(linkAbs), targetAbs)
	if err != nil {
		return err
	}
	return root.Symlink(relTarget, root.NewPath(linkAbs))
}

// walkDir dispatches to fs.WalkDir (root-scoped) or filepath.WalkDir (unscoped).
//
// Parameters:
//   - osRoot: scoped [op.Root]; when non-nil, the walk uses [fs.WalkDir] over its filesystem and rebases
//     relative paths back to absolute via [op.Root.Name].
//   - absoluteRoot: absolute filesystem path of the walk root.
//   - walkFn: callback invoked for each entry.
//
// Returns:
//   - error: any error from the underlying walk implementation.
func (p *Provider) walkDir(osRoot op.Root, absoluteRoot string, walkFn func(string, fs.DirEntry, error) error) error {
	if osRoot != nil {
		relRoot := osRoot.NewPath(absoluteRoot).Rel()
		return fs.WalkDir(osRoot.FS(), relRoot, func(relPath string, d fs.DirEntry, walkDirErr error) error {
			return walkFn(filepath.Join(osRoot.Name(), relPath), d, walkDirErr)
		})
	}
	return filepath.WalkDir(absoluteRoot, walkFn)
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
//   - Receipt: compensation state for undoing the write
//   - error: any error from writing
func (p *Provider) write(resource *Resource, data []byte, mode os.FileMode) (result *Resource, undo Receipt, err error) {
	result, undo, err = p.prepareWrite(resource)
	if err != nil {
		return nil, Receipt{}, err
	}

	if mode == 0 {
		mode = 0o644
	}

	f, err := p.openFile(result.SourcePath.Abs(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return result, undo, err
	}
	defer iox.Close(&err, f)

	hasher := sha256.New()
	mw := io.MultiWriter(f, hasher)

	_, err = mw.Write(data)
	if err != nil {
		return result, undo, err
	}

	if err = f.Sync(); err != nil {
		return result, undo, err
	}

	err = result.RefreshWith(hex.EncodeToString(hasher.Sum(nil)))
	if err != nil {
		return result, undo, err
	}

	return result, undo, nil
}

// Actions

// isIgnored answers whether the gitignore tracker considers path filtered.
//
// Computes the relative path from tracker.Root to path, stats the entry to determine its directory-ness, then asks the
// tracker. Failures along the way default to "not ignored" so the entry survives — this is graceful-degradation in the
// same spirit as [Glob]'s tracker-init failure handling.
//
// Parameters:
//   - tracker: a constructed [gitignore.Tracker] rooted at the provider's Root.
//   - path: an absolute filesystem path produced by globbing.
//
// Returns:
//   - bool: true only when the rel-path computation succeeds and the tracker reports the path as ignored.
func (p *Provider) isIgnored(tracker *gitignore.Tracker, path string) bool {

	rel, err := filepath.Rel(tracker.Root(), path)
	if err != nil {
		return false
	}

	info, statErr := p.stat(path)
	isDir := statErr == nil && info.IsDir()

	ignored, _ := tracker.IsIgnored(rel, isDir)
	return ignored
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

// endregion

// endregion

// region Helpers

// checksumBytes computes "sha256:<hex>" for content bytes.
func checksumBytes(data []byte) string {
	h := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(h[:])
}

// checksumFile reads a path and returns its "sha256:<hex>" checksum. I/O is scoped through [op.Root]. Returns empty
// string if the file cannot be read.
func checksumFile(root op.Root, path string) string {

	data, err := root.ReadFile(root.NewPath(path))
	if err != nil {
		return ""
	}

	return checksumBytes(data)
}

// isDirNotEmpty reports whether err is the "directory not empty" error returned by the kernel when attempting to
// remove a non-empty directory.
//
// Parameters:
//   - err: the error returned by a remove attempt.
//
// Returns:
//   - bool: true if err is ENOTEMPTY (or its platform equivalent).
func isDirNotEmpty(err error) bool {
	return errors.Is(err, syscall.ENOTEMPTY)
}

// matchDoubleStar matches a path against a pattern containing ** wildcards.
//
// `**` matches zero or more directory levels; other wildcards follow [filepath.Match] rules. Patterns with multiple
// `**` segments fall back to a tail match against the path's basename.
//
// Parameters:
//   - pattern: glob pattern with optional `**` segments.
//   - path: candidate path (typically relative to a walk root).
//
// Returns:
//   - bool: true when the path matches the pattern.
func matchDoubleStar(pattern, path string) bool {

	parts := strings.Split(pattern, "**")
	if len(parts) == 1 {
		return pathMatch(pattern, path)
	}

	// Common case: prefix**suffix (e.g., "**/*.go" → prefix="", suffix="/*.go").
	if len(parts) == 2 {
		return matchDoubleStarSingle(parts[0], parts[1], path)
	}

	// Multiple ** segments — fall back to simple tail match.
	tail := strings.TrimLeft(parts[len(parts)-1], string(filepath.Separator))
	return pathMatch(tail, filepath.Base(path))
}

// matchDoubleStarSingle handles patterns with exactly one `**` wildcard.
//
// Trims the prefix/suffix's path separators, requires the path to start with prefix, then matches suffix
// against every possible tail of the remainder.
//
// Parameters:
//   - rawPrefix: portion of the pattern before `**`.
//   - rawSuffix: portion of the pattern after `**`.
//   - path: candidate path.
//
// Returns:
//   - bool: true when prefix matches the start and suffix matches some tail of the remainder.
func matchDoubleStarSingle(rawPrefix, rawSuffix, path string) bool {

	prefix := strings.TrimRight(rawPrefix, string(filepath.Separator))
	suffix := strings.TrimLeft(rawSuffix, string(filepath.Separator))

	if prefix != "" {
		if !strings.HasPrefix(path, prefix+string(filepath.Separator)) && path != prefix {
			return false
		}
		path = strings.TrimPrefix(path, prefix+string(filepath.Separator))
	}

	// Match suffix against every possible tail of the remaining path.

	segments := strings.Split(path, string(filepath.Separator))

	for i := range segments {
		tail := strings.Join(segments[i:], string(filepath.Separator))
		if pathMatch(suffix, tail) {
			return true
		}
	}

	return false
}

// pathMatch wraps [filepath.Match], treating errors as non-matches.
//
// Parameters:
//   - pattern: glob pattern.
//   - name: candidate name.
//
// Returns:
//   - bool: true when [filepath.Match] returns ok with no error.
func pathMatch(pattern, name string) bool {
	ok, err := filepath.Match(pattern, name)
	return err == nil && ok
}

// splitFindPattern splits a pattern like "src/**/*.go" into a root directory ("src") and a match pattern
// ("**/*.go"). If the pattern starts with `**`, root is empty.
//
// Parameters:
//   - pattern: glob pattern, optionally containing `**`.
//
// Returns:
//   - root: directory portion preceding the first `**` (or [filepath.Dir] of pattern when no `**` present).
//   - match: portion of the pattern from the first `**` onward (or [filepath.Base] of pattern when no `**`).
func splitFindPattern(pattern string) (root, match string) {

	idx := strings.Index(pattern, "**")
	if idx < 0 {
		// No ** — treat the directory part as root, filename as match.
		return filepath.Dir(pattern), filepath.Base(pattern)
	}

	root = strings.TrimRight(pattern[:idx], string(filepath.Separator))
	match = pattern[idx:]

	return root, match
}
