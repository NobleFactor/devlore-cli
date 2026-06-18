// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package archive provides archive extraction actions for the operation graph.
package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
)

var _ op.Provider = (*Provider)(nil) // Interface Guard

// Provider provides archive extraction actions.
//
// +devlore:access=planned
type Provider struct {
	op.ProviderBase
}

// NewProvider creates an archive Provider bound to the given runtime environment.
//
// Parameters:
//   - `runtimeEnvironment`: the session runtime environment used as the provider's [op.ProviderBase] handle.
//
// Returns:
//   - `*Provider`: the constructed provider ready for plan-time invocation.
func NewProvider(runtimeEnvironment *op.RuntimeEnvironment) *Provider {

	return &Provider{ProviderBase: op.NewProviderBase(runtimeEnvironment)}
}

// region EXPORTED METHODS

// region Behaviors

// Compensable actions

// Extract extracts an archive (tar.gz or zip) from `source` into the directory at `prefixPath`.
//
// The prefix directory must already exist as a directory — Extract does not create it; callers are responsible for
// arranging the prefix (e.g., via plan.file.mkdir upstream). This mirrors the semantics of the tar(1) -C flag, which
// fails if the target directory is missing. Extract returns an error when `prefixPath` does not exist or exists but is
// not a directory. The archive format is detected from the source file's extension.
//
// Per file.Provider's recovery model, target files that already exist at extraction time are archived to
// [op.RecoverySite] before being overwritten; brand-new targets just get written. Each extracted file is represented
// by a [file.Resource] interned through the [op.ResourceCatalog] and a corresponding [file.Receipt] whose boundary is
// the destination directory — compensation removes the new file, restores the archived prior content if any, and walks
// the boundary chain cleaning up directories that the extraction created.
//
// Parameters:
//   - `activationRecord`: the per-dispatch activation; its `Unit` is recorded as the producer of every interned
//     [file.Resource] so the catalog routes compensation back through this provider.
//   - `source`: [file.Resource] identifying the archive file (tar.gz, tgz, or zip); the path is read at dispatch time.
//   - `prefixPath`: the extraction directory path. Must exist as a directory; Extract does not create it.
//
// Returns:
//   - `[]*file.Resource`: one entry per file the extraction created or replaced; each is the canonical catalog entry
//     for its URI, in extraction order.
//   - `[]op.Receipt`: one [file.Receipt] per extracted file, in extraction order. Compensation runs them in reverse via
//     [file.Provider.compensateWrite] (see [Method.Invoke]'s sub-stack wrapping).
//   - `error`: any error from format detection, extraction, archive-on-displace, or catalog/receipt construction.
func (p *Provider) Extract(
	activationRecord *op.ActivationRecord,
	source *file.Resource,
	prefixPath string,
) ([]*file.Resource, []op.Receipt, error) {

	runtimeEnvironment := p.RuntimeEnvironment()

	// destination is discovery — the prefix directory must already exist (we error below if not), so archive isn't
	// producing it. DiscoverResource registers without claiming production.
	destination, err := file.DiscoverResource(runtimeEnvironment, prefixPath)
	if err != nil {
		return nil, nil, err
	}

	if err := destination.Resolve(); err != nil {
		return nil, nil, err
	}

	if !destination.Exists() {
		return nil, nil, fmt.Errorf("prefix directory does not exist: %s", prefixPath)
	}

	if !destination.IsDir() {
		return nil, nil, fmt.Errorf("prefix path is not a directory: %s", prefixPath)
	}

	var entries []extractedEntry

	lower := strings.ToLower(source.SourcePath.Abs())
	switch {
	case strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
		entries, err = p.extractTarGz(source.SourcePath.Abs(), destination.SourcePath.Abs())
	case strings.HasSuffix(lower, ".zip"):
		entries, err = p.extractZip(source.SourcePath.Abs(), destination.SourcePath.Abs())
	default:
		return nil, nil, fmt.Errorf("unsupported archive format: %s", source.SourcePath.Abs())
	}

	if err != nil {
		return nil, nil, err
	}

	products := make([]*file.Resource, 0, len(entries))
	receipts := make([]op.Receipt, 0, len(entries))

	for _, entry := range entries {

		// archive is producing each extracted file. file.NewResource(runtimeEnvironment, unit, ...) interns + stamps in
		// one call — replaces the earlier NewResource + Catalog.GetOrCreate pattern.
		product, err := file.NewResource(runtimeEnvironment, activationRecord.Unit, entry.Path)
		if err != nil {
			return products, receipts, fmt.Errorf("archive: catalog %q: %w", entry.Path, err)
		}

		if err := product.Resolve(); err != nil {
			return products, receipts, fmt.Errorf("archive: resolve %q: %w", entry.Path, err)
		}

		// TODO(#277): thread entry.PriorArchiveID into the receipt's TransactionID so the executor's compensation path
		// can locate the archived prior content via [op.RecoverySite.RestoreFile]. Today the receipt's TransactionID is
		// minted independently at Commit time, so PriorArchiveID is recorded but unused on the restore path until #277.
		_ = entry.PriorArchiveID

		products = append(products, product)
		receipts = append(receipts, file.NewReceiptWithBoundary(product, destination))
	}

	return products, receipts, nil
}

// CompensateExtract undoes one extracted file's effect on disk.
//
// Removes the file, restores any prior content archived to [op.RecoverySite] at extraction time, and walks the file's
// [file.Receipt.Boundary] chain removing empty directories that the extraction created.
//
// Each [file.Receipt] produced by [Provider.Extract] is dispatched to this method by the executor at unwind time. The
// classifier requires this companion even though every receipt is a [file.Receipt]: the receipts were committed under
// archive.Provider.Extract's action name, so the registry routes their compensation here. Implementation delegates to
// [file.Provider.CompensateWriteText], which is itself a thin wrapper over the canonical file.compensateWrite logic —
// so archive's compensation contract is identical to file's.
//
// Parameters:
//   - `receipt`: the [file.Receipt] for one extracted file; nil receipts return nil without dispatching to the
//     underlying file provider.
//
// Returns:
//   - `error`: any error from file removal, [op.RecoverySite] archive restore, or the boundary-chain directory walk.
func (p *Provider) CompensateExtract(receipt *file.Receipt) error {

	if receipt == nil {
		return nil
	}

	fileProvider, err := provider.Instance[file.Provider](p.RuntimeEnvironment())
	if err != nil {
		return err
	}

	return fileProvider.CompensateWriteText(receipt)
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// extractTarGz reads a gzipped tar archive at `source` and writes its file entries under `prefix`.
//
// For each file entry that displaces existing content, the prior content is archived via [op.RecoverySite.ArchiveFile]
// (through the receiver's [op.RuntimeEnvironment]) before the new content is written; the returned recovery ID rides
// on the entry record so the caller can thread it onto the resulting [file.Receipt]. Directory entries are ensured to
// exist (Mkdir if missing) but don't produce entries — they're part of the file's boundary chain and are cleaned up by
// compensation's boundary walk.
//
// Parameters:
//   - `source`: absolute path to the tar.gz archive on disk; opened read-only, decompressed via [gzip.NewReader].
//   - `prefix`: absolute path to the destination directory (must exist); used as the join base for every entry path.
//
// Returns:
//   - `[]extractedEntry`: one record per file written, in extraction order; directory-only entries do not appear here.
//   - `error`: any read, write, or archive failure encountered during the walk; partial entries are still returned.
func (p *Provider) extractTarGz(source, prefix string) (entries []extractedEntry, err error) {

	f, err := os.Open(source)
	if err != nil {
		return nil, err
	}
	defer iox.Close(&err, f)

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	defer func() {
		if closeErr := gz.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("gzip close: %w", closeErr)
		}
	}()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return entries, fmt.Errorf("tar: %w", err)
		}

		target := filepath.Join(prefix, filepath.Clean(hdr.Name))
		if !strings.HasPrefix(target, filepath.Clean(prefix)+string(os.PathSeparator)) {
			continue // skip entries that escape the prefix (zip slip protection)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode&0o777)); err != nil {
				return entries, err
			}
		case tar.TypeReg:
			entry, err := p.writeExtractedFile(target, io.LimitReader(tr, 1<<30), os.FileMode(hdr.Mode&0o777))
			if err != nil {
				return entries, err
			}
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

// extractZip reads a zip archive at `source` and writes its file entries under `prefix`.
//
// Same archive-on-displace semantics as [Provider.extractTarGz]: the receiver's [op.RuntimeEnvironment] supplies
// [op.RecoverySite] for the displacement archive step, and entries that escape the prefix via traversal sequences are
// skipped (zip-slip protection).
//
// Parameters:
//   - `source`: absolute path to the zip archive on disk; opened read-only via [zip.OpenReader].
//   - `prefix`: absolute path to the destination directory (must exist); used as the join base for every entry path.
//
// Returns:
//   - `[]extractedEntry`: one record per file written, in extraction order; directory-only entries do not appear here.
//   - `error`: any read, write, or archive failure encountered during the walk; partial entries are still returned.
func (p *Provider) extractZip(source, prefix string) (entries []extractedEntry, err error) {

	r, err := zip.OpenReader(source)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := r.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("zip close: %w", closeErr)
		}
	}()

	for _, f := range r.File {
		target := filepath.Join(prefix, filepath.Clean(f.Name))
		if !strings.HasPrefix(target, filepath.Clean(prefix)+string(os.PathSeparator)) {
			continue // zip slip protection
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, f.Mode()); err != nil {
				return entries, err
			}
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return entries, err
		}

		entry, writeErr := p.writeExtractedFile(target, io.LimitReader(rc, 1<<30), f.Mode())
		closeErr := rc.Close()

		if writeErr != nil {
			return entries, errors.Join(writeErr, closeErr)
		}
		if closeErr != nil {
			return entries, closeErr
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// writeExtractedFile writes one extracted file's content to `target` with archive-on-displace semantics.
//
// When `target` already exists, the prior content is archived via [op.RecoverySite.ArchiveFile] (through the receiver's
// [op.RuntimeEnvironment]) first; the returned recovery ID rides on the returned [extractedEntry] so the caller can
// thread it onto a [file.Receipt]. Parent directories are created on demand via [os.MkdirAll]. The archive-on-displace
// step mirrors [file.Provider.prepareWrite]'s behavior so compensation can restore the prior content via
// [op.RecoverySite.RestoreFile] keyed by the returned recovery ID.
//
// Parameters:
//   - `target`: absolute path of the file to write; parent directories are created if missing.
//   - `content`: the source reader; consumed once via [io.Copy] without seeking or re-reading.
//   - `mode`: the file mode applied to the newly created file (existing mode bits at `target` are discarded).
//
// Returns:
//   - `extractedEntry`: the path written and the recovery ID (empty when no prior content was archived).
//   - `error`: any stat, archive, mkdir, open, copy, or close failure encountered during the write operation.
func (p *Provider) writeExtractedFile(target string, content io.Reader, mode os.FileMode) (extractedEntry, error) {

	var priorArchiveID string

	if _, err := os.Lstat(target); err == nil {
		runtimeEnvironment := p.RuntimeEnvironment()
		recID, archiveErr := runtimeEnvironment.RecoverySite.ArchiveFile(runtimeEnvironment.Root.NewPath(target))
		if archiveErr != nil {
			return extractedEntry{}, fmt.Errorf("archive prior content at %q: %w", target, archiveErr)
		}
		priorArchiveID = recID
	} else if !os.IsNotExist(err) {
		return extractedEntry{}, fmt.Errorf("stat %q: %w", target, err)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return extractedEntry{}, err
	}

	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return extractedEntry{}, err
	}

	if _, copyErr := io.Copy(out, content); copyErr != nil {
		return extractedEntry{}, errors.Join(copyErr, out.Close())
	}

	if err := out.Close(); err != nil {
		return extractedEntry{}, err
	}

	return extractedEntry{Path: target, PriorArchiveID: priorArchiveID}, nil
}

// endregion

// endregion

// region SUPPORTING TYPES

// extractedEntry records one file produced by an archive extraction.
//
// PriorArchiveID is non-empty when the target path was occupied by existing content archived to [op.RecoverySite]
// before the new content was written; empty when the target was new.
type extractedEntry struct {

	// Path is the absolute path of the extracted file on disk; the entry corresponds to one regular file written by
	// [Provider.extractTarGz] or [Provider.extractZip] (directory-only tar/zip entries do not produce extractedEntry
	// records).
	Path string

	// PriorArchiveID holds the [op.RecoverySite] archive ID for prior content displaced at Path. Empty when the target
	// was new and no displacement archive was needed; non-empty IDs are threaded onto the resulting [file.Receipt]
	// (post-#277) so compensation can restore the prior bytes.
	PriorArchiveID string
}

// endregion
