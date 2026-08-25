// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// --- Interface guards ---

func TestResource_ImplementsInterface(t *testing.T) {
	var _ op.Resource = (*resource)(nil)
}

// --- Test helpers ---

func newTestRuntimeEnvironment(t *testing.T) *op.RuntimeEnvironment {
	t.Helper()

	runtimeEnvironment, err := op.NewRuntimeEnvironment(context.Background(),
		op.NewRuntimeEnvironmentSpec("test").
			WithRoot(t.TempDir()).
			WithApplication(&application.Application{Name: "test"}))
	if err != nil {
		t.Fatalf("op.NewRuntimeEnvironment: %v", err)
	}
	t.Cleanup(func() { _ = runtimeEnvironment.Close() })

	return runtimeEnvironment
}

func testActivation(t *testing.T, runtimeEnvironment *op.RuntimeEnvironment) *op.ActivationRecord {
	t.Helper()
	return op.NewActivationRecord(nil, "", runtimeEnvironment)
}

// concrete reaches the struct behind a sealed [Resource].
//
// [op.Resource] declares neither ReachabilityURI nor the marshalers, so the sealed interface does not expose
// them either — before sealing they were reachable only because embedding leaked [op.ResourceBase]'s whole
// surface. Every caller of those methods is in this package, so a white-box test asserting to the
// implementation is the honest way to reach them rather than widening the exported contract.
func concrete(t *testing.T, r Resource) *resource {
	t.Helper()
	c, ok := r.(*resource)
	if !ok {
		t.Fatalf("Resource is %T, want *resource", r)
	}
	return c
}

// --- NewResource ---

func TestNewResource_FromName(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)

	r, err := NewResource(runtimeEnvironment, "", "nginx")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	if r.Name() != "nginx" {
		t.Errorf("Name = %q, want %q", r.Name(), "nginx")
	}
	if got := concrete(t, r).ReachabilityURI(); got != "svc:nginx" {
		t.Errorf("ReachabilityURI = %q, want %q", got, "svc:nginx")
	}
}

func TestNewResource_FromTagURI(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)

	first, err := NewResource(runtimeEnvironment, "", "sshd")
	if err != nil {
		t.Fatalf("NewResource(name): %v", err)
	}

	second, err := NewResource(runtimeEnvironment, "", first.URI())
	if err != nil {
		t.Fatalf("NewResource(URI): %v", err)
	}
	if second.URI() != first.URI() {
		t.Errorf("URI from URI input = %q, want %q", second.URI(), first.URI())
	}
	if second.Name() != "sshd" {
		t.Errorf("Name = %q, want %q", second.Name(), "sshd")
	}
}

func TestNewResource_RejectsNonString(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	if _, err := NewResource(runtimeEnvironment, "", 42); err == nil {
		t.Fatal("expected error for non-string input")
	}
}

func TestNewResource_StampsProducerID(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	activation := testActivation(t, runtimeEnvironment)

	r, err := NewResource(activation.RuntimeEnvironment, activation.CallerID, "nginx")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	if got := r.ProducerID(); got != "" {
		t.Errorf("ProducerID = %q, want empty (no caller id)", got)
	}
}

// --- Addressing / Digest ---

func TestAddressing_ReturnsLocation(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	r, _ := NewResource(runtimeEnvironment, "", "nginx")
	if got := r.Addressing(); got != op.AddressingLocation {
		t.Errorf("Addressing() = %v, want %v", got, op.AddressingLocation)
	}
}

func TestDigest_HashesURI(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	r, err := NewResource(runtimeEnvironment, "", "nginx")
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
	r, _ := NewResource(runtimeEnvironment, "", "nginx")

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

	r1, _ := NewResource(activation.RuntimeEnvironment, activation.CallerID, "nginx")
	r2, _ := NewResource(activation.RuntimeEnvironment, activation.CallerID, "nginx")
	if !concrete(t, r1).Equal(r2) {
		t.Error("expected concrete(t, r1).Equal(r2) for same-name resources")
	}
}

func TestEqual_DifferentName(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	activation := testActivation(t, runtimeEnvironment)

	r1, _ := NewResource(activation.RuntimeEnvironment, activation.CallerID, "nginx")
	r2, _ := NewResource(activation.RuntimeEnvironment, activation.CallerID, "sshd")
	if concrete(t, r1).Equal(r2) {
		t.Error("expected Equal to be false for distinct names")
	}
}

func TestEqual_RejectsNonResource(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	r, _ := NewResource(runtimeEnvironment, "", "nginx")

	if concrete(t, r).Equal("not a resource") {
		t.Error("expected Equal to reject non-Resource")
	}
	if concrete(t, r).Equal(nil) {
		t.Error("expected Equal to reject nil")
	}
}

// --- Marshalers (URI round-trip) ---

func TestUnmarshalJSON_RehydratesFromURI(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	original, err := NewResource(runtimeEnvironment, "", "nginx")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	data, err := json.Marshal(original.URI())
	if err != nil {
		t.Fatalf("Marshal URI: %v", err)
	}

	seededResource, err := DiscoverResource(runtimeEnvironment, original.URI())
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	seeded := concrete(t, seededResource)

	if err := seeded.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if seeded.URI() != original.URI() {
		t.Errorf("URI after unmarshal = %q, want %q", seeded.URI(), original.URI())
	}
	if seeded.Name() != "nginx" {
		t.Errorf("Name after unmarshal = %q, want %q", seeded.Name(), "nginx")
	}
}

func TestUnmarshalJSON_RequiresRuntimeEnvironment(t *testing.T) {
	r := &resource{}
	if err := concrete(t, r).UnmarshalJSON([]byte(`"tag:..:svc:nginx#"`)); err == nil ||
		!strings.Contains(err.Error(), "RuntimeEnvironment") {
		t.Errorf("expected RuntimeEnvironment error, got %v", err)
	}
}

func TestUnmarshalText_RehydratesFromURI(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	original, err := NewResource(runtimeEnvironment, "", "sshd")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	seededResource, err := DiscoverResource(runtimeEnvironment, original.URI())
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	seeded := concrete(t, seededResource)

	if err := seeded.UnmarshalText([]byte(original.URI())); err != nil {
		t.Fatalf("UnmarshalText: %v", err)
	}
	if seeded.URI() != original.URI() {
		t.Errorf("URI after unmarshal = %q, want %q", seeded.URI(), original.URI())
	}
}

func TestUnmarshalYAML_RehydratesFromURI(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	original, err := NewResource(runtimeEnvironment, "", "postgres")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	seededResource, err := DiscoverResource(runtimeEnvironment, original.URI())
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	seeded := concrete(t, seededResource)

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
