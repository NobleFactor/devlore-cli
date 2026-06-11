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

// Reducer is a function called for each file or directory in a [#Provider.WalkTree] operation.
type Reducer func(initial any, resource *Resource, relativePath string, stack *op.RecoveryStack) (result any, err error)

// region EXPORTED METHODS

// region State management

// Root returns the root path of the file system scope, or empty if no root is set.
func (p *Provider) Root() string {
	if p.RuntimeEnvironment().Root == nil {
		return ""
	}
	return p.RuntimeEnvironment().Root.Name()
}

// endregion

// region Behaviors

// Compensable actions

// Backup moves the file at "path" to a timestamped backup location.
func (p *Provider) Backup(activationRecord *op.ActivationRecord, source *Resource, backupSuffix string) (*Resource, *Receipt, error) {

	if backupSuffix == "" {
		backupSuffix = ".devlore-backup"
	}

	timestamp := time.Now().Format("20060102-150405")
	backupPath := source.SourcePath.Abs() + backupSuffix + "." + timestamp

	return p.Move(activationRecord, source, backupPath)
}

// CompensateBackup undoes a Backup by delegating to [Provider.CompensateMove].
func (p *Provider) CompensateBackup(receipt *Receipt) error {
	return p.CompensateMove(receipt)
}

// Copy copies source's contents to a new file at destinationPath with the given mode and ownership.
//
// chown is the Dockerfile-style ownership string (`"user[:group]"`, `":group"`, `"uid[:gid]"`, or empty
// for "no change"). When non-empty it is parsed and applied via os.Chown after the file is created.
//
// +devlore:defaults chmod={{ umask 0o755 }}, chown=""
func (p *Provider) Copy(activationRecord *op.ActivationRecord, source *Resource, destinationPath string, chmod os.FileMode, chown string) (product *Resource, receipt *Receipt, err error) {

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

// CompensateCopy undoes a Copy by restoring the original file from recovery.
func (p *Provider) CompensateCopy(receipt *Receipt) error {
	return p.compensateWrite(receipt)
}

// Link creates a symbolic link at targetPath pointing to source.
func (p *Provider) Link(activationRecord *op.ActivationRecord, source *Resource, targetPath string) (product *Resource, receipt *Receipt, err error) {

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

		boundary, _, err := p.closestExistingDir(parentPath)
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

// CompensateLink undoes a Link by removing the symlink and restoring whatever was there before.
func (p *Provider) CompensateLink(receipt *Receipt) error {
	return p.compensateWrite(receipt)
}

// Mkdir creates a directory (and any missing parents) with the given mode and ownership.
//
// chown is the Dockerfile-style ownership string (`"user[:group]"`, `":group"`, `"uid[:gid]"`, or empty
// for "no change"). When non-empty it is applied via os.Chown to the leaf directory only — intermediate
// parents created by the call are NOT chown'd, since their semantic role is "existed before this call"
// rather than "created by this call."
//
// +devlore:defaults chmod={{ umask 0o777 }}, chown=""
func (p *Provider) Mkdir(activationRecord *op.ActivationRecord, path string, chmod os.FileMode, chown string) (product *Resource, receipt *Receipt, err error) {

	product, err = NewResource(p.RuntimeEnvironment(), activationRecord.Unit, path)
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

// CompensateMkdir undoes a [Provider.Mkdir] by walking up from the receipt's resource and removing each entry
// until it reaches the boundary recorded on the receipt (exclusive).
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

// Move moves a file from source to destinationPath.
func (p *Provider) Move(activationRecord *op.ActivationRecord, source *Resource, destinationPath string) (product *Resource, receipt *Receipt, err error) {

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

// CompensateMove undoes a Move by moving the file from destination back to source and restoring the old destination.
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

// Remove deletes the file at "path".
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

	recoveryID, digest, err := p.archiveAndPrune(resource, prune, boundary)
	if err != nil {
		return nil, nil, err
	}

	receipt = NewReceipt(resource)
	_ = receipt.SetRecoveryID(recoveryID)
	receipt.SetRecoveryDigest(digest)

	return nil, receipt, nil
}

// CompensateRemove undoes a Remove by restoring the file from recovery.
func (p *Provider) CompensateRemove(receipt *Receipt) error {
	return p.CompensateUnlink(receipt)
}

// RemoveAll removes the file at "path" and any children it contains.
func (p *Provider) RemoveAll(resource *Resource, prune bool, boundary *Resource) (product *Resource, receipt *Receipt, err error) {

	recoveryID, digest, err := p.archiveAndPrune(resource, prune, boundary)
	if err != nil {
		return nil, nil, err
	}

	receipt = NewReceipt(resource)
	_ = receipt.SetRecoveryID(recoveryID)
	receipt.SetRecoveryDigest(digest)

	return nil, receipt, nil
}

// CompensateRemoveAll undoes a RemoveAll by restoring from recovery.
func (p *Provider) CompensateRemoveAll(receipt *Receipt) error {
	return p.CompensateUnlink(receipt)
}

// Unlink removes a symlink.
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

	recoveryID, digest, err := p.archiveAndPrune(resource, prune, boundary)
	if err != nil {
		return nil, nil, err
	}

	receipt = NewReceipt(resource)
	_ = receipt.SetRecoveryID(recoveryID)
	receipt.SetRecoveryDigest(digest)

	return nil, receipt, nil
}

// CompensateUnlink undoes an Unlink by restoring the symlink from recovery.
func (p *Provider) CompensateUnlink(receipt *Receipt) error {

	if receipt == nil || receipt.Resource() == nil {
		return nil
	}

	resource, ok := receipt.Resource().(*Resource)
	if !ok {
		return fmt.Errorf("compensate unlink: unexpected resource type %T", receipt.Resource())
	}

	// ALWAYS remove the new file before attempting to restore. RestoreFile uses os.Rename which fails if target exists.
	if err := p.remove(resource.SourcePath.Abs()); err != nil && !os.IsNotExist(err) {
		return err
	}

	recoveryID := receipt.RecoveryID()
	if recoveryID == "" {
		return nil
	}

	return p.RuntimeEnvironment().RecoverySite.RestoreFile(resource.SourcePath, recoveryID)
}

// WalkTree performs a depth-first traversal.
//
// WalkTree is a discovery operation — the walker observes existing filesystem entries; it does not produce
// them. The Resources it interns into the catalog are discovered, not authored, so they carry no `producerID`
// stamp from this method.
//
// +devlore:defaults includeGitignored=false
func (p *Provider) WalkTree(root *Resource, fn Reducer, includeGitignored bool) (product any, stack *op.RecoveryStack, err error) {

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

// CompensateWalkTree unwinds the RecoveryStack returned by WalkTree in LIFO order.
func (p *Provider) CompensateWalkTree(stack *op.RecoveryStack) error {
	if stack == nil {
		return nil
	}
	return stack.Unwind()
}

// WriteBytes writes inline byte content to a file.
//
// chown is the Dockerfile-style ownership string (`"user[:group]"`, `":group"`, `"uid[:gid]"`, or empty
// for "no change"). When non-empty it is applied via os.Chown after the file is written.
//
// +devlore:defaults chmod={{ umask 0o666 }}, chown=""
func (p *Provider) WriteBytes(activationRecord *op.ActivationRecord, destinationPath string, content string, chmod os.FileMode, chown string) (product *Resource, receipt *Receipt, err error) {

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

// CompensateWriteBytes undoes a WriteBytes by restoring the original file.
func (p *Provider) CompensateWriteBytes(receipt *Receipt) error {
	return p.compensateWrite(receipt)
}

// WriteText writes inline content to a file.
//
// chown is the Dockerfile-style ownership string (`"user[:group]"`, `":group"`, `"uid[:gid]"`, or empty
// for "no change"). When non-empty it is applied via os.Chown after the file is written.
//
// +devlore:defaults chmod={{ umask 0o666 }}, chown=""
func (p *Provider) WriteText(activationRecord *op.ActivationRecord, destinationPath string, content string, chmod os.FileMode, chown string) (product *Resource, receipt *Receipt, err error) {

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

// CompensateWriteText undoes a WriteText by restoring the original file.
func (p *Provider) CompensateWriteText(receipt *Receipt) error {
	return p.compensateWrite(receipt)
}

// Fallible actions

// Exists returns true if the file at "path" exists.
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

	err = p.walkDir(p.RuntimeEnvironment().Root, absoluteRoot, func(absolutePath string, dirEntry fs.DirEntry, err error) error {

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
	})

	if err != nil {
		return nil, fmt.Errorf("find: walk %q: %w", absoluteRoot, err)
	}

	return p.resources(matches)
}

// Glob returns [Resource] entries for filesystem paths matching the pattern.
//
// +devlore:defaults includeGitignored=false
func (p *Provider) Glob(pattern string, includeGitignored bool) ([]*Resource, error) {

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	if includeGitignored {
		return p.resources(matches)
	}

	tracker, err := gitignore.NewTracker(p.Root())
	if err != nil {
		return p.resources(matches) //nolint:nilerr // graceful degradation
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

// Observe captures the runtime-observed state of `resource` as a [*Observation].
//
// Stats the file at `resource.SourcePath`. When the file exists, the Observation carries the
// stat-derived metadata (`Size`, `Mode`, `ModTime`, `Inode`, `Device`) with `Exists` set to true.
// When the file does not exist (`os.ErrNotExist`), the Observation carries zero metadata with
// `Exists` set to false — not-exist is a valid observation outcome, not an error. Any other stat
// failure returns nil and the underlying error.
//
// Provider methods that previously called `r.Resolve()` to populate the Resource's mutating fields
// now call `p.Observe(r)` instead. The observation is a value returned to the caller; the framework
// catalogs identity Resources, not observations on identity Resources.
//
// Parameters:
//   - `resource`: the [*Resource] whose current filesystem state to observe.
//
// Returns:
//   - *Observation: the constructed observation; never nil on a nil-error return.
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

// ReadBytes returns the contents of a file [Resource].
func (p *Provider) ReadBytes(resource *Resource) (product []byte, err error) {

	buffer, err := p.read(resource)
	if err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

// ReadText returns the contents of a file [Resource].
func (p *Provider) ReadText(resource *Resource) (product string, err error) {

	buffer, err := p.read(resource)
	if err != nil {
		return "", err
	}

	return buffer.String(), nil
}

// Actions

// Join joins path components using the OS path separator.
func (p *Provider) Join(parts ...string) string {
	return filepath.Join(parts...)
}

// Name returns the last element of "path" (a file or directory name).
func (p *Provider) Name(path string) string {
	return filepath.Base(path)
}

// Parent returns the directory containing the file at "path".
func (p *Provider) Parent(path string) string {
	return filepath.Dir(path)
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// Fallible actions

// applyGitignore checks if a directory entry should be skipped based on gitignore rules.
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

func (p *Provider) compensateWrite(receipt *Receipt) error {

	if receipt == nil || receipt.Resource() == nil {
		return nil
	}

	resource, ok := receipt.Resource().(*Resource)
	if !ok {
		return fmt.Errorf("compensate write: unexpected resource type %T", receipt.Resource())
	}

	// ALWAYS remove the new file before attempting to restore. RestoreFile uses os.Rename which fails if target exists.
	if err := p.remove(resource.SourcePath.Abs()); err != nil && !os.IsNotExist(err) {
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

// isDirAndNotEmpty checks if the path is a directory that contains at least one entry.
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
func (p *Provider) lstat(abs string) (os.FileInfo, error) {
	root := p.RuntimeEnvironment().Root
	return root.Lstat(root.NewPath(abs))
}

// closestExistingDir walks up from path to find the nearest existing entry within the scoped [Provider.Root] hierarchy.
func (p *Provider) closestExistingDir(path string) (ancestor *Resource, info os.FileInfo, err error) {

	root := p.Root()

	rel, relErr := filepath.Rel(root, path)
	if relErr != nil || strings.HasPrefix(rel, "..") {
		return nil, nil, fmt.Errorf("%s lies outside scoped root %s", path, root)
	}

	runtimeEnvironment := p.RuntimeEnvironment()
	current := path

	for {
		if info, err = p.stat(current); err == nil {
			// closestExistingDir is discovery — walking up the parent chain to find an existing directory.
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

// mkdirAll creates a directory and all parents.
func (p *Provider) mkdirAll(abs string, perm os.FileMode) error {
	root := p.RuntimeEnvironment().Root
	return root.MkdirAll(root.NewPath(abs), perm)
}

// newTrackerIfEnabled creates a gitignore tracker if honorGitignore is true.
func (p *Provider) newTrackerIfEnabled(rootPath string, honorGitignore bool) (*gitignore.Tracker, error) {
	if !honorGitignore {
		return nil, nil
	}
	return gitignore.NewTracker(rootPath)
}

// open opens a file for reading.
func (p *Provider) open(abs string) (*os.File, error) {
	root := p.RuntimeEnvironment().Root
	return root.Open(root.NewPath(abs))
}

// openFile opens a file with the given flags and permissions.
func (p *Provider) openFile(abs string, flag int, perm os.FileMode) (*os.File, error) {
	root := p.RuntimeEnvironment().Root
	return root.OpenFile(root.NewPath(abs), flag, perm)
}

// prepareWrite handles pre-write backup. Uses [buildCandidate] (not NewResource or DiscoverResource) because
// the producer caller has already interned the same URI; this helper just needs a fresh handle for resolving
// stat metadata.
func (p *Provider) prepareWrite(resource *Resource) (product *Resource, receipt *Receipt, err error) {

	if product, err = buildCandidate(p.RuntimeEnvironment(), resource.SourcePath.Abs()); err != nil {
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
func (p *Provider) read(resource *Resource) (*bytes.Buffer, error) {

	root := p.RuntimeEnvironment().Root
	data, err := root.ReadFile(root.NewPath(resource.SourcePath.Abs()))

	if err != nil {
		return nil, err
	}

	return bytes.NewBuffer(data), nil
}

// readLink reads the destination of a symlink.
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

// remove removes a file or empty directory.
func (p *Provider) remove(abs string) error {
	root := p.RuntimeEnvironment().Root
	return root.Remove(root.NewPath(abs))
}

// rename moves a file from oldAbs to newAbs.
func (p *Provider) rename(oldAbs, newAbs string) error {
	root := p.RuntimeEnvironment().Root
	return root.Rename(root.NewPath(oldAbs), root.NewPath(newAbs))
}

// resources constructs a [Resource] for each input path.
func (p *Provider) resources(paths []string) (product []*Resource, err error) {

	runtimeEnvironment := p.RuntimeEnvironment()
	resources := make([]*Resource, len(paths))

	for i, path := range paths {
		// resources is discovery — build catalog handles for caller-supplied paths without claiming
		// production. DiscoverResource handles construction + Catalog.Discover internally.
		concrete, derr := DiscoverResource(runtimeEnvironment, path)
		if derr != nil {
			return nil, derr
		}
		resources[i] = concrete
	}

	return resources, nil
}

// stat returns file info following symlinks.
func (p *Provider) stat(abs string) (os.FileInfo, error) {
	root := p.RuntimeEnvironment().Root
	return root.Stat(root.NewPath(abs))
}

// symlink creates a symbolic link.
func (p *Provider) symlink(targetAbs, linkAbs string) error {

	root := p.RuntimeEnvironment().Root
	relTarget, err := filepath.Rel(filepath.Dir(linkAbs), targetAbs)

	if err != nil {
		return err
	}

	return root.Symlink(relTarget, root.NewPath(linkAbs))
}

// walkDir dispatches to fs.WalkDir.
func (p *Provider) walkDir(osRoot op.Root, absoluteRoot string, walkFn func(string, fs.DirEntry, error) error) error {
	if osRoot != nil {
		relRoot := osRoot.NewPath(absoluteRoot).Rel()
		return fs.WalkDir(osRoot.FS(), relRoot, func(relPath string, d fs.DirEntry, walkDirErr error) error {
			return walkFn(filepath.Join(osRoot.Name(), relPath), d, walkDirErr)
		})
	}
	return filepath.WalkDir(absoluteRoot, walkFn)
}

// write writes data to the specified path with the given mode and ownership.
//
// chown follows the same Dockerfile-style format as the public Write* methods; empty means no ownership change. The
// chown is applied after the file is fully written and synced — placing it before the close would risk applying
// ownership to a file the kernel may yet reject.
func (p *Provider) write(resource *Resource, data []byte, chmod os.FileMode, chown string) (product *Resource, receipt *Receipt, err error) {

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

// isIgnored answers whether the gitignore tracker considers path filtered.
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

// pruneEmptyParents removes empty parent directories.
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

// archiveAndPrune moves resource to the recovery site, capturing the archived bytes' digest beforehand.
//
// The returned digest is what compensation will compare against to detect tampering of the recovery archive between the
// forward action and compensation. Empty digest is returned (and not an error) when the file could not be hashed —
// typically because it was a symlink or otherwise unreadable; the archive proceeds regardless.
func (p *Provider) archiveAndPrune(resource *Resource, prune bool, boundary *Resource) (recoveryID string, digest op.Digest, err error) {

	digest = preArchiveDigest(p.RuntimeEnvironment().Root, resource.SourcePath.Abs())

	recoveryID, err = p.RuntimeEnvironment().RecoverySite.ArchiveFile(resource.SourcePath)
	if err != nil {
		return "", op.Digest{}, err
	}

	p.pruneEmptyParents(resource, prune, boundary)
	return recoveryID, digest, nil
}

// preArchiveDigest computes the digest of the bytes at path before archival.
//
// Returns the zero op.Digest (not an error) when the file cannot be hashed — symlinks, unreadable files, etc. Callers
// can record the digest when available without blocking the archive when not.
func preArchiveDigest(root op.Root, path string) op.Digest {

	checksum := checksumFile(root, path)
	if checksum == "" {
		return op.Digest{}
	}

	digest, err := op.ParseDigest(checksum)
	if err != nil {
		return op.Digest{}
	}

	return digest
}

// endregion

// endregion
