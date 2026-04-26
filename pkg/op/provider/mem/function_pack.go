// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package mem

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// Pack format for mem.Function recovery entries.
//
// A Function archives a single recovery file containing both the synthesized source text and the compiled
// starlark bytecode. The format is a fixed 48-byte little-endian header followed by the two payloads. Fixed
// header offsets let mmap'd consumers construct [io.SectionReader] views of each payload without sequential
// parsing.
//
// Layout:
//
//	offset  size   field                    notes
//	0       4      magic = "memf"           detect corruption / other file types
//	4       4      format_version (uint32)  this pack format; currently 1
//	8       4      compiler_version (uint32) starlark.CompilerVersion at pack time
//	12      4      reserved (uint32)        zero; reserved for future use
//	16      8      source_offset (uint64)   = 48 (right after header)
//	24      8      source_size (uint64)
//	32      8      compiled_offset (uint64) = source_offset + source_size
//	40      8      compiled_size (uint64)   may be 0 if compilation was deferred
//	48..    ..     source bytes
//	..             compiled bytes
const (
	functionPackMagic         uint32 = 0x666d656d // "memf" little-endian bytes (6d 65 6d 66)
	functionPackFormatVersion uint32 = 1
	functionPackHeaderSize    int    = 48
)

// functionPackHeader is the on-disk header for a packed mem.Function recovery entry.
//
// Field ordering matches the on-disk layout; [binary.Read] / [binary.Write] use the struct's declared order
// when the type has no variable-length fields. All multi-byte fields serialize little-endian.
type functionPackHeader struct {
	Magic           uint32
	FormatVersion   uint32
	CompilerVersion uint32
	Reserved        uint32
	SourceOffset    uint64
	SourceSize      uint64
	CompiledOffset  uint64
	CompiledSize    uint64
}

// writeFunctionPack writes the 48-byte header followed by source bytes then compiled bytes into w.
//
// Total bytes written = [functionPackHeaderSize] + len(source) + len(compiled). w is typically a buffer
// that is subsequently written to the Function's URI-derived SourcePath as a single atomic write.
//
// Parameters:
//   - w:               destination writer.
//   - source:          the synthesized starlark source text.
//   - compiled:        the compiled starlark bytecode; may be empty.
//   - compilerVersion: [starlark.CompilerVersion] at pack time, used by readers to detect staleness.
//
// Returns:
//   - error: any error from the underlying writes.
func writeFunctionPack(w io.Writer, source, compiled []byte, compilerVersion uint32) error {

	sourceOffset := uint64(functionPackHeaderSize)
	compiledOffset := sourceOffset + uint64(len(source))

	header := functionPackHeader{
		Magic:           functionPackMagic,
		FormatVersion:   functionPackFormatVersion,
		CompilerVersion: compilerVersion,
		SourceOffset:    sourceOffset,
		SourceSize:      uint64(len(source)),
		CompiledOffset:  compiledOffset,
		CompiledSize:    uint64(len(compiled)),
	}

	if err := binary.Write(w, binary.LittleEndian, header); err != nil {
		return fmt.Errorf("mem.Function pack: write header: %w", err)
	}

	if _, err := w.Write(source); err != nil {
		return fmt.Errorf("mem.Function pack: write source: %w", err)
	}

	if len(compiled) > 0 {
		if _, err := w.Write(compiled); err != nil {
			return fmt.Errorf("mem.Function pack: write compiled: %w", err)
		}
	}

	return nil
}

// readFunctionPackHeader reads and validates the header from the start of a mmap'd pack file.
//
// Parameters:
//   - ra: random-access reader positioned over the pack file.
//
// Returns:
//   - functionPackHeader: the decoded header.
//   - error:              any read or validation error (bad magic, unsupported format version).
func readFunctionPackHeader(ra io.ReaderAt) (functionPackHeader, error) {

	buf := make([]byte, functionPackHeaderSize)
	if _, err := ra.ReadAt(buf, 0); err != nil {
		return functionPackHeader{}, fmt.Errorf("mem.Function pack: read header: %w", err)
	}

	var h functionPackHeader
	if err := binary.Read(bytes.NewReader(buf), binary.LittleEndian, &h); err != nil {
		return h, fmt.Errorf("mem.Function pack: decode header: %w", err)
	}

	if h.Magic != functionPackMagic {
		return h, fmt.Errorf("mem.Function pack: bad magic %#08x", h.Magic)
	}
	if h.FormatVersion != functionPackFormatVersion {
		return h, fmt.Errorf("mem.Function pack: unsupported format version %d", h.FormatVersion)
	}

	return h, nil
}

// sourceReader returns an [io.SectionReader] over the source bytes of a mmap'd pack.
//
// Parameters:
//   - ra: the pack's random-access reader.
//   - h:  the previously-decoded header.
//
// Returns:
//   - *io.SectionReader: reader covering source_offset..source_offset+source_size.
func sourceReader(ra io.ReaderAt, h functionPackHeader) *io.SectionReader {
	return io.NewSectionReader(ra, int64(h.SourceOffset), int64(h.SourceSize))
}

// compiledReader returns an [io.SectionReader] over the compiled bytes of a mmap'd pack.
//
// Returned reader has zero size when the pack carries no compiled bytecode (CompiledSize == 0).
//
// Parameters:
//   - ra: the pack's random-access reader.
//   - h:  the previously-decoded header.
//
// Returns:
//   - *io.SectionReader: reader covering compiled_offset..compiled_offset+compiled_size.
func compiledReader(ra io.ReaderAt, h functionPackHeader) *io.SectionReader {
	return io.NewSectionReader(ra, int64(h.CompiledOffset), int64(h.CompiledSize))
}
