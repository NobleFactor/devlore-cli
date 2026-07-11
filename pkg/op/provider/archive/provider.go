// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package archive provides archive extraction actions for the operation graph.
package archive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
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

// maxEntryBytes caps a single extracted entry at 1 GiB, bounding decompression-bomb exposure on the read path.
const maxEntryBytes = 1 << 30

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

// Extract extracts an archive from `source` into the directory at `prefixPath`.
//
// The prefix directory must already exist as a directory — Extract does not create it; callers are responsible for
// arranging the prefix (e.g., via plan.file.mkdir upstream). This mirrors the semantics of the tar(1) -C flag, which
// fails if the target directory is missing. Extract returns an error when `prefixPath` does not exist or exists but is
// not a directory. The archive format is detected from the file's leading bytes — its compression or container magic —
// never from its name: gzip-compressed tar, plain (ustar) tar, and zip extract today, while the bzip2, xz, and zstd
// magics are recognized but rejected until their decompressors land.
//
// Each entry is materialized through the file provider's unified mutation surface — a directory via
// [file.Provider.Mkdir] and a regular file via [file.Provider.WriteFile], which streams the body with [io.Copy]
// (constant memory) and archives any displaced prior content to [op.RecoverySite]. Every call yields a self-describing
// [file.Receipt] that names [file.Provider.CompensateFileMutation] as its undo; Extract commits each receipt and
// pushes it onto a single [op.RecoveryStack], so a failure mid-extraction returns the partial stack and the saga
// boundary unwinds it before any retry. Compensation removes created files and directories and restores displaced
// content from recovery.
//
// Parameters:
//   - `activationRecord`: the per-dispatch activation; its `Unit` stamps the producer of every interned
//     [file.Resource] and the `forwardAction` of every receipt.
//   - `source`: [file.Resource] identifying the archive file; the format is read from its content at dispatch time.
//   - `prefixPath`: the extraction directory path. Must exist as a directory; Extract does not create it.
//
// Returns:
//   - `[]*file.Resource`: one entry per file the extraction created or replaced, in extraction order.
//   - `*op.RecoveryStack`: a recovery stack carrying one self-describing [file.Receipt] per created file or directory,
//     in extraction order, so a failed run unwinds it in reverse.
//   - `error`: any error from format detection, extraction, archive-on-displace, or catalog/receipt construction.
func (p *Provider) Extract(
	activationRecord *op.ActivationRecord,
	source *file.Resource,
	prefixPath string,
) (products []*file.Resource, stack *op.RecoveryStack, err error) {

	runtimeEnvironment := activationRecord.RuntimeEnvironment
	stack = op.NewRecoveryStack()

	fileProvider, err := provider.Instance[file.Provider](runtimeEnvironment)
	if err != nil {
		return nil, nil, err
	}

	// Destination is discovery — the prefix directory must already exist (we error below if not), so archive isn't
	// producing it. DiscoverResource registers without claiming production.

	destination, err := file.DiscoverResource(runtimeEnvironment, prefixPath)
	if err != nil {
		return nil, nil, err
	}

	if err = destination.Resolve(); err != nil {
		return nil, nil, err
	}

	if !destination.Exists() {
		return nil, nil, fmt.Errorf("prefix directory does not exist: %s", prefixPath)
	}

	if !destination.IsDir() {
		return nil, nil, fmt.Errorf("prefix path is not a directory: %s", prefixPath)
	}

	reader, err := p.openArchive(source.SourcePath.Abs())
	if err != nil {
		return nil, nil, err
	}
	defer iox.Close(&err, reader)

	prefix := destination.SourcePath.Abs()
	guard := filepath.Clean(prefix) + string(os.PathSeparator)

	for {
		entry, readErr := reader.Next()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return products, stack, fmt.Errorf("archive: read: %w", readErr)
		}

		target := filepath.Join(prefix, filepath.Clean(entry.Name))
		if !strings.HasPrefix(target, guard) {
			continue // skip entries that escape the prefix (zip-slip protection)
		}

		var (
			product *file.Resource
			receipt *file.Receipt
		)

		if entry.IsDir {
			if product, receipt, err = fileProvider.Mkdir(activationRecord, target, entry.Mode, ""); err != nil {
				return products, stack, fmt.Errorf("archive: mkdir %q: %w", target, err)
			}
		} else {
			if product, err = file.NewResource(runtimeEnvironment, activationRecord.Unit, target); err != nil {
				return products, stack, fmt.Errorf("archive: catalog %q: %w", target, err)
			}
			if _, receipt, err = fileProvider.WriteFile(product, entry.Reader, entry.Mode); err != nil {
				return products, stack, fmt.Errorf("archive: write %q: %w", target, err)
			}
			products = append(products, product)
		}

		// The receipt is its own compensator: commit it so it is compensable (an uncommitted receipt has no compensator
		// and Unwind walks past it). forwardAction is stamped archive.extract; compensatingAction stays the file
		// compensator the receipt's constructor named, so Unwind routes it to file.CompensateFileMutation.
		if err = receipt.Commit(activationRecord.Unit, product, receipt, nil); err != nil {
			return products, stack, fmt.Errorf("archive: commit receipt %q: %w", target, err)
		}

		stack.Push(receipt)
	}

	return products, stack, nil
}

// CompensateExtract undoes a [Provider.Extract] by unwinding its recovery stack.
//
// Extract returns a [op.RecoveryStack] holding one self-describing [file.Receipt] per created file or directory.
// Unwinding it compensates each in reverse order — removing created files and directories and restoring any prior
// content archived to [op.RecoverySite] — so the filesystem returns to its pre-extraction state.
//
// Parameters:
//   - `activation`: the per-dispatch record; supplies the [*op.RuntimeEnvironment] passed to [op.RecoveryStack.Unwind].
//   - `stack`: the recovery stack [Provider.Extract] returned as its compensator; a nil stack returns nil.
//
// Returns:
//   - `error`: the joined errors from the per-entry compensations, or nil when all succeed.
func (p *Provider) CompensateExtract(activation *op.ActivationRecord, stack *op.RecoveryStack) error {

	if stack == nil {
		return nil
	}
	return stack.Unwind(activation.RuntimeEnvironment)
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// openArchive opens the archive at `source`, detecting its format from the file's leading bytes.
//
// Detection sniffs up to `headerSniffLen` bytes and matches compression and container magic numbers — never the file
// name. A gzip match wraps the rewound file in a gzip-decompressed tar stream; a ustar magic at offset 257 selects the
// plain-tar (identity) path; a zip match reopens the path via [zip.OpenReader] (zip needs random access to its central
// directory). The bzip2, xz, and zstd magics are recognized but their decompressors are later slices, so they return a
// named unsupported error. Pre-POSIX V7 tar carries no magic, is not content-detectable, and resolves to unsupported.
// The returned [archiveReader] yields entries in storage order and must be closed by the caller.
//
// Parameters:
//   - `source`: absolute path to the archive file on disk.
//
// Returns:
//   - `archiveReader`: an entry iterator over the archive; the caller closes it.
//   - `error`: an unsupported or undetectable format, or any open/sniff/decompress failure.
func (p *Provider) openArchive(source string) (archiveReader, error) {

	archiveFile, err := os.Open(source)
	if err != nil {
		return nil, err
	}

	format, err := detectFormat(archiveFile)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("archive: detect %q: %w", source, err), archiveFile.Close())
	}

	switch format {
	case formatGzip:
		return newTarGzArchiveReader(archiveFile)
	case formatTar:
		return newTarArchiveReader(archiveFile), nil
	case formatZip:
		if err := archiveFile.Close(); err != nil {
			return nil, err
		}
		return newZipArchiveReader(source)
	case formatBzip2, formatXz, formatZstd:
		return nil, errors.Join(
			fmt.Errorf("archive: %s compression detected but not yet supported: %s", format, source),
			archiveFile.Close(),
		)
	default:
		return nil, errors.Join(fmt.Errorf("unsupported archive format: %s", source), archiveFile.Close())
	}
}

// endregion

// endregion

// region SUPPORTING TYPES

// archiveEntry is one entry yielded by an [archiveReader]: a directory, or a regular file with its body reader.
type archiveEntry struct {

	// Name is the entry's path as stored in the archive (joined against the extraction prefix by the caller).
	Name string

	// Mode is the entry's permission bits.
	Mode os.FileMode

	// IsDir is true for a directory entry, in which case Reader is nil.
	IsDir bool

	// Reader is the file body, valid only until the next [archiveReader.Next] call; nil for a directory.
	Reader io.Reader
}

// archiveFormat identifies the outer layer detected from an archive's leading bytes: a compression wrapper, the zip
// container, plain (ustar) tar, or unknown.
type archiveFormat int

// The outer formats detection can name. formatUnknown is the zero value — no known magic matched, which includes
// pre-POSIX V7 tar (it carries no magic and is therefore not content-detectable).
const (
	formatUnknown archiveFormat = iota
	formatGzip
	formatBzip2
	formatXz
	formatZstd
	formatZip
	formatTar
)

// headerSniffLen is the longest leading-byte prefix detection reads: the ustar magic ends at byte 262 (offset 257 + 5).
const headerSniffLen = 262

// tarMagicOffset is the offset of the "ustar" magic within a POSIX/GNU tar header.
const tarMagicOffset = 257

// magicTable maps leading-byte signatures to outer formats (§3 of docs/architecture/3.5.1-archive-provider.md);
// entries are checked in order and the first match wins — every compression and zip magic sits at offset 0, the ustar
// probe at offset 257.
var magicTable = []struct {
	format archiveFormat
	offset int
	magic  []byte
}{
	{formatGzip, 0, []byte{0x1F, 0x8B}},
	{formatBzip2, 0, []byte("BZh")},
	{formatXz, 0, []byte{0xFD, 0x37, 0x7A, 0x58, 0x5A, 0x00}},
	{formatZstd, 0, []byte{0x28, 0xB5, 0x2F, 0xFD}},
	{formatZip, 0, []byte{0x50, 0x4B, 0x03, 0x04}},
	{formatZip, 0, []byte{0x50, 0x4B, 0x05, 0x06}}, // empty archive
	{formatZip, 0, []byte{0x50, 0x4B, 0x07, 0x08}}, // spanned archive
	{formatTar, tarMagicOffset, []byte("ustar")},
}

// detectFormat identifies the outer format of the archive open on `archiveFile`, leaving the file rewound.
//
// It reads up to `headerSniffLen` leading bytes with [io.ReadFull], tolerating [io.EOF] and [io.ErrUnexpectedEOF] so a
// file shorter than the ustar probe can still match the compression and zip magics (which need only the first 2–6
// bytes), then seeks back to byte zero so the selected reader consumes the stream from the start.
//
// Parameters:
//   - `archiveFile`: the open archive, positioned at byte zero.
//
// Returns:
//   - `archiveFormat`: the first matching entry in `magicTable`, or `formatUnknown` when no magic matches.
//   - `error`: any non-EOF read failure, or the rewinding seek failure.
func detectFormat(archiveFile *os.File) (archiveFormat, error) {

	header := make([]byte, headerSniffLen)
	n, err := io.ReadFull(archiveFile, header)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return formatUnknown, err
	}
	header = header[:n]

	if _, err := archiveFile.Seek(0, io.SeekStart); err != nil {
		return formatUnknown, err
	}

	for _, candidate := range magicTable {
		end := candidate.offset + len(candidate.magic)
		if end <= len(header) && bytes.Equal(header[candidate.offset:end], candidate.magic) {
			return candidate.format, nil
		}
	}
	return formatUnknown, nil
}

// String returns the format's conventional name for diagnostics.
//
// Returns:
//   - `string`: one of "unknown", "gzip", "bzip2", "xz", "zstd", "zip", or "tar".
func (f archiveFormat) String() string {

	switch f {
	case formatGzip:
		return "gzip"
	case formatBzip2:
		return "bzip2"
	case formatXz:
		return "xz"
	case formatZstd:
		return "zstd"
	case formatZip:
		return "zip"
	case formatTar:
		return "tar"
	default:
		return "unknown"
	}
}

// archiveReader iterates an archive's entries in storage order; the caller closes it when done.
type archiveReader interface {

	// Next advances to the next entry, returning [io.EOF] when the archive is exhausted.
	Next() (archiveEntry, error)

	io.Closer
}

// tarArchiveReader iterates a tar stream — plain or decompressed — skipping entry types other than regular files and
// directories (symlinks, devices, FIFOs).
type tarArchiveReader struct {
	file         *os.File
	decompressor io.Closer // the Layer-A decompressor wrapping file; nil on the identity (plain tar) path
	tr           *tar.Reader
}

// newTarArchiveReader reads the open, rewound `archiveFile` as an uncompressed (identity) tar stream.
//
// Parameters:
//   - `archiveFile`: the open archive positioned at byte zero; ownership transfers to the returned reader.
//
// Returns:
//   - `*tarArchiveReader`: the entry iterator; the caller closes it.
func newTarArchiveReader(archiveFile *os.File) *tarArchiveReader {

	return &tarArchiveReader{file: archiveFile, tr: tar.NewReader(archiveFile)}
}

// newTarGzArchiveReader wraps the open, rewound `archiveFile` in a gzip-decompressed tar stream.
//
// Parameters:
//   - `archiveFile`: the open archive positioned at byte zero; closed on a gzip-header failure, otherwise ownership
//     transfers to the returned reader.
//
// Returns:
//   - `*tarArchiveReader`: the entry iterator; the caller closes it.
//   - `error`: any gzip-header failure.
func newTarGzArchiveReader(archiveFile *os.File) (*tarArchiveReader, error) {

	gz, err := gzip.NewReader(archiveFile)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("gzip: %w", err), archiveFile.Close())
	}

	return &tarArchiveReader{file: archiveFile, decompressor: gz, tr: tar.NewReader(gz)}, nil
}

// Next advances to the next regular-file or directory entry, skipping all other tar entry types.
//
// Returns:
//   - `archiveEntry`: the next entry; its Reader (for files) is valid until the following Next call.
//   - `error`: [io.EOF] at end of archive, or any tar read failure.
func (r *tarArchiveReader) Next() (archiveEntry, error) {

	for {
		hdr, err := r.tr.Next()
		if err != nil {
			return archiveEntry{}, err // includes io.EOF
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			return archiveEntry{Name: hdr.Name, Mode: os.FileMode(hdr.Mode & 0o777), IsDir: true}, nil
		case tar.TypeReg:
			return archiveEntry{
				Name:   hdr.Name,
				Mode:   os.FileMode(hdr.Mode & 0o777),
				Reader: io.LimitReader(r.tr, maxEntryBytes),
			}, nil
		}
	}
}

// Close closes the decompressor (when present) and the underlying file, joining any errors.
//
// Returns:
//   - `error`: the joined close errors, or nil.
func (r *tarArchiveReader) Close() error {

	if r.decompressor != nil {
		return errors.Join(r.decompressor.Close(), r.file.Close())
	}
	return r.file.Close()
}

// zipArchiveReader iterates a zip archive's central directory, opening each file entry's body on demand and closing it
// when the iteration advances.
type zipArchiveReader struct {
	rc      *zip.ReadCloser
	index   int
	current io.ReadCloser
}

// newZipArchiveReader opens `source` as a zip archive.
//
// Parameters:
//   - `source`: absolute path to the zip archive on disk.
//
// Returns:
//   - `*zipArchiveReader`: the entry iterator; the caller closes it.
//   - `error`: any open failure.
func newZipArchiveReader(source string) (*zipArchiveReader, error) {

	rc, err := zip.OpenReader(source)
	if err != nil {
		return nil, err
	}

	return &zipArchiveReader{rc: rc}, nil
}

// Next advances to the next entry, closing the previous entry's body reader first.
//
// Returns:
//   - `archiveEntry`: the next entry; its Reader (for files) is valid until the following Next call.
//   - `error`: [io.EOF] at end of archive, or any entry-open failure.
func (r *zipArchiveReader) Next() (archiveEntry, error) {

	if r.current != nil {
		_ = r.current.Close()
		r.current = nil
	}

	if r.index >= len(r.rc.File) {
		return archiveEntry{}, io.EOF
	}

	entry := r.rc.File[r.index]
	r.index++

	if entry.FileInfo().IsDir() {
		return archiveEntry{Name: entry.Name, Mode: entry.Mode(), IsDir: true}, nil
	}

	body, err := entry.Open()
	if err != nil {
		return archiveEntry{}, err
	}
	r.current = body

	return archiveEntry{Name: entry.Name, Mode: entry.Mode(), Reader: io.LimitReader(body, maxEntryBytes)}, nil
}

// Close closes the current entry body (if any) and the zip reader, joining any errors.
//
// Returns:
//   - `error`: the joined close errors, or nil.
func (r *zipArchiveReader) Close() error {

	if r.current != nil {
		return errors.Join(r.current.Close(), r.rc.Close())
	}
	return r.rc.Close()
}

// endregion
