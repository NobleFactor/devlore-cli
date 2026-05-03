// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import (
	"testing"
)

// region PackageManagerByName

func TestPackageManagerByNameLooksUp(t *testing.T) {

	apt := &aptManager{}
	snap := &snapManager{}
	p := &platform{
		availablePackageManagers: map[string]PackageManager{
			"apt":  apt,
			"snap": snap,
		},
	}

	got := p.PackageManagerByName("apt")
	if got != apt {
		t.Errorf("PackageManagerByName(apt) = %v, want apt instance", got)
	}

	got = p.PackageManagerByName("snap")
	if got != snap {
		t.Errorf("PackageManagerByName(snap) = %v, want snap instance", got)
	}
}

func TestPackageManagerByNameReturnsNilForUnknown(t *testing.T) {

	p := &platform{
		availablePackageManagers: map[string]PackageManager{
			"apt": &aptManager{},
		},
	}

	got := p.PackageManagerByName("unknown")
	if got != nil {
		t.Errorf("PackageManagerByName(unknown) = %v, want nil", got)
	}
}

func TestPackageManagerByNameReturnsNilOnNilMap(t *testing.T) {

	p := &platform{}

	got := p.PackageManagerByName("apt")
	if got != nil {
		t.Errorf("PackageManagerByName on nil map = %v, want nil", got)
	}
}

// endregion

// region InstalledBy / AllInstalledBy

// fakePM is a [PackageManager] that reports a configurable set of packages as installed. Used for
// InstalledBy / AllInstalledBy tests where invoking the real shell-out methods would depend on host
// state.
type fakePM struct {
	name      string
	installed map[string]bool
}

func (f *fakePM) Name() string                  { return f.name }
func (f *fakePM) ParsePURL(string) PURL          { return PURL{} }
func (f *fakePM) Installed(name string) bool     { return f.installed[name] }
func (f *fakePM) Version(string) string          { return "" }
func (f *fakePM) Available(string) bool          { return false }
func (f *fakePM) Search(string, int) []SearchResult { return nil }
func (f *fakePM) Install(...string) PlatformResult  { return PlatformResult{} }
func (f *fakePM) Remove(string) PlatformResult      { return PlatformResult{} }
func (f *fakePM) Update() PlatformResult            { return PlatformResult{} }
func (f *fakePM) AddRepo(_, _, _ string) PlatformResult {
	return PlatformResult{}
}
func (f *fakePM) NeedsSudo() bool { return false }

func TestInstalledByChecksDefaultFirst(t *testing.T) {

	defaultPM := &fakePM{name: "apt", installed: map[string]bool{"jq": true}}
	other := &fakePM{name: "snap", installed: map[string]bool{"jq": true}}

	p := &platform{
		defaultPackageManager: defaultPM,
		availablePackageManagers: map[string]PackageManager{
			"apt":  defaultPM,
			"snap": other,
		},
	}

	got := p.InstalledBy("jq")
	if got != defaultPM {
		t.Errorf("InstalledBy returned %v, want defaultPM (default checked first)", got)
	}
}

func TestInstalledByFallsBackToOthers(t *testing.T) {

	defaultPM := &fakePM{name: "apt", installed: map[string]bool{}}
	other := &fakePM{name: "snap", installed: map[string]bool{"firefox": true}}

	p := &platform{
		defaultPackageManager: defaultPM,
		availablePackageManagers: map[string]PackageManager{
			"apt":  defaultPM,
			"snap": other,
		},
	}

	got := p.InstalledBy("firefox")
	if got != other {
		t.Errorf("InstalledBy returned %v, want other (default doesn't have it)", got)
	}
}

func TestInstalledByReturnsNilWhenAbsent(t *testing.T) {

	defaultPM := &fakePM{name: "apt", installed: map[string]bool{}}
	other := &fakePM{name: "snap", installed: map[string]bool{}}

	p := &platform{
		defaultPackageManager: defaultPM,
		availablePackageManagers: map[string]PackageManager{
			"apt":  defaultPM,
			"snap": other,
		},
	}

	got := p.InstalledBy("nonexistent")
	if got != nil {
		t.Errorf("InstalledBy returned %v, want nil", got)
	}
}

func TestAllInstalledByReturnsEverySource(t *testing.T) {

	apt := &fakePM{name: "apt", installed: map[string]bool{"git": true}}
	snap := &fakePM{name: "snap", installed: map[string]bool{"git": true}}
	flatpak := &fakePM{name: "flatpak", installed: map[string]bool{}}

	p := &platform{
		availablePackageManagers: map[string]PackageManager{
			"apt":     apt,
			"snap":    snap,
			"flatpak": flatpak,
		},
	}

	got := p.AllInstalledBy("git")
	if len(got) != 2 {
		t.Errorf("AllInstalledBy returned %d managers, want 2", len(got))
	}

	// Order is unspecified; build a set for comparison.
	names := make(map[string]bool, len(got))
	for _, m := range got {
		names[m.Name()] = true
	}
	if !names["apt"] || !names["snap"] {
		t.Errorf("AllInstalledBy returned %v, want apt and snap", got)
	}
}

func TestAllInstalledByReturnsEmptyWhenAbsent(t *testing.T) {

	p := &platform{
		availablePackageManagers: map[string]PackageManager{
			"apt": &fakePM{name: "apt", installed: map[string]bool{}},
		},
	}

	got := p.AllInstalledBy("nonexistent")
	if len(got) != 0 {
		t.Errorf("AllInstalledBy returned %v, want empty", got)
	}
}

// endregion
