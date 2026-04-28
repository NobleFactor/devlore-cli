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
func (p *Provider) Backup(source *Resource, backupSuffix string) (*Resource, *Receipt, error) {

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
func (p *Provider) CompensateBackup(receipt *Receipt) error {
	return p.CompensateMove(receipt)
}

// Copy copies source's contents to a new file at destinationPath with the given mode.
//
// Identity for the destination is constructed by [file.NewResource]. If the destination already exists, it is archived
// to the recovery site before writing. After the `write` succeeds, the destination resource's metadata is populated via
// [Resource.Resolve], which is what the executor's post-dispatch [op.ResourceCatalog.Transition] consumes to fill the
// pending entry in place.
//
// Parameters:
//   - source: [file.Resource] of the file to copy from.
//   - destinationPath: the path to write to.
//   - mode: the file mode for the new file.
//
// Returns:
//   - product: the destination file with populated metadata.
//   - receipt: for restoring the original state at destination.
//   - err: any error from identity construction, backup, or the copy itself.
func (p *Provider) Copy(source *Resource, destinationPath string, mode os.FileMode) (product *Resource, receipt *Receipt, err error) {

	product, err = NewResource(p.ExecutionContext(), destinationPath)
	if err != nil {
		return nil, nil, err
	}

	product, receipt, err = p.prepareWrite(product)
	if err != nil {
		return nil, nil, err
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
//   - receipt: [file.Receipt] returned by [Provider.Copy]
//
// Returns:
//   - error: any error from restoring the original file
func (p *Provider) CompensateCopy(receipt *Receipt) error {
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
func (p *Provider) Link(source *Resource, targetPath string) (product *Resource, receipt *Receipt, err error) {

	product, err = NewResource(p.ExecutionContext(), targetPath)
	if err != nil {
		return nil, nil, err
	}

	if info, err := p.lstat(product.SourcePath.Abs()); err == nil {

		if info.Mode()&os.ModeSymlink != 0 {
			existing, readErr := p.readLink(product.SourcePath.Abs())
			if readErr == nil && existing == source.SourcePath.Abs() {
				return product, nil, nil // Already correct — no change
			}
		}

		// Something exists at the target — archive it before creating the symlink.

		if _, archiveErr := p.ExecutionContext().RecoverySite.ArchiveFile(product.SourcePath); archiveErr != nil {
			return nil, nil, archiveErr
		}
	}

	parentPath := filepath.Dir(product.SourcePath.Abs())

	boundary, _, err := p.closestExistingDir(parentPath)
	if err != nil {
		return nil, nil, err
	}

	receipt = NewReceiptWithBoundary(product, boundary)

	if err = p.mkdirAll(parentPath, 0o750); err != nil {
		return nil, receipt, err
	}

	if err = p.symlink(source.SourcePath.Abs(), product.SourcePath.Abs()); err != nil {
		return nil, receipt, err
	}

	if err = product.Resolve(); err != nil {
		return nil, receipt, err
	}

	return product, receipt, nil
}

// CompensateLink undoes a Link by removing the symlink and restoring whatever was there before.
//
// Parameters:
//   - receipt: [file.Receipt] returned by [Provider.Link]
//
// Returns:
//   - error: any error from restoring the previous state
func (p *Provider) CompensateLink(receipt *Receipt) error {
	return p.compensateWrite(receipt)
}

// Mkdir creates a directory (and any missing parents) with the given mode.
//
// Idempotent: if the target directory already exists, calling this function is a no-op and the returned receipt is
// empty (nothing to compensate). Otherwise, the receipt's resource records the directory created. The boundary
// compensation walks up to (exclusive) when removing the created directories.
//
// Parameters:
//   - path: absolute path of the directory to create.
//   - mode: directory permission bits (e.g., 0o755). Defaults to 0755 when 0.
//
// Returns:
//   - product: the [file.Resource] created including populated metadata.
//   - receipt: Marks the directory created; empty when the target directory already existed.
//   - err: any error from resource construction, directory creation, or metadata resolution.
func (p *Provider) Mkdir(path string, mode os.FileMode) (product *Resource, receipt *Receipt, err error) {

	product, err = NewResource(p.ExecutionContext(), path)
	if err != nil {
		return nil, nil, err
	}

	leaf := product.SourcePath.Abs()

	boundary, info, err := p.closestExistingDir(leaf)
	if err != nil {
		return nil, nil, err
	}

	if boundary.SourcePath.Abs() == leaf {
		if info.IsDir() {
			return product, nil, nil // directory exists and there's nothing to compensate
		}
		return nil, nil, fmt.Errorf("%s exists, but is not a directory", path)
	}

	receipt = NewReceiptWithBoundary(product, boundary)

	if err = p.mkdirAll(leaf, mode); err != nil {
		return nil, receipt, err
	}

	if err = product.Resolve(); err != nil {
		return nil, receipt, err
	}

	return product, receipt, nil
}

// CompensateMkdir undoes a [Provider.Mkdir] by walking up from the receipt's resource and removing each entry
// until it reaches the boundary recorded on the receipt (exclusive).
//
// The boundary is the nearest pre-existing ancestor that [Provider.Mkdir] discovered at forward time;
// everything between the leaf and the boundary was created by that call and is owned by this compensation.
// The walk stops at boundary because boundary itself was not created — removing it would erase pre-existing
// state.
//
// Tolerated conditions during the walk:
//   - [os.ErrNotExist] on a removal: something else already removed the entry; keep walking up so any
//     remaining ancestors that we did create still get cleaned.
//   - "directory not empty": a later, still-live action adopted the directory. The walk halts cleanly and
//     leaves the surviving subtree alone.
//
// Tamper guard: the receipt's resource must be a descendant of (or equal to) its boundary. If
// [filepath.Rel] reports otherwise, compensation refuses to walk — removing entries outside the boundary
// subtree would erase state the forward call never touched.
//
// Receipts that carry no resource (e.g., [Provider.Mkdir]'s idempotent "already exists" path returns the
// zero [Receipt]) and receipts that carry no boundary (legacy or non-creating receipts) are no-ops.
//
// Parameters:
//   - receipt: [Receipt] returned by [Provider.Mkdir].
//
// Returns:
//   - error: a malformed-receipt error if the boundary is not an ancestor of the resource, an
//     unexpected-resource-type error if the receipt's resource is not a [*file.Resource], or any
//     non-tolerated removal error from the walk.
func (p *Provider) CompensateMkdir(receipt *Receipt) (err error) {

	if receipt == nil || receipt.Resource() == nil {
		return nil
	}

	resource, ok := receipt.Resource().(*Resource)
	if !ok {
		return fmt.Errorf("unexpected resource type %T", receipt.Resource())
	}

	boundary := receipt.Boundary()
	if boundary == nil {
		return nil // no recorded boundary — receipt does not own a creation subtree
	}

	boundaryPath := boundary.SourcePath.Abs()
	current := resource.SourcePath.Abs()

	// Tamper guard: current must lie inside the boundary subtree. Walking outside would remove pre-existing entries the
	// forward Mkdir never created.

	var relativePath string
	relativePath, err = filepath.Rel(boundaryPath, current)

	if err != nil || strings.HasPrefix(relativePath, "..") {
		return fmt.Errorf("resource %s is not under boundary %s", current, boundaryPath)
	}

	for current != boundaryPath {

		if err := p.remove(current); err != nil {
			if isDirNotEmpty(err) {
				return nil // sibling adopted the dir; leave it alone
			}
			if !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}

		parent := filepath.Dir(current)

		if parent == current {
			break // filesystem root — defensive; rel-check above should make this unreachable
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
//   - product: the destination [file.Resource] with populated metadata.
//   - receipt: [file.Receipt] for moving the file back.
//   - err: any error.
func (p *Provider) Move(source *Resource, destinationPath string) (product *Resource, receipt *Receipt, err error) {

	product, err = NewResource(p.ExecutionContext(), destinationPath)
	if err != nil {
		return nil, nil, err
	}

	if _, err = p.stat(source.SourcePath.Abs()); err != nil {
		return nil, nil, err
	}

	parentPath := filepath.Dir(product.SourcePath.Abs())

	boundary, _, err := p.closestExistingDir(parentPath)
	if err != nil {
		return nil, nil, err
	}

	receipt = NewReceiptWithBoundary(product, boundary)

	if err = p.mkdirAll(parentPath, 0o750); err != nil {
		return nil, receipt, err
	}

	if err = p.rename(source.SourcePath.Abs(), product.SourcePath.Abs()); err != nil {
		return nil, receipt, err
	}

	if err = product.Resolve(); err != nil {
		return nil, receipt, err
	}

	return product, receipt, nil
}

// CompensateMove undoes a Move by moving the file back to its original location.
//
// Move uses a plain rename (not RecoverySite), so compensation renames back directly. The resource's checksum is
// verified before restoring; a mismatch indicates external modification.
//
// Parameters:
//   - receipt: [file.Receipt] returned by [Provider.Move]
//
// Returns:
//   - error: any error from restoring the original file
func (p *Provider) CompensateMove(receipt *Receipt) error {

	if receipt == nil || receipt.Resource() == nil {
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
func (p *Provider) Remove(resource *Resource, prune bool, boundary *Resource) (product *Resource, receipt *Receipt, err error) {

	nonEmptyDirectory, err := p.isDirAndNotEmpty(resource.SourcePath.Abs())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	if nonEmptyDirectory {
		return nil, nil, fmt.Errorf("directory %s is not empty", resource.SourcePath.Abs())
	}

	if err := p.archiveAndPrune(resource, prune, boundary); err != nil {
		return nil, nil, err
	}

	return nil, NewReceipt(resource), nil
}

// CompensateRemove undoes a Remove by restoring the file from recovery.
//
// Parameters:
//   - receipt: [file.Receipt] returned by [Provider.Remove]
//
// Returns:
//   - error: any error from restoring the removed file
func (p *Provider) CompensateRemove(receipt *Receipt) error {

	if receipt == nil || receipt.Resource() == nil {
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
func (p *Provider) RemoveAll(resource *Resource, prune bool, boundary *Resource) (product *Resource, receipt *Receipt, err error) {

	if err := p.archiveAndPrune(resource, prune, boundary); err != nil {
		return nil, nil, err
	}

	return nil, NewReceipt(resource), nil
}

// CompensateRemoveAll undoes a RemoveAll by restoring from recovery.
//
// Parameters:
//   - receipt: [file.Receipt] returned by [Provider.RemoveAll]
//
// Returns:
//   - error: any error from restoring the removed files
func (p *Provider) CompensateRemoveAll(receipt *Receipt) error {

	if receipt == nil || receipt.Resource() == nil {
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
func (p *Provider) Unlink(resource *Resource, prune bool, boundary *Resource) (product *Resource, receipt *Receipt, err error) {

	info, err := p.lstat(resource.SourcePath.Abs())
	if os.IsNotExist(err) {
		return nil, nil, nil // Already gone — no change
	}

	if err != nil {
		return nil, nil, err
	}

	if info.Mode()&os.ModeSymlink == 0 {
		return nil, nil, fmt.Errorf("%s is not a symlink", resource.SourcePath.Abs())
	}

	if err := p.archiveAndPrune(resource, prune, boundary); err != nil {
		return nil, nil, err
	}

	return nil, NewReceipt(resource), nil
}

// CompensateUnlink undoes an Unlink by restoring the symlink from recovery.
//
// Parameters:
//   - receipt: [file.Receipt] returned by [Provider.Unlink]
//
// Returns:
//   - error: any error from restoring the symlink
func (p *Provider) CompensateUnlink(receipt *Receipt) error {

	if receipt == nil || receipt.Resource() == nil {
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
// The visitor can push compensable operations onto the stack during traversal. On any error, walking stops
// and the partial stack is returned alongside the error — the caller is responsible for invoking
// [Provider.CompensateWalkTree] to unwind it. On success, the accumulated result and the populated stack are
// returned; the stack serves as the undo receipt.
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
func (p *Provider) WalkTree(root *Resource, fn Reducer, honorGitignore bool) (product any, stack *op.RecoveryStack, err error) {

	stack = op.NewRecoveryStack()

	var tracker *gitignore.Tracker

	tracker, err = p.newTrackerIfEnabled(root.SourcePath.Abs(), honorGitignore)
	if err != nil {
		return nil, nil, err
	}

	var absoluteRoot string

	absoluteRoot, err = filepath.Abs(root.SourcePath.Abs())
	if err != nil {
		return nil, nil, err
	}

	if _, err = p.stat(absoluteRoot); err != nil {
		return nil, nil, err
	}

	osRoot := p.ExecutionContext().Root

	walkFn := func(entryAbs string, d fs.DirEntry, walkDirErr error) error {

		if walkDirErr != nil {
			return walkDirErr
		}

		var relativePath string

		relativePath, err = filepath.Rel(absoluteRoot, entryAbs)
		if err != nil {
			return err
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

		// WalkTree is an observation path: the entries we visit are existing filesystem state, not action outputs.
		// Route through the catalog so two scans that hit the same path return the same canonical *Resource.
		ctx := p.ExecutionContext()
		candidate, err := NewResource(ctx, entryAbs)
		if err != nil {
			return err
		}

		got, err := ctx.Catalog.GetOrCreate(candidate.URI(), func() (op.Resource, error) {
			return candidate, nil
		})
		if err != nil {
			return err
		}

		resource, ok := got.(*Resource)
		if !ok {
			return fmt.Errorf("walk tree: catalog entry for %q is %T, want *file.Resource", candidate.URI(), got)
		}

		if err = resource.Resolve(); err != nil {
			return err
		}

		product, err = fn(product, resource, relativePath, stack)
		return err
	}

	if err = p.walkDir(osRoot, absoluteRoot, walkFn); err != nil {
		return nil, stack, err
	}

	return product, stack, nil
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
//   - receipt: [file.Receipt] for restoring the previous state.
//   - err: any error that occurred while writing.
func (p *Provider) WriteBytes(destinationPath string, content string, mode os.FileMode) (product *Resource, receipt *Receipt, err error) {

	product, err = NewResource(p.ExecutionContext(), destinationPath)
	if err != nil {
		return nil, nil, err
	}

	return p.write(product, []byte(content), mode)
}

// CompensateWriteBytes undoes a WriteBytes by restoring the original file.
//
// Parameters:
//   - receipt: [file.Receipt] returned by [Provider.WriteBytes]
//
// Returns:
//   - error: any error from restoring the original file
func (p *Provider) CompensateWriteBytes(receipt *Receipt) error {
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
//   - mode: File permission bits (e.g., 0o644). The +devlore:defaults directive supplies 0o666 once the
//     codegen extension lands (13.0(f)); until then, callers must pass mode explicitly.
//
// Returns:
//   - result: the destination [file.Resource] with populated metadata.
//   - receipt: [file.Receipt] for restoring the previous state.
//   - err: any error that occurred while writing.
//
// +devlore:defaults mode=0o666
func (p *Provider) WriteText(destinationPath string, content string, mode os.FileMode) (product *Resource, receipt *Receipt, err error) {

	product, err = NewResource(p.ExecutionContext(), destinationPath)
	if err != nil {
		return nil, nil, err
	}

	return p.write(product, []byte(content), mode)
}

// CompensateWriteText undoes a WriteText by restoring the original file.
//
// Parameters:
//   - receipt: [file.Receipt] returned by [Provider.WriteText]
//
// Returns:
//   - error: any error from restoring the original file
func (p *Provider) CompensateWriteText(receipt *Receipt) error {
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
// Unlike [Glob], which uses Go's [filepath.Glob] (no ** support), Find walks the directory tree and matches each entry
// against the pattern. The ** wildcard matches zero or more directory levels.
//
// Parameters:
//   - pattern: Glob pattern with ** support (e.g., "**/*.go", "src/**/*.yaml")
//   - honorGitignore: If true, filter results using gitignore rules
//
// Returns:
//   - []*Resource: matching file resources.
//   - error: any error from walking the directory tree.
//
// +devlore:defaults honorGitignore=true
func (p *Provider) Find(pattern string, honorGitignore bool) (product []*Resource, err error) {

	scopedRoot := p.Root()

	root, matchPattern := splitFindPattern(pattern)
	if root == "" {
		root = "."
	}

	var absoluteRoot string

	if filepath.IsAbs(root) {
		absoluteRoot = filepath.Clean(root)
	} else {
		absoluteRoot = filepath.Clean(filepath.Join(scopedRoot, root))
	}

	var relativePath string
	relativePath, err = filepath.Rel(scopedRoot, absoluteRoot)

	if err != nil || strings.HasPrefix(relativePath, "..") {
		return nil, fmt.Errorf("find: pattern %q resolves to %s, which lies outside scoped root %s",
			pattern,
			absoluteRoot,
			scopedRoot)
	}

	var tracker *gitignore.Tracker

	tracker, err = p.newTrackerIfEnabled(absoluteRoot, honorGitignore)
	if err != nil {
		return nil, fmt.Errorf("find: gitignore tracker: %w", err)
	}

	matches := make([]string, 0, 8192)

	err = p.walkDir(p.ExecutionContext().Root, absoluteRoot, func(absolutePath string, dirEntry fs.DirEntry, err error) error {

		if err != nil {
			return err
		}

		var relativePath string

		relativePath, err = filepath.Rel(absoluteRoot, absolutePath)
		if err != nil {
			return err
		}

		if relativePath == "." {
			return nil
		}

		if skip := p.applyGitignore(tracker, dirEntry, relativePath); skip != nil {
			if errors.Is(skip, errSkipEntry) {
				return nil
			}
			return skip
		}

		if dirEntry.IsDir() {
			return nil
		}

		if matchDoubleStar(matchPattern, relativePath) {
			matches = append(matches, absolutePath)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("find: walk %q: %w", absoluteRoot, err)
	}

	return p.resources(matches)
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
func (p *Provider) ReadBytes(resource *Resource) (product []byte, err error) {

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
func (p *Provider) ReadText(resource *Resource) (product string, err error) {

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
//   - tracker: gitignore tracker (it may be nil to skip gitignore enforcement).
//   - d: directory entry from the walker.
//   - relativePath: entry path relative to the walk root.
//
// Returns:
//   - error: SkipDir for ignored directories, errSkipEntry for ignored files, nil to proceed, or a tracker push error.
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

// compensateWrite reverts a write or link operation by removing the new file and restoring whatever existed before.
//
// The resource's SourcePath is the file's true home — where the new file was written. An empty TransactionID means
// no file existed before, so the new file is simply removed; a non-empty TransactionID means the old data is
// restored via [op.RecoverySite.RestoreFile] back to SourcePath after removing the new file.
//
// When the receipt carries a [Receipt.Boundary], the forward action created intermediate parent directories that
// did not exist before. After the file itself is removed (or restored), the walk climbs from the file's parent
// up to (but excluding) the boundary, removing each empty directory in turn — same pattern as [Provider.CompensateMkdir].
// Tolerated conditions: a non-empty directory halts the walk cleanly (a sibling action adopted the directory),
// and a not-exist error keeps walking up. A receipt without a boundary skips the directory walk entirely.
//
// Parameters:
//   - receipt: [Receipt] from the forward write or link operation.
//
// Returns:
//   - error: a malformed-receipt error if boundary is not an ancestor of the resource, an unexpected-resource-type
//     error if the receipt's resource is not a [*file.Resource], or any non-tolerated removal or restoration error.
func (p *Provider) compensateWrite(receipt *Receipt) error {

	if receipt == nil || receipt.Resource() == nil {
		return nil
	}

	resource, ok := receipt.Resource().(*Resource)
	if !ok {
		return fmt.Errorf("compensate write: unexpected resource type %T", receipt.Resource())
	}

	if err := p.remove(resource.SourcePath.Abs()); err != nil && !os.IsNotExist(err) {
		return err
	}

	if err := p.ExecutionContext().RecoverySite.RestoreFile(resource.SourcePath, receipt.TransactionID()); err != nil {
		if !errors.Is(err, op.ErrRecoverySourceNotFound) {
			return err
		}
		// No archive: the forward action created new state without displacing existing state. The earlier remove
		// already cleaned the leaf; nothing to restore.
	}

	boundary := receipt.Boundary()
	if boundary == nil {
		return nil
	}

	boundaryPath := boundary.SourcePath.Abs()
	current := filepath.Dir(resource.SourcePath.Abs())

	relativePath, err := filepath.Rel(boundaryPath, current)
	if err != nil || strings.HasPrefix(relativePath, "..") {
		return fmt.Errorf("resource %s is not under boundary %s", current, boundaryPath)
	}

	for current != boundaryPath {

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

// closestExistingDir walks up from path to find the nearest existing entry within the scoped [Provider.Root] hierarchy.
//
// The returned [Resource] is the transactional boundary that forward methods record on the [Receipt] they emit; later
// compensation walks from the created leaf up to but excluding this boundary and removes each empty directory along the
// way. The walk clamps at [Provider.Root] and returns a scope-violation error rather than handing an outside ancestor.
//
// The returned [os.FileInfo] is the value [Provider.stat] produced for ancestor's path. Callers that passed a path that
// is expected to be newly created compare ancestor's absolute path to their input to recognize the already-exists case,
// and consult info to recognize the type-mismatch case (a regular file where a directory was expected). Callers that
// only need the boundary discard info.
//
// Parameters:
//   - path: an absolute path inside the scoped [Provider.Root].
//
// Returns:
//   - ancestor: the [Resource] for the nearest existing entry at or above path.
//   - info: the [os.FileInfo] for ancestor; meaningful when ancestor's absolute path equals the input path.
//   - err: a scope-violation error if path lies outside [Provider.Root], or a root-missing error if the scoped root
//     itself cannot be statted.
func (p *Provider) closestExistingDir(path string) (ancestor *Resource, info os.FileInfo, err error) {

	root := p.Root()

	rel, relErr := filepath.Rel(root, path)
	if relErr != nil || strings.HasPrefix(rel, "..") {
		return nil, nil, fmt.Errorf("%s lies outside scoped root %s", path, root)
	}

	ctx := p.ExecutionContext()
	current := path

	for {
		if info, err = p.stat(current); err == nil {
			candidate, buildErr := NewResource(ctx, current)
			if buildErr != nil {
				return nil, nil, buildErr
			}
			got, getErr := ctx.Catalog.GetOrCreate(candidate.URI(), func() (op.Resource, error) {
				return candidate, nil
			})
			if getErr != nil {
				return nil, nil, getErr
			}
			a, ok := got.(*Resource)
			if !ok {
				return nil, nil, fmt.Errorf("closest existing dir: catalog entry for %q is %T, want *file.Resource", candidate.URI(), got)
			}
			return a, info, nil
		}

		if current == root {
			return nil, nil, fmt.Errorf("scoped root %s does not exist or is not accessible", root)
		}

		current = filepath.Dir(current)
	}
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
//   - product: resolved destination resource
//   - receipt: compensation state for undoing the write
//   - error: any error from backup or directory creation
func (p *Provider) prepareWrite(resource *Resource) (product *Resource, receipt *Receipt, err error) {

	if product, err = NewResource(p.ExecutionContext(), resource.SourcePath.Abs()); err != nil {
		return nil, nil, err
	}

	if err = product.Resolve(); err != nil {
		return nil, nil, err
	}

	if !product.Exists() {

		parentPath := filepath.Dir(product.SourcePath.Abs())

		boundary, _, err := p.closestExistingDir(parentPath)
		if err != nil {
			return nil, nil, err
		}

		receipt = NewReceiptWithBoundary(product, boundary)

		if err = p.mkdirAll(parentPath, 0o750); err != nil {
			return nil, receipt, err
		}

		return product, receipt, nil
	}

	_, receipt, err = p.Remove(product, false, nil)

	if err != nil {
		return nil, nil, fmt.Errorf("failed to backup existing file: %w", err)
	}

	return product, receipt, nil
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

// resources constructs a [Resource] for each input path, routing through [op.ResourceCatalog.GetOrCreate].
//
// This is an observation path — Find / Glob have already matched paths against existing filesystem state. Routing
// through the catalog gives every match canonical identity: two scans that hit the same path return the same
// *Resource. URIs not yet in the catalog become discovery entries (empty originID); existing entries are re-used.
//
// Parameters:
//   - paths: filesystem paths in the order callers want preserved on the output.
//
// Returns:
//   - []*Resource: one entry per input path or nil, if an error is encountered
//   - error: first error encountered or nil
func (p *Provider) resources(paths []string) (product []*Resource, err error) {

	ctx := p.ExecutionContext()
	resources := make([]*Resource, len(paths))

	for i, path := range paths {
		var candidate *Resource
		candidate, err = NewResource(ctx, path)
		if err != nil {
			return nil, err
		}
		var got op.Resource
		got, err = ctx.Catalog.GetOrCreate(candidate.URI(), func() (op.Resource, error) {
			return candidate, nil
		})
		if err != nil {
			return nil, err
		}
		concrete, ok := got.(*Resource)
		if !ok {
			return nil, fmt.Errorf("resources: catalog entry for %q is %T, want *file.Resource", candidate.URI(), got)
		}
		resources[i] = concrete
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
//   - product: resolved resource for the written file
//   - receipt: compensation state for undoing the write
//   - error: any error from writing
func (p *Provider) write(resource *Resource, data []byte, mode os.FileMode) (product *Resource, receipt *Receipt, err error) {

	product, receipt, err = p.prepareWrite(resource)
	if err != nil {
		return nil, nil, err
	}

	f, err := p.openFile(product.SourcePath.Abs(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return product, receipt, err
	}
	defer iox.Close(&err, f)

	hasher := sha256.New()
	mw := io.MultiWriter(f, hasher)

	_, err = mw.Write(data)
	if err != nil {
		return product, receipt, err
	}

	if err = f.Sync(); err != nil {
		return product, receipt, err
	}

	err = product.RefreshWith(hex.EncodeToString(hasher.Sum(nil)))
	if err != nil {
		return product, receipt, err
	}

	return product, receipt, nil
}

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

// pruneEmptyParents removes empty parent directories of resource up to (but excluding) the boundary directory.
//
// The walk starts at resource's parent and steps upward, removing each directory whose remove call succeeds, until
// it either reaches boundary or hits a directory whose remove fails (typically because the directory is non-empty
// or the caller lacks permission). The first failure halts the walk silently — pruning is hygiene, not a recovery
// step, so the partial result is acceptable. A nil boundary clamps the walk at [Provider.Root]; passing prune=false
// bypasses the walk entirely.
//
// Parameters:
//   - resource: The [Resource] whose parents are candidates for pruning; its path is the walk's starting point.
//   - prune: If true, walk upward and remove empty parents; if false, return without touching the filesystem.
//   - boundary: The [Resource] at which the walk halts (exclusive); nil clamps the walk at [Provider.Root].
func (p *Provider) pruneEmptyParents(resource *Resource, prune bool, boundary *Resource) {

	if !prune {
		return
	}

	boundaryPath := p.Root()

	if boundary != nil {
		boundaryPath = boundary.SourcePath.Abs()
	}

	dir := filepath.Dir(resource.SourcePath.Abs())

	for dir != boundaryPath && dir != "." && dir != "/" {
		if err := p.remove(dir); err != nil {
			return // not empty or permission error
		}
		dir = filepath.Dir(dir)
	}
}

// archiveAndPrune archives resource to the recovery site and prunes empty parent directories toward boundary if asked.
//
// This helper consolidates the prologue that [Provider.Remove], [Provider.RemoveAll], and [Provider.Unlink] each need
// before returning their compensation receipt: archive the about-to-be-removed entry so [Provider.compensateWrite] can
// restore it, then optionally walk up the parent chain removing now-empty directories. The archive step is mandatory
// (its failure aborts the operation); the prune step is best-effort and any failure inside it is swallowed by
// [Provider.pruneEmptyParents].
//
// Parameters:
//   - resource: The [Resource] being removed; archived under its [op.RecoverySite] key before the prune walk runs.
//   - prune: If true, remove empty parent directories after the archive succeeds.
//   - boundary: Optional ceiling for the prune walk; nil clamps the walk at [Provider.Root].
//
// Returns:
//   - err: any error from [op.RecoverySite.ArchiveFile]; prune itself is best-effort and never propagates errors.
func (p *Provider) archiveAndPrune(resource *Resource, prune bool, boundary *Resource) error {

	if _, err := p.ExecutionContext().RecoverySite.ArchiveFile(resource.SourcePath); err != nil {
		return err
	}

	p.pruneEmptyParents(resource, prune, boundary)
	return nil
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
