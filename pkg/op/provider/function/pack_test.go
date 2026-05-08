// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package function

import (
	"bytes"
	"io"
	"testing"
)

// --- writeFunctionPack / readFunctionPackHeader round-trip ---

func TestFunctionPack_RoundTrip(t *testing.T) {

	source := []byte("def foo(x):\n    return x + 1\n")
	compiled := []byte("\x00\x01\x02\x03fake-bytecode")
	const compilerVersion uint32 = 42

	var buf bytes.Buffer
	if err := writeFunctionPack(&buf, source, compiled, compilerVersion); err != nil {
		t.Fatalf("writeFunctionPack: %v", err)
	}

	wantSize := functionPackHeaderSize + len(source) + len(compiled)
	if buf.Len() != wantSize {
		t.Errorf("pack size = %d, want %d", buf.Len(), wantSize)
	}

	ra := bytes.NewReader(buf.Bytes())

	h, err := readFunctionPackHeader(ra)
	if err != nil {
		t.Fatalf("readFunctionPackHeader: %v", err)
	}

	if h.Magic != functionPackMagic {
		t.Errorf("Magic = %#08x, want %#08x", h.Magic, functionPackMagic)
	}
	if h.FormatVersion != functionPackFormatVersion {
		t.Errorf("FormatVersion = %d, want %d", h.FormatVersion, functionPackFormatVersion)
	}
	if h.CompilerVersion != compilerVersion {
		t.Errorf("CompilerVersion = %d, want %d", h.CompilerVersion, compilerVersion)
	}
	if h.SourceOffset != uint64(functionPackHeaderSize) {
		t.Errorf("SourceOffset = %d, want %d", h.SourceOffset, functionPackHeaderSize)
	}
	if h.SourceSize != uint64(len(source)) {
		t.Errorf("SourceSize = %d, want %d", h.SourceSize, len(source))
	}
	if h.CompiledSize != uint64(len(compiled)) {
		t.Errorf("CompiledSize = %d, want %d", h.CompiledSize, len(compiled))
	}
}

func TestFunctionPack_SourceSectionReads(t *testing.T) {

	source := []byte("original source text")
	compiled := []byte("xxx")

	var buf bytes.Buffer
	if err := writeFunctionPack(&buf, source, compiled, 1); err != nil {
		t.Fatalf("writeFunctionPack: %v", err)
	}

	ra := bytes.NewReader(buf.Bytes())
	h, err := readFunctionPackHeader(ra)
	if err != nil {
		t.Fatal(err)
	}

	got, err := io.ReadAll(sourceReader(ra, h))
	if err != nil {
		t.Fatalf("read source section: %v", err)
	}
	if !bytes.Equal(got, source) {
		t.Errorf("source section = %q, want %q", got, source)
	}
}

func TestFunctionPack_CompiledSectionReads(t *testing.T) {

	source := []byte("source")
	compiled := []byte("bytecode payload")

	var buf bytes.Buffer
	if err := writeFunctionPack(&buf, source, compiled, 1); err != nil {
		t.Fatalf("writeFunctionPack: %v", err)
	}

	ra := bytes.NewReader(buf.Bytes())
	h, err := readFunctionPackHeader(ra)
	if err != nil {
		t.Fatal(err)
	}

	got, err := io.ReadAll(compiledReader(ra, h))
	if err != nil {
		t.Fatalf("read compiled section: %v", err)
	}
	if !bytes.Equal(got, compiled) {
		t.Errorf("compiled section = %q, want %q", got, compiled)
	}
}

func TestFunctionPack_EmptyCompiled(t *testing.T) {

	source := []byte("source only")

	var buf bytes.Buffer
	if err := writeFunctionPack(&buf, source, nil, 1); err != nil {
		t.Fatalf("writeFunctionPack: %v", err)
	}

	ra := bytes.NewReader(buf.Bytes())
	h, err := readFunctionPackHeader(ra)
	if err != nil {
		t.Fatal(err)
	}

	if h.CompiledSize != 0 {
		t.Errorf("CompiledSize = %d, want 0", h.CompiledSize)
	}

	got, err := io.ReadAll(compiledReader(ra, h))
	if err != nil {
		t.Fatalf("read compiled section: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("compiled section should be empty, got %q", got)
	}
}

// --- readFunctionPackHeader validation ---

func TestReadFunctionPackHeader_BadMagic(t *testing.T) {

	// Construct a 48-byte blob with a bogus magic.
	blob := make([]byte, functionPackHeaderSize)
	blob[0], blob[1], blob[2], blob[3] = 0xde, 0xad, 0xbe, 0xef

	if _, err := readFunctionPackHeader(bytes.NewReader(blob)); err == nil {
		t.Fatal("expected error on bad magic")
	}
}

func TestReadFunctionPackHeader_Truncated(t *testing.T) {

	// Less than header size.
	if _, err := readFunctionPackHeader(bytes.NewReader(make([]byte, 10))); err == nil {
		t.Fatal("expected error on truncated header")
	}
}
