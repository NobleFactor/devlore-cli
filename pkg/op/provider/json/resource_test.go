// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package json

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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

func newTestCtx(t *testing.T) *op.RuntimeEnvironment {
	t.Helper()
	root := op.NewRootReaderWriter(t.TempDir())
	ctx := &op.RuntimeEnvironment{Root: root}
	ctx.RecoverySite = op.NewRecoverySite(ctx)
	ctx.Catalog = op.NewResourceCatalog()
	return ctx
}

func testActivation(t *testing.T, ctx *op.RuntimeEnvironment) *op.ActivationRecord {
	t.Helper()
	return op.NewActivationRecord(nil, nil, ctx)
}

// --- NewResource: bytes input ---

func TestNewResource_BytesHashesCanonical(t *testing.T) {
	ctx := newTestCtx(t)
	input := []byte(`{"a":1,"b":2}`)

	r, err := NewResource(testActivation(t, ctx), input)
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	want := sha256.Sum256([]byte(`{"a":1,"b":2}`))
	if r.Hash != hex.EncodeToString(want[:]) {
		t.Errorf("Hash = %q, want sha256 of canonical form", r.Hash)
	}
}

func TestNewResource_StampsProducerID(t *testing.T) {
	ctx := newTestCtx(t)
	activation := testActivation(t, ctx)

	r, err := NewResource(activation, []byte(`{"x":1}`))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	if got := r.ProducerID(); got != "" {
		t.Errorf("ProducerID = %q, want empty (nil Unit)", got)
	}
}

func TestNewResource_RejectsInvalidJSON(t *testing.T) {
	ctx := newTestCtx(t)
	if _, err := NewResource(testActivation(t, ctx), []byte(`{not valid`)); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestNewResource_RejectsUnsupportedType(t *testing.T) {
	ctx := newTestCtx(t)
	if _, err := NewResource(testActivation(t, ctx), 42); err == nil {
		t.Fatal("expected error for non-[]byte/non-string input")
	}
}

// --- io.Reader input ---

func TestNewResource_ReaderMatchesBytesURI(t *testing.T) {
	ctx := newTestCtx(t)
	activation := testActivation(t, ctx)

	fromBytes, err := NewResource(activation, []byte(`{"a":1,"b":2}`))
	if err != nil {
		t.Fatalf("bytes: %v", err)
	}

	fromReader, err := NewResource(activation, bytes.NewReader([]byte(`{"a":1,"b":2}`)))
	if err != nil {
		t.Fatalf("reader: %v", err)
	}

	if fromBytes.URI() != fromReader.URI() {
		t.Errorf("URI mismatch: bytes=%q reader=%q", fromBytes.URI(), fromReader.URI())
	}
}

// --- Canonicalization correctness gate ---

func TestNewResource_KeyOrderingDoesNotAffectURI(t *testing.T) {
	ctx := newTestCtx(t)
	activation := testActivation(t, ctx)

	r1, err := NewResource(activation, []byte(`{"a":1,"b":2}`))
	if err != nil {
		t.Fatalf("r1: %v", err)
	}
	r2, err := NewResource(activation, []byte(`{"b":2,"a":1}`))
	if err != nil {
		t.Fatalf("r2: %v", err)
	}

	if r1.URI() != r2.URI() {
		t.Errorf("URIs differ for semantically-equal inputs:\n  r1 = %q\n  r2 = %q", r1.URI(), r2.URI())
	}
}

func TestNewResource_WhitespaceDoesNotAffectURI(t *testing.T) {
	ctx := newTestCtx(t)
	activation := testActivation(t, ctx)

	r1, _ := NewResource(activation, []byte(`{"a":1,"b":2}`))
	r2, _ := NewResource(activation, []byte("  {\n  \"a\": 1,\n  \"b\": 2\n}\n"))

	if r1.URI() != r2.URI() {
		t.Errorf("URIs differ for whitespace variants:\n  r1 = %q\n  r2 = %q", r1.URI(), r2.URI())
	}
}

func TestNewResource_NestedKeyOrderingDoesNotAffectURI(t *testing.T) {
	ctx := newTestCtx(t)
	activation := testActivation(t, ctx)

	r1, _ := NewResource(activation, []byte(`{"outer":{"a":1,"b":2}}`))
	r2, _ := NewResource(activation, []byte(`{"outer":{"b":2,"a":1}}`))

	if r1.URI() != r2.URI() {
		t.Errorf("URIs differ for nested key-order variants:\n  r1 = %q\n  r2 = %q", r1.URI(), r2.URI())
	}
}

func TestNewResource_ArrayOrderingDoesAffectURI(t *testing.T) {
	ctx := newTestCtx(t)
	activation := testActivation(t, ctx)

	r1, _ := NewResource(activation, []byte(`[1,2,3]`))
	r2, _ := NewResource(activation, []byte(`[3,2,1]`))

	if r1.URI() == r2.URI() {
		t.Error("expected different URIs for differently-ordered arrays (arrays are ordered in JSON)")
	}
}

func TestNewResource_DataIsCanonical(t *testing.T) {
	ctx := newTestCtx(t)

	r, err := NewResource(testActivation(t, ctx), []byte("{  \"b\": 2,\n  \"a\": 1  }"))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	// Data should be the canonical bytes (sorted keys, no whitespace), NOT the original input.
	if !bytes.Equal(r.Data, []byte(`{"a":1,"b":2}`)) {
		t.Errorf("Data = %q, want canonical form %q", r.Data, `{"a":1,"b":2}`)
	}
}

func TestNewResource_ParsedAvailable(t *testing.T) {
	ctx := newTestCtx(t)

	r, _ := NewResource(testActivation(t, ctx), []byte(`{"a":1}`))
	parsed, ok := r.Parsed().(map[string]any)
	if !ok {
		t.Fatalf("Parsed() returned %T, want map[string]any", r.Parsed())
	}
	if parsed["a"] != float64(1) {
		t.Errorf("parsed[a] = %v, want 1", parsed["a"])
	}
}

// --- DiscoverResource ---

func TestDiscoverResource_RoundTripsURI(t *testing.T) {
	ctx := newTestCtx(t)
	original, _ := NewResource(testActivation(t, ctx), []byte(`{"x":1}`))

	discovered, err := DiscoverResource(op.NewActivationRecord(nil, nil, ctx), original.URI())
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
		"tag:devlore.noblefactor.com,2026-01-01:#github.com/NobleFactor/devlore-cli/pkg/op/provider/json.Resource",
		"tag:devlore.noblefactor.com,2026-01-01:sha256:abc#github.com/NobleFactor/devlore-cli/pkg/op/provider/json.Resource",
		"tag:devlore.noblefactor.com,2026-01-01:json:not-hex#github.com/NobleFactor/devlore-cli/pkg/op/provider/json.Resource",
	}

	for _, uri := range cases {
		if _, err := DiscoverResource(op.NewActivationRecord(nil, nil, ctx), uri); err == nil {
			t.Errorf("expected error for malformed URI %q", uri)
		}
	}
}

// --- Addressing / Digest / Etag ---

func TestAddressing_ReturnsContent(t *testing.T) {
	ctx := newTestCtx(t)
	r, _ := NewResource(testActivation(t, ctx), []byte(`{}`))
	if got := r.Addressing(); got != op.AddressingContent {
		t.Errorf("Addressing() = %v, want %v", got, op.AddressingContent)
	}
}

func TestDigest_MatchesHash(t *testing.T) {
	ctx := newTestCtx(t)
	r, _ := NewResource(testActivation(t, ctx), []byte(`{"k":1}`))

	d, err := r.Digest()
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}
	if d.Algorithm != "sha256" {
		t.Errorf("Algorithm = %q, want sha256", d.Algorithm)
	}
	wantBytes, _ := hex.DecodeString(r.Hash)
	if !bytes.Equal(d.Bytes, wantBytes) {
		t.Errorf("Bytes = %x, want %x", d.Bytes, wantBytes)
	}
}

// --- Equal ---

func TestEqual_SameContent(t *testing.T) {
	ctx := newTestCtx(t)
	activation := testActivation(t, ctx)

	r1, _ := NewResource(activation, []byte(`{"a":1}`))
	r2, _ := NewResource(activation, []byte(`{"a":1}`))
	if !r1.Equal(r2) {
		t.Error("expected r1.Equal(r2) for identical content")
	}
}

func TestEqual_DifferentContent(t *testing.T) {
	ctx := newTestCtx(t)
	activation := testActivation(t, ctx)

	r1, _ := NewResource(activation, []byte(`{"a":1}`))
	r2, _ := NewResource(activation, []byte(`{"a":2}`))
	if r1.Equal(r2) {
		t.Error("expected Equal to be false for distinct content")
	}
}

func TestEqual_RejectsNonResource(t *testing.T) {
	ctx := newTestCtx(t)
	r, _ := NewResource(testActivation(t, ctx), []byte(`{}`))

	if r.Equal("not a resource") {
		t.Error("expected Equal to reject non-*Resource")
	}
	if r.Equal(nil) {
		t.Error("expected Equal to reject nil")
	}
}


// --- Marshalers ---

func TestUnmarshalJSON_RehydratesFromURI(t *testing.T) {
	ctx := newTestCtx(t)
	original, _ := NewResource(testActivation(t, ctx), []byte(`{"m":1}`))

	data, _ := json.Marshal(original.URI())

	seeded, _ := DiscoverResource(op.NewActivationRecord(nil, nil, ctx), original.URI())
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
	if err := r.UnmarshalJSON([]byte(`"tag:..:json:abc#"`)); err == nil || !strings.Contains(err.Error(), "RuntimeEnvironment") {
		t.Errorf("expected RuntimeEnvironment error, got %v", err)
	}
}

func TestUnmarshalText_RehydratesFromURI(t *testing.T) {
	ctx := newTestCtx(t)
	original, _ := NewResource(testActivation(t, ctx), []byte(`{"t":1}`))

	seeded, _ := DiscoverResource(op.NewActivationRecord(nil, nil, ctx), original.URI())
	if err := seeded.UnmarshalText([]byte(original.URI())); err != nil {
		t.Fatalf("UnmarshalText: %v", err)
	}
	if seeded.URI() != original.URI() {
		t.Errorf("URI after unmarshal = %q, want %q", seeded.URI(), original.URI())
	}
}

func TestUnmarshalYAML_RehydratesFromURI(t *testing.T) {
	ctx := newTestCtx(t)
	original, _ := NewResource(testActivation(t, ctx), []byte(`{"y":1}`))

	seeded, _ := DiscoverResource(op.NewActivationRecord(nil, nil, ctx), original.URI())

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

// --- Validate (sanity, not exhaustive) ---

func TestValidate_AcceptsConformingDocument(t *testing.T) {
	ctx := newTestCtx(t)
	r, _ := NewResource(testActivation(t, ctx), []byte(`{"name":"x"}`))

	result, err := r.Validate(`{"type":"object","required":["name"]}`)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !result.Valid {
		t.Errorf("Valid = false, errors = %v", result.Errors)
	}
}

// Compile-time use of reflect to silence the import if no test needs it; keeps the file's import set stable
// against minor changes.
var _ = reflect.TypeFor[Resource]