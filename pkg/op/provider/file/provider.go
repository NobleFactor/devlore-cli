// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package file provides file system actions for the operation graph.
package file

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/gitignore"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/op"
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
// +devlore:access=both
type Provider struct {
	op.ProviderBase
}

// NewProvider creates a file provider bound to the given context.
func NewProvider(runtimeEnvironment *op.RuntimeEnvironment) *Provider {

	return &Provider{ProviderBase: op.NewProviderBase(runtimeEnvironment)}
}

// region EXPORTED METHODS

// region State management

// Root returns the root path of the file-system scope, or the empty string when no root is set.
//
// Returns:
//   - `string`: the scoped root path, or "" when [RuntimeEnvironment.Root] is nil.
func (p *Provider) Root() string {

	if p.RuntimeEnvironment().Root == nil {
		return ""
	}
	return p.RuntimeEnvironment().Root.Name()
}

// endregion

// region Behaviors

// Compensable actions

// Backup moves `source` to a timestamped backup location, delegating to [Provider.Move].
//
// Parameters:
//   - `activationRecord`: the dispatch activation threaded to [Provider.Move].
//   - `source`: the [*Resource] to back up.
//   - `backupSuffix`: the suffix inserted before the timestamp; empty defaults to ".devlore-backup".
//
// Returns:
//   - `*Resource`: the backup destination resource.
//   - `*Receipt`: the compensation receipt for undo.
//   - `error`: non-nil on move failure.
func (p *Provider) Backup(
	activationRecord *op.ActivationRecord,
	source *Resource,
	backupSuffix string,
) (*Resource, *Receipt, error) {

	if backupSuffix == "" {
		backupSuffix = p.RuntimeEnvironment().BackupSuffix
	}

	timestamp := time.Now().Format("20060102-150405")
	backupPath := source.SourcePath.Abs() + backupSuffix + "." + timestamp

	return p.Move(activationRecord, source, backupPath)
}

// CompensateBackup undoes a [Provider.Backup] by delegating to [Provider.CompensateMove].
//
// Parameters:
//   - `receipt`: the [*Receipt] returned by [Provider.Backup].
//
// Returns:
//   - `error`: non-nil on restore failure.
func (p *Provider) CompensateBackup(receipt *Receipt) error {
	return p.CompensateMove(receipt)
}

// Copy copies `source`'s contents to a new file at `destinationPath` with the given mode and ownership.
//
// `chown` is the Dockerfile-style ownership string (`"user[:group]"`, `":group"`, `"uid[:gid]"`, or empty for no
// change). When non-empty it is parsed and applied via os.Chown after the file is created.
//
// Parameters:
//   - `activationRecord`: the dispatch activation; its `Unit` stamps the produced [*Resource]'s producerID.
//   - `source`: the [*Resource] whose contents are copied.
//   - `destinationPath`: the destination path for the new file.
//   - `chmod`: the [os.FileMode] applied to the created file.
//   - `chown`: the Dockerfile-style ownership string, or empty for no ownership change.
//
// Returns:
//   - `*Resource`: the created destination resource, resolved against the filesystem.
//   - `*Receipt`: the compensation receipt for undo.
//   - `error`: non-nil on resource construction, write preparation, copy, chown, or resolve failure.
//
// +devlore:defaults chmod={{ umask 0o755 }}, chown=""
func (p *Provider) Copy(
	activationRecord *op.ActivationRecord,
	source *Resource,
	destinationPath string,
	chmod os.FileMode,
	chown string,
) (product *Resource, receipt *Receipt, err error) {

	product, err = NewResource(p.RuntimeEnvironment(), activationRecord.Unit, destinationPath)
	if err != nil {
		return nil, nil, err
	}

	product, receipt, err = p.prepareWrite(product)
	if err != nil {
		return nil, nil, err
	}

	if receipt != nil && receipt.RecoveryID() != "" {
		_ = receipt.SetRecoveryID(receipt.RecoveryID())
	}

	src, err := p.open(source.SourcePath.Abs())
	if err != nil {
		return product, receipt, err
	}
	defer iox.Close(&err, src)

	dst, err := p.openFile(product.SourcePath.Abs(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, chmod)
	if err != nil {
		return product, receipt, err
	}
	defer iox.Close(&err, dst)

	if _, err := io.Copy(dst, src); err != nil {
		return product, receipt, err
	}

	if err := applyChown(product.SourcePath.Abs(), chown); err != nil {
		return product, receipt, err
	}

	if err := product.Resolve(); err != nil {
		return product, receipt, err
	}

	return product, receipt, nil
}

// CompensateCopy undoes a [Provider.Copy] by restoring the original file from recovery via [Provider.compensateWrite].
//
// Parameters:
//   - `receipt`: the [*Receipt] returned by [Provider.Copy].
//
// Returns:
//   - `error`: non-nil on restore failure.
func (p *Provider) CompensateCopy(receipt *Receipt) error {
	return p.compensateWrite(receipt)
}

// Link creates a symbolic link at `targetPath` pointing to `source`, archiving any existing entry first.
//
// When an entry already exists at `targetPath`: if it is a symlink already pointing at `source`, Link is a no-op;
// otherwise the existing entry is archived to the [op.RecoverySite] before the new link is created. When nothing
// exists, the parent directory chain is created and its boundary recorded on the receipt for compensation.
//
// Parameters:
//   - `activationRecord`: the dispatch activation; its `Unit` stamps the produced [*Resource]'s producerID.
//   - `source`: the [*Resource] the link points to.
//   - `targetPath`: the path at which the symlink is created.
//
// Returns:
//   - `*Resource`: the link resource (resolved when created; the matched resource when already correct).
//   - `*Receipt`: the compensation receipt for undo, or nil when no change was made.
//   - `error`: non-nil on resource construction, archive, parent creation, symlink, or resolve failure.
func (p *Provider) Link(
	activationRecord *op.ActivationRecord,
	source *Resource,
	targetPath string,
) (product *Resource, receipt *Receipt, err error) {

	product, err = NewResource(p.RuntimeEnvironment(), activationRecord.Unit, targetPath)
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

		preDigest := preArchiveDigest(p.RuntimeEnvironment().Root, product.SourcePath.Abs())

		recoveryID, archiveErr := p.RuntimeEnvironment().RecoverySite.ArchiveFile(product.SourcePath)
		if archiveErr != nil {
			return nil, nil, archiveErr
		}

		receipt = NewReceipt(product)
		_ = receipt.SetRecoveryID(recoveryID)
		receipt.SetRecoveryDigest(preDigest)

	} else {

		// Does not exist — standard parent directory creation.
		parentPath := filepath.Dir(product.SourcePath.Abs())

		boundary, _, err := p.findClosestExistingDir(parentPath)
		if err != nil {
			return nil, nil, err
		}

		receipt = NewReceiptWithBoundary(product, boundary)

		if err = p.mkdirAll(parentPath, 0o750); err != nil {
			return nil, receipt, err
		}
	}

	if err = p.symlink(source.SourcePath.Abs(), product.SourcePath.Abs()); err != nil {
		return nil, receipt, err
	}

	if err = product.Resolve(); err != nil {
		return nil, receipt, err
	}

	return product, receipt, nil
}

// CompensateLink undoes a [Provider.Link] by removing the symlink and restoring whatever was archived before it.
//
// Parameters:
//   - `receipt`: the [*Receipt] returned by [Provider.Link].
//
// Returns:
//   - `error`: non-nil on removal or restore failure.
func (p *Provider) CompensateLink(receipt *Receipt) error {
	return p.compensateWrite(receipt)
}

// Mkdir creates a directory (and any missing parents) at `path` with the given mode and ownership.
//
// `chown` is the Dockerfile-style ownership string (`"user[:group]"`, `":group"`, `"uid[:gid]"`, or empty for
// "no change"). When non-empty it is applied via os.Chown to the leaf directory only — intermediate parents
// created by the call are NOT chown'd, since their role is "existed before this call" rather than "created here."
//
// Parameters:
//   - `activationRecord`: the dispatch activation; its `Unit` stamps the produced [*Resource]'s producerID.
//   - `path`: the directory path to create.
//   - `chmod`: the [os.FileMode] applied to the leaf directory.
//   - `chown`: the Dockerfile-style ownership string applied to the leaf directory, or empty for no change.
//
// Returns:
//   - `*Resource`: the created directory resource, resolved; a nil receipt accompanies an already-existing directory.
//   - `*Receipt`: the compensation receipt recording the creation boundary for undo.
//   - `error`: non-nil when `path` exists as a non-directory, or on construction, mkdir, chown, or resolve failure.
//
// +devlore:defaults chmod={{ umask 0o777 }}, chown=""
func (p *Provider) Mkdir(
	activationRecord *op.ActivationRecord,
	path string,
	chmod os.FileMode,
	chown string,
) (product *Resource, receipt *Receipt, err error) {

	product, err = NewResource(p.RuntimeEnvironment(), activationRecord.Unit, path)
	if err != nil {
		return nil, nil, err
	}

	leaf := product.SourcePath.Abs()

	boundary, info, err := p.findClosestExistingDir(leaf)
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

	if err = p.mkdirAll(leaf, chmod); err != nil {
		return nil, receipt, err
	}

	if err = applyChown(leaf, chown); err != nil {
		return nil, receipt, err
	}

	if err = product.Resolve(); err != nil {
		return nil, receipt, err
	}

	return product, receipt, nil
}

// CompensateMkdir undoes a [Provider.Mkdir] by removing the directory subtree it created.
//
// Walks up from the receipt's resource, removing each entry until it reaches the boundary recorded on the receipt
// (exclusive). A non-empty directory encountered along the way (a sibling adopted it) stops the unwind without error.
//
// Parameters:
//   - `receipt`: the [*Receipt] returned by [Provider.Mkdir]; a nil receipt or nil boundary is a no-op.
//
// Returns:
//   - `error`: non-nil when the receipt's resource is the wrong type, lies outside its boundary, or removal fails.
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
			break
		}

		current = parent
	}

	return nil
}

// Move moves the file at `source` to `destinationPath`, archiving any existing destination first.
//
// The destination's parents are created when absent. When an entry already exists at `destinationPath` it is archived
// for compensation; a failed rename attempts to restore that archived destination before returning the error.
//
// Parameters:
//   - `activationRecord`: the dispatch activation; its `Unit` stamps the produced [*Resource]'s producerID.
//   - `source`: the [*Resource] to move.
//   - `destinationPath`: the path to move `source` to.
//
// Returns:
//   - `*Resource`: the destination resource, resolved.
//   - `*Receipt`: the compensation receipt recording the source and any archived destination for undo.
//   - `error`: non-nil on construction, write preparation, rename, or resolve failure.
func (p *Provider) Move(
	activationRecord *op.ActivationRecord,
	source *Resource,
	destinationPath string,
) (product *Resource, receipt *Receipt, err error) {

	product, err = NewResource(p.RuntimeEnvironment(), activationRecord.Unit, destinationPath)
	if err != nil {
		return nil, nil, err
	}

	// Prepare destination (handle overwrite and parent creation).
	product, receipt, err = p.prepareWrite(product)
	if err != nil {
		return nil, nil, err
	}

	// Set source in receipt so we know where to move it back.
	if receipt == nil {
		receipt = NewReceipt(product)
	}

	receipt.SetSource(source)

	if err = p.rename(source.SourcePath.Abs(), product.SourcePath.Abs()); err != nil {
		// Attempt to restore destination on failure if we archived it.
		if receipt.RecoveryID() != "" {
			_ = p.RuntimeEnvironment().RecoverySite.RestoreFile(product.SourcePath, receipt.RecoveryID())
		}
		return nil, nil, err
	}

	if err = product.Resolve(); err != nil {
		return nil, receipt, err
	}

	return product, receipt, nil
}

// CompensateMove undoes a [Provider.Move] by moving the file from destination back to source.
//
// After moving back, any destination archived by the forward Move is restored — but only after verifying the recovery
// archive's bytes still match the digest captured at archive time, so tampering is detected before restoration.
//
// Parameters:
//   - `receipt`: the [*Receipt] returned by [Provider.Move]; a nil receipt or nil resource is a no-op.
//
// Returns:
//   - `error`: non-nil on wrong resource type, missing source, move-back failure, digest mismatch, or restore failure.
func (p *Provider) CompensateMove(receipt *Receipt) error {

	if receipt == nil || receipt.Resource() == nil {
		return nil
	}

	product, ok := receipt.Resource().(*Resource)
	if !ok {
		return fmt.Errorf("compensate move: unexpected resource type %T", receipt.Resource())
	}

	source := receipt.Source()
	if source == nil {
		return fmt.Errorf("compensate move: receipt missing source resource")
	}

	// Move back from destination to source.
	if err := p.rename(product.SourcePath.Abs(), source.SourcePath.Abs()); err != nil {
		return fmt.Errorf("compensate move: move back failed: %w", err)
	}

	// Restore old destination if it was archived.
	recoveryID := receipt.RecoveryID()
	if recoveryID != "" {

		// Verify the recovery archive has not been tampered with by comparing its current bytes' digest
		// against the digest captured at archive time (stored on the receipt).
		expected := receipt.RecoveryDigest()
		if expected.Algorithm != "" {

			recoveryPath := ".devlore/recovery/" + recoveryID
			actualStr := checksumFile(p.RuntimeEnvironment().Root, recoveryPath)

			if actualStr == "" {
				return fmt.Errorf("cannot read %s for verification", recoveryID)
			}

			actual, err := op.ParseDigest(actualStr)
			if err != nil {
				return fmt.Errorf("compensate move: parse recovery checksum: %w", err)
			}

			if !actual.Equal(expected) {
				return fmt.Errorf("%s has been modified (digest mismatch)", recoveryID)
			}
		}

		if err := p.RuntimeEnvironment().RecoverySite.RestoreFile(product.SourcePath, recoveryID); err != nil {
			return fmt.Errorf("compensate move: restore old destination failed: %w", err)
		}
	}

	return nil
}

// Remove deletes the file or empty directory at `resource`, archiving it for compensation.
//
// A non-existent target is a no-op (nil product, nil receipt, nil error). A non-empty directory is an error — use
// [Provider.RemoveAll] for recursive deletion. When `prune` is set, now-empty parents up to `boundary` are removed.
//
// Parameters:
//   - `resource`: the [*Resource] to delete.
//   - `prune`: whether to remove now-empty parent directories up to `boundary`.
//   - `boundary`: the [*Resource] at which parent pruning stops; nil prunes to the scoped root.
//
// Returns:
//   - `*Resource`: always nil — Remove produces no resource.
//   - `*Receipt`: the compensation receipt recording the recovery archive for undo.
//   - `error`: non-nil when the target is a non-empty directory, or on stat or archive failure.
func (p *Provider) Remove(
	resource *Resource,
	prune bool,
	boundary *Resource,
) (product *Resource, receipt *Receipt, err error) {

	nonEmptyDirectory, err := p.isDirAndNotEmpty(resource.SourcePath.Abs())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	if nonEmptyDirectory {
		return nil, nil, fmt.Errorf("directory %s is not empty", resource.SourcePath.Abs())
	}

	recoveryID, digest, err := p.archiveAndPrune(resource, prune, boundary)
	if err != nil {
		return nil, nil, err
	}

	receipt = NewReceipt(resource)
	_ = receipt.SetRecoveryID(recoveryID)
	receipt.SetRecoveryDigest(digest)

	return nil, receipt, nil
}

// CompensateRemove undoes a [Provider.Remove] by restoring the file from recovery via [Provider.CompensateUnlink].
//
// Parameters:
//   - `receipt`: the [*Receipt] returned by [Provider.Remove].
//
// Returns:
//   - `error`: non-nil on removal or restore failure.
func (p *Provider) CompensateRemove(receipt *Receipt) error {
	return p.CompensateUnlink(receipt)
}

// RemoveAll removes `resource` and any children it contains, archiving the subtree for compensation.
//
// Unlike [Provider.Remove], a non-empty directory is removed recursively. When `prune` is set, now-empty parents up
// to `boundary` are removed afterward.
//
// Parameters:
//   - `resource`: the [*Resource] to remove recursively.
//   - `prune`: whether to remove now-empty parent directories up to `boundary`.
//   - `boundary`: the [*Resource] at which parent pruning stops; nil prunes to the scoped root.
//
// Returns:
//   - `*Resource`: always nil — RemoveAll produces no resource.
//   - `*Receipt`: the compensation receipt recording the recovery archive for undo.
//   - `error`: non-nil on archive failure.
func (p *Provider) RemoveAll(
	resource *Resource,
	prune bool,
	boundary *Resource,
) (product *Resource, receipt *Receipt, err error) {

	recoveryID, digest, err := p.archiveAndPrune(resource, prune, boundary)
	if err != nil {
		return nil, nil, err
	}

	receipt = NewReceipt(resource)
	_ = receipt.SetRecoveryID(recoveryID)
	receipt.SetRecoveryDigest(digest)

	return nil, receipt, nil
}

// CompensateRemoveAll undoes a [Provider.RemoveAll], restoring the subtree from recovery.
//
// Parameters:
//   - `receipt`: the [*Receipt] returned by [Provider.RemoveAll].
//
// Returns:
//   - `error`: non-nil on removal or restore failure.
func (p *Provider) CompensateRemoveAll(receipt *Receipt) error {
	return p.CompensateUnlink(receipt)
}

// Unlink removes the symlink at `resource`, archiving it for compensation.
//
// A non-existent target is a no-op. A target that exists but is not a symlink is an error. When `prune` is set,
// now-empty parents up to `boundary` are removed afterward.
//
// Parameters:
//   - `resource`: the [*Resource] symlink to remove.
//   - `prune`: whether to remove now-empty parent directories up to `boundary`.
//   - `boundary`: the [*Resource] at which parent pruning stops; nil prunes to the scoped root.
//
// Returns:
//   - `*Resource`: always nil — Unlink produces no resource.
//   - `*Receipt`: the compensation receipt recording the recovery archive for undo.
//   - `error`: non-nil when the target exists but is not a symlink, or on stat or archive failure.
func (p *Provider) Unlink(
	resource *Resource,
	prune bool,
	boundary *Resource,
) (product *Resource, receipt *Receipt, err error) {

	info, err := p.lstat(resource.SourcePath.Abs())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil // Already gone — no change
	}

	if err != nil {
		return nil, nil, err
	}

	if info.Mode()&os.ModeSymlink == 0 {
		return nil, nil, fmt.Errorf("%s is not a symlink", resource.SourcePath.Abs())
	}

	recoveryID, digest, err := p.archiveAndPrune(resource, prune, boundary)
	if err != nil {
		return nil, nil, err
	}

	receipt = NewReceipt(resource)
	_ = receipt.SetRecoveryID(recoveryID)
	receipt.SetRecoveryDigest(digest)

	return nil, receipt, nil
}

// CompensateUnlink undoes an [Provider.Unlink] by restoring the symlink from recovery.
//
// The current entry at the resource path is always removed first — [op.RecoverySite.RestoreFile] uses os.Rename,
// which fails if the target exists. An empty recovery ID means nothing was archived, so removal alone suffices.
//
// Parameters:
//   - `receipt`: the [*Receipt] returned by [Provider.Unlink]; a nil receipt or nil resource is a no-op.
//
// Returns:
//   - `error`: non-nil on wrong resource type, removal failure, or restore failure.
func (p *Provider) CompensateUnlink(receipt *Receipt) error {

	if receipt == nil || receipt.Resource() == nil {
		return nil
	}

	resource, ok := receipt.Resource().(*Resource)
	if !ok {
		return fmt.Errorf("compensate unlink: unexpected resource type %T", receipt.Resource())
	}

	// ALWAYS remove the new file before attempting to restore. RestoreFile uses os.Rename which fails if target exists.
	if err := p.remove(resource.SourcePath.Abs()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	recoveryID := receipt.RecoveryID()
	if recoveryID == "" {
		return nil
	}

	return p.RuntimeEnvironment().RecoverySite.RestoreFile(resource.SourcePath, recoveryID)
}

// WalkTree performs a depth-first traversal of `root`, folding each entry through `fn`.
//
// WalkTree is a discovery operation — the walker observes existing filesystem entries; it does not produce them. The
// Resources it interns into the catalog are discovered, not authored, so they carry no `producerID` stamp from this
// method. Gitignored entries are skipped unless `includeGitignored` is set; the `.git` directory is always skipped.
//
// Parameters:
//   - `root`: the [*Resource] directory to traverse.
//   - `fn`: the [Reducer] invoked for each entry, threading an accumulator and the recovery stack.
//   - `includeGitignored`: when false, entries matched by gitignore rules are skipped.
//
// Returns:
//   - `any`: the final accumulator value returned by the last `fn` invocation.
//   - `*op.RecoveryStack`: the recovery stack accumulated during the walk, for compensation.
//   - `error`: non-nil on tracker construction, stat, or any error returned by `fn`.
//
// +devlore:defaults includeGitignored=false
func (p *Provider) WalkTree(
	root *Resource,
	fn Reducer,
	includeGitignored bool,
) (product any, stack *op.RecoveryStack, err error) {

	stack = op.NewRecoveryStack()

	tracker, err := p.newTrackerIfEnabled(root.SourcePath.Abs(), !includeGitignored)
	if err != nil {
		return nil, nil, err
	}

	absoluteRoot, err := filepath.Abs(root.SourcePath.Abs())
	if err != nil {
		return nil, nil, err
	}

	if _, err = p.stat(absoluteRoot); err != nil {
		return nil, nil, err
	}

	osRoot := p.RuntimeEnvironment().Root

	walkFn := func(entryAbs string, d fs.DirEntry, walkDirErr error) error {

		if walkDirErr != nil {
			return walkDirErr
		}

		relativePath, err := filepath.Rel(absoluteRoot, entryAbs)
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

		runtimeEnvironment := p.RuntimeEnvironment()
		// WalkTree is discovery — found files pre-existed; no production claim. DiscoverResource handles
		// construction + Catalog.Discover internally.
		resource, err := DiscoverResource(runtimeEnvironment, entryAbs)
		if err != nil {
			return err
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

// CompensateWalkTree unwinds the [op.RecoveryStack] returned by [Provider.WalkTree] in LIFO order.
//
// Parameters:
//   - `stack`: the [*op.RecoveryStack] returned by [Provider.WalkTree]; a nil stack is a no-op.
//
// Returns:
//   - `error`: non-nil when unwinding any recorded compensation fails.
func (p *Provider) CompensateWalkTree(stack *op.RecoveryStack) error {
	if stack == nil {
		return nil
	}
	return stack.Unwind()
}

// WriteBytes writes inline byte `content` to a file at `destinationPath` with the given mode and ownership.
//
// `chown` is the Dockerfile-style ownership string (`"user[:group]"`, `":group"`, `"uid[:gid]"`, or empty for "no
// change"). When non-empty it is applied via os.Chown after the file is written. Any existing file is archived for
// compensation before the write.
//
// Parameters:
//   - `activationRecord`: the dispatch activation; its `Unit` stamps the produced [*Resource]'s producerID.
//   - `destinationPath`: the path of the file to write.
//   - `content`: the bytes to write, carried as a string.
//   - `chmod`: the [os.FileMode] applied to the written file.
//   - `chown`: the Dockerfile-style ownership string, or empty for no ownership change.
//
// Returns:
//   - `*Resource`: the written resource.
//   - `*Receipt`: the compensation receipt for undo.
//   - `error`: non-nil on construction or write failure.
//
// +devlore:defaults chmod={{ umask 0o666 }}, chown=""
func (p *Provider) WriteBytes(
	activationRecord *op.ActivationRecord,
	destinationPath string,
	content string,
	chmod os.FileMode,
	chown string,
) (product *Resource, receipt *Receipt, err error) {

	product, err = NewResource(p.RuntimeEnvironment(), activationRecord.Unit, destinationPath)
	if err != nil {
		return nil, nil, err
	}

	product, receipt, err = p.write(product, []byte(content), chmod, chown)
	if err != nil {
		return product, receipt, err
	}

	return product, receipt, nil
}

// CompensateWriteBytes undoes a [Provider.WriteBytes] by restoring the original file via [Provider.compensateWrite].
//
// Parameters:
//   - `receipt`: the [*Receipt] returned by [Provider.WriteBytes].
//
// Returns:
//   - `error`: non-nil on removal or restore failure.
func (p *Provider) CompensateWriteBytes(receipt *Receipt) error {
	return p.compensateWrite(receipt)
}

// WriteText writes inline text `content` to a file at `destinationPath` with the given mode and ownership.
//
// `chown` is the Dockerfile-style ownership string (`"user[:group]"`, `":group"`, `"uid[:gid]"`, or empty for "no
// change"). When non-empty it is applied via os.Chown after the file is written. Any existing file is archived for
// compensation before the write.
//
// Parameters:
//   - `activationRecord`: the dispatch activation; its `Unit` stamps the produced [*Resource]'s producerID.
//   - `destinationPath`: the path of the file to write.
//   - `content`: the text to write.
//   - `chmod`: the [os.FileMode] applied to the written file.
//   - `chown`: the Dockerfile-style ownership string, or empty for no ownership change.
//
// Returns:
//   - `*Resource`: the written resource.
//   - `*Receipt`: the compensation receipt for undo.
//   - `error`: non-nil on construction or write failure.
//
// +devlore:defaults chmod={{ umask 0o666 }}, chown=""
func (p *Provider) WriteText(
	activationRecord *op.ActivationRecord,
	destinationPath string,
	content string,
	chmod os.FileMode,
	chown string,
) (product *Resource, receipt *Receipt, err error) {

	product, err = NewResource(p.RuntimeEnvironment(), activationRecord.Unit, destinationPath)
	if err != nil {
		return nil, nil, err
	}

	product, receipt, err = p.write(product, []byte(content), chmod, chown)
	if err != nil {
		return product, receipt, err
	}

	return product, receipt, nil
}

// CompensateWriteText undoes a [Provider.WriteText] by restoring the original file via [Provider.compensateWrite].
//
// Parameters:
//   - `receipt`: the [*Receipt] returned by [Provider.WriteText].
//
// Returns:
//   - `error`: non-nil on removal or restore failure.
func (p *Provider) CompensateWriteText(receipt *Receipt) error {
	return p.compensateWrite(receipt)
}

// Fallible actions

// Exists reports whether an entry exists at `resource`'s path, examining the link itself (lstat semantics).
//
// A not-exist result is reported as `(false, nil)`, not an error; only a genuine stat failure returns a non-nil error.
//
// Parameters:
//   - `resource`: the [*Resource] whose path is probed.
//
// Returns:
//   - `bool`: true when an entry exists at the path.
//   - `error`: non-nil on any stat failure other than not-exist.
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

// Find returns the file resources matching `pattern`, with recursive `**` support, beneath the scoped root.
//
// The pattern is split into a base directory and a match expression; the base is resolved against the scoped root and
// must not escape it. Matching walks the tree, skipping gitignored entries unless `includeGitignored` is set.
//
// Parameters:
//   - `pattern`: the glob pattern, which may contain `**` for recursive matching.
//   - `includeGitignored`: when false, entries matched by gitignore rules are skipped.
//
// Returns:
//   - `[]*Resource`: the matching file resources, in walk order.
//   - `error`: non-nil when the pattern escapes the scoped root, or on tracker construction or walk failure.
//
// +devlore:defaults includeGitignored=false
func (p *Provider) Find(pattern string, includeGitignored bool) (product []*Resource, err error) {

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

	tracker, err := p.newTrackerIfEnabled(absoluteRoot, !includeGitignored)
	if err != nil {
		return nil, fmt.Errorf("find: gitignore tracker: %w", err)
	}

	matches := make([]string, 0, 8192)

	walk := func(absolutePath string, dirEntry fs.DirEntry, err error) error {

		if err != nil {
			return err
		}

		relativePath, err := filepath.Rel(absoluteRoot, absolutePath)
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
	}

	err = p.walkDir(p.RuntimeEnvironment().Root, absoluteRoot, walk)
	if err != nil {
		return nil, fmt.Errorf("find: walk %q: %w", absoluteRoot, err)
	}

	return p.discoverResources(matches)
}

// Glob returns the [Resource] entries for filesystem paths matching `pattern` via [filepath.Glob].
//
// Unlike [Provider.Find], matching is non-recursive (no `**`). Gitignored matches are dropped unless
// `includeGitignored` is set; a gitignore tracker that fails to construct degrades to returning all matches.
//
// Parameters:
//   - `pattern`: the [filepath.Glob] pattern to match.
//   - `includeGitignored`: when false, matches filtered by gitignore rules are dropped.
//
// Returns:
//   - `[]*Resource`: the matching resources.
//   - `error`: non-nil on a malformed pattern.
//
// +devlore:defaults includeGitignored=false
func (p *Provider) Glob(pattern string, includeGitignored bool) ([]*Resource, error) {

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	if includeGitignored {
		return p.discoverResources(matches)
	}

	tracker, err := gitignore.NewTracker(p.Root())
	if err != nil {
		return p.discoverResources(matches) //nolint:nilerr // graceful degradation
	}

	kept := matches[:0]
	for _, match := range matches {
		if !p.isIgnored(tracker, match) {
			kept = append(kept, match)
		}
	}

	return p.discoverResources(kept)
}

// IsDir reports whether `resource` exists and is a directory, following symlinks (stat semantics).
//
// A not-exist result is reported as `(false, nil)`, not an error.
//
// Parameters:
//   - `resource`: the [*Resource] whose path is probed.
//
// Returns:
//   - `bool`: true when the path exists and is a directory.
//   - `error`: non-nil on any stat failure other than not-exist.
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

// IsFile reports whether `resource` exists and is a regular file, following symlinks (stat semantics).
//
// A not-exist result is reported as `(false, nil)`, not an error.
//
// Parameters:
//   - `resource`: the [*Resource] whose path is probed.
//
// Returns:
//   - `bool`: true when the path exists and is a regular file.
//   - `error`: non-nil on any stat failure other than not-exist.
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

// Observe captures the runtime-observed state of `resource` as an [*Observation].
//
// Stats the file at `resource.SourcePath`. When the file exists, the Observation carries the stat-derived metadata
// (`Size`, `Mode`, `ModTime`, `Inode`, `Device`) with `Exists` set to true. When the file does not exist
// (`os.ErrNotExist`), the Observation carries zero metadata with `Exists` set to false — not-exist is a valid
// observation outcome, not an error. Any other stat failure returns nil and the underlying error.
//
// Parameters:
//   - `resource`: the [*Resource] whose current filesystem state to observe.
//
// Returns:
//   - `*Observation`: the constructed observation; never nil on a nil-error return.
//   - `error`: any stat failure other than not-exist, or any [NewObservation] construction failure.
func (p *Provider) Observe(resource *Resource) (*Observation, error) {

	root := p.RuntimeEnvironment().Root
	absPath := root.NewPath(resource.SourcePath.Abs())

	info, err := root.Stat(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return NewObservation(p.RuntimeEnvironment(), resource, false, 0, 0, time.Time{}, 0, 0)
		}
		return nil, fmt.Errorf("file.Provider.Observe: stat %s: %w", resource.SourcePath.Abs(), err)
	}

	var inode, device uint64
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		inode = stat.Ino
		device = uint64(stat.Dev) //nolint:gosec // G115: Dev is platform-specific; overflow is not a practical concern.
	}

	return NewObservation(
		p.RuntimeEnvironment(),
		resource,
		true,
		info.Size(),
		info.Mode(),
		info.ModTime(),
		inode,
		device,
	)
}

// ReadBytes returns the contents of the file `resource` as bytes.
//
// Parameters:
//   - `resource`: the file [*Resource] to read.
//
// Returns:
//   - `[]byte`: the file contents.
//   - `error`: non-nil on read failure.
func (p *Provider) ReadBytes(resource *Resource) (product []byte, err error) {

	buffer, err := p.read(resource)
	if err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

// ReadText returns the contents of the file `resource` as text.
//
// Parameters:
//   - `resource`: the file [*Resource] to read.
//
// Returns:
//   - `string`: the file contents.
//   - `error`: non-nil on read failure.
func (p *Provider) ReadText(resource *Resource) (product string, err error) {

	buffer, err := p.read(resource)
	if err != nil {
		return "", err
	}

	return buffer.String(), nil
}

// Actions

// Join joins path components using the OS path separator via [filepath.Join].
//
// Parameters:
//   - `parts`: the path components to join.
//
// Returns:
//   - `string`: the joined path.
func (p *Provider) Join(parts ...string) string {
	return filepath.Join(parts...)
}

// Name returns the last element of `path` (a file or directory name) via [filepath.Base].
//
// Parameters:
//   - `path`: the path whose last element is returned.
//
// Returns:
//   - `string`: the last path element.
func (p *Provider) Name(path string) string {
	return filepath.Base(path)
}

// Parent returns the directory containing the file at `path` via [filepath.Dir].
//
// Parameters:
//   - `path`: the path whose containing directory is returned.
//
// Returns:
//   - `string`: the parent directory path.
func (p *Provider) Parent(path string) string {
	return filepath.Dir(path)
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// Fallible actions

// applyGitignore decides whether a walked directory entry should be skipped under gitignore rules.
//
// The `.git` directory is always skipped. With a nil tracker, nothing is skipped. Directory entries are pushed onto
// the tracker so nested rules apply; an ignored directory yields [SkipDir] and an ignored file yields [errSkipEntry].
//
// Parameters:
//   - `tracker`: the [*gitignore.Tracker] holding active ignore rules, or nil to disable filtering.
//   - `d`: the [fs.DirEntry] being visited.
//   - `relativePath`: the entry's path relative to the walk root.
//
// Returns:
//   - `error`: [SkipDir] to skip a directory, [errSkipEntry] to skip a file, a tracker push error, or nil to keep it.
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

// archiveAndPrune moves resource to the recovery site, capturing the archived bytes' digest beforehand.
//
// The returned digest is what compensation will compare against to detect tampering of the recovery archive between
// the forward action and compensation. An empty digest is returned (and not an error) when the file could not be
// hashed — typically because it was a symlink or otherwise unreadable; the archive proceeds regardless.
//
// Parameters:
//   - `resource`: the [*Resource] to archive and remove.
//   - `prune`: whether to remove now-empty parent directories up to `boundary`.
//   - `boundary`: the [*Resource] at which parent pruning stops; nil prunes to the scoped root.
//
// Returns:
//   - `string`: the recovery-site identifier for the archived bytes.
//   - `op.Digest`: the pre-archive digest, or the zero value when the bytes could not be hashed.
//   - `error`: non-nil on archive failure.
func (p *Provider) archiveAndPrune(
	resource *Resource,
	prune bool,
	boundary *Resource,
) (recoveryID string, digest op.Digest, err error) {

	digest = preArchiveDigest(p.RuntimeEnvironment().Root, resource.SourcePath.Abs())

	recoveryID, err = p.RuntimeEnvironment().RecoverySite.ArchiveFile(resource.SourcePath)
	if err != nil {
		return "", op.Digest{}, err
	}

	p.pruneEmptyParents(resource, prune, boundary)
	return recoveryID, digest, nil
}

// compensateWrite reverses a forward write by removing the written file and restoring any archived predecessor.
//
// The written file is always removed first — [op.RecoverySite.RestoreFile] uses os.Rename, which fails if the target
// exists. A recorded recovery ID is then restored (a missing recovery source is tolerated). Finally, when the receipt
// carries a boundary, now-empty parent directories created by the forward write are pruned up to that boundary.
//
// Parameters:
//   - `receipt`: the [*Receipt] captured by the forward write; a nil receipt or nil resource is a no-op.
//
// Returns:
//   - `error`: non-nil on wrong resource type, removal failure, restore failure, or a boundary outside the resource.
func (p *Provider) compensateWrite(receipt *Receipt) error {

	if receipt == nil || receipt.Resource() == nil {
		return nil
	}

	resource, ok := receipt.Resource().(*Resource)
	if !ok {
		return fmt.Errorf("compensate write: unexpected resource type %T", receipt.Resource())
	}

	// ALWAYS remove the new file before attempting to restore. RestoreFile uses os.Rename which fails if target exists.
	if err := p.remove(resource.SourcePath.Abs()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	recoveryID := receipt.RecoveryID()
	if recoveryID != "" {
		if err := p.RuntimeEnvironment().RecoverySite.RestoreFile(resource.SourcePath, recoveryID); err != nil {
			if !errors.Is(err, op.ErrRecoverySourceNotFound) {
				return err
			}
		}
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

// discoverResources constructs a discovered [Resource] for each input path without claiming production.
//
// Parameters:
//   - `paths`: the absolute paths to build catalog handles for.
//
// Returns:
//   - `[]*Resource`: the discovered resources, one per input path, in order.
//   - `error`: non-nil on any resource discovery failure.
func (p *Provider) discoverResources(paths []string) (product []*Resource, err error) {

	runtimeEnvironment := p.RuntimeEnvironment()
	resources := make([]*Resource, len(paths))

	for i, path := range paths {
		// discoverResources is discovery — build catalog handles for caller-supplied paths without claiming
		// production. DiscoverResource handles construction + Catalog.Discover internally.
		concrete, derr := DiscoverResource(runtimeEnvironment, path)
		if derr != nil {
			return nil, derr
		}
		resources[i] = concrete
	}

	return resources, nil
}

// findClosestExistingDir walks up from `path` to the nearest existing entry under the scoped [Provider.Root].
//
// Parameters:
//   - `path`: the absolute path whose nearest existing ancestor is sought.
//
// Returns:
//   - `*Resource`: the discovered ancestor resource.
//   - `os.FileInfo`: the stat info for the discovered ancestor.
//   - `error`: non-nil when `path` lies outside the scoped root or the root itself is inaccessible.
func (p *Provider) findClosestExistingDir(path string) (ancestor *Resource, info os.FileInfo, err error) {

	root := p.Root()

	rel, relErr := filepath.Rel(root, path)
	if relErr != nil || strings.HasPrefix(rel, "..") {
		return nil, nil, fmt.Errorf("%s lies outside scoped root %s", path, root)
	}

	runtimeEnvironment := p.RuntimeEnvironment()
	current := path

	for {
		if info, err = p.stat(current); err == nil {
			// findClosestExistingDir is discovery — walking up the parent chain to find an existing directory.
			// DiscoverResource handles construction + Catalog.Discover internally.
			a, derr := DiscoverResource(runtimeEnvironment, current)
			if derr != nil {
				return nil, nil, derr
			}
			return a, info, nil
		}

		if current == root {
			return nil, nil, fmt.Errorf("scoped root %s does not exist or is not accessible", root)
		}

		current = filepath.Dir(current)
	}
}

// isDirAndNotEmpty reports whether `abs` is a directory that contains at least one entry.
//
// Parameters:
//   - `abs`: the absolute path to inspect.
//
// Returns:
//   - `bool`: true when the path is a directory holding one or more entries.
//   - `error`: non-nil on open or stat failure.
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

// lstat returns file info for `abs` without following symlinks.
//
// Parameters:
//   - `abs`: the absolute path to stat.
//
// Returns:
//   - `os.FileInfo`: the stat info for the entry itself (the link, not its target).
//   - `error`: non-nil on stat failure.
func (p *Provider) lstat(abs string) (os.FileInfo, error) {
	root := p.RuntimeEnvironment().Root
	return root.Lstat(root.NewPath(abs))
}

// mkdirAll creates the directory `abs` and all missing parents with the given permissions.
//
// Parameters:
//   - `abs`: the absolute directory path to create.
//   - `perm`: the [os.FileMode] applied to created directories.
//
// Returns:
//   - `error`: non-nil on creation failure.
func (p *Provider) mkdirAll(abs string, perm os.FileMode) error {
	root := p.RuntimeEnvironment().Root
	return root.MkdirAll(root.NewPath(abs), perm)
}

// newTrackerIfEnabled constructs a gitignore tracker rooted at `rootPath`, or returns nil when filtering is disabled.
//
// Parameters:
//   - `rootPath`: the directory the tracker scans for gitignore rules.
//   - `honorGitignore`: when false, no tracker is built and (nil, nil) is returned.
//
// Returns:
//   - `*gitignore.Tracker`: the constructed tracker, or nil when `honorGitignore` is false.
//   - `error`: non-nil on tracker construction failure.
func (p *Provider) newTrackerIfEnabled(rootPath string, honorGitignore bool) (*gitignore.Tracker, error) {
	if !honorGitignore {
		return nil, nil
	}
	return gitignore.NewTracker(rootPath)
}

// open opens the file at `abs` for reading.
//
// Parameters:
//   - `abs`: the absolute path to open.
//
// Returns:
//   - `*os.File`: the opened file.
//   - `error`: non-nil on open failure.
func (p *Provider) open(abs string) (*os.File, error) {
	root := p.RuntimeEnvironment().Root
	return root.Open(root.NewPath(abs))
}

// openFile opens the file at `abs` with the given flags and permissions.
//
// Parameters:
//   - `abs`: the absolute path to open.
//   - `flag`: the open flags (the os.O_* bitmask).
//   - `perm`: the [os.FileMode] applied when the file is created.
//
// Returns:
//   - `*os.File`: the opened file.
//   - `error`: non-nil on open failure.
func (p *Provider) openFile(abs string, flag int, perm os.FileMode) (*os.File, error) {
	root := p.RuntimeEnvironment().Root
	return root.OpenFile(root.NewPath(abs), flag, perm)
}

// prepareWrite stages a write: resolves the target, then creates its parent chain or archives any existing file.
//
// Uses [buildCandidate] (not NewResource or DiscoverResource) because the producer caller has already interned the
// same URI; this helper just needs a fresh handle for resolving stat metadata. When the target does not exist, the
// parent chain is created and its boundary recorded; when it exists, it is archived via [Provider.Remove].
//
// Parameters:
//   - `resource`: the [*Resource] to be written.
//
// Returns:
//   - `*Resource`: the staged target resource.
//   - `*Receipt`: the compensation receipt recording the boundary or archived predecessor.
//   - `error`: non-nil on candidate construction, resolve, parent creation, or archive failure.
func (p *Provider) prepareWrite(resource *Resource) (product *Resource, receipt *Receipt, err error) {

	if product, err = buildCandidate(p.RuntimeEnvironment(), resource.SourcePath.Abs()); err != nil {
		return nil, nil, err
	}

	if err = product.Resolve(); err != nil {
		return nil, nil, err
	}

	if !product.Exists() {

		parentPath := filepath.Dir(product.SourcePath.Abs())

		boundary, _, err := p.findClosestExistingDir(parentPath)
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

// read returns the contents of the file `resource` as an in-memory buffer.
//
// Parameters:
//   - `resource`: the file [*Resource] to read.
//
// Returns:
//   - `*bytes.Buffer`: a buffer over the file contents.
//   - `error`: non-nil on read failure.
func (p *Provider) read(resource *Resource) (*bytes.Buffer, error) {

	root := p.RuntimeEnvironment().Root
	data, err := root.ReadFile(root.NewPath(resource.SourcePath.Abs()))

	if err != nil {
		return nil, err
	}

	return bytes.NewBuffer(data), nil
}

// readLink reads the destination of the symlink at `abs`, resolving it to a cleaned absolute path.
//
// A relative link target is joined against the link's directory before cleaning.
//
// Parameters:
//   - `abs`: the absolute path of the symlink.
//
// Returns:
//   - `string`: the cleaned absolute path the symlink points to.
//   - `error`: non-nil on readlink failure.
func (p *Provider) readLink(abs string) (string, error) {

	root := p.RuntimeEnvironment().Root
	target, err := root.Readlink(root.NewPath(abs))

	if err != nil {
		return "", err
	}

	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(abs), target)
	}

	return filepath.Clean(target), nil
}

// remove deletes the file or empty directory at `abs`.
//
// Parameters:
//   - `abs`: the absolute path to remove.
//
// Returns:
//   - `error`: non-nil on removal failure.
func (p *Provider) remove(abs string) error {
	root := p.RuntimeEnvironment().Root
	return root.Remove(root.NewPath(abs))
}

// rename moves the entry at `oldAbs` to `newAbs`.
//
// Parameters:
//   - `oldAbs`: the absolute source path.
//   - `newAbs`: the absolute destination path.
//
// Returns:
//   - `error`: non-nil on rename failure.
func (p *Provider) rename(oldAbs, newAbs string) error {
	root := p.RuntimeEnvironment().Root
	return root.Rename(root.NewPath(oldAbs), root.NewPath(newAbs))
}

// stat returns file info for `abs`, following symlinks.
//
// Parameters:
//   - `abs`: the absolute path to stat.
//
// Returns:
//   - `os.FileInfo`: the stat info for the entry (or its symlink target).
//   - `error`: non-nil on stat failure.
func (p *Provider) stat(abs string) (os.FileInfo, error) {
	root := p.RuntimeEnvironment().Root
	return root.Stat(root.NewPath(abs))
}

// symlink creates a symbolic link at `linkAbs` pointing to `targetAbs`, stored as a path relative to the link.
//
// Parameters:
//   - `targetAbs`: the absolute path the link should resolve to.
//   - `linkAbs`: the absolute path at which the link is created.
//
// Returns:
//   - `error`: non-nil when the relative target cannot be computed or the link cannot be created.
func (p *Provider) symlink(targetAbs, linkAbs string) error {

	root := p.RuntimeEnvironment().Root
	relTarget, err := filepath.Rel(filepath.Dir(linkAbs), targetAbs)

	if err != nil {
		return err
	}

	return root.Symlink(relTarget, root.NewPath(linkAbs))
}

// walkDir dispatches a directory walk to [fs.WalkDir] over the scoped root's filesystem, or to [filepath.WalkDir].
//
// When an [fsroot.Root] is present, paths are walked relative to it and rejoined to absolute form for `walkFn`;
// otherwise the walk runs directly against the OS filesystem.
//
// Parameters:
//   - `osRoot`: the scoped [fsroot.Root] to walk, or nil to walk the OS filesystem directly.
//   - `absoluteRoot`: the absolute path at which the walk begins.
//   - `walkFn`: the per-entry callback receiving the absolute path, [fs.DirEntry], and any walk error.
//
// Returns:
//   - `error`: the first error returned by `walkFn` or the underlying walker.
func (p *Provider) walkDir(
	osRoot fsroot.Root,
	absoluteRoot string,
	walkFn func(string, fs.DirEntry, error) error,
) error {
	if osRoot != nil {
		relRoot := osRoot.NewPath(absoluteRoot).Rel()
		return fs.WalkDir(osRoot.FS(), relRoot, func(relPath string, d fs.DirEntry, walkDirErr error) error {
			return walkFn(filepath.Join(osRoot.Name(), relPath), d, walkDirErr)
		})
	}
	return filepath.WalkDir(absoluteRoot, walkFn)
}

// write writes `data` to the staged target with the given mode and ownership.
//
// `chown` follows the same Dockerfile-style format as the public Write* methods; empty means no ownership change. The
// chown is applied after the file is fully written and synced — placing it before the close would risk applying
// ownership to a file the kernel may yet reject.
//
// Parameters:
//   - `resource`: the [*Resource] to write.
//   - `data`: the bytes to write.
//   - `chmod`: the [os.FileMode] applied to the written file.
//   - `chown`: the Dockerfile-style ownership string, or empty for no ownership change.
//
// Returns:
//   - `*Resource`: the written resource.
//   - `*Receipt`: the compensation receipt for undo.
//   - `error`: non-nil on write preparation, open, write, sync, or chown failure.
func (p *Provider) write(
	resource *Resource,
	data []byte,
	chmod os.FileMode,
	chown string,
) (product *Resource, receipt *Receipt, err error) {

	product, receipt, err = p.prepareWrite(resource)
	if err != nil {
		return nil, nil, err
	}

	f, err := p.openFile(product.SourcePath.Abs(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, chmod)
	if err != nil {
		return product, receipt, err
	}
	defer iox.Close(&err, f)

	if _, err = f.Write(data); err != nil {
		return product, receipt, err
	}

	if err = f.Sync(); err != nil {
		return product, receipt, err
	}

	if err = applyChown(product.SourcePath.Abs(), chown); err != nil {
		return product, receipt, err
	}

	return product, receipt, nil
}

// Actions

// isIgnored reports whether the gitignore `tracker` filters `path`.
//
// A path that cannot be made relative to the tracker root, or that fails to stat, is treated as not ignored.
//
// Parameters:
//   - `tracker`: the [*gitignore.Tracker] holding active ignore rules.
//   - `path`: the absolute path to test.
//
// Returns:
//   - `bool`: true when the tracker considers `path` ignored.
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

// pruneEmptyParents removes now-empty parent directories of `resource`, stopping at `boundary`.
//
// Pruning is a no-op when `prune` is false. The boundary defaults to the scoped [Provider.Root] when `boundary` is
// nil. Removal stops at the first non-empty directory (a failed remove ends the walk silently).
//
// Parameters:
//   - `resource`: the [*Resource] whose parent chain is pruned.
//   - `prune`: whether pruning runs at all.
//   - `boundary`: the [*Resource] at which pruning stops; nil stops at the scoped root.
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

// endregion

// endregion

// region SUPPORTING TYPES

// Reducer folds one filesystem entry into an accumulator during a [Provider.WalkTree] traversal.
//
// WalkTree calls the Reducer once per discovered entry, threading the prior `result` back in as `initial` so the
// final return value is the fold over the whole tree. The recovery `stack` is available for recording compensation.
//
// Parameters:
//   - `initial`: the accumulator returned by the previous invocation (nil on the first call).
//   - `resource`: the [*Resource] for the current entry.
//   - `relativePath`: the entry's path relative to the walk root.
//   - `stack`: the [*op.RecoveryStack] for recording compensation actions.
//
// Returns:
//   - `any`: the updated accumulator, threaded into the next invocation.
//   - `error`: non-nil to abort the traversal.
type Reducer func(initial any, resource *Resource, relativePath string, stack *op.RecoveryStack) (result any, err error)

// endregion
