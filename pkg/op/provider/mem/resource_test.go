// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package mem

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
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

// newTestCtx returns a RuntimeEnvironment with a Root anchored at a fresh temp dir and a populated
// RecoverySite — the shape Resource construction requires when value is []byte or io.Reader.
func newTestCtx(t *testing.T) *op.RuntimeEnvironment {
	t.Helper()
	root := op.NewRootReaderWriter(t.TempDir())
	ctx := &op.RuntimeEnvironment{Root: root}
	ctx.RecoverySite = op.NewRecoverySite(ctx)
	ctx.Catalog = op.NewResourceCatalog()
	return ctx
}

// testActivation wraps ctx in an [op.ActivationRecord] with a test-derived SiteID. Sufficient for production-claim
// calls (non-nil + non-empty SiteID).
func testActivation(t *testing.T, ctx *op.RuntimeEnvironment) *op.ActivationRecord {
	t.Helper()
	return &op.ActivationRecord{Runtime: ctx, SiteID: "test:" + t.Name()}
}

// sha256Hex returns the lowercase hex SHA-256 of data; used to assert digest equality.
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// --- NewResource: []byte ---

func TestNewResource_BytesHashesContent(t *testing.T) {
	ctx := newTestCtx(t)
	payload := []byte("hello")

	r, err := NewResource(testActivation(t, ctx), payload)
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	if r.Hash != sha256Hex(payload) {
		t.Errorf("Hash = %q, want %q", r.Hash, sha256Hex(payload))
	}
}

func TestNewResource_BytesURIEncodesDigest(t *testing.T) {
	ctx := newTestCtx(t)
	payload := []byte("uri test")

	r, err := NewResource(testActivation(t, ctx), payload)
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	want := "sha256:" + sha256Hex(payload)
	if got := r.ReachabilityURI(); got != want {
		t.Errorf("ReachabilityURI = %q, want %q", got, want)
	}
}

func TestNewResource_BytesContentReadback(t *testing.T) {
	ctx := newTestCtx(t)
	payload := []byte("readback")

	r, err := NewResource(testActivation(t, ctx), payload)
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	rc, err := r.Reader()
	if err != nil {
		t.Fatalf("Reader: %v", err)
	}
	defer func() { _ = rc.Close() }()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("readback %q, want %q", got, payload)
	}
}

func TestNewResource_BytesEmpty(t *testing.T) {
	ctx := newTestCtx(t)

	r, err := NewResource(testActivation(t, ctx), []byte{})
	if err != nil {
		t.Fatalf("NewResource(empty bytes): %v", err)
	}
	if r.Hash != sha256Hex([]byte{}) {
		t.Errorf("Hash for empty content = %q, want %q", r.Hash, sha256Hex([]byte{}))
	}
}

// --- NewResource: io.Reader ---

func TestNewResource_ReaderMatchesBytesURI(t *testing.T) {
	ctx := newTestCtx(t)
	payload := []byte("identity")

	fromBytes, err := NewResource(testActivation(t, ctx), payload)
	if err != nil {
		t.Fatalf("bytes: %v", err)
	}

	fromReader, err := NewResource(testActivation(t, ctx), bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("reader: %v", err)
	}

	if fromBytes.URI() != fromReader.URI() {
		t.Errorf("URI mismatch: bytes=%q reader=%q", fromBytes.URI(), fromReader.URI())
	}
}

func TestNewResource_ReaderContentReadback(t *testing.T) {
	ctx := newTestCtx(t)
	payload := []byte("streamed content")

	r, err := NewResource(testActivation(t, ctx), bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	rc, err := r.Reader()
	if err != nil {
		t.Fatalf("Reader: %v", err)
	}
	defer func() { _ = rc.Close() }()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("readback %q, want %q", got, payload)
	}
}

// --- NewResource: dispatch errors ---

func TestNewResource_RejectsUnsupportedType(t *testing.T) {
	ctx := newTestCtx(t)
	_, err := NewResource(testActivation(t, ctx), 42)
	if err == nil {
		t.Fatal("expected error for int input")
	}
}

func TestNewResource_RejectsNil(t *testing.T) {
	ctx := newTestCtx(t)
	_, err := NewResource(testActivation(t, ctx), nil)
	if err == nil {
		t.Fatal("expected error for nil input")
	}
}

// --- NewResource: producer stamp ---

func TestNewResource_StampsProducerID(t *testing.T) {
	ctx := newTestCtx(t)
	activation := testActivation(t, ctx)

	r, err := NewResource(activation, []byte("stamp"))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	if got, want := r.ProducerID(), activation.SiteID; got != want {
		t.Errorf("ProducerID = %q, want %q", got, want)
	}
}

func TestNewResource_NilCatalogReturnsUnlinkedCandidate(t *testing.T) {
	root := op.NewRootReaderWriter(t.TempDir())
	ctx := &op.RuntimeEnvironment{Root: root}
	activation := &op.ActivationRecord{Runtime: ctx, SiteID: "test"}

	r, err := NewResource(activation, []byte("no-catalog"))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil candidate")
	}
}

// --- CAS dedup ---

func TestNewResource_SameBytesSameURI(t *testing.T) {
	ctx := newTestCtx(t)
	activation := testActivation(t, ctx)

	r1, err := NewResource(activation, []byte("dedup"))
	if err != nil {
		t.Fatalf("r1: %v", err)
	}
	r2, err := NewResource(activation, []byte("dedup"))
	if err != nil {
		t.Fatalf("r2: %v", err)
	}
	if r1.URI() != r2.URI() {
		t.Errorf("URI mismatch: %q vs %q", r1.URI(), r2.URI())
	}
}

func TestNewResource_DifferentBytesDifferentURI(t *testing.T) {
	ctx := newTestCtx(t)
	activation := testActivation(t, ctx)

	r1, err := NewResource(activation, []byte("a"))
	if err != nil {
		t.Fatalf("r1: %v", err)
	}
	r2, err := NewResource(activation, []byte("b"))
	if err != nil {
		t.Fatalf("r2: %v", err)
	}
	if r1.URI() == r2.URI() {
		t.Errorf("URIs unexpectedly equal: %q", r1.URI())
	}
}

// --- SourcePath sharding ---

func TestSourcePath_ShardedLayout(t *testing.T) {
	ctx := newTestCtx(t)

	r, err := NewResource(testActivation(t, ctx), []byte("shard test"))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	want := filepath.Join(".devlore", "mem", "resource", "sha256", r.Hash[0:2], r.Hash)
	if got := r.SourcePath().Rel(); got != want {
		t.Errorf("SourcePath = %q, want %q", got, want)
	}
}

// --- DiscoverResource ---

func TestDiscoverResource_RoundTripsURI(t *testing.T) {
	ctx := newTestCtx(t)
	activation := testActivation(t, ctx)

	original, err := NewResource(activation, []byte("roundtrip"))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	discovered, err := DiscoverResource(&op.ActivationRecord{Runtime: ctx}, original.URI())
	if err != nil {
		t.Fatalf("DiscoverResource: %v", err)
	}
	if discovered.URI() != original.URI() {
		t.Errorf("URI = %q, want %q", discovered.URI(), original.URI())
	}
	if discovered.Hash != original.Hash {
		t.Errorf("Hash = %q, want %q", discovered.Hash, original.Hash)
	}
}

func TestDiscoverResource_RejectsMalformedURI(t *testing.T) {
	ctx := newTestCtx(t)

	cases := []string{
		"not a uri",
		"tag:devlore.noblefactor.com,2026-01-01:#github.com/NobleFactor/devlore-cli/pkg/op/provider/mem.Resource",
		"tag:devlore.noblefactor.com,2026-01-01:md5:abc#github.com/NobleFactor/devlore-cli/pkg/op/provider/mem.Resource",
		"tag:devlore.noblefactor.com,2026-01-01:sha256:not-hex#github.com/NobleFactor/devlore-cli/pkg/op/provider/mem.Resource",
	}

	for _, uri := range cases {
		if _, err := DiscoverResource(&op.ActivationRecord{Runtime: ctx}, uri); err == nil {
			t.Errorf("expected error for malformed URI %q", uri)
		}
	}
}

// --- ConvertTo ---

func TestConvertTo_Bytes(t *testing.T) {
	ctx := newTestCtx(t)
	payload := []byte("convert bytes")

	r, err := NewResource(testActivation(t, ctx), payload)
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	got, err := r.ConvertTo(reflect.TypeFor[[]byte]())
	if err != nil {
		t.Fatalf("ConvertTo []byte: %v", err)
	}
	gotBytes, ok := got.([]byte)
	if !ok {
		t.Fatalf("ConvertTo returned %T, want []byte", got)
	}
	if !bytes.Equal(gotBytes, payload) {
		t.Errorf("got %q, want %q", gotBytes, payload)
	}
}

func TestConvertTo_String(t *testing.T) {
	ctx := newTestCtx(t)

	r, err := NewResource(testActivation(t, ctx), []byte("convert string"))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	got, err := r.ConvertTo(reflect.TypeFor[string]())
	if err != nil {
		t.Fatalf("ConvertTo string: %v", err)
	}
	if got.(string) != "convert string" {
		t.Errorf("got %q", got)
	}
}

func TestConvertTo_UnsupportedTarget(t *testing.T) {
	ctx := newTestCtx(t)

	r, err := NewResource(testActivation(t, ctx), []byte("x"))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	_, err = r.ConvertTo(reflect.TypeFor[int]())
	if err == nil {
		t.Fatal("expected error for unsupported target")
	}
}

// --- CanConvertTo ---

func TestCanConvertTo(t *testing.T) {
	ctx := newTestCtx(t)
	r, err := NewResource(testActivation(t, ctx), []byte("x"))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	cases := []struct {
		target reflect.Type
		want   bool
	}{
		{reflect.TypeFor[[]byte](), true},
		{reflect.TypeFor[string](), true},
		{reflect.TypeFor[int](), false},
	}

	for _, c := range cases {
		if got := r.CanConvertTo(c.target); got != c.want {
			t.Errorf("CanConvertTo(%s) = %v, want %v", c.target, got, c.want)
		}
	}
}

// --- Equal ---

func TestEqual_SameBytes(t *testing.T) {
	ctx := newTestCtx(t)
	activation := testActivation(t, ctx)

	r1, err := NewResource(activation, []byte("eq"))
	if err != nil {
		t.Fatalf("r1: %v", err)
	}
	r2, err := NewResource(activation, []byte("eq"))
	if err != nil {
		t.Fatalf("r2: %v", err)
	}
	if !r1.Equal(r2) {
		t.Error("expected r1.Equal(r2) for byte-identical inputs")
	}
}

func TestEqual_DifferentBytes(t *testing.T) {
	ctx := newTestCtx(t)
	activation := testActivation(t, ctx)

	r1, _ := NewResource(activation, []byte("a"))
	r2, _ := NewResource(activation, []byte("b"))
	if r1.Equal(r2) {
		t.Error("expected Equal to be false for distinct content")
	}
}

func TestEqual_RejectsNonResource(t *testing.T) {
	ctx := newTestCtx(t)
	r, _ := NewResource(testActivation(t, ctx), []byte("x"))

	if r.Equal("not a resource") {
		t.Error("expected Equal to reject non-*Resource")
	}
	if r.Equal(nil) {
		t.Error("expected Equal to reject nil")
	}
}

// --- Marshalers (URI round-trip) ---

func TestUnmarshalJSON_RehydratesFromURI(t *testing.T) {
	ctx := newTestCtx(t)
	activation := testActivation(t, ctx)

	original, err := NewResource(activation, []byte("marshal-json"))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	data, err := json.Marshal(original.URI())
	if err != nil {
		t.Fatalf("Marshal URI: %v", err)
	}

	seeded, err := DiscoverResource(&op.ActivationRecord{Runtime: ctx}, original.URI())
	if err != nil {
		t.Fatalf("DiscoverResource seed: %v", err)
	}

	if err := seeded.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if seeded.URI() != original.URI() {
		t.Errorf("URI after unmarshal = %q, want %q", seeded.URI(), original.URI())
	}
	if seeded.Hash != original.Hash {
		t.Errorf("Hash after unmarshal = %q, want %q", seeded.Hash, original.Hash)
	}
}

func TestUnmarshalJSON_RequiresRuntimeEnvironment(t *testing.T) {
	r := &Resource{}
	if err := r.UnmarshalJSON([]byte(`"tag:..:sha256:abc#"`)); err == nil || !strings.Contains(err.Error(), "RuntimeEnvironment") {
		t.Errorf("expected RuntimeEnvironment error, got %v", err)
	}
}

func TestUnmarshalText_RehydratesFromURI(t *testing.T) {
	ctx := newTestCtx(t)
	activation := testActivation(t, ctx)

	original, err := NewResource(activation, []byte("marshal-text"))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	seeded, err := DiscoverResource(&op.ActivationRecord{Runtime: ctx}, original.URI())
	if err != nil {
		t.Fatalf("DiscoverResource seed: %v", err)
	}

	if err := seeded.UnmarshalText([]byte(original.URI())); err != nil {
		t.Fatalf("UnmarshalText: %v", err)
	}
	if seeded.URI() != original.URI() {
		t.Errorf("URI after unmarshal = %q, want %q", seeded.URI(), original.URI())
	}
}

func TestUnmarshalYAML_RehydratesFromURI(t *testing.T) {
	ctx := newTestCtx(t)
	activation := testActivation(t, ctx)

	original, err := NewResource(activation, []byte("marshal-yaml"))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	seeded, err := DiscoverResource(&op.ActivationRecord{Runtime: ctx}, original.URI())
	if err != nil {
		t.Fatalf("DiscoverResource seed: %v", err)
	}

	target := original.URI()
	decode := func(v any) error {
		ptr, ok := v.(*string)
		if !ok {
			return errors.New("unsupported target")
		}
		*ptr = target
		return nil
	}

	if err := seeded.UnmarshalYAML(decode); err != nil {
		t.Fatalf("UnmarshalYAML: %v", err)
	}
	if seeded.URI() != original.URI() {
		t.Errorf("URI after unmarshal = %q, want %q", seeded.URI(), original.URI())
	}
}

// --- Reader error path ---

func TestReader_RejectsMissingSourcePath(t *testing.T) {
	r := &Resource{}
	if _, err := r.Reader(); err == nil {
		t.Fatal("expected error for missing SourcePath")
	}
}