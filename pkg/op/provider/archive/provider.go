// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package archive provides archive extraction actions for the operation graph.
package archive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"

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
//   - `source`: [file.Regular] identifying the archive file; the format is read from its content at dispatch time.
//   - `prefixPath`: the extraction directory path. Must exist as a directory; Extract does not create it.
//
// Returns:
//   - `[]file.Entry`: one entry per file the extraction created or replaced, in extraction order.
//   - `*op.RecoveryStack`: a recovery stack carrying one self-describing [file.Receipt] per created file or directory,
//     in extraction order, so a failed run unwinds it in reverse.
//   - `error`: any error from format detection, extraction, archive-on-displace, or catalog/receipt construction.
func (p *Provider) Extract(
	activationRecord *op.ActivationRecord,
	source *file.Regular,
	prefixPath string,
) (products []file.Entry, stack *op.RecoveryStack, err error) {

	reader, err := p.openArchive(source.SourcePath.Abs())
	if err != nil {
		return nil, nil, err
	}

	return p.extractEntries(activationRecord, reader, prefixPath)
}

// ExtractStream extracts an archive arriving as a forward-only byte stream into the existing directory
// `prefixPath`.
//
// The stream counterpart of [Provider.Extract] (§10 ruling 5's sanctioned add): the leading bytes are sniffed for
// the format magic and stitched back onto the stream, the tar family extracts stream-natively, and a zip — whose
// authoritative central directory sits at the end of the file — spools to a temporary file and takes the same
// random-access path as a disk zip (one zip reader, one authority). Everything downstream — the entry-kind
// dispatch, the containment guard, receipts, and compensation — is shared with Extract; the returned stack unwinds
// via [Provider.CompensateExtractStream].
//
// Parameters:
//   - `activationRecord`: the dispatch activation; its `Unit` stamps every produced entry's producerID and the
//     `forwardAction` of every receipt.
//   - `src`: the archive bytes, consumed exactly once from the current position.
//   - `prefixPath`: the existing directory the archive extracts into.
//
// Returns:
//   - `[]file.Entry`: one entry per file, symlink, or hardlink copy the extraction created or replaced.
//   - `*op.RecoveryStack`: one self-describing [file.Receipt] per created entry, in extraction order.
//   - `error`: any error from sniffing, spooling, extraction, or receipt construction.
func (p *Provider) ExtractStream(
	activationRecord *op.ActivationRecord,
	src io.Reader,
	prefixPath string,
) (products []file.Entry, stack *op.RecoveryStack, err error) {

	reader, err := openArchiveStream(src)
	if err != nil {
		return nil, nil, err
	}

	return p.extractEntries(activationRecord, reader, prefixPath)
}

// extractEntries validates the prefix and folds every archive entry through the kind dispatch, the §10 guards, and
// the receipt machinery — the trunk [Provider.Extract] and [Provider.ExtractStream] share.
//
// Ownership: `reader` is closed here on every path.
//
// Parameters:
//   - `activationRecord`: the dispatch activation.
//   - `reader`: the open entry iterator; ownership transfers.
//   - `prefixPath`: the existing directory the archive extracts into.
//
// Returns:
//   - `[]file.Entry`: the produced entries, in extraction order.
//   - `*op.RecoveryStack`: the receipts, in extraction order.
//   - `error`: any validation, guard, extraction, or receipt failure.
func (p *Provider) extractEntries(
	activationRecord *op.ActivationRecord,
	reader archiveReader,
	prefixPath string,
) (products []file.Entry, stack *op.RecoveryStack, err error) {

	defer iox.Close(&err, reader)

	runtimeEnvironment := activationRecord.RuntimeEnvironment
	stack = op.NewRecoveryStack()

	fileProvider, err := provider.Instance[file.Provider](runtimeEnvironment)
	if err != nil {
		return nil, nil, err
	}

	prefix, err := resolveExtractPrefix(runtimeEnvironment, prefixPath)
	if err != nil {
		return nil, nil, err
	}

	for {
		entry, readErr := reader.Next()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return products, stack, fmt.Errorf("archive: read: %w", readErr)
		}

		// §10 ruling 3: escape intent is an error, and a symlink diverting the path is an error — never a
		// silent skip, never a silent redirect.
		target, guardErr := containedTarget(prefix, entry.Name)
		if guardErr != nil {
			return products, stack, guardErr
		}

		product, receipt, extractErr := extractEntry(activationRecord, fileProvider, prefix, entry, target)
		if extractErr != nil {
			return products, stack, extractErr
		}

		// A nil product+receipt pair is a policy no-op (an already-correct link, an existing directory, or a
		// conflict-skip): nothing was produced and nothing needs compensation.
		if product == nil && receipt == nil {
			continue
		}

		if entry.Kind != entryDir {
			products = append(products, product)
		}

		// The receipt is its own compensator: commit it so it is compensable (an uncommitted receipt has no
		// compensator and Unwind walks past it). forwardAction is stamped archive.extract; compensatingAction
		// stays the file compensator the receipt's constructor named, so Unwind routes it to
		// file.CompensateFileMutation. A no-change receipt (nil) has nothing to commit or push.
		if receipt == nil {
			continue
		}
		if err = receipt.Commit(activationRecord, product, receipt, nil); err != nil {
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

// CompensateExtractStream undoes a [Provider.ExtractStream] by unwinding its recovery stack.
//
// Identical to [Provider.CompensateExtract] — the stream and disk paths share the receipt machinery — and paired
// by name so the compensator index routes stream extractions here.
//
// Parameters:
//   - `activation`: the per-dispatch record; supplies the [*op.RuntimeEnvironment] passed to Unwind.
//   - `stack`: the [*op.RecoveryStack] returned by [Provider.ExtractStream]; a nil stack is a no-op.
//
// Returns:
//   - `error`: non-nil when unwinding any recorded compensation fails.
func (p *Provider) CompensateExtractStream(activation *op.ActivationRecord, stack *op.RecoveryStack) error {
	return p.CompensateExtract(activation, stack)
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// openArchive opens the archive at `source`, detecting its format from the file's leading bytes.
//
// Detection sniffs up to `headerSniffLen` bytes and matches compression and container magic numbers — never the file
// name. A compression match (gzip, bzip2, xz, zstd) wraps the rewound file in the matching decompressor feeding one
// tar reader — the design's Layer-A table (§2 of docs/architecture/3.5.1-archive-provider.md); a ustar magic at
// offset 257 selects the plain-tar (identity) path; a zip match hands the same open handle to [zip.NewReader]
// (zip needs random access to its central directory). Pre-POSIX V7 tar carries no magic, is not
// content-detectable, and resolves to unsupported. Every open routes through [fsroot.Root] (#225).
// The returned [archiveReader] yields entries in storage order and must be closed by the caller.
//
// Parameters:
//   - `source`: absolute path to the archive file on disk.
//
// Returns:
//   - `archiveReader`: an entry iterator over the archive; the caller closes it.
//   - `error`: an unsupported or undetectable format, or any open/sniff/decompress failure.
func (p *Provider) openArchive(source string) (archiveReader, error) {

	root := p.RuntimeEnvironment().Root

	archiveFile, err := root.Open(root.NewPath(source))
	if err != nil {
		return nil, err
	}

	format, err := detectFormat(archiveFile)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("archive: detect %q: %w", source, err), archiveFile.Close())
	}

	switch format {
	case formatGzip, formatBzip2, formatXz, formatZstd, formatTar:
		return tarReaderFor(format, archiveFile, archiveFile)
	case formatZip:
		return newZipArchiveReader(archiveFile)
	default:
		return nil, errors.Join(fmt.Errorf("unsupported archive format: %s", source), archiveFile.Close())
	}
}

// openArchiveStream opens a forward-only archive stream, detecting its format from the sniffed leading bytes.
//
// The sniffed prefix is stitched back onto the stream with [io.MultiReader], so the selected reader consumes the
// bytes from position zero. The tar family reads the stitched stream directly; a zip spools the whole stream to a
// temporary file and takes the same random-access path as a disk zip (§10 ruling 5: the central directory is the
// sole authority, and stream-shaped sources spool to disk), with the temporary file removed when the reader
// closes.
//
// Parameters:
//   - `src`: the archive bytes, consumed exactly once from the current position.
//
// Returns:
//   - `archiveReader`: an entry iterator over the stream; the caller closes it.
//   - `error`: an undetectable format, or any sniff/spool/decompress failure.
func openArchiveStream(src io.Reader) (archiveReader, error) {

	header := make([]byte, headerSniffLen)
	n, err := io.ReadFull(src, header)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, fmt.Errorf("archive: sniff stream: %w", err)
	}

	stitched := io.MultiReader(bytes.NewReader(header[:n]), src)

	format := matchFormat(header[:n])
	switch format {
	case formatGzip, formatBzip2, formatXz, formatZstd, formatTar:
		return tarReaderFor(format, stitched, nil)
	case formatZip:
		return spoolZipStream(stitched)
	default:
		return nil, fmt.Errorf("unsupported archive format on stream")
	}
}

// resolveExtractPrefix discovers and validates the extraction prefix directory.
//
// Destination is discovery — the prefix directory must already exist, so archive isn't producing it.
// DiscoverDirectory registers without claiming production (step 23, ruling 6a).
//
// Parameters:
//   - `runtimeEnvironment`: the session environment.
//   - `prefixPath`: the requested prefix directory.
//
// Returns:
//   - `string`: the prefix's absolute path.
//   - `error`: non-nil when the prefix is missing or not a directory.
func resolveExtractPrefix(runtimeEnvironment *op.RuntimeEnvironment, prefixPath string) (string, error) {

	destination, err := file.DiscoverDirectory(runtimeEnvironment, prefixPath)
	if err != nil {
		return "", err
	}

	if err := destination.Resolve(); err != nil {
		return "", err
	}

	if !destination.Exists() {
		return "", fmt.Errorf("prefix directory does not exist: %s", prefixPath)
	}

	if !destination.IsDir() {
		return "", fmt.Errorf("prefix path is not a directory: %s", prefixPath)
	}

	return destination.SourcePath.Abs(), nil
}

// extractEntry materializes one archive entry through the file provider, per its kind.
//
// Parameters:
//   - `activationRecord`: the dispatch activation.
//   - `fileProvider`: the file provider performing the mutations.
//   - `prefix`: the extraction prefix's absolute path.
//   - `entry`: the archive entry.
//   - `target`: the entry's contained target path.
//
// Returns:
//   - `file.Entry`: the produced entry; nil for a policy no-op.
//   - `*file.Receipt`: the mutation receipt; nil for a no-change outcome.
//   - `error`: non-nil on any guard or mutation failure.
func extractEntry(
	activationRecord *op.ActivationRecord, fileProvider *file.Provider, prefix string, entry archiveEntry, target string,
) (file.Entry, *file.Receipt, error) {

	switch entry.Kind {
	case entryDir:
		product, receipt, err := fileProvider.Mkdir(activationRecord, target, entry.Mode, "")
		if err != nil {
			return nil, nil, fmt.Errorf("archive: mkdir %q: %w", target, err)
		}
		return product, receipt, nil
	case entryFile:
		product, receipt, err := fileProvider.WriteFile(activationRecord, target, entry.Reader, entry.Mode)
		if err != nil {
			return nil, nil, fmt.Errorf("archive: write %q: %w", target, err)
		}
		return product, receipt, nil
	case entrySymlink:
		// §10 ruling 1a: contained targets only; the link lands verbatim so the on-disk content — and the
		// SymbolicLink digest, which hashes the literal target — stays faithful to the archive.
		if guardErr := containedLinkTarget(entry.Name, entry.Linkname); guardErr != nil {
			return nil, nil, guardErr
		}
		product, receipt, err := fileProvider.Link(activationRecord, entry.Linkname, target, true)
		if err != nil {
			return nil, nil, fmt.Errorf("archive: link %q: %w", target, err)
		}
		return product, receipt, nil
	case entryHardlink:
		return copyHardlinkEntry(activationRecord, fileProvider, prefix, entry, target)
	}

	return nil, nil, nil
}

// copyHardlinkEntry materializes a hard-link entry as a content copy of its already-extracted
// referent (archive-root-relative by tar convention) — §10 ruling 1b: a hard link is an aliasing
// property, not a kind.
//
// Parameters:
//   - `activationRecord`: the dispatch activation.
//   - `fileProvider`: the file provider performing the write.
//   - `prefix`: the extraction prefix's absolute path.
//   - `entry`: the hard-link entry.
//   - `target`: the entry's contained target path.
//
// Returns:
//   - `file.Entry`: the produced entry; nil for a policy no-op.
//   - `*file.Receipt`: the mutation receipt; nil for a no-change outcome.
//   - `error`: non-nil on a guard, open, write, or close failure.
func copyHardlinkEntry(
	activationRecord *op.ActivationRecord, fileProvider *file.Provider, prefix string, entry archiveEntry, target string,
) (file.Entry, *file.Receipt, error) {

	referent, refErr := containedTarget(prefix, entry.Linkname)
	if refErr != nil {
		return nil, nil, refErr
	}

	root := activationRecord.RuntimeEnvironment.Root
	referentFile, openErr := root.Open(root.NewPath(referent))
	if openErr != nil {
		return nil, nil, fmt.Errorf(
			"archive: entry %q: hardlink referent %q is not extracted: %w", entry.Name, entry.Linkname, openErr)
	}

	product, receipt, err := fileProvider.WriteFile(activationRecord, target, referentFile, entry.Mode)
	if closeErr := referentFile.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		return nil, nil, fmt.Errorf("archive: hardlink copy %q: %w", target, err)
	}

	return product, receipt, nil
}

// spoolZipStream drains `stream` to a temporary file and opens it as a zip, removing the file on close.
//
// Parameters:
//   - `stream`: the complete zip bytes.
//
// Returns:
//   - `archiveReader`: the zip entry iterator over the spooled file; closing it also removes the file.
//   - `error`: any spool or zip-open failure (the temporary file is removed on failure).
func spoolZipStream(stream io.Reader) (archiveReader, error) {

	// Confinement: the spool is process scratch in the system temp dir, not target-tree I/O (§10 ruling 5).
	spool, err := os.CreateTemp("", "devlore-archive-*.zip")
	if err != nil {
		return nil, fmt.Errorf("archive: spool zip stream: %w", err)
	}

	if _, err = io.Copy(spool, stream); err != nil {
		//nolint:gosec // G703: the path is os.CreateTemp's own name, not external input.
		return nil, errors.Join(fmt.Errorf("archive: spool zip stream: %w", err), spool.Close(), os.Remove(spool.Name()))
	}
	inner, err := newZipArchiveReader(spool)
	if err != nil {
		//nolint:gosec // G703: the path is os.CreateTemp's own name, not external input.
		return nil, errors.Join(err, os.Remove(spool.Name()))
	}

	return &spooledZipReader{zipArchiveReader: inner, spoolPath: spool.Name()}, nil
}

// endregion

// endregion

// region SUPPORTING TYPES

// archiveEntry is one entry yielded by an [archiveReader]: a directory, a regular file with its body reader, a
// symlink, or a hardlink (§10 ruling 1 — special kinds surface as entries; nothing is silently skipped).
type archiveEntry struct {

	// Name is the entry's path as stored in the archive (joined against the extraction prefix by the caller).
	Name string

	// Mode is the entry's permission bits.
	Mode os.FileMode

	// Kind is the entry's kind; the zero value is a regular file.
	Kind entryKind

	// Linkname is the link target: for a symlink, the literal archived target (entry-directory-relative by tar
	// convention); for a hardlink, the archive-root-relative referent path. Empty otherwise.
	Linkname string

	// Reader is the file body, valid only until the next [archiveReader.Next] call; nil for non-file kinds.
	Reader io.Reader
}

// entryKind names the archive entry kinds extraction handles (§10 ruling 1).
type entryKind int

// The entry kinds. entryFile is the zero value — the common case.
const (
	entryFile entryKind = iota
	entryDir
	entrySymlink
	entryHardlink
)

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

	return matchFormat(header), nil
}

// matchFormat matches `header` against the magic table, first match wins.
//
// Parameters:
//   - `header`: the leading archive bytes (up to `headerSniffLen`; shorter is fine).
//
// Returns:
//   - `archiveFormat`: the matched format, or `formatUnknown`.
func matchFormat(header []byte) archiveFormat {

	for _, candidate := range magicTable {
		end := candidate.offset + len(candidate.magic)
		if end <= len(header) && bytes.Equal(header[candidate.offset:end], candidate.magic) {
			return candidate.format
		}
	}

	return formatUnknown
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
	file         *os.File      // nil in stream mode ([Provider.ExtractStream])
	decompressor io.Closer     // the Layer-A decompressor wrapping the source; nil when it has no Close
	format       archiveFormat // the detected outer format, named in diagnostics (§10 ruling 4)
	sawEntry     bool          // whether any header has been read — gates the not-a-tar diagnostics
	tr           *tar.Reader
}

// tarReaderFor wraps `stream` in the decompressor `format` names and returns the tar entry iterator over it.
//
// The one constructor for the whole tar family — the design's Layer-A table realized: gzip via [gzip.NewReader],
// bzip2 via the standard library (reader-only, no Close), xz via [xz.NewReader] (no Close), zstd via
// [zstd.NewReader] with its [zstd.Decoder.IOReadCloser] projection riding the decompressor slot (the decoder owns
// goroutine-backed state), and the identity path for plain (ustar) tar. `file` is the backing file when the
// source is on disk — closed on a decompressor-header failure, otherwise ownership transfers to the returned
// reader — or nil in stream mode ([Provider.ExtractStream]), where the caller owns the source.
//
// Parameters:
//   - `format`: the detected outer format; must be one of the tar-family formats.
//   - `stream`: the raw archive bytes, positioned at byte zero.
//   - `file`: the backing [*os.File] when the source is a disk file; nil in stream mode.
//
// Returns:
//   - `*tarArchiveReader`: the entry iterator; the caller closes it.
//   - `error`: a decompressor-header failure (the backing file, when present, is closed).
func tarReaderFor(format archiveFormat, stream io.Reader, backing *os.File) (*tarArchiveReader, error) {

	closeFileOn := func(err error) error {
		if backing != nil {
			return errors.Join(err, backing.Close())
		}
		return err
	}

	reader := &tarArchiveReader{file: backing, format: format}

	switch format {
	case formatTar:
		reader.tr = tar.NewReader(stream)
	case formatGzip:
		gz, err := gzip.NewReader(stream)
		if err != nil {
			return nil, closeFileOn(fmt.Errorf("gzip: %w", err))
		}
		reader.decompressor = gz
		reader.tr = tar.NewReader(gz)
	case formatBzip2:
		reader.tr = tar.NewReader(bzip2.NewReader(stream))
	case formatXz:
		xzReader, err := xz.NewReader(stream)
		if err != nil {
			return nil, closeFileOn(fmt.Errorf("xz: %w", err))
		}
		reader.tr = tar.NewReader(xzReader)
	case formatZstd:
		decoder, err := zstd.NewReader(stream)
		if err != nil {
			return nil, closeFileOn(fmt.Errorf("zstd: %w", err))
		}
		closer := decoder.IOReadCloser()
		reader.decompressor = closer
		reader.tr = tar.NewReader(closer)
	default:
		return nil, closeFileOn(fmt.Errorf("tarReaderFor: %s is not a tar-family format", format))
	}

	return reader, nil
}

// Next advances to the next entry, surfacing every kind extraction handles and erring on the rest.
//
// §10 ruling 1: directories, regular files, symlinks, and hardlinks yield entries; devices, FIFOs, and any other
// typeflag error naming the entry and its kind — nothing is silently skipped. §10 ruling 4: on the compressed
// paths, a first-header failure (or an immediately empty payload) reports that the decompressed payload is not a
// tar archive, naming the detected outer format; errors after the first entry keep their cause under the same
// format prefix, so genuine mid-archive corruption stays distinguishable from wrong-container input.
//
// Returns:
//   - `archiveEntry`: the next entry; its Reader (for regular files) is valid until the following Next call.
//   - `error`: [io.EOF] at end of archive, a not-a-tar diagnostic, an unsupported entry kind, or a read failure.
func (r *tarArchiveReader) Next() (archiveEntry, error) {

	hdr, err := r.tr.Next()
	if err != nil {
		return archiveEntry{}, r.diagnose(err)
	}
	r.sawEntry = true

	mode := os.FileMode(hdr.Mode & 0o777)

	switch hdr.Typeflag {
	case tar.TypeDir:
		return archiveEntry{Name: hdr.Name, Mode: mode, Kind: entryDir}, nil
	case tar.TypeReg:
		return archiveEntry{Name: hdr.Name, Mode: mode, Reader: r.tr}, nil
	case tar.TypeSymlink:
		return archiveEntry{Name: hdr.Name, Mode: mode, Kind: entrySymlink, Linkname: hdr.Linkname}, nil
	case tar.TypeLink:
		return archiveEntry{Name: hdr.Name, Mode: mode, Kind: entryHardlink, Linkname: hdr.Linkname}, nil
	default:
		return archiveEntry{}, fmt.Errorf(
			"archive: entry %q: unsupported tar entry kind %s (typeflag %q)",
			hdr.Name, tarTypeflagName(hdr.Typeflag), hdr.Typeflag)
	}
}

// diagnose translates a tar-read failure per §10 ruling 4.
//
// On a compressed path before any entry was read: an immediate [io.EOF] means the decompressed payload was empty,
// and any other failure means it was not a tar — both report the detected outer format and the missing container.
// After the first entry (or on the magic-gated identity path), [io.EOF] passes through as the normal end of
// archive and other failures keep their cause under the format-naming prefix.
//
// Parameters:
//   - `err`: the error from the underlying tar reader.
//
// Returns:
//   - `error`: the translated error.
func (r *tarArchiveReader) diagnose(err error) error {

	if r.format != formatTar && !r.sawEntry {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("archive: %s-compressed payload is empty — not a tar archive", r.format)
		}
		return fmt.Errorf("archive: %s-compressed payload is not a tar archive: %w", r.format, err)
	}

	if errors.Is(err, io.EOF) {
		return io.EOF
	}

	return fmt.Errorf("archive: %s tar: %w", r.format, err)
}

// Close closes the decompressor (when present) and the underlying file, joining any errors.
//
// Returns:
//   - `error`: the joined close errors, or nil.
func (r *tarArchiveReader) Close() error {

	var closeErrs []error

	if r.decompressor != nil {
		closeErrs = append(closeErrs, r.decompressor.Close())
	}
	if r.file != nil {
		closeErrs = append(closeErrs, r.file.Close())
	}

	return errors.Join(closeErrs...)
}

// zipArchiveReader iterates a zip archive's central directory, opening each file entry's body on demand and closing it
// when the iteration advances.
type zipArchiveReader struct {
	zr      *zip.Reader
	closer  io.Closer
	index   int
	current io.ReadCloser
}

// newZipArchiveReader wraps the already-open zip archive `file` as an entry iterator.
//
// Taking the open handle rather than a path keeps every archive open on the caller's [fsroot.Root] route
// (#225); the handle is owned by the returned reader, which closes it — including on its own error paths.
//
// Parameters:
//   - `archive`: the open zip archive; zip needs its random access for the central directory.
//
// Returns:
//   - `*zipArchiveReader`: the entry iterator; the caller closes it.
//   - `error`: a stat or central-directory failure.
func newZipArchiveReader(archive *os.File) (*zipArchiveReader, error) {

	info, err := archive.Stat()
	if err != nil {
		return nil, errors.Join(err, archive.Close())
	}

	zr, err := zip.NewReader(archive, info.Size())
	if err != nil {
		return nil, errors.Join(err, archive.Close())
	}

	return &zipArchiveReader{zr: zr, closer: archive}, nil
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

	if r.index >= len(r.zr.File) {
		return archiveEntry{}, io.EOF
	}

	entry := r.zr.File[r.index]
	r.index++

	if entry.FileInfo().IsDir() {
		return archiveEntry{Name: entry.Name, Mode: entry.Mode(), Kind: entryDir}, nil
	}

	// A zip symlink stores its target as the entry body (unix external attributes carry the mode). Surfacing it
	// as a symlink entry ends another silent corruption: the target string used to be written out as a FILE.
	if entry.Mode()&os.ModeSymlink != 0 {
		body, err := entry.Open()
		if err != nil {
			return archiveEntry{}, err
		}
		target, err := io.ReadAll(body)
		if closeErr := body.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return archiveEntry{}, fmt.Errorf("archive: entry %q: read zip symlink target: %w", entry.Name, err)
		}
		return archiveEntry{Name: entry.Name, Mode: entry.Mode(), Kind: entrySymlink, Linkname: string(target)}, nil
	}

	body, err := entry.Open()
	if err != nil {
		return archiveEntry{}, err
	}
	r.current = body

	return archiveEntry{Name: entry.Name, Mode: entry.Mode(), Reader: body}, nil
}

// Close closes the current entry body (if any) and the zip reader, joining any errors.
//
// Returns:
//   - `error`: the joined close errors, or nil.
func (r *zipArchiveReader) Close() error {

	if r.current != nil {
		return errors.Join(r.current.Close(), r.closer.Close())
	}
	return r.closer.Close()
}

// endregion

// spooledZipReader wraps a [zipArchiveReader] over a spooled temporary file, removing the file on close.
type spooledZipReader struct {
	*zipArchiveReader
	spoolPath string
}

// Close closes the underlying zip reader and removes the spooled temporary file, joining any errors.
//
// Returns:
//   - `error`: the joined close/remove errors, or nil.
func (r *spooledZipReader) Close() error {
	return errors.Join(r.zipArchiveReader.Close(), os.Remove(r.spoolPath))
}

// region HELPER FUNCTIONS

// containedTarget joins `name` onto `prefix` under the §10 ruling-3 guard: escape intent errors, and a symlink
// diverting the path errors.
//
// Layer 1 (policy): a name that is absolute or `..`-escaping after cleaning is refused outright. Layer 2
// (resolution): [securejoin.SecureJoin] resolves existing symlink components against the real filesystem; ANY
// divergence from the lexical join means a symlink interfered with the path — an error naming both forms, never a
// silent redirect (modern tar parity: "cannot extract through symlink"). Layer 3, the [os.Root] syscall backstop,
// lives in `fsroot` and needs nothing here.
//
// Parameters:
//   - `prefix`: the absolute extraction prefix.
//   - `name`: the entry's archived path.
//
// Returns:
//   - `string`: the safe absolute target (the lexical join, proven divergence-free).
//   - `error`: escape intent, resolution failure, or symlink divergence.
func containedTarget(prefix, name string) (string, error) {

	cleaned := filepath.Clean(name)

	if filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("archive: entry %q escapes the extraction prefix", name)
	}

	lexical := filepath.Join(prefix, cleaned)

	resolved, err := securejoin.SecureJoin(prefix, cleaned)
	if err != nil {
		return "", fmt.Errorf("archive: entry %q: resolve against %q: %w", name, prefix, err)
	}

	if resolved != lexical {
		return "", fmt.Errorf(
			"archive: entry %q: path traverses a symlink (resolves to %q, not %q)", name, resolved, lexical)
	}

	return lexical, nil
}

// containedLinkTarget judges a symlink entry's target under §10 ruling 1a: relative and non-escaping only.
//
// The target is entry-directory-relative by tar convention, so containment is judged from the entry's own
// directory. An absolute target, or one that climbs above the extraction prefix after cleaning, is a hard error
// naming the entry — deploy-domain archives whose links point outside their own tree are suspect input. The link's
// own location was already judged by [containedTarget]; the target is judged lexically (it may legally dangle, so
// there is nothing on disk to resolve).
//
// Parameters:
//   - `entryName`: the symlink entry's archived path.
//   - `linkname`: the literal archived target.
//
// Returns:
//   - `error`: non-nil when the target is absolute or escapes the extraction prefix.
func containedLinkTarget(entryName, linkname string) error {

	if filepath.IsAbs(linkname) {
		return fmt.Errorf("archive: entry %q: symlink target %q is absolute", entryName, linkname)
	}

	resolved := filepath.Clean(filepath.Join(filepath.Dir(filepath.Clean(entryName)), linkname))

	if resolved == ".." || strings.HasPrefix(resolved, ".."+string(filepath.Separator)) {
		return fmt.Errorf("archive: entry %q: symlink target %q escapes the extraction prefix", entryName, linkname)
	}

	return nil
}

// tarTypeflagName names a tar typeflag for the unsupported-entry diagnostics (§10 ruling 1c).
//
// Parameters:
//   - `typeflag`: the tar header typeflag byte.
//
// Returns:
//   - `string`: the human name of the kind.
func tarTypeflagName(typeflag byte) string {

	switch typeflag {
	case tar.TypeChar:
		return "character device"
	case tar.TypeBlock:
		return "block device"
	case tar.TypeFifo:
		return "FIFO"
	default:
		return "unknown"
	}
}

// endregion
