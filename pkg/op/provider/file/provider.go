// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

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

// Backup moves the entry at `sourcePath` to a timestamped backup location, delegating to [Provider.Move].
//
// Takes a path, not a resource (step 23, ruling 2): Backup renames — it never reads content — so the entry it
// displaces is identified by location and the produced backup resource is the return value.
//
// Parameters:
//   - `activationRecord`: the dispatch activation threaded to [Provider.Move].
//   - `sourcePath`: the path of the entry to back up.
//   - `backupSuffix`: the suffix inserted before the timestamp; empty defaults to the runtime environment's
//     `BackupSuffix` (the spec path derives it as ".<ProgramName>-backup", e.g. ".devlore-backup").
//
// Returns:
//   - `Entry`: the backup destination resource, minted as the moved entry's observed kind.
//   - `*Receipt`: the compensation receipt for undo.
//   - `error`: non-nil on move failure.
func (p *Provider) Backup(
	activationRecord *op.ActivationRecord,
	sourcePath string,
	backupSuffix string,
) (Entry, *Receipt, error) {

	if backupSuffix == "" {
		backupSuffix = p.RuntimeEnvironment().BackupSuffix
	}

	sourceAbs := p.RuntimeEnvironment().Root.NewPath(sourcePath).Abs()
	timestamp := time.Now().Format("20060102-150405")
	backupPath := sourceAbs + backupSuffix + "." + timestamp

	return p.Move(activationRecord, sourcePath, backupPath)
}

// Copy copies `source`'s contents to a new file at `destinationPath` with the given mode and ownership.
//
// `chown` is the Dockerfile-style ownership string (`"user[:group]"`, `":group"`, `"uid[:gid]"`, or empty for no
// change). When non-empty it is parsed and applied via os.Chown after the file is created.
//
// Parameters:
//   - `activationRecord`: the dispatch activation; its `Unit` stamps the produced [*Regular]'s producerID.
//   - `source`: the [*Regular] whose contents are copied — a content read, so the parameter is the resource
//     (step 23, ruling 2).
//   - `destinationPath`: the destination path for the new file.
//   - `chmod`: the [os.FileMode] applied to the created file.
//   - `chown`: the Dockerfile-style ownership string, or empty for no ownership change.
//
// Returns:
//   - `*Regular`: the created destination resource, resolved against the filesystem.
//   - `*Receipt`: the compensation receipt for undo.
//   - `error`: non-nil on resource construction, write preparation, copy, chown, or resolve failure.
//
// +devlore:defaults chmod={{ umask 0o755 }}, chown=""
func (p *Provider) Copy(
	activationRecord *op.ActivationRecord,
	source *Regular,
	destinationPath string,
	chmod os.FileMode,
	chown string,
) (product *Regular, receipt *Receipt, err error) {

	product, err = NewRegular(p.RuntimeEnvironment(), activationRecord.CallerID, destinationPath)
	if err != nil {
		return nil, nil, err
	}

	spec, err := p.stageWrite(product)
	if errors.Is(err, errConflictSkip) {
		return nil, nil, nil // Occupied target left untouched per the skip policy.
	}
	if err != nil {
		return nil, nil, err
	}
	receipt = NewReceipt(spec)

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

// Link creates a symbolic link at `targetPath` pointing to `sourcePath`, archiving any existing entry first.
//
// Takes paths, not resources (step 23, ruling 2): the symlink stores a name — nothing is read from the source,
// which may legally dangle. By default the stored name is `sourcePath` canonicalized to its absolute form (the
// deploy posture: links across trees stay valid from any working directory); with `verbatim` set, the LITERAL
// `sourcePath` string becomes the link's content, uninterpreted (the extraction posture — archive §10 ruling 1a:
// a tar entry's relative target lands on disk exactly as archived, which also keeps the [SymbolicLink.Digest]
// literal-target hash faithful to the archive). When an entry already exists at `targetPath`: if it is a symlink
// already pointing at the stored name, Link is a no-op; otherwise the existing entry is archived to the
// [op.RecoverySite] before the new link is created. When nothing exists, the parent directory chain is created
// and its boundary recorded on the receipt for compensation.
//
// Parameters:
//   - `activationRecord`: the dispatch activation; its `Unit` stamps the produced [*SymbolicLink]'s producerID.
//   - `sourcePath`: the path the link points to.
//   - `targetPath`: the path at which the symlink is created.
//   - `verbatim`: when true, store `sourcePath` in the link exactly as given instead of absolutizing it.
//
// Returns:
//   - `*SymbolicLink`: the link resource (resolved when created; the matched resource when already correct).
//   - `*Receipt`: the compensation receipt for undo, or nil when no change was made.
//   - `error`: non-nil on resource construction, archive, parent creation, symlink, or resolve failure.
//
// +devlore:defaults verbatim=false
func (p *Provider) Link(
	activationRecord *op.ActivationRecord,
	sourcePath string,
	targetPath string,
	verbatim bool,
) (product *SymbolicLink, receipt *Receipt, err error) {

	storedName := p.RuntimeEnvironment().Root.NewPath(sourcePath).Abs()
	if verbatim {
		storedName = sourcePath
	}

	product, err = NewSymbolicLink(p.RuntimeEnvironment(), activationRecord.CallerID, targetPath)
	if err != nil {
		return nil, nil, err
	}

	if info, err := p.lstat(product.SourcePath.Abs()); err == nil {

		if info.Mode()&os.ModeSymlink != 0 && p.existingLinkMatches(product.SourcePath.Abs(), storedName, verbatim) {
			return product, nil, nil // Already correct — no change
		}

		// Something exists at the target — the write-seam conflict policy governs (phase-8 step 49).
		switch p.conflictPolicy() {
		case op.ConflictStop:
			return nil, nil, fmt.Errorf(
				"target %s is occupied and the conflict policy is stop (replace archives and overwrites; skip leaves it)",
				product.SourcePath.Abs())
		case op.ConflictSkip:
			return nil, nil, nil
		case op.ConflictReplace:
		}

		receipt, err = p.archiveOccupant(product)
		if err != nil {
			return nil, nil, err
		}
	} else {

		// Does not exist — standard parent directory creation.
		parentPath := filepath.Dir(product.SourcePath.Abs())

		boundary, _, err := p.findClosestExistingDir(parentPath)
		if err != nil {
			return nil, nil, err
		}

		receipt = NewReceipt(NewReceiptSpec(product, MutationCreateFile).WithBoundary(boundary))

		if err := p.mkdirAll(parentPath, 0o750); err != nil {
			return nil, receipt, err
		}
	}

	if verbatim {
		err = p.symlinkRaw(storedName, product.SourcePath.Abs())
	} else {
		err = p.symlink(storedName, product.SourcePath.Abs())
	}
	if err != nil {
		return nil, receipt, err
	}

	if err := product.Resolve(); err != nil {
		return nil, receipt, err
	}

	return product, receipt, nil
}

// existingLinkMatches reports whether the symlink at `linkPath` already stores the canonical name.
//
// The default path stores a relativized target (see [Provider.symlink]); the absolutized read is what
// matches the canonical stored name. Verbatim links compare the raw stored target.
//
// Parameters:
//   - `linkPath`: the symlink's absolute path.
//   - `storedName`: the canonical stored name to match.
//   - `verbatim`: whether the link was stored verbatim.
//
// Returns:
//   - `bool`: true when the existing link already matches.
func (p *Provider) existingLinkMatches(linkPath, storedName string, verbatim bool) bool {

	existing, readErr := p.rawReadLink(linkPath)
	if !verbatim {
		existing, readErr = p.readLink(linkPath)
	}

	return readErr == nil && existing == storedName
}

// archiveOccupant archives whatever occupies the product's path ahead of a replace, minting the
// update receipt that restores it on compensation.
//
// Parameters:
//   - `product`: the symbolic link whose path is occupied.
//
// Returns:
//   - `*Receipt`: the update receipt carrying the recovery ID and pre-archive digest.
//   - `error`: non-nil when archiving fails.
func (p *Provider) archiveOccupant(product *SymbolicLink) (*Receipt, error) {

	preDigest := preArchiveDigest(p.RuntimeEnvironment().Root, product.SourcePath.Abs())

	recoveryID, archiveErr := p.RuntimeEnvironment().RecoverySite.ArchiveFile(product.SourcePath)
	if archiveErr != nil {
		return nil, archiveErr
	}

	return NewReceipt(NewReceiptSpec(product, MutationUpdateFile).WithRecovery(recoveryID, preDigest)), nil
}

// Mkdir creates a directory (and any missing parents) at `path` with the given mode and ownership.
//
// `chown` is the Dockerfile-style ownership string (`"user[:group]"`, `":group"`, `"uid[:gid]"`, or empty for
// "no change"). When non-empty it is applied via os.Chown to the leaf directory only — intermediate parents
// created by the call are NOT chown'd, since their role is "existed before this call" rather than "created here."
//
// Parameters:
//   - `activationRecord`: the dispatch activation; its `Unit` stamps the produced [*Directory]'s producerID.
//   - `path`: the directory path to create.
//   - `chmod`: the [os.FileMode] applied to the leaf directory.
//   - `chown`: the Dockerfile-style ownership string applied to the leaf directory, or empty for no change.
//
// Returns:
//   - `*Directory`: the created directory resource, resolved; a nil receipt accompanies an already-existing
//     directory.
//   - `*Receipt`: the compensation receipt recording the creation boundary for undo.
//   - `error`: non-nil when `path` exists as a non-directory, or on construction, mkdir, chown, or resolve failure.
//
// +devlore:defaults chmod={{ umask 0o777 }}, chown=""
func (p *Provider) Mkdir(
	activationRecord *op.ActivationRecord,
	path string,
	chmod os.FileMode,
	chown string,
) (product *Directory, receipt *Receipt, err error) {

	leaf := p.RuntimeEnvironment().Root.NewPath(path).Abs()

	// Observe before claiming: an occupant of another kind gets the plain refusal rather than the catalog's
	// cross-kind collision (the claim below would collide with the occupant's discovered entry).
	if info, statErr := p.lstat(leaf); statErr == nil && !info.IsDir() {
		return nil, nil, fmt.Errorf("%s exists, but is not a directory", path)
	}

	product, err = NewDirectory(p.RuntimeEnvironment(), activationRecord.CallerID, path)
	if err != nil {
		return nil, nil, err
	}

	boundary, info, err := p.findClosestExistingDir(leaf)
	if err != nil {
		return nil, nil, err
	}

	if boundary.Path().Abs() == leaf {
		if info.IsDir() {
			return product, nil, nil // directory exists and there's nothing to compensate
		}
		return nil, nil, fmt.Errorf("%s exists, but is not a directory", path)
	}

	receipt = NewReceipt(NewReceiptSpec(product, MutationCreateDir).WithBoundary(boundary))

	if err := p.mkdirAll(leaf, chmod); err != nil {
		return nil, receipt, err
	}

	if err := applyChown(leaf, chown); err != nil {
		return nil, receipt, err
	}

	if err := product.Resolve(); err != nil {
		return nil, receipt, err
	}

	return product, receipt, nil
}

// compensateMakeDir inverts a directory-create mutation by removing the directory subtree it created.
//
// Walks up from the receipt's resource, removing each entry until it reaches the boundary recorded on the receipt
// (exclusive). A non-empty directory encountered along the way (a sibling adopted it) stops the unwind without error.
// [Provider.CompensateFileMutation] dispatches here for [MutationCreateDir].
//
// Parameters:
//   - `receipt`: the directory-create [*Receipt]; a nil receipt or nil boundary is a no-op.
//
// Returns:
//   - `error`: non-nil when the receipt's resource is the wrong type, lies outside its boundary, or removal fails.
func (p *Provider) compensateMakeDir(receipt *Receipt) (err error) {

	if receipt == nil || receipt.Resource() == nil {
		return nil
	}

	resource, ok := receipt.Resource().(Entry)
	if !ok {
		return fmt.Errorf("unexpected resource type %T", receipt.Resource())
	}

	boundary := receipt.Boundary()
	if boundary == nil {
		return nil // no recorded boundary — receipt does not own a creation subtree
	}

	boundaryPath := boundary.Path().Abs()
	current := resource.Path().Abs()

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

// Move moves the entry at `sourcePath` to `destinationPath`, archiving any existing destination first.
//
// Takes paths, not resources (step 23, ruling 2): a move renames — it never reads content. The destination
// product is minted as the moved entry's observed kind (the mutator is at execution time with the disk in hand),
// and the source identity rides the receipt so compensation can move the entry back. The destination's parents
// are created when absent. When an entry already exists at `destinationPath` it is archived for compensation; a
// failed rename attempts to restore that archived destination before returning the error.
//
// Parameters:
//   - `activationRecord`: the dispatch activation; its `Unit` stamps the produced [Entry]'s producerID.
//   - `sourcePath`: the path of the entry to move.
//   - `destinationPath`: the path to move the entry to.
//
// Returns:
//   - `Entry`: the destination resource, minted as the source's observed kind, resolved.
//   - `*Receipt`: the compensation receipt recording the source and any archived destination for undo.
//   - `error`: non-nil when the source does not exist, or on construction, write preparation, rename, or resolve
//     failure.
func (p *Provider) Move(
	activationRecord *op.ActivationRecord,
	sourcePath string,
	destinationPath string,
) (product Entry, receipt *Receipt, err error) {

	sourceAbs := p.RuntimeEnvironment().Root.NewPath(sourcePath).Abs()

	sourceInfo, err := p.lstat(sourceAbs)
	if err != nil {
		return nil, nil, fmt.Errorf("move: source %s: %w", sourceAbs, err)
	}

	product, err = p.produceEntryAt(activationRecord.CallerID, destinationPath, sourceInfo.Mode())
	if err != nil {
		return nil, nil, err
	}

	// The receipt's source is an unlinked identity handle — compensation renames by path; no catalog claim. The
	// handle is a variant candidate (the sealed Entry set excludes the base), kinded by the same observation.
	source, err := candidateOfMode(p.RuntimeEnvironment(), sourceAbs, sourceInfo.Mode())
	if err != nil {
		return nil, nil, err
	}

	// Prepare destination (handle overwrite and parent creation), then record the source so compensation moves the
	// file back — CompensateFileMutation routes a file receipt carrying a source through the move-back undo.
	spec, err := p.stageWrite(product)
	if errors.Is(err, errConflictSkip) {
		return nil, nil, nil // Occupied destination left untouched per the skip policy.
	}
	if err != nil {
		return nil, nil, err
	}
	receipt = NewReceipt(spec.WithSource(source))

	if err = p.rename(sourceAbs, product.Path().Abs()); err != nil {
		// Attempt to restore destination on failure if we archived it.
		if receipt.RecoveryID() != "" {
			//nolint:errcheck // diagnose-ignored-error: rename error wins; see docs/architecture/2.8-eventing-infrastructure.md
			_ = p.RuntimeEnvironment().RecoverySite.RestoreFile(product.Path(), receipt.RecoveryID())
		}
		return nil, nil, err
	}

	if err := product.Resolve(); err != nil {
		return product, receipt, err
	}

	return product, receipt, nil
}

// compensateMove inverts a move by moving the file from destination back to source.
//
// After moving back, any destination archived by the forward move is restored — but only after verifying the recovery
// archive's bytes still match the digest captured at archive time, so tampering is detected before restoration.
// [Provider.CompensateFileMutation] dispatches here when a file-mutation receipt records a source (a move).
//
// Parameters:
//   - `receipt`: the move's [*Receipt]; a nil receipt or nil resource is a no-op.
//
// Returns:
//   - `error`: non-nil on wrong resource type, missing source, move-back failure, digest mismatch, or restore failure.
func (p *Provider) compensateMove(receipt *Receipt) error {

	if receipt == nil || receipt.Resource() == nil {
		return nil
	}

	product, ok := receipt.Resource().(Entry)
	if !ok {
		return fmt.Errorf("compensate move: unexpected resource type %T", receipt.Resource())
	}

	source := receipt.Source()
	if source == nil {
		return fmt.Errorf("compensate move: receipt missing source resource")
	}

	// Move back from destination to source.
	if err := p.rename(product.Path().Abs(), source.Path().Abs()); err != nil {
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

		if err := p.RuntimeEnvironment().RecoverySite.RestoreFile(product.Path(), recoveryID); err != nil {
			return fmt.Errorf("compensate move: restore old destination failed: %w", err)
		}
	}

	return nil
}

// CompensateFileMutation inverts any file or directory mutation by dispatching on the receipt's [MutationKind].
//
// It is the single undo for every file.Receipt: a receipt names [compensateFileMutationAction] as its compensating
// action at construction, so the recovery machinery routes here regardless of which method or dispatcher produced it.
// Create / update / delete of a file restores via [Provider.compensateWrite] (remove the new file, restore any archived
// predecessor, prune boundary directories) — except a file receipt that recorded a source (a move), which reverses via
// [Provider.compensateMove]. A directory create reverses via [Provider.compensateMakeDir] and a directory delete via
// [Provider.compensateRemoveDir].
//
// Parameters:
//   - `activationRecord`: the dispatch activation (the required floor for compensating actions — step 27).
//   - `receipt`: the [*Receipt] to invert; a nil receipt is a no-op.
//
// Returns:
//   - `error`: the underlying compensation error, or a wrapped error for an unknown kind.
func (p *Provider) CompensateFileMutation(activationRecord *op.ActivationRecord, receipt *Receipt) error {

	if receipt == nil {
		return nil
	}

	switch receipt.Kind() {

	case MutationCreateFile, MutationUpdateFile, MutationDeleteFile:
		if receipt.Source() != nil {
			return p.compensateMove(receipt)
		}
		return p.compensateWrite(receipt)

	case MutationCreateDir:
		return p.compensateMakeDir(receipt)

	case MutationDeleteDir:
		return p.compensateRemoveDir(receipt)

	default:
		return fmt.Errorf("compensate file mutation: unknown kind %q", receipt.Kind())
	}
}

// compensateRemoveDir inverts a directory-delete mutation by recreating the removed directory.
//
// Mode fidelity is a known gap: the receipt does not capture the removed directory's permissions, so the directory is
// recreated with a default mode until a directory-delete forward (RemoveDir, a later slice) records it. No forward
// produces [MutationDeleteDir] yet, so this path is reached only once that lands.
//
// Parameters:
//   - `receipt`: the directory-delete [*Receipt]; a nil receipt or nil resource is a no-op.
//
// Returns:
//   - `error`: non-nil when the receipt's resource is the wrong type, or recreation fails.
func (p *Provider) compensateRemoveDir(receipt *Receipt) error {

	if receipt == nil || receipt.Resource() == nil {
		return nil
	}

	resource, ok := receipt.Resource().(Entry)
	if !ok {
		return fmt.Errorf("compensate remove dir: unexpected resource type %T", receipt.Resource())
	}

	return p.mkdirAll(resource.Path().Abs(), 0o750)
}

// Remove deletes the file or empty directory at `path`, archiving it for compensation.
//
// Takes a path (step 23, ruling 2) and discharges the delete invariants itself (ruling 3): the entry is interned
// via its Discover constructor as the observed kind (termination, not production — no producer stamp), moved to
// the recovery site, and its catalog entry marked [op.Gone] on success. A non-existent target is a no-op (nil
// product, nil receipt, nil error). A non-empty directory is an error — use [Provider.RemoveAll] for recursive
// deletion. When `prune` is set, now-empty parents up to `boundary` are removed.
//
// Parameters:
//   - `activationRecord`: the dispatch activation (the required floor for compensable actions — step 27).
//   - `path`: the path of the entry to delete.
//   - `prune`: whether to remove now-empty parent directories up to `boundary`.
//   - `boundary`: the path at which parent pruning stops; empty prunes to the scoped root.
//
// Returns:
//   - `Entry`: always nil — Remove produces no resource.
//   - `*Receipt`: the compensation receipt recording the recovery archive for undo.
//   - `error`: non-nil when the target is a non-empty directory, or on stat or archive failure.
func (p *Provider) Remove(
	activationRecord *op.ActivationRecord,
	path string,
	prune bool,
	boundary string,
) (product Entry, receipt *Receipt, err error) {

	abs := p.RuntimeEnvironment().Root.NewPath(path).Abs()

	nonEmptyDirectory, err := p.isDirAndNotEmpty(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	if nonEmptyDirectory {
		return nil, nil, fmt.Errorf("directory %s is not empty", abs)
	}

	entry, err := p.discoverEntryAt(abs)
	if err != nil {
		return nil, nil, err
	}

	recoveryID, digest, err := p.archiveAndPrune(entry, prune, boundary)
	if err != nil {
		return nil, nil, err
	}

	receipt = NewReceipt(NewReceiptSpec(entry, MutationDeleteFile).WithRecovery(recoveryID, digest))
	p.markEntryGone(entry)

	return nil, receipt, nil
}

// RemoveAll removes `resource` and any children it contains, archiving the subtree for compensation.
//
// Unlike [Provider.Remove], a non-empty directory is removed recursively. Takes a path and discharges the delete
// invariants (step 23, rulings 2 and 3): the entry is interned via its Discover constructor as the observed kind,
// moved to the recovery site, and its catalog entry marked [op.Gone] on success. A non-existent target is a no-op.
// When `prune` is set, now-empty parents up to `boundary` are removed afterward.
//
// Parameters:
//   - `activationRecord`: the dispatch activation (the required floor for compensable actions — step 27).
//   - `path`: the path of the entry to remove recursively.
//   - `prune`: whether to remove now-empty parent directories up to `boundary`.
//   - `boundary`: the path at which parent pruning stops; empty prunes to the scoped root.
//
// Returns:
//   - `Entry`: always nil — RemoveAll produces no resource.
//   - `*Receipt`: the compensation receipt recording the recovery archive for undo.
//   - `error`: non-nil on archive failure.
func (p *Provider) RemoveAll(
	activationRecord *op.ActivationRecord,
	path string,
	prune bool,
	boundary string,
) (product Entry, receipt *Receipt, err error) {

	abs := p.RuntimeEnvironment().Root.NewPath(path).Abs()

	entry, err := p.discoverEntryAt(abs)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}

	recoveryID, digest, err := p.archiveAndPrune(entry, prune, boundary)
	if err != nil {
		return nil, nil, err
	}

	receipt = NewReceipt(NewReceiptSpec(entry, MutationDeleteFile).WithRecovery(recoveryID, digest))
	p.markEntryGone(entry)

	return nil, receipt, nil
}

// Unlink removes the symlink at `path`, archiving it for compensation.
//
// Takes a path and discharges the delete invariants (step 23, rulings 2 and 3): the link is interned via
// [DiscoverSymbolicLink] (the kind is fixed by Unlink's own semantics), moved to the recovery site, and its
// catalog entry marked [op.Gone] on success. A non-existent target is a no-op. A target that exists but is not a
// symlink is an error. When `prune` is set, now-empty parents up to `boundary` are removed afterward.
//
// Parameters:
//   - `activationRecord`: the dispatch activation (the required floor for compensable actions — step 27).
//   - `path`: the path of the symlink to remove.
//   - `prune`: whether to remove now-empty parent directories up to `boundary`.
//   - `boundary`: the path at which parent pruning stops; empty prunes to the scoped root.
//
// Returns:
//   - `Entry`: always nil — Unlink produces no resource.
//   - `*Receipt`: the compensation receipt recording the recovery archive for undo.
//   - `error`: non-nil when the target exists but is not a symlink, or on stat or archive failure.
func (p *Provider) Unlink(
	activationRecord *op.ActivationRecord,
	path string,
	prune bool,
	boundary string,
) (product Entry, receipt *Receipt, err error) {

	abs := p.RuntimeEnvironment().Root.NewPath(path).Abs()

	info, err := p.lstat(abs)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil // Already gone — no change
	}

	if err != nil {
		return nil, nil, err
	}

	if info.Mode()&os.ModeSymlink == 0 {
		return nil, nil, fmt.Errorf("%s is not a symlink", abs)
	}

	entry, err := DiscoverSymbolicLink(p.RuntimeEnvironment(), abs)
	if err != nil {
		return nil, nil, err
	}

	recoveryID, digest, err := p.archiveAndPrune(entry, prune, boundary)
	if err != nil {
		return nil, nil, err
	}

	receipt = NewReceipt(NewReceiptSpec(entry, MutationDeleteFile).WithRecovery(recoveryID, digest))
	p.markEntryGone(entry)

	return nil, receipt, nil
}

// WalkTree performs a depth-first traversal of `root`, folding each entry through `fn`.
//
// WalkTree is a discovery operation — the walker observes existing filesystem entries; it does not produce them. The
// Resources it interns into the catalog are discovered, not authored, so they carry no `producerID` stamp from this
// method. Gitignored entries are skipped unless `includeGitignored` is set; the `.git` directory is always skipped.
//
// Parameters:
//   - `activationRecord`: the dispatch activation (the required floor for compensable actions — step 27).
//   - `root`: the [*Directory] to traverse — a content read of the tree, so the parameter is the resource
//     (step 23, ruling 2).
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
	activationRecord *op.ActivationRecord,
	root *Directory,
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

		// WalkTree is discovery — found entries pre-existed; no production claim. The walker holds the
		// [fs.DirEntry], so the observed kind is free (step 23, ruling 2's enumerator clause).
		entry, err := p.discoverEntryOfMode(entryAbs, d.Type())
		if err != nil {
			return err
		}

		if err := entry.Resolve(); err != nil {
			return err
		}

		product, err = fn(product, entry, relativePath, stack)
		return err
	}

	if err := p.walkDir(osRoot, absoluteRoot, walkFn); err != nil {
		return nil, stack, err
	}

	return product, stack, nil
}

// CompensateWalkTree unwinds the [op.RecoveryStack] returned by [Provider.WalkTree] in LIFO order.
//
// Parameters:
//   - `activation`: the per-dispatch record; supplies the [*op.RuntimeEnvironment] passed to [op.RecoveryStack.Unwind].
//   - `stack`: the [*op.RecoveryStack] returned by [Provider.WalkTree]; a nil stack is a no-op.
//
// Returns:
//   - `error`: non-nil when unwinding any recorded compensation fails.
func (p *Provider) CompensateWalkTree(activation *op.ActivationRecord, stack *op.RecoveryStack) error {
	if stack == nil {
		return nil
	}
	return stack.Unwind(activation.RuntimeEnvironment)
}

// WriteBytes writes inline byte `content` to a file at `destinationPath` with the given mode and ownership.
//
// `chown` is the Dockerfile-style ownership string (`"user[:group]"`, `":group"`, `"uid[:gid]"`, or empty for "no
// change"). When non-empty it is applied via os.Chown after the file is written. Any existing file is archived for
// compensation before the write.
//
// Parameters:
//   - `activationRecord`: the dispatch activation; its `Unit` stamps the produced [*Regular]'s producerID.
//   - `destinationPath`: the path of the file to write.
//   - `content`: the bytes to write, carried as a string.
//   - `chmod`: the [os.FileMode] applied to the written file.
//   - `chown`: the Dockerfile-style ownership string, or empty for no ownership change.
//
// Returns:
//   - `*Regular`: the written resource.
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
) (product *Regular, receipt *Receipt, err error) {

	product, err = NewRegular(p.RuntimeEnvironment(), activationRecord.CallerID, destinationPath)
	if err != nil {
		return nil, nil, err
	}

	product, receipt, err = p.write(product, strings.NewReader(content), chmod, chown)
	if err != nil {
		return product, receipt, err
	}

	return product, receipt, nil
}

// WriteFile creates or updates the file at `targetPath` by streaming `src` to disk.
//
// Any displaced content is archived for compensation. It is the exported form of the streaming write
// core: bytes flow through [io.Copy] (constant memory, and the
// kernel copy_file_range/sendfile fast path when `src` is an [*os.File]), and any content already at `targetPath`
// is archived to [op.RecoverySite] before the overwrite. Takes a path (step 23, ruling 2) and mints the
// [*Regular] product internally with the activation's producer stamp. WriteFile applies no ownership change
// (callers needing `chown` use [Provider.WriteText] / [Provider.WriteBytes]). The returned [*Receipt] names
// [Provider.CompensateFileMutation] as its undo.
//
// Parameters:
//   - `activationRecord`: the dispatch activation; its `Unit` stamps the produced [*Regular]'s producerID.
//   - `targetPath`: the path of the file to write.
//   - `src`: the byte source, streamed once via [io.Copy] without seeking or re-reading.
//   - `mode`: the [os.FileMode] applied to the written file.
//
// Returns:
//   - `*Regular`: the written resource.
//   - `*Receipt`: the self-describing compensation receipt naming [Provider.CompensateFileMutation] as its undo.
//   - `error`: non-nil on construction, archive, or write failure.
func (p *Provider) WriteFile(
	activationRecord *op.ActivationRecord,
	targetPath string,
	src io.Reader,
	mode os.FileMode,
) (product *Regular, receipt *Receipt, err error) {

	product, err = NewRegular(p.RuntimeEnvironment(), activationRecord.CallerID, targetPath)
	if err != nil {
		return nil, nil, err
	}

	return p.write(product, src, mode, "")
}

// WriteText writes inline text `content` to a file at `destinationPath` with the given mode and ownership.
//
// `chown` is the Dockerfile-style ownership string (`"user[:group]"`, `":group"`, `"uid[:gid]"`, or empty for "no
// change"). When non-empty it is applied via os.Chown after the file is written. Any existing file is archived for
// compensation before the write.
//
// Parameters:
//   - `activationRecord`: the dispatch activation; its `Unit` stamps the produced [*Regular]'s producerID.
//   - `destinationPath`: the path of the file to write.
//   - `content`: the text to write.
//   - `chmod`: the [os.FileMode] applied to the written file.
//   - `chown`: the Dockerfile-style ownership string, or empty for no ownership change.
//
// Returns:
//   - `*Regular`: the written resource.
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
) (product *Regular, receipt *Receipt, err error) {

	product, err = NewRegular(p.RuntimeEnvironment(), activationRecord.CallerID, destinationPath)
	if err != nil {
		return nil, nil, err
	}

	product, receipt, err = p.write(product, strings.NewReader(content), chmod, chown)
	if err != nil {
		return product, receipt, err
	}

	return product, receipt, nil
}

// Fallible actions

// Exists reports whether an entry exists at `path`, examining the link itself (lstat semantics).
//
// A location query takes a path (step 23, ruling 2) — no content is read and no resource is minted. A not-exist
// result is reported as `(false, nil)`, not an error; only a genuine stat failure returns a non-nil error.
//
// Parameters:
//   - `path`: the path to probe.
//
// Returns:
//   - `bool`: true when an entry exists at the path.
//   - `error`: non-nil on any stat failure other than not-exist.
func (p *Provider) Exists(path string) (bool, error) {

	_, err := p.lstat(p.RuntimeEnvironment().Root.NewPath(path).Abs())
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
//   - `[]Entry`: the matching entries, in walk order, each minted as its observed kind.
//   - `error`: non-nil when the pattern escapes the scoped root, or on tracker construction or walk failure.
//
// +devlore:defaults includeGitignored=false
func (p *Provider) Find(pattern string, includeGitignored bool) (product []Entry, err error) {

	scopedRoot := p.Root()

	root, matchPattern := splitFindPattern(pattern)
	if root == "" {
		root = "."
	}

	absoluteRoot, err := resolveFindRoot(scopedRoot, root)
	if err != nil {
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
	walk := p.findWalkFunc(absoluteRoot, matchPattern, tracker, &matches)

	err = p.walkDir(p.RuntimeEnvironment().Root, absoluteRoot, walk)
	if err != nil {
		return nil, fmt.Errorf("find: walk %q: %w", absoluteRoot, err)
	}

	return p.discoverEntries(matches)
}

// resolveFindRoot resolves a find pattern's root against the scoped root, confining the result.
//
// Parameters:
//   - `scopedRoot`: the provider's scoped root.
//   - `root`: the pattern's root segment (absolute or scoped-relative).
//
// Returns:
//   - `string`: the cleaned absolute root.
//   - `error`: non-nil when the root escapes the scoped root.
func resolveFindRoot(scopedRoot, root string) (string, error) {

	var absoluteRoot string
	if filepath.IsAbs(root) {
		absoluteRoot = filepath.Clean(root)
	} else {
		absoluteRoot = filepath.Clean(filepath.Join(scopedRoot, root))
	}

	relativePath, err := filepath.Rel(scopedRoot, absoluteRoot)
	if err != nil || strings.HasPrefix(relativePath, "..") {
		return absoluteRoot, fmt.Errorf("outside scoped root")
	}

	return absoluteRoot, nil
}

// findWalkFunc builds the Find walk callback: gitignore-filtered, directory-skipping, double-star
// matched, accumulating absolute paths into `matches`.
//
// Parameters:
//   - `absoluteRoot`: the walk root.
//   - `matchPattern`: the double-star pattern, root-relative.
//   - `tracker`: the gitignore tracker; nil disables filtering.
//   - `matches`: the accumulator for matched absolute paths.
//
// Returns:
//   - `fs.WalkDirFunc`: the walk callback.
func (p *Provider) findWalkFunc(
	absoluteRoot, matchPattern string, tracker *gitignore.Tracker, matches *[]string,
) fs.WalkDirFunc {

	return func(absolutePath string, dirEntry fs.DirEntry, err error) error {

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

		// The matcher is slash-native (glob patterns are a slash-form language on every
		// platform); the walked path is OS-native and converts at this boundary.
		if matchDoubleStar(matchPattern, filepath.ToSlash(relativePath)) {
			*matches = append(*matches, absolutePath)
		}

		return nil
	}
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
//   - `[]Entry`: the matching entries, each minted as its observed kind.
//   - `error`: non-nil on a malformed pattern.
//
// +devlore:defaults includeGitignored=false
func (p *Provider) Glob(pattern string, includeGitignored bool) ([]Entry, error) {

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	if includeGitignored {
		return p.discoverEntries(matches)
	}

	tracker, err := gitignore.NewTracker(p.Root())
	if err != nil {
		return p.discoverEntries(matches) //nolint:nilerr // graceful degradation
	}

	kept := matches[:0]
	for _, match := range matches {
		if !p.isIgnored(tracker, match) {
			kept = append(kept, match)
		}
	}

	return p.discoverEntries(kept)
}

// IsDir reports whether `path` exists and is a directory, following symlinks (stat semantics).
//
// A location query takes a path (step 23, ruling 2). A not-exist result is reported as `(false, nil)`, not an
// error.
//
// Parameters:
//   - `path`: the path to probe.
//
// Returns:
//   - `bool`: true when the path exists and is a directory.
//   - `error`: non-nil on any stat failure other than not-exist.
func (p *Provider) IsDir(path string) (bool, error) {

	info, err := p.stat(p.RuntimeEnvironment().Root.NewPath(path).Abs())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}

	return info.IsDir(), nil
}

// IsFile reports whether `path` exists and is a regular file, following symlinks (stat semantics).
//
// A location query takes a path (step 23, ruling 2). A not-exist result is reported as `(false, nil)`, not an
// error.
//
// Parameters:
//   - `path`: the path to probe.
//
// Returns:
//   - `bool`: true when the path exists and is a regular file.
//   - `error`: non-nil on any stat failure other than not-exist.
func (p *Provider) IsFile(path string) (bool, error) {

	info, err := p.stat(p.RuntimeEnvironment().Root.NewPath(path).Abs())

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
//   - `resource`: the [Entry] whose current filesystem state to observe — observation minting is
//     resource-coupled (step 23, ruling 2), and any taxonomy variant may be observed.
//
// Returns:
//   - `*Observation`: the constructed observation; never nil on a nil-error return.
//   - `error`: any stat failure other than not-exist.
func (p *Provider) Observe(resource Entry) (*Observation, error) {

	root := p.RuntimeEnvironment().Root
	absPath := root.NewPath(resource.Path().Abs())

	info, err := root.Stat(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return NewObservation(resource, false, 0, 0, time.Time{}, 0, 0), nil
		}
		return nil, fmt.Errorf("file.Provider.Observe: stat %s: %w", resource.Path().Abs(), err)
	}

	inode, device := statIdentity(info)

	return NewObservation(
		resource,
		true,
		info.Size(),
		info.Mode(),
		info.ModTime(),
		inode,
		device,
	), nil
}

// ReadBytes returns the contents of the file `resource` as bytes.
//
// Parameters:
//   - `resource`: the [*Regular] to read — a content read, so the parameter is the resource (step 23, ruling 2).
//
// Returns:
//   - `[]byte`: the file contents.
//   - `error`: non-nil on read failure.
func (p *Provider) ReadBytes(resource *Regular) (product []byte, err error) {

	buffer, err := p.read(resource)
	if err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

// ReadText returns the contents of the file `resource` as text.
//
// Parameters:
//   - `resource`: the [*Regular] to read — a content read, so the parameter is the resource (step 23, ruling 2).
//
// Returns:
//   - `string`: the file contents.
//   - `error`: non-nil on read failure.
func (p *Provider) ReadText(resource *Regular) (product string, err error) {

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
//   - `entry`: the [Entry] to archive and remove.
//   - `prune`: whether to remove now-empty parent directories up to `boundary`.
//   - `boundary`: the path at which parent pruning stops; empty prunes to the scoped root.
//
// Returns:
//   - `string`: the recovery-site identifier for the archived bytes.
//   - `op.Digest`: the pre-archive digest, or the zero value when the bytes could not be hashed.
//   - `error`: non-nil on archive failure.
func (p *Provider) archiveAndPrune(
	entry Entry,
	prune bool,
	boundary string,
) (recoveryID string, digest op.Digest, err error) {

	digest = preArchiveDigest(p.RuntimeEnvironment().Root, entry.Path().Abs())

	recoveryID, err = p.RuntimeEnvironment().RecoverySite.ArchiveFile(entry.Path())
	if err != nil {
		return "", op.Digest{}, err
	}

	p.pruneEmptyParents(entry.Path().Abs(), prune, boundary)
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

	resource, ok := receipt.Resource().(Entry)
	if !ok {
		return fmt.Errorf("compensate write: unexpected resource type %T", receipt.Resource())
	}

	// ALWAYS remove the new file before attempting to restore. RestoreFile uses os.Rename which fails if target exists.
	if err := p.remove(resource.Path().Abs()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	recoveryID := receipt.RecoveryID()
	if recoveryID != "" {
		if err := p.RuntimeEnvironment().RecoverySite.RestoreFile(resource.Path(), recoveryID); err != nil {
			if !errors.Is(err, op.ErrRecoverySourceNotFound) {
				return err
			}
		}
	}

	boundary := receipt.Boundary()
	if boundary == nil {
		return nil
	}

	return p.pruneTowardBoundary(filepath.Dir(resource.Path().Abs()), boundary.Path().Abs())
}

// pruneTowardBoundary removes the now-empty directories from `current` up to (excluding) the
// boundary, stopping at the first non-empty directory.
//
// Parameters:
//   - `current`: the deepest directory to prune.
//   - `boundaryPath`: the boundary's absolute path; never removed.
//
// Returns:
//   - `error`: non-nil when `current` lies outside the boundary or a removal genuinely fails.
func (p *Provider) pruneTowardBoundary(current, boundaryPath string) error {

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
func (p *Provider) discoverEntries(paths []string) (product []Entry, err error) {

	entries := make([]Entry, len(paths))

	for i, path := range paths {
		// Enumeration discovery — the disk was just walked, so the observed kind is authoritative; no
		// production claim.
		entry, derr := p.discoverEntryAt(p.RuntimeEnvironment().Root.NewPath(path).Abs())
		if derr != nil {
			return nil, derr
		}
		entries[i] = entry
	}

	return entries, nil
}

// discoverEntryAt mints the observed-kind [Entry] for the existing entry at `abs` without claiming production.
//
// Parameters:
//   - `abs`: the absolute path of the existing entry.
//
// Returns:
//   - `Entry`: the discovered entry, minted as its lstat-observed kind.
//   - `error`: the lstat failure (including not-exist), an unsupported entry kind, or a discovery failure.
func (p *Provider) discoverEntryAt(abs string) (Entry, error) {

	info, err := p.lstat(abs)
	if err != nil {
		return nil, err
	}

	return p.discoverEntryOfMode(abs, info.Mode())
}

// discoverEntryOfMode mints the [Entry] for `abs` from an already-observed mode, without claiming production.
//
// The enumerator trunk (step 23, ruling 2): walkers and stat-holding callers already know the kind, so the
// matching Discover constructor is chosen directly. An entry of any other kind (FIFO, socket, device) is an
// error — the taxonomy has no variant for kinds no action produces or consumes (ruling 1).
//
// Parameters:
//   - `abs`: the absolute path of the entry.
//   - `mode`: the observed [os.FileMode] (full mode or type bits).
//
// Returns:
//   - `Entry`: the discovered entry as its observed kind.
//   - `error`: an unsupported entry kind, or a discovery failure.
func (p *Provider) discoverEntryOfMode(abs string, mode os.FileMode) (Entry, error) {

	runtimeEnvironment := p.RuntimeEnvironment()

	switch {
	case mode&os.ModeSymlink != 0:
		return DiscoverSymbolicLink(runtimeEnvironment, abs)
	case mode.IsDir():
		return DiscoverDirectory(runtimeEnvironment, abs)
	case mode.IsRegular():
		return DiscoverRegular(runtimeEnvironment, abs)
	default:
		return nil, fmt.Errorf("file: %s: unsupported entry kind %s (no taxonomy variant)", abs, mode)
	}
}

// findClosestExistingDir walks up from `path` to the nearest existing entry under the scoped [Provider.Root].
//
// Parameters:
//   - `path`: the absolute path whose nearest existing ancestor is sought.
//
// Returns:
//   - `Entry`: the discovered ancestor, minted as its observed kind.
//   - `os.FileInfo`: the stat info for the discovered ancestor.
//   - `error`: non-nil when `path` lies outside the scoped root or the root itself is inaccessible.
func (p *Provider) findClosestExistingDir(path string) (ancestor Entry, info os.FileInfo, err error) {

	root := p.Root()

	rel, relErr := filepath.Rel(root, path)
	if relErr != nil || strings.HasPrefix(rel, "..") {
		return nil, nil, fmt.Errorf("%s lies outside scoped root %s", path, root)
	}

	current := path

	for {
		if info, err = p.stat(current); err == nil {
			// Discovery — walking up the parent chain to find an existing entry; the stat is in hand, so the
			// observed kind is free.
			a, derr := p.discoverEntryOfMode(current, info.Mode())
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

// markEntryGone records a successful deletion on the catalog when one is present and the entry was interned.
//
// The delete trio's ruling-3 tail: nil-catalog environments (test fixtures, library callers) and unlinked
// candidates are tolerated silently — there is no ledger to update.
//
// Parameters:
//   - `entry`: the deleted [Entry].
func (p *Provider) markEntryGone(entry Entry) {

	if catalog := p.RuntimeEnvironment().ResourceCatalog; catalog != nil && entry.ID() != "" {
		catalog.MarkGone(entry)
	}
}

// produceEntryAt mints the production-claimed [Entry] at `path` for an already-observed source mode.
//
// [Provider.Move]'s product trunk: the destination's kind is the moved entry's observed kind (the mutator is at
// execution time with the disk in hand), and the claim is stamped with `producerID`. An entry of any other kind is an
// error, mirroring [Provider.discoverEntryOfMode].
//
// Parameters:
//   - `producerID`: the producing caller's id, or "" for caller-less dispatch.
//   - `path`: the destination path to claim.
//   - `mode`: the source entry's observed [os.FileMode].
//
// Returns:
//   - `Entry`: the production-claimed entry as the observed kind.
//   - `error`: an unsupported entry kind, or a construction failure.
func (p *Provider) produceEntryAt(producerID, path string, mode os.FileMode) (Entry, error) {

	runtimeEnvironment := p.RuntimeEnvironment()

	switch {
	case mode&os.ModeSymlink != 0:
		return NewSymbolicLink(runtimeEnvironment, producerID, path)
	case mode.IsDir():
		return NewDirectory(runtimeEnvironment, producerID, path)
	case mode.IsRegular():
		return NewRegular(runtimeEnvironment, producerID, path)
	default:
		return nil, fmt.Errorf("file: %s: unsupported entry kind %s (no taxonomy variant)", path, mode)
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

// errConflictSkip signals that the write-seam conflict policy elected to leave an occupied target untouched.
//
// Callers translate it to the no-op success shape (nil product, nil receipt, nil error), mirroring
// [Provider.Remove]'s already-gone behavior.
var errConflictSkip = errors.New("conflict policy skip: occupied target left untouched")

// conflictPolicy returns the write-seam conflict policy for this run (phase-8 step 49).
//
// Interim channel (the dry-run precedent): the application flag map carries the typed value
// (`Flags["conflict"]`, an [op.ConflictPolicy]) until the config loader delivers the cli source; absent, the
// announced runtime section's floor applies ([op.ConflictStop]).
//
// Returns:
//   - `op.ConflictPolicy`: the policy governing occupied write targets in this run.
func (p *Provider) conflictPolicy() op.ConflictPolicy {

	if app := p.RuntimeEnvironment().Application; app != nil {
		if value, ok := app.Flags["conflict"].(op.ConflictPolicy); ok {
			return value
		}
	}
	return op.NewRuntimeEnvironmentConfig().ConflictPolicy
}

// stageWrite prepares the disk for writing `product` and returns the receipt spec bound to it.
//
// The write-seam trunk shared by the creating mutators (step 23: the caller mints the typed product first; the
// stage binds the receipt to that canonical entry). When nothing occupies the target, the parent chain is created
// and the creation boundary recorded on the spec. When the target is occupied, the write-seam conflict policy
// governs (phase-8 step 49): replace archives the occupant (compensation restores from the receipt's pre-archive
// digest); skip surfaces [errConflictSkip]; stop refuses.
//
// Parameters:
//   - `product`: the minted, catalog-interned product the write will produce.
//
// Returns:
//   - `*ReceiptSpec`: the spec recording the creation boundary or the occupant's recovery archive.
//   - `error`: [errConflictSkip] under the skip policy, or any stat, boundary, mkdir, or archive failure.
func (p *Provider) stageWrite(product Entry) (spec *ReceiptSpec, err error) {

	abs := product.Path().Abs()

	if _, statErr := p.lstat(abs); errors.Is(statErr, os.ErrNotExist) {

		parentPath := filepath.Dir(abs)

		boundary, _, err := p.findClosestExistingDir(parentPath)
		if err != nil {
			return nil, err
		}

		spec = NewReceiptSpec(product, MutationCreateFile).WithBoundary(boundary)

		if err := p.mkdirAll(parentPath, 0o750); err != nil {
			return spec, err
		}

		return spec, nil
	} else if statErr != nil {
		return nil, statErr
	}

	// The target is occupied — the write-seam conflict policy governs (phase-8 step 49).
	switch p.conflictPolicy() {
	case op.ConflictStop:
		return nil, fmt.Errorf(
			"target %s is occupied and the conflict policy is stop (replace archives and overwrites; skip leaves it)",
			abs)
	case op.ConflictSkip:
		return nil, errConflictSkip
	case op.ConflictReplace:
	}

	// Reject a non-empty directory, as Remove does; archive the occupant for the overwrite (update).
	nonEmptyDirectory, err := p.isDirAndNotEmpty(abs)
	if err != nil {
		return nil, err
	}
	if nonEmptyDirectory {
		return nil, fmt.Errorf("cannot overwrite non-empty directory %s", abs)
	}

	recoveryID, digest, err := p.archiveAndPrune(product, false, "")
	if err != nil {
		return nil, fmt.Errorf("failed to backup existing file: %w", err)
	}

	spec = NewReceiptSpec(product, MutationUpdateFile).WithRecovery(recoveryID, digest)

	return spec, nil
}

// read returns the contents of the file `resource` as an in-memory buffer.
//
// Parameters:
//   - `resource`: the [*Regular] to read.
//
// Returns:
//   - `*bytes.Buffer`: a buffer over the file contents.
//   - `error`: non-nil on read failure.
func (p *Provider) read(resource *Regular) (*bytes.Buffer, error) {

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
//
// rawReadLink returns the symlink target at `abs` exactly as stored — no absolutization, no cleaning.
//
// The verbatim counterpart of [Provider.readLink]: [Provider.Link]'s already-correct comparison matches the
// stored name against what the link actually contains, which for a verbatim link is the literal archived string.
//
// Parameters:
//   - `abs`: the absolute path of the symlink.
//
// Returns:
//   - `string`: the raw readlink result.
//   - `error`: non-nil on readlink failure.
func (p *Provider) rawReadLink(abs string) (string, error) {
	root := p.RuntimeEnvironment().Root
	return root.Readlink(root.NewPath(abs))
}

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

// symlinkRaw creates a symbolic link at `linkAbs` whose stored content is `target` exactly as given.
//
// The verbatim counterpart of [Provider.symlink], which relativizes: extraction fidelity (archive §10 ruling 1a)
// stores the archived target uninterpreted.
//
// Parameters:
//   - `target`: the literal link content.
//   - `linkAbs`: the absolute path at which the symlink is created.
//
// Returns:
//   - `error`: non-nil on symlink failure.
func (p *Provider) symlinkRaw(target, linkAbs string) error {
	root := p.RuntimeEnvironment().Root
	return root.Symlink(target, root.NewPath(linkAbs))
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

// write streams `src` to the staged target with the given mode and ownership.
//
// The content is copied with [io.Copy], so it streams in constant memory and engages the kernel
// copy_file_range/sendfile fast path when `src` is an [*os.File]; no full-content buffer is materialized. `chown`
// follows the same Dockerfile-style format as the public Write* methods; empty means no ownership change. The chown is
// applied after the file is fully written and synced — placing it before the close would risk applying ownership to a
// file the kernel may yet reject.
//
// Parameters:
//   - `target`: the minted [*Regular] to write.
//   - `src`: the byte source streamed to the file.
//   - `chmod`: the [os.FileMode] applied to the written file.
//   - `chown`: the Dockerfile-style ownership string, or empty for no ownership change.
//
// Returns:
//   - `*Regular`: the written resource (`target`).
//   - `*Receipt`: the compensation receipt for undo.
//   - `error`: non-nil on write preparation, open, write, sync, or chown failure.
func (p *Provider) write(
	target *Regular,
	src io.Reader,
	chmod os.FileMode,
	chown string,
) (product *Regular, receipt *Receipt, err error) {

	product = target

	spec, err := p.stageWrite(product)
	if errors.Is(err, errConflictSkip) {
		return nil, nil, nil // Occupied target left untouched per the skip policy.
	}
	if err != nil {
		return nil, nil, err
	}
	receipt = NewReceipt(spec)

	f, err := p.openFile(product.SourcePath.Abs(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, chmod)
	if err != nil {
		return product, receipt, err
	}
	defer iox.Close(&err, f)

	if _, err = io.Copy(f, src); err != nil {
		return product, receipt, err
	}

	if err := f.Sync(); err != nil {
		return product, receipt, err
	}

	if err := applyChown(product.SourcePath.Abs(), chown); err != nil {
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

// pruneEmptyParents removes now-empty parent directories of `path`, stopping at `boundary`.
//
// Pruning is a no-op when `prune` is false. The boundary defaults to the scoped [Provider.Root] when `boundary` is
// empty. Removal stops at the first non-empty directory (a failed remove ends the walk silently).
//
// Parameters:
//   - `path`: the absolute path whose parent chain is pruned.
//   - `prune`: whether pruning runs at all.
//   - `boundary`: the path at which pruning stops; empty stops at the scoped root.
func (p *Provider) pruneEmptyParents(path string, prune bool, boundary string) {

	if !prune {
		return
	}

	boundaryPath := p.Root()

	if boundary != "" {
		boundaryPath = p.RuntimeEnvironment().Root.NewPath(boundary).Abs()
	}

	dir := filepath.Dir(path)

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
//   - `entry`: the [Entry] for the current filesystem entry, minted as its observed kind.
//   - `relativePath`: the entry's path relative to the walk root.
//   - `stack`: the [*op.RecoveryStack] for recording compensation actions.
//
// Returns:
//   - `any`: the updated accumulator, threaded into the next invocation.
//   - `error`: non-nil to abort the traversal.
type Reducer func(initial any, entry Entry, relativePath string, stack *op.RecoveryStack) (result any, err error)

// endregion
