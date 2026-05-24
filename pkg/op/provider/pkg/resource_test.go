// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import (
	"reflect"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/platform"
)

func resCtx(managerName string) *op.RuntimeEnvironment {
	mgr := &mockPackageManager{
		name:      managerName,
		installed: make(map[string]bool),
		versions:  make(map[string]string),
	}
	return &op.RuntimeEnvironment{
		Platform: &mockPlatform{
			defaultPM: mgr,
			available: map[string]platform.PackageManager{managerName: mgr},
		},
	}
}

// testActivation returns an [op.ActivationRecord] for non-graph dispatch. Graph and Unit are nil
// — Resources produced via this activation carry an empty producer stamp. Runtime is the
// resCtx-built environment (carries Platform; Catalog is nil so the unlinked candidate is returned).
func testActivation(t *testing.T, managerName string) *op.ActivationRecord {
	t.Helper()
	return op.NewActivationRecord(nil, nil, resCtx(managerName))
}

// --- NewResource ---

func TestNewResource(t *testing.T) {
	r, err := NewResource(testActivation(t, "apt"), "jq")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	if r.Name != "jq" {
		t.Errorf("Name = %q, want %q", r.Name, "jq")
	}
	if r.Type != "apt" {
		t.Errorf("Type = %q, want %q", r.Type, "apt")
	}
}

func TestNewResource_WithPrefix(t *testing.T) {
	r, err := NewResource(testActivation(t, "brew"), "brew:jq")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	if r.Name != "jq" {
		t.Errorf("Name = %q, want %q", r.Name, "jq")
	}
	if r.Type != "brew" {
		t.Errorf("Type = %q, want %q", r.Type, "brew")
	}
}

// --- URI ---

func TestResourceURI(t *testing.T) {
	r, err := NewResource(testActivation(t, "brew"), "jq")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	if got := r.ReachabilityURI(); got != "pkg:brew/jq" {
		t.Errorf("ReachabilityURI() = %q, want %q", got, "pkg:brew/jq")
	}
}

// --- Interface guards ---

func TestResourceImplementsInterface(t *testing.T) {
	var _ op.Resource = (*Resource)(nil)
}

func TestReceiptImplementsInterface(t *testing.T) {
	var _ op.Receipt = (*Receipt)(nil)
}

// --- Addressing ---

func TestResource_Addressing_IsLocation(t *testing.T) {

	r, err := NewResource(testActivation(t, "apt"), "jq")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	if got := r.Addressing(); got != op.AddressingLocation {
		t.Errorf("Addressing() = %v, want AddressingLocation", got)
	}
}

// --- Etag ---

func TestResource_Etag_NotInstalledIsEmpty(t *testing.T) {

	r, err := NewResource(testActivation(t, "apt"), "jq")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	etag, err := r.Etag()
	if err != nil {
		t.Fatalf("Etag: %v", err)
	}

	if etag != "" {
		t.Errorf("Etag of uninstalled package = %q, want \"\"", etag)
	}
}

func TestResource_Etag_InstalledReturnsVersion(t *testing.T) {

	mgr := &mockPackageManager{
		name:      "apt",
		installed: map[string]bool{"jq": true},
		versions:  map[string]string{"jq": "1.7.1"},
	}
	activation := op.NewActivationRecord(nil, nil, &op.RuntimeEnvironment{
		Platform: &mockPlatform{
			defaultPM: mgr,
			available: map[string]platform.PackageManager{"apt": mgr},
		},
	})

	r, err := NewResource(activation, "jq")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	etag, err := r.Etag()
	if err != nil {
		t.Fatalf("Etag: %v", err)
	}

	if etag != "1.7.1" {
		t.Errorf("Etag = %q, want %q", etag, "1.7.1")
	}
}

func TestResource_Etag_NoPlatformErrors(t *testing.T) {

	base, err := op.NewResourceBase(&op.RuntimeEnvironment{}, "pkg:apt/jq", reflect.TypeFor[*Resource]())
	if err != nil {
		t.Fatalf("NewResourceBase: %v", err)
	}
	r := &Resource{ResourceBase: base, Name: "jq", Type: "apt"}

	if _, err := r.Etag(); err == nil {
		t.Error("Etag on no-Platform runtime succeeded; want error")
	}
}

func TestResource_Etag_UnknownManagerErrors(t *testing.T) {

	mgr := &mockPackageManager{
		name:      "apt",
		installed: make(map[string]bool),
		versions:  make(map[string]string),
	}
	ctx := &op.RuntimeEnvironment{
		Platform: &mockPlatform{
			defaultPM: mgr,
			available: map[string]platform.PackageManager{"apt": mgr},
		},
	}
	base, err := op.NewResourceBase(ctx, "pkg:brew/jq", reflect.TypeFor[*Resource]())
	if err != nil {
		t.Fatalf("NewResourceBase: %v", err)
	}
	r := &Resource{ResourceBase: base, Name: "jq", Type: "brew"}

	if _, err := r.Etag(); err == nil {
		t.Error("Etag with unknown manager succeeded; want error")
	}
}

// --- Digest ---

func TestResource_Digest_StableAcrossCalls(t *testing.T) {

	r, err := NewResource(testActivation(t, "apt"), "jq")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	first, err := r.Digest()
	if err != nil {
		t.Fatalf("Digest (first): %v", err)
	}

	second, err := r.Digest()
	if err != nil {
		t.Fatalf("Digest (second): %v", err)
	}

	if !first.Equal(second) {
		t.Errorf("two Digest calls disagree: %s vs %s", first.String(), second.String())
	}
}

func TestResource_Digest_ChangesWithVersion(t *testing.T) {

	mgr := &mockPackageManager{
		name:      "apt",
		installed: map[string]bool{"jq": true},
		versions:  map[string]string{"jq": "1.7.1"},
	}
	activation := op.NewActivationRecord(nil, nil, &op.RuntimeEnvironment{
		Platform: &mockPlatform{
			defaultPM: mgr,
			available: map[string]platform.PackageManager{"apt": mgr},
		},
	})

	r, err := NewResource(activation, "jq")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	before, err := r.Digest()
	if err != nil {
		t.Fatalf("Digest (before): %v", err)
	}

	// Simulate an upgrade.
	mgr.versions["jq"] = "1.7.2"

	after, err := r.Digest()
	if err != nil {
		t.Fatalf("Digest (after): %v", err)
	}

	if before.Equal(after) {
		t.Errorf("Digest did not change after version bump: %s", before.String())
	}
}

func TestResource_Digest_DiffersAcrossPackages(t *testing.T) {

	a, err := NewResource(testActivation(t, "apt"), "jq")
	if err != nil {
		t.Fatalf("NewResource(jq): %v", err)
	}
	b, err := NewResource(testActivation(t, "apt"), "curl")
	if err != nil {
		t.Fatalf("NewResource(curl): %v", err)
	}

	dA, err := a.Digest()
	if err != nil {
		t.Fatalf("Digest(jq): %v", err)
	}
	dB, err := b.Digest()
	if err != nil {
		t.Fatalf("Digest(curl): %v", err)
	}

	if dA.Equal(dB) {
		t.Errorf("digests collided across distinct packages: %s", dA.String())
	}
}

func TestResource_Digest_RoundTripsThroughParseDigest(t *testing.T) {

	r, err := NewResource(testActivation(t, "apt"), "jq")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	got, err := r.Digest()
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}

	roundTrip, err := op.ParseDigest(got.String())
	if err != nil {
		t.Fatalf("ParseDigest(%q): %v", got.String(), err)
	}

	if !roundTrip.Equal(got) {
		t.Errorf("ParseDigest round-trip changed value: %s vs %s", roundTrip.String(), got.String())
	}
}

// --- Equal ---

func TestResource_Equal_StrictType(t *testing.T) {

	r, err := NewResource(testActivation(t, "apt"), "jq")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	if !r.Equal(r) {
		t.Error("Equal(self) returned false")
	}
	if r.Equal(nil) {
		t.Error("Equal(nil) returned true")
	}
	if r.Equal("not a resource") {
		t.Error("Equal(non-Resource) returned true")
	}
}
