// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import (
	"testing"
)

// debianPlatform seals a Debian [Platform] for the routing/resolution tests. Debian's single leaf is
// apt, whose purl type is "deb", so it exercises the name→type divergence the router resolves.
func debianPlatform(t *testing.T) Platform {

	t.Helper()

	p, err := New(Debian().WithArch("amd64"))
	if err != nil {
		t.Fatalf("New(Debian): %v", err)
	}
	return p
}

// region ResolvePurlType

func TestResolvePurlTypeByManagerName(t *testing.T) {

	p := debianPlatform(t)

	got, ok := p.ResolvePurlType("apt")
	if !ok {
		t.Fatal("ResolvePurlType(apt) reported unknown, want known")
	}
	if got != "deb" {
		t.Errorf("ResolvePurlType(apt) = %q, want deb", got)
	}
}

func TestResolvePurlTypeByPurlType(t *testing.T) {

	p := debianPlatform(t)

	got, ok := p.ResolvePurlType("deb")
	if !ok {
		t.Fatal("ResolvePurlType(deb) reported unknown, want known")
	}
	if got != "deb" {
		t.Errorf("ResolvePurlType(deb) = %q, want deb", got)
	}
}

func TestResolvePurlTypeUnknownPrefix(t *testing.T) {

	p := debianPlatform(t)

	got, ok := p.ResolvePurlType("nope")
	if ok {
		t.Error("ResolvePurlType(nope) reported known, want unknown")
	}
	if got != "" {
		t.Errorf("ResolvePurlType(nope) = %q, want \"\"", got)
	}
}

// endregion

// region DefaultPurlType

func TestDefaultPurlTypeIsNativeManager(t *testing.T) {

	p := debianPlatform(t)

	if got := p.DefaultPurlType(); got != "deb" {
		t.Errorf("DefaultPurlType = %q, want deb", got)
	}
}

// endregion

// region PackageManager routing

// TestPackageManagerRoutesUnknownTypeToFalse confirms the Composite router fails closed for a purl
// whose Type names no leaf on the platform: queries report absent rather than panicking or routing
// to an arbitrary leaf.
func TestPackageManagerRoutesUnknownTypeToFalse(t *testing.T) {

	router := debianPlatform(t).PackageManager()

	unknown := PURL{Type: "npm", Name: "left-pad"}

	if router.Installed(unknown) {
		t.Error("Installed(unknown type) = true, want false")
	}
	if got := router.Version(unknown); got != "" {
		t.Errorf("Version(unknown type) = %q, want \"\"", got)
	}
	if router.Available(unknown) {
		t.Error("Available(unknown type) = true, want false")
	}
}

// TestPackageManagerInstallReceiptsUnknownType confirms a mutating verb produces one failed receipt
// per package whose Type names no leaf, leaving the package's identity intact in the receipt.
func TestPackageManagerInstallReceiptsUnknownType(t *testing.T) {

	router := debianPlatform(t).PackageManager()

	unknown := PURL{Type: "npm", Name: "left-pad"}

	receipts, err := router.Install([]PURL{unknown}, nil)
	if err == nil {
		t.Error("Install(unknown type) returned nil error, want a routing error")
	}
	if len(receipts) != 1 {
		t.Fatalf("Install returned %d receipts, want 1", len(receipts))
	}
	if receipts[0].Err == nil {
		t.Error("receipt Err is nil, want a no-package-manager error")
	}
	if receipts[0].Purl.Name != "left-pad" {
		t.Errorf("receipt Purl.Name = %q, want left-pad", receipts[0].Purl.Name)
	}
}

// endregion
