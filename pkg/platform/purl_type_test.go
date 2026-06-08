// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import "testing"

// TestResolvePurlType verifies a manager prefix (a manager name or a purl type) resolves to the canonical purl type for
// each platform, and that an off-platform prefix does not resolve.
//
// The matrix spans every leaf across the named platform factories — apt→deb, dnf→rpm, pacman→alpm, brew→brew,
// port→port, winget→winget, flatpak→flatpak, snap→snap — plus the identity case (a purl type resolves to itself) and
// the negative case (a prefix whose manager is absent on the platform returns ("", false)).
func TestResolvePurlType(t *testing.T) {

	cases := []struct {
		name   string
		spec   *Spec
		prefix string
		want   string
		wantOK bool
	}{
		// Darwin: brew (default) + port.
		{"darwin/brew", Darwin(), "brew", "brew", true},
		{"darwin/port", Darwin(), "port", "port", true},
		{"darwin/off-platform apt", Darwin(), "apt", "", false},

		// Debian: apt → deb.
		{"debian/apt name", Debian(), "apt", "deb", true},
		{"debian/deb type", Debian(), "deb", "deb", true},
		{"debian/off-platform brew", Debian(), "brew", "", false},

		// Fedora: dnf → rpm, + flatpak.
		{"fedora/dnf name", Fedora(), "dnf", "rpm", true},
		{"fedora/rpm type", Fedora(), "rpm", "rpm", true},
		{"fedora/flatpak", Fedora(), "flatpak", "flatpak", true},

		// Ubuntu: apt → deb, + snap.
		{"ubuntu/apt name", Ubuntu(), "apt", "deb", true},
		{"ubuntu/snap", Ubuntu(), "snap", "snap", true},

		// Windows: winget.
		{"windows/winget", Windows(), "winget", "winget", true},

		// Arch: pacman → alpm (not "arch" — the canonical purl type is alpm).
		{"arch/pacman name", archSpec(), "pacman", "alpm", true},
		{"arch/alpm type", archSpec(), "alpm", "alpm", true},

		// Manjaro: pacman → alpm, + snap + flatpak.
		{"manjaro/pacman", manjaroSpec(), "pacman", "alpm", true},
		{"manjaro/flatpak", manjaroSpec(), "flatpak", "flatpak", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {

			p, err := New(c.spec)
			if err != nil {
				t.Fatalf("New(%s): %v", c.name, err)
			}

			got, ok := p.ResolvePurlType(c.prefix)
			if got != c.want || ok != c.wantOK {
				t.Errorf("ResolvePurlType(%q) = (%q, %v), want (%q, %v)", c.prefix, got, ok, c.want, c.wantOK)
			}
		})
	}
}

// TestDefaultPurlType verifies each platform's default purl type — the purl type of its default manager, used when a
// pkg.Resource identifier carries no manager prefix.
func TestDefaultPurlType(t *testing.T) {

	cases := []struct {
		name string
		spec *Spec
		want string
	}{
		{"darwin", Darwin(), "brew"},
		{"debian", Debian(), "deb"},
		{"fedora", Fedora(), "rpm"},
		{"ubuntu", Ubuntu(), "deb"},
		{"windows", Windows(), "winget"},
		{"arch", archSpec(), "alpm"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {

			p, err := New(c.spec)
			if err != nil {
				t.Fatalf("New(%s): %v", c.name, err)
			}

			if got := p.DefaultPurlType(); got != c.want {
				t.Errorf("DefaultPurlType() = %q, want %q", got, c.want)
			}
		})
	}
}
