// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
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

func newTestRuntimeEnvironment(t *testing.T) *op.RuntimeEnvironment {
	t.Helper()
	root := fsroot.OpenWritableUnconfined(t.TempDir())
	runtimeEnvironment := &op.RuntimeEnvironment{Root: root}
	runtimeEnvironment.RecoverySite = op.NewRecoverySite(runtimeEnvironment)
	runtimeEnvironment.ResourceCatalog = op.NewResourceCatalog()
	return runtimeEnvironment
}

func testActivation(t *testing.T, runtimeEnvironment *op.RuntimeEnvironment) *op.ActivationRecord {
	t.Helper()
	return op.NewActivationRecord(nil, nil, runtimeEnvironment)
}

// --- NewResource ---

func TestNewResource_FromName(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)

	r, err := NewResource(runtimeEnvironment, nil, "nginx")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	if r.Name != "nginx" {
		t.Errorf("Name = %q, want %q", r.Name, "nginx")
	}
	if got := r.ReachabilityURI(); got != "svc:nginx" {
		t.Errorf("ReachabilityURI = %q, want %q", got, "svc:nginx")
	}
}

func TestNewResource_FromTagURI(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)

	first, err := NewResource(runtimeEnvironment, nil, "sshd")
	if err != nil {
		t.Fatalf("NewResource(name): %v", err)
	}

	second, err := NewResource(runtimeEnvironment, nil, first.URI())
	if err != nil {
		t.Fatalf("NewResource(URI): %v", err)
	}
	if second.URI() != first.URI() {
		t.Errorf("URI from URI input = %q, want %q", second.URI(), first.URI())
	}
	if second.Name != "sshd" {
		t.Errorf("Name = %q, want %q", second.Name, "sshd")
	}
}

func TestNewResource_RejectsNonString(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	if _, err := NewResource(runtimeEnvironment, nil, 42); err == nil {
		t.Fatal("expected error for non-string input")
	}
}

func TestNewResource_StampsProducerID(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	activation := testActivation(t, runtimeEnvironment)

	r, err := NewResource(activation.RuntimeEnvironment, activation.Unit, "nginx")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	if got := r.ProducerID(); got != "" {
		t.Errorf("ProducerID = %q, want empty (nil Unit)", got)
	}
}

// --- Addressing / Digest ---

func TestAddressing_ReturnsLocation(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	r, _ := NewResource(runtimeEnvironment, nil, "nginx")
	if got := r.Addressing(); got != op.AddressingLocation {
		t.Errorf("Addressing() = %v, want %v", got, op.AddressingLocation)
	}
}

func TestDigest_HashesURI(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	r, err := NewResource(runtimeEnvironment, nil, "nginx")
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
	want := sha256.Sum256([]byte(r.URI()))
	if !bytes.Equal(d.Bytes, want[:]) {
		t.Errorf("Bytes = %x, want %x", d.Bytes, want[:])
	}
}

// --- Etag ---

func TestEtag_ReturnsURI(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	r, _ := NewResource(runtimeEnvironment, nil, "nginx")

	etag, err := r.Etag()
	if err != nil {
		t.Fatalf("Etag: %v", err)
	}
	if etag != r.URI() {
		t.Errorf("Etag = %q, want %q", etag, r.URI())
	}
}

// --- Equal ---

func TestEqual_SameName(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	activation := testActivation(t, runtimeEnvironment)

	r1, _ := NewResource(activation.RuntimeEnvironment, activation.Unit, "nginx")
	r2, _ := NewResource(activation.RuntimeEnvironment, activation.Unit, "nginx")
	if !r1.Equal(r2) {
		t.Error("expected r1.Equal(r2) for same-name resources")
	}
}

func TestEqual_DifferentName(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	activation := testActivation(t, runtimeEnvironment)

	r1, _ := NewResource(activation.RuntimeEnvironment, activation.Unit, "nginx")
	r2, _ := NewResource(activation.RuntimeEnvironment, activation.Unit, "sshd")
	if r1.Equal(r2) {
		t.Error("expected Equal to be false for distinct names")
	}
}

func TestEqual_RejectsNonResource(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	r, _ := NewResource(runtimeEnvironment, nil, "nginx")

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
	original, err := NewResource(runtimeEnvironment, nil, "nginx")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	data, err := json.Marshal(original.URI())
	if err != nil {
		t.Fatalf("Marshal URI: %v", err)
	}

	seeded, err := DiscoverResource(runtimeEnvironment, original.URI())
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := seeded.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if seeded.URI() != original.URI() {
		t.Errorf("URI after unmarshal = %q, want %q", seeded.URI(), original.URI())
	}
	if seeded.Name != "nginx" {
		t.Errorf("Name after unmarshal = %q, want %q", seeded.Name, "nginx")
	}
}

func TestUnmarshalJSON_RequiresRuntimeEnvironment(t *testing.T) {
	r := &Resource{}
	if err := r.UnmarshalJSON([]byte(`"tag:..:svc:nginx#"`)); err == nil ||
		!strings.Contains(err.Error(), "RuntimeEnvironment") {
		t.Errorf("expected RuntimeEnvironment error, got %v", err)
	}
}

func TestUnmarshalText_RehydratesFromURI(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	original, err := NewResource(runtimeEnvironment, nil, "sshd")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	seeded, err := DiscoverResource(runtimeEnvironment, original.URI())
	if err != nil {
		t.Fatalf("seed: %v", err)
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
	original, err := NewResource(runtimeEnvironment, nil, "postgres")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	seeded, err := DiscoverResource(runtimeEnvironment, original.URI())
	if err != nil {
		t.Fatalf("seed: %v", err)
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
