// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package mem

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// --- Interface guards ---

func TestResourceImplementsInterface(t *testing.T) {
	var _ op.Resource = (*Resource)(nil)
}

// --- Test helpers ---

// newTestCtx returns an RuntimeEnvironment with a Root anchored at a fresh temp dir and a populated
// RecoverySite — the shape Resource construction requires when Data is []byte.
func newTestCtx(t *testing.T) *op.RuntimeEnvironment {
	t.Helper()
	root := op.NewRootReaderWriter(t.TempDir())
	ctx := &op.RuntimeEnvironment{Root: root}
	ctx.RecoverySite = op.NewRecoverySite(ctx)
	return ctx
}

// newRes is a convenience for tests that don't care about the RuntimeEnvironment beyond RecoverySite wiring.
func newRes(t *testing.T, ctx *op.RuntimeEnvironment, spec ResourceSpec) *Resource {
	t.Helper()
	r, err := NewResource(ctx, spec)
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	return r
}

// --- NewResource ---

func TestNewResource_MetadataOnly(t *testing.T) {

	ctx := newTestCtx(t)

	r := newRes(t, ctx, ResourceSpec{ContentType: "callable", Namespace: "file.Reducer", Name: "myfn"})

	if r.ContentType != "callable" {
		t.Errorf("ContentType = %q, want %q", r.ContentType, "callable")
	}
	if r.Namespace != "file.Reducer" {
		t.Errorf("Namespace = %q, want %q", r.Namespace, "file.Reducer")
	}
	if r.Name != "myfn" {
		t.Errorf("Name = %q, want %q", r.Name, "myfn")
	}
	if r.ReachabilityURI() != "mem:callable/file.Reducer/myfn" {
		t.Errorf("ReachabilityURI() = %q, want %q", r.ReachabilityURI(), "mem:callable/file.Reducer/myfn")
	}
	assertArchivedFileAbsent(t, r)
	if r.Hash != "" {
		t.Errorf("Hash = %q, want empty (no Data archived)", r.Hash)
	}
}

func TestNewResource_NoNamespace(t *testing.T) {

	ctx := newTestCtx(t)

	r := newRes(t, ctx, ResourceSpec{ContentType: "json", Name: "config"})

	if r.ReachabilityURI() != "mem:json/config" {
		t.Errorf("ReachabilityURI() = %q, want %q", r.ReachabilityURI(), "mem:json/config")
	}
}

func TestNewResource_ContentTypeOnly(t *testing.T) {

	ctx := newTestCtx(t)

	r := newRes(t, ctx, ResourceSpec{ContentType: "json"})

	if r.ReachabilityURI() != "mem:json" {
		t.Errorf("ReachabilityURI() = %q, want %q", r.ReachabilityURI(), "mem:json")
	}
}

func TestNewResource_WithBytes(t *testing.T) {

	ctx := newTestCtx(t)
	data := []byte("hello world")

	r := newRes(t, ctx, ResourceSpec{ContentType: "template", Name: "greeting", Data: data})

	assertArchivedFileExists(t, r)

	h := sha256.Sum256(data)
	want := hex.EncodeToString(h[:])
	if r.Hash != want {
		t.Errorf("Hash = %q, want %q", r.Hash, want)
	}

	// Content round-trips.
	assertArchivedContent(t, r, data)
}

func TestNewResource_WithString(t *testing.T) {

	ctx := newTestCtx(t)
	data := "hello world"

	r := newRes(t, ctx, ResourceSpec{ContentType: "template", Name: "s", Data: data})

	assertArchivedFileExists(t, r)

	h := sha256.Sum256([]byte(data))
	if want := hex.EncodeToString(h[:]); r.Hash != want {
		t.Errorf("Hash = %q, want %q", r.Hash, want)
	}

	assertArchivedContent(t, r, []byte(data))
}

func TestNewResource_WithReader_Streams(t *testing.T) {

	ctx := newTestCtx(t)

	// A 512 KiB payload — large enough that any in-memory buffering would show up in profiles; io.TeeReader
	// should stream this to RecoverySite without materializing it in Go heap memory.
	const size = 512 * 1024
	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i)
	}

	r := newRes(t, ctx, ResourceSpec{
		ContentType: "blob",
		Name:        "big",
		Data:        bytes.NewReader(payload),
	})

	assertArchivedFileExists(t, r)

	h := sha256.Sum256(payload)
	if want := hex.EncodeToString(h[:]); r.Hash != want {
		t.Errorf("Hash = %q, want %q", r.Hash, want)
	}

	assertArchivedContent(t, r, payload)
}

func TestNewResource_WithBytesMethod(t *testing.T) {

	ctx := newTestCtx(t)

	// *bytes.Buffer satisfies both io.Reader and the Bytes() method — io.Reader wins per dispatch order,
	// draining the buffer. To exercise the Bytes() branch we need a type that has Bytes() but NOT io.Reader.
	holder := &bytesOnly{content: []byte("from Bytes() method")}

	r := newRes(t, ctx, ResourceSpec{ContentType: "blob", Name: "b", Data: holder})

	assertArchivedFileExists(t, r)

	assertArchivedContent(t, r, holder.content)
}

func TestNewResource_WithBinaryMarshaler(t *testing.T) {

	ctx := newTestCtx(t)

	marshaler := &binaryOnly{content: []byte("binary marshaled form")}

	r := newRes(t, ctx, ResourceSpec{ContentType: "blob", Name: "bm", Data: marshaler})

	assertArchivedFileExists(t, r)

	assertArchivedContent(t, r, marshaler.content)
}

func TestNewResource_WithTextMarshaler(t *testing.T) {

	ctx := newTestCtx(t)

	marshaler := &textOnly{text: []byte("text marshaled form")}

	r := newRes(t, ctx, ResourceSpec{ContentType: "blob", Name: "tm", Data: marshaler})

	assertArchivedFileExists(t, r)

	assertArchivedContent(t, r, marshaler.text)
}

func TestNewResource_StreamingErrorPropagates(t *testing.T) {

	ctx := newTestCtx(t)

	reader := &erroringReader{after: 5, err: errSyntheticRead}

	if _, err := NewResource(ctx, ResourceSpec{ContentType: "blob", Name: "err", Data: reader}); err == nil {
		t.Fatal("expected error when io.Reader returns mid-stream error")
	}
}

func TestNewResource_UnsupportedDataType(t *testing.T) {

	ctx := newTestCtx(t)

	// An int doesn't satisfy any of the accepted interfaces.
	_, err := NewResource(ctx, ResourceSpec{ContentType: "blob", Name: "unsupported", Data: 42})
	if err == nil {
		t.Fatal("expected error for unsupported Data type")
	}
	if !strings.Contains(err.Error(), "unsupported Data type") {
		t.Errorf("error = %q, want message containing 'unsupported Data type'", err)
	}
}

func TestNewResource_EmptyContentType(t *testing.T) {

	ctx := newTestCtx(t)

	if _, err := NewResource(ctx, ResourceSpec{}); err == nil {
		t.Fatal("expected error for empty content type")
	}
}

func TestNewResource_WrongType(t *testing.T) {

	ctx := newTestCtx(t)

	if _, err := NewResource(ctx, 42); err == nil {
		t.Fatal("expected error for non-ResourceSpec")
	}
}

// --- Reader ---

func TestResource_Reader_ReturnsArchivedContent(t *testing.T) {

	ctx := newTestCtx(t)
	content := []byte("streamed through mmap")

	r := newRes(t, ctx, ResourceSpec{ContentType: "json", Name: "data", Data: content})

	rc, err := r.Reader()
	if err != nil {
		t.Fatalf("Reader: %v", err)
	}
	defer func() { _ = rc.Close() }()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("Reader content = %q, want %q", got, content)
	}
}

func TestResource_Reader_NoContentErrors(t *testing.T) {

	ctx := newTestCtx(t)

	r := newRes(t, ctx, ResourceSpec{ContentType: "json", Name: "data"})

	if _, err := r.Reader(); err == nil {
		t.Fatal("Reader on metadata-only Resource should error")
	}
}

// --- Convert / CanConvert ---

func TestResource_CanConvert_BytesAndString(t *testing.T) {

	ctx := newTestCtx(t)

	r := newRes(t, ctx, ResourceSpec{ContentType: "json", Name: "cfg", Data: []byte("x")})

	if !r.CanConvertTo(reflect.TypeFor[[]byte]()) {
		t.Error("CanConvert([]byte) = false, want true")
	}
	if !r.CanConvertTo(reflect.TypeFor[string]()) {
		t.Error("CanConvert(string) = false, want true")
	}
	if r.CanConvertTo(reflect.TypeFor[int]()) {
		t.Error("CanConvert(int) = true, want false")
	}
}

func TestResource_Convert_ToBytes(t *testing.T) {

	ctx := newTestCtx(t)
	content := []byte(`{"key":"value"}`)

	r := newRes(t, ctx, ResourceSpec{ContentType: "json", Name: "cfg", Data: content})

	got, err := r.ConvertTo(reflect.TypeFor[[]byte]())
	if err != nil {
		t.Fatalf("Convert([]byte): %v", err)
	}

	gotBytes, ok := got.([]byte)
	if !ok {
		t.Fatalf("Convert([]byte) returned %T, want []byte", got)
	}
	if string(gotBytes) != string(content) {
		t.Errorf("Convert([]byte) = %q, want %q", gotBytes, content)
	}
}

func TestResource_Convert_ToString(t *testing.T) {

	ctx := newTestCtx(t)
	content := []byte("text content\nline two")

	r := newRes(t, ctx, ResourceSpec{ContentType: "template", Name: "t", Data: content})

	got, err := r.ConvertTo(reflect.TypeFor[string]())
	if err != nil {
		t.Fatalf("Convert(string): %v", err)
	}

	gotString, ok := got.(string)
	if !ok {
		t.Fatalf("Convert(string) returned %T, want string", got)
	}
	if gotString != string(content) {
		t.Errorf("Convert(string) = %q, want %q", gotString, content)
	}
}

func TestResource_Convert_UnsupportedTarget(t *testing.T) {

	ctx := newTestCtx(t)

	r := newRes(t, ctx, ResourceSpec{ContentType: "json", Name: "cfg", Data: []byte("x")})

	if _, err := r.ConvertTo(reflect.TypeFor[int]()); err == nil {
		t.Fatal("Convert(int) should error — not a supported target")
	}
}

// --- Hash determinism ---

func TestNewResource_Hash_Deterministic(t *testing.T) {

	ctx := newTestCtx(t)

	r1 := newRes(t, ctx, ResourceSpec{ContentType: "json", Name: "a", Data: []byte("same")})
	r2 := newRes(t, ctx, ResourceSpec{ContentType: "json", Name: "b", Data: []byte("same")})

	if r1.Hash != r2.Hash {
		t.Errorf("same data different hash: %q vs %q", r1.Hash, r2.Hash)
	}
}

func TestNewResource_Hash_DifferentData(t *testing.T) {

	ctx := newTestCtx(t)

	r1 := newRes(t, ctx, ResourceSpec{ContentType: "json", Name: "a", Data: []byte("one")})
	r2 := newRes(t, ctx, ResourceSpec{ContentType: "json", Name: "a", Data: []byte("two")})

	if r1.Hash == r2.Hash {
		t.Error("different data produced same hash")
	}
}

// --- Assertion + fixture helpers ---

// assertArchivedFileExists asserts the archive file at r.SourcePath exists on disk.
func assertArchivedFileExists(t *testing.T, r *Resource) {
	t.Helper()
	if _, err := os.Stat(r.SourcePath.Abs()); err != nil {
		t.Fatalf("archive file missing at %q: %v", r.SourcePath.Abs(), err)
	}
}

// assertArchivedFileAbsent asserts the archive file at r.SourcePath does NOT exist — used for
// metadata-only Resource constructions.
func assertArchivedFileAbsent(t *testing.T, r *Resource) {
	t.Helper()
	if _, err := os.Stat(r.SourcePath.Abs()); err == nil {
		t.Errorf("archive file at %q exists but should not (metadata-only Resource)", r.SourcePath.Abs())
	} else if !os.IsNotExist(err) {
		t.Fatalf("unexpected stat error on %q: %v", r.SourcePath.Abs(), err)
	}
}

// assertArchivedContent reads r's archived content and compares to want.
func assertArchivedContent(t *testing.T, r *Resource, want []byte) {
	t.Helper()

	rc, err := r.Reader()
	if err != nil {
		t.Fatalf("Reader: %v", err)
	}
	defer func() { _ = rc.Close() }()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("archived content = %q, want %q", got, want)
	}
}

// bytesOnly exposes Bytes() without implementing io.Reader — isolates the Bytes() dispatch branch.
type bytesOnly struct {
	content []byte
}

func (b *bytesOnly) Bytes() []byte { return b.content }

// binaryOnly implements encoding.BinaryMarshaler and nothing earlier in the switch.
type binaryOnly struct {
	content []byte
}

func (b *binaryOnly) MarshalBinary() ([]byte, error) { return b.content, nil }

// textOnly implements encoding.TextMarshaler and nothing earlier in the switch.
type textOnly struct {
	text []byte
}

func (t *textOnly) MarshalText() ([]byte, error) { return t.text, nil }

// erroringReader yields `after` zero bytes and then returns a synthetic error.
type erroringReader struct {
	after int
	read  int
	err   error
}

func (r *erroringReader) Read(p []byte) (int, error) {
	if r.read >= r.after {
		return 0, r.err
	}
	n := len(p)
	if r.read+n > r.after {
		n = r.after - r.read
	}
	for i := range n {
		p[i] = 0
	}
	r.read += n
	return n, nil
}

var errSyntheticRead = errors.New("synthetic read error")
