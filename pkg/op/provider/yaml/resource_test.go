// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package yaml

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// --- Interface guards ---

func TestResourceImplementsInterface(t *testing.T) {
	var _ op.Resource = (*Resource)(nil)
}

// --- Test helpers ---

func newTestRuntimeEnvironment(t *testing.T) *op.RuntimeEnvironment {
	t.Helper()
	root := op.NewRootReaderWriter(t.TempDir())
	runtimeEnvironment := &op.RuntimeEnvironment{Root: root}
	runtimeEnvironment.RecoverySite = op.NewRecoverySite(runtimeEnvironment)
	runtimeEnvironment.ResourceCatalog = op.NewResourceCatalog()
	return runtimeEnvironment
}

func testActivation(t *testing.T, runtimeEnvironment *op.RuntimeEnvironment) *op.ActivationRecord {
	t.Helper()
	return op.NewActivationRecord(nil, nil, runtimeEnvironment)
}

// --- NewResource: bytes input ---

func TestNewResource_BytesHashesCanonicalJSON(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)

	r, err := NewResource(runtimeEnvironment, nil, []byte("a: 1\nb: 2\n"))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	// Canonical form is JSON with sorted keys, no whitespace.
	want := sha256.Sum256([]byte(`{"a":1,"b":2}`))
	if r.Hash != hex.EncodeToString(want[:]) {
		t.Errorf("Hash = %q, want sha256 of canonical JSON form", r.Hash)
	}
}

func TestNewResource_StampsProducerID(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	activation := testActivation(t, runtimeEnvironment)

	r, err := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("x: 1"))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	if got := r.ProducerID(); got != "" {
		t.Errorf("ProducerID = %q, want empty (nil Unit)", got)
	}
}

func TestNewResource_RejectsInvalidYAML(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	if _, err := NewResource(runtimeEnvironment, nil, []byte("a: [unclosed")); err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestNewResource_RejectsUnsupportedType(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	if _, err := NewResource(runtimeEnvironment, nil, 42); err == nil {
		t.Fatal("expected error for non-[]byte/non-string input")
	}
}

// --- io.Reader input ---

func TestNewResource_ReaderMatchesBytesURI(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	activation := testActivation(t, runtimeEnvironment)

	fromBytes, err := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("a: 1\nb: 2\n"))
	if err != nil {
		t.Fatalf("bytes: %v", err)
	}

	fromReader, err := NewResource(activation.RuntimeEnvironment, activation.Unit, bytes.NewReader([]byte("a: 1\nb: 2\n")))
	if err != nil {
		t.Fatalf("reader: %v", err)
	}

	if fromBytes.URI() != fromReader.URI() {
		t.Errorf("URI mismatch: bytes=%q reader=%q", fromBytes.URI(), fromReader.URI())
	}
}

// --- Canonicalization correctness gate ---

func TestNewResource_KeyOrderingDoesNotAffectURI(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	activation := testActivation(t, runtimeEnvironment)

	r1, _ := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("a: 1\nb: 2\n"))
	r2, _ := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("b: 2\na: 1\n"))

	if r1.URI() != r2.URI() {
		t.Errorf("URIs differ for semantically-equal YAML inputs:\n  r1 = %q\n  r2 = %q", r1.URI(), r2.URI())
	}
}

func TestNewResource_IndentationDoesNotAffectURI(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	activation := testActivation(t, runtimeEnvironment)

	r1, _ := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("outer:\n  a: 1\n  b: 2\n"))
	r2, _ := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("outer:\n    a: 1\n    b: 2\n"))

	if r1.URI() != r2.URI() {
		t.Errorf("URIs differ for different indentation:\n  r1 = %q\n  r2 = %q", r1.URI(), r2.URI())
	}
}

func TestNewResource_CommentsDoNotAffectURI(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	activation := testActivation(t, runtimeEnvironment)

	r1, _ := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("# leading comment\na: 1\n"))
	r2, _ := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("a: 1\n# trailing comment\n"))
	r3, _ := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("a: 1\n"))

	if r1.URI() != r3.URI() {
		t.Errorf("leading-comment URI differs from no-comment URI:\n  r1 = %q\n  r3 = %q", r1.URI(), r3.URI())
	}
	if r2.URI() != r3.URI() {
		t.Errorf("trailing-comment URI differs from no-comment URI:\n  r2 = %q\n  r3 = %q", r2.URI(), r3.URI())
	}
}

func TestNewResource_FlowAndBlockStyleEquivalent(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	activation := testActivation(t, runtimeEnvironment)

	r1, _ := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("a: 1\nb: 2\n"))
	r2, _ := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("{a: 1, b: 2}\n"))

	if r1.URI() != r2.URI() {
		t.Errorf("URIs differ for block-vs-flow style:\n  r1 = %q\n  r2 = %q", r1.URI(), r2.URI())
	}
}

func TestNewResource_DataIsCanonicalJSON(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)

	r, err := NewResource(runtimeEnvironment, nil, []byte("b: 2\na: 1\n"))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	if !bytes.Equal(r.Data, []byte(`{"a":1,"b":2}`)) {
		t.Errorf("Data = %q, want canonical JSON %q", r.Data, `{"a":1,"b":2}`)
	}
}

func TestNewResource_ParsedAvailable(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)

	r, _ := NewResource(runtimeEnvironment, nil, []byte("a: 1\n"))
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
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	original, _ := NewResource(runtimeEnvironment, nil, []byte("x: 1\n"))

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
		"tag:devlore.noblefactor.com,2026-01-01:#github.com/NobleFactor/devlore-cli/pkg/op/provider/yaml.Resource",
		"tag:devlore.noblefactor.com,2026-01-01:json:abc#github.com/NobleFactor/devlore-cli/pkg/op/provider/yaml.Resource",
		"tag:devlore.noblefactor.com,2026-01-01:yaml:not-hex#github.com/NobleFactor/devlore-cli/pkg/op/provider/yaml.Resource",
	}

	for _, uri := range cases {
		if _, err := DiscoverResource(runtimeEnvironment, uri); err == nil {
			t.Errorf("expected error for malformed URI %q", uri)
		}
	}
}

// --- Addressing / Digest / Etag ---

func TestAddressing_ReturnsContent(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	r, _ := NewResource(runtimeEnvironment, nil, []byte("a: 1"))
	if got := r.Addressing(); got != op.AddressingContent {
		t.Errorf("Addressing() = %v, want %v", got, op.AddressingContent)
	}
}

func TestDigest_MatchesHash(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	r, _ := NewResource(runtimeEnvironment, nil, []byte("k: 1\n"))

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
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	activation := testActivation(t, runtimeEnvironment)

	r1, _ := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("a: 1\n"))
	r2, _ := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("a: 1\n"))
	if !r1.Equal(r2) {
		t.Error("expected r1.Equal(r2) for identical content")
	}
}

func TestEqual_DifferentContent(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	activation := testActivation(t, runtimeEnvironment)

	r1, _ := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("a: 1\n"))
	r2, _ := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("a: 2\n"))
	if r1.Equal(r2) {
		t.Error("expected Equal to be false for distinct content")
	}
}

func TestEqual_RejectsNonResource(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	r, _ := NewResource(runtimeEnvironment, nil, []byte("a: 1\n"))

	if r.Equal("not a resource") {
		t.Error("expected Equal to reject non-*Resource")
	}
	if r.Equal(nil) {
		t.Error("expected Equal to reject nil")
	}
}

// --- Marshalers ---

func TestUnmarshalJSON_RehydratesFromURI(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	original, _ := NewResource(runtimeEnvironment, nil, []byte("m: 1\n"))

	data, _ := json.Marshal(original.URI())

	seeded, _ := DiscoverResource(runtimeEnvironment, original.URI())
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
	if err := r.UnmarshalJSON([]byte(`"tag:..:yaml:abc#"`)); err == nil || !strings.Contains(err.Error(), "RuntimeEnvironment") {
		t.Errorf("expected RuntimeEnvironment error, got %v", err)
	}
}

func TestUnmarshalText_RehydratesFromURI(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	original, _ := NewResource(runtimeEnvironment, nil, []byte("t: 1\n"))

	seeded, _ := DiscoverResource(runtimeEnvironment, original.URI())
	if err := seeded.UnmarshalText([]byte(original.URI())); err != nil {
		t.Fatalf("UnmarshalText: %v", err)
	}
	if seeded.URI() != original.URI() {
		t.Errorf("URI after unmarshal = %q, want %q", seeded.URI(), original.URI())
	}
}

func TestUnmarshalYAML_RehydratesFromURI(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	original, _ := NewResource(runtimeEnvironment, nil, []byte("y: 1\n"))

	seeded, _ := DiscoverResource(runtimeEnvironment, original.URI())

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

// --- Validate (sanity) ---

func TestValidate_AcceptsConformingDocument(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	r, _ := NewResource(runtimeEnvironment, nil, []byte("name: x\n"))

	result, err := r.Validate(`{"type":"object","required":["name"]}`)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !result.Valid {
		t.Errorf("Valid = false, errors = %v", result.Errors)
	}
}

// --- YAML/JSON canonical-form parity ---

func TestNewResource_YAMLAndJSONAgreeOnHash(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	activation := testActivation(t, runtimeEnvironment)

	// Semantically-equal YAML and JSON inputs should produce the same Hash (different URIs because the
	// scheme prefix differs, but the digest is over the same canonical-JSON bytes).
	fromYAML, _ := NewResource(activation.RuntimeEnvironment, activation.Unit, []byte("a: 1\nb: 2\n"))
	wantBytes := []byte(`{"a":1,"b":2}`)
	wantSum := sha256.Sum256(wantBytes)
	wantHash := hex.EncodeToString(wantSum[:])

	if fromYAML.Hash != wantHash {
		t.Errorf("YAML Hash = %q, want %q (sha256 of canonical JSON form)", fromYAML.Hash, wantHash)
	}
}
