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

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// --- Interface guards ---

func TestResource_ImplementsInterface(t *testing.T) {
	var _ op.Resource = (*Resource)(nil)
}

// --- Test helpers ---

// newTestRuntimeEnvironment returns a RuntimeEnvironment with a Root anchored at a fresh temp dir and a populated
// RecoverySite — the shape Resource construction requires when value is []byte or io.Reader.
func newTestRuntimeEnvironment(t *testing.T) *op.RuntimeEnvironment {
	t.Helper()
	root := fsroot.OpenWritableUnconfined(t.TempDir())
	runtimeEnvironment := &op.RuntimeEnvironment{Root: root}
	runtimeEnvironment.RecoverySite = op.NewRecoverySite(runtimeEnvironment)
	runtimeEnvironment.ResourceCatalog = op.NewResourceCatalog()
	return runtimeEnvironment
}

// testActivation wraps runtimeEnvironment in an [op.ActivationRecord] for non-graph dispatch. Graph and Unit are
// nil — production-claim calls produce Resources with empty producer stamps.
func testActivation(t *testing.T, runtimeEnvironment *op.RuntimeEnvironment) *op.ActivationRecord {
	t.Helper()
	return op.NewActivationRecord(nil, nil, runtimeEnvironment)
}

// sha256Hex returns the lowercase hex SHA-256 of data; used to assert digest equality.
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// --- NewResource: []byte ---

func TestNewResource_BytesHashesContent(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	payload := []byte("hello")

	r, err := NewResource(runtimeEnvironment, nil, payload)
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	if r.Hash != sha256Hex(payload) {
		t.Errorf("Hash = %q, want %q", r.Hash, sha256Hex(payload))
	}
}

func TestNewResource_BytesURIEncodesDigest(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	payload := []byte("uri test")

	r, err := NewResource(runtimeEnvironment, nil, payload)
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	want := "sha256:" + sha256Hex(payload)
	if got := r.ReachabilityURI(); got != want {
		t.Errorf("ReachabilityURI = %q, want %q", got, want)
	}
}

func TestNewResource_BytesContentReadback(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	payload := []byte("readback")

	r, err := NewResource(runtimeEnvironment, nil, payload)
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
	runtimeEnvironment := newTestRuntimeEnvironment(t)

	r, err := NewResource(runtimeEnvironment, nil, []byte{})
	if err != nil {
		t.Fatalf("NewResource(empty bytes): %v", err)
	}
	if r.Hash != sha256Hex([]byte{}) {
		t.Errorf("Hash for empty content = %q, want %q", r.Hash, sha256Hex([]byte{}))
	}
}

// --- NewResource: io.Reader ---

func TestNewResource_ReaderMatchesBytesURI(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	payload := []byte("identity")

	fromBytes, err := NewResource(runtimeEnvironment, nil, payload)
	if err != nil {
		t.Fatalf("bytes: %v", err)
	}

	fromReader, err := NewResource(runtimeEnvironment, nil, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("reader: %v", err)
	}

	if fromBytes.URI() != fromReader.URI() {
		t.Errorf("URI mismatch: bytes=%q reader=%q", fromBytes.URI(), fromReader.URI())
	}
}

func TestNewResource_ReaderContentReadback(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	payload := []byte("streamed content")

	r, err := NewResource(runtimeEnvironment, nil, bytes.NewReader(payload))
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
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	_, err := NewResource(runtimeEnvironment, nil, 42)
	if err == nil {
		t.Fatal("expected error for int input")
	}
}

func TestNewResource_RejectsNil(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	_, err := NewResource(runtimeEnvironment, nil, nil)
	if err == nil {
		t.Fatal("expected error for nil input")
	}
}

// --- NewResource: producer stamp ---

func TestNewResource_StampsProducerID(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	activation := testActivation(t, runtimeEnvironment)

	r, err := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("stamp"))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	if got := r.ProducerID(); got != "" {
		t.Errorf("ProducerID = %q, want empty (nil Unit)", got)
	}
}

func TestNewResource_NilCatalogReturnsUnlinkedCandidate(t *testing.T) {
	root := fsroot.OpenWritableUnconfined(t.TempDir())
	runtimeEnvironment := &op.RuntimeEnvironment{Root: root}

	r, err := NewResource(runtimeEnvironment, nil, []byte("no-catalog"))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil candidate")
	}
}

// --- CAS dedup ---

func TestNewResource_SameBytesSameURI(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	activation := testActivation(t, runtimeEnvironment)

	r1, err := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("dedup"))
	if err != nil {
		t.Fatalf("r1: %v", err)
	}
	r2, err := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("dedup"))
	if err != nil {
		t.Fatalf("r2: %v", err)
	}
	if r1.URI() != r2.URI() {
		t.Errorf("URI mismatch: %q vs %q", r1.URI(), r2.URI())
	}
}

func TestNewResource_DifferentBytesDifferentURI(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	activation := testActivation(t, runtimeEnvironment)

	r1, err := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("a"))
	if err != nil {
		t.Fatalf("r1: %v", err)
	}
	r2, err := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("b"))
	if err != nil {
		t.Fatalf("r2: %v", err)
	}
	if r1.URI() == r2.URI() {
		t.Errorf("URIs unexpectedly equal: %q", r1.URI())
	}
}

// --- SourcePath sharding ---

func TestSourcePath_ShardedLayout(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)

	r, err := NewResource(runtimeEnvironment, nil, []byte("shard test"))
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
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	activation := testActivation(t, runtimeEnvironment)

	original, err := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("roundtrip"))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	discovered, err := DiscoverResource(runtimeEnvironment, original.URI())
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
	runtimeEnvironment := newTestRuntimeEnvironment(t)

	cases := []string{
		"not a uri",
		"tag:devlore.noblefactor.com,2026-01-01:#github.com/NobleFactor/devlore-cli/pkg/op/provider/mem.Resource",
		"tag:devlore.noblefactor.com,2026-01-01:md5:abc#github.com/NobleFactor/devlore-cli/pkg/op/provider/mem.Resource",
		"tag:devlore.noblefactor.com,2026-01-01:sha256:not-hex#github.com/NobleFactor/devlore-cli/pkg/op/provider/mem.Resource",
	}

	for _, uri := range cases {
		if _, err := DiscoverResource(runtimeEnvironment, uri); err == nil {
			t.Errorf("expected error for malformed URI %q", uri)
		}
	}
}

// --- ConvertTo ---

func TestConvertTo_Bytes(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	payload := []byte("convert bytes")

	r, err := NewResource(runtimeEnvironment, nil, payload)
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
	runtimeEnvironment := newTestRuntimeEnvironment(t)

	r, err := NewResource(runtimeEnvironment, nil, []byte("convert string"))
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
	runtimeEnvironment := newTestRuntimeEnvironment(t)

	r, err := NewResource(runtimeEnvironment, nil, []byte("x"))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	_, err = r.ConvertTo(reflect.TypeFor[int]())
	if err == nil {
		t.Fatal("expected error for unsupported target")
	}
}

// --- CanConvertTo ---

func TestCanConvertTo_AcceptsBytesAndString(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	r, err := NewResource(runtimeEnvironment, nil, []byte("x"))
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
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	activation := testActivation(t, runtimeEnvironment)

	r1, err := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("eq"))
	if err != nil {
		t.Fatalf("r1: %v", err)
	}
	r2, err := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("eq"))
	if err != nil {
		t.Fatalf("r2: %v", err)
	}
	if !r1.Equal(r2) {
		t.Error("expected r1.Equal(r2) for byte-identical inputs")
	}
}

func TestEqual_DifferentBytes(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	activation := testActivation(t, runtimeEnvironment)

	r1, _ := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("a"))
	r2, _ := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("b"))
	if r1.Equal(r2) {
		t.Error("expected Equal to be false for distinct content")
	}
}

func TestEqual_RejectsNonResource(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	r, _ := NewResource(runtimeEnvironment, nil, []byte("x"))

	if r.Equal("not a resource") {
		t.Error("expected Equal to reject non-*Resource")
	}
	if r.Equal(nil) {
		t.Error("expected Equal to reject nil")
	}
}

// --- Marshalers (URI round-trip) ---

func TestUnmarshalJSON_RehydratesFromURI(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	activation := testActivation(t, runtimeEnvironment)

	original, err := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("marshal-json"))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	data, err := json.Marshal(original.URI())
	if err != nil {
		t.Fatalf("Marshal URI: %v", err)
	}

	seeded, err := DiscoverResource(runtimeEnvironment, original.URI())
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
	if err := r.UnmarshalJSON([]byte(`"tag:..:sha256:abc#"`)); err == nil ||
		!strings.Contains(err.Error(), "RuntimeEnvironment") {
		t.Errorf("expected RuntimeEnvironment error, got %v", err)
	}
}

func TestUnmarshalText_RehydratesFromURI(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	activation := testActivation(t, runtimeEnvironment)

	original, err := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("marshal-text"))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	seeded, err := DiscoverResource(runtimeEnvironment, original.URI())
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
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	activation := testActivation(t, runtimeEnvironment)

	original, err := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("marshal-yaml"))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	seeded, err := DiscoverResource(runtimeEnvironment, original.URI())
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

// --- Addressing / Digest ---

func TestAddressing_ReturnsContent(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	r, err := NewResource(runtimeEnvironment, nil, []byte("addressing"))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	if got := r.Addressing(); got != op.AddressingContent {
		t.Errorf("Addressing() = %v, want %v", got, op.AddressingContent)
	}
}

func TestDigest_MatchesHash(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	payload := []byte("digest test")

	r, err := NewResource(runtimeEnvironment, nil, payload)
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	d, err := r.Digest()
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}
	if d.Algorithm != "sha256" {
		t.Errorf("Algorithm = %q, want \"sha256\"", d.Algorithm)
	}

	wantBytes, err := hex.DecodeString(r.Hash)
	if err != nil {
		t.Fatalf("decode Hash: %v", err)
	}
	if !bytes.Equal(d.Bytes, wantBytes) {
		t.Errorf("Bytes = %x, want %x", d.Bytes, wantBytes)
	}
}

// --- Reader error path ---

func TestReader_RejectsMissingSourcePath(t *testing.T) {
	r := &Resource{}
	if _, err := r.Reader(); err == nil {
		t.Fatal("expected error for missing SourcePath")
	}
}
