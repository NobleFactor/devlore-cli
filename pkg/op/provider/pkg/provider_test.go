// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import (
	"reflect"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/platform"
)

// --- Mocks ---

type mockPackageManager struct {
	name       string
	installed  map[string]bool
	versions   map[string]string
	installErr string
	removeErr  string
	updateErr  string
}

func (m *mockPackageManager) Name() string            { return m.name }
func (m *mockPackageManager) Installed(n string) bool { return m.installed[n] }
func (m *mockPackageManager) Version(n string) string { return m.versions[n] }
func (m *mockPackageManager) Available(_ string) bool { return true }

func (m *mockPackageManager) Search(_ string, _ int) []platform.SearchResult { return nil }

func (m *mockPackageManager) Install(packages ...string) platform.PlatformResult {
	if m.installErr != "" {
		return platform.PlatformResult{OK: false, Stderr: m.installErr, Code: 1}
	}
	for _, packageName := range packages {
		m.installed[packageName] = true
	}
	return platform.PlatformResult{OK: true}
}

func (m *mockPackageManager) Remove(name string) platform.PlatformResult {
	if m.removeErr != "" {
		return platform.PlatformResult{OK: false, Stderr: m.removeErr, Code: 1}
	}
	delete(m.installed, name)
	return platform.PlatformResult{OK: true}
}

func (m *mockPackageManager) Update() platform.PlatformResult {
	if m.updateErr != "" {
		return platform.PlatformResult{OK: false, Stderr: m.updateErr, Code: 1}
	}
	return platform.PlatformResult{OK: true}
}

func (m *mockPackageManager) AddRepo(_, _, _ string) platform.PlatformResult {
	return platform.PlatformResult{OK: true}
}

func (m *mockPackageManager) NeedsSudo() bool { return false }
func (m *mockPackageManager) ParsePURL(id string) platform.PURL {
	return platform.PURL{Type: m.name, Name: id}
}

// mockPlatform is a minimal [platform.Platform] used by the package-provider tests. Only the
// methods the provider exercises are wired; the rest return zero values.
type mockPlatform struct {
	defaultPM platform.PackageManager
	available map[string]platform.PackageManager
}

func (m *mockPlatform) OS() string                                                { return "" }
func (m *mockPlatform) Arch() string                                              { return "" }
func (m *mockPlatform) Distro() string                                            { return "" }
func (m *mockPlatform) Version() string                                           { return "" }
func (m *mockPlatform) Hostname() string                                          { return "" }
func (m *mockPlatform) DefaultConcurrency() int                                   { return 1 }
func (m *mockPlatform) DefaultPackageManager() platform.PackageManager            { return m.defaultPM }
func (m *mockPlatform) AvailablePackageManagers() map[string]platform.PackageManager {
	return m.available
}
func (m *mockPlatform) PackageManagerByName(name string) platform.PackageManager {
	return m.available[name]
}
func (m *mockPlatform) InstalledBy(name string) platform.PackageManager {
	if m.defaultPM != nil && m.defaultPM.Installed(name) {
		return m.defaultPM
	}
	for _, pm := range m.available {
		if pm.Installed(name) {
			return pm
		}
	}
	return nil
}
func (m *mockPlatform) AllInstalledBy(name string) []platform.PackageManager {
	var out []platform.PackageManager
	for _, pm := range m.available {
		if pm.Installed(name) {
			out = append(out, pm)
		}
	}
	return out
}
func (m *mockPlatform) ServiceManager() platform.ServiceManager { return nil }

// --- Helpers ---

func newMockPackageManager() *mockPackageManager {
	return &mockPackageManager{
		name:      "apt",
		installed: make(map[string]bool),
		versions:  make(map[string]string),
	}
}

func newTestProvider(packageManager *mockPackageManager) *Provider {
	return &Provider{
		ProviderBase: op.NewProviderBase(&op.RuntimeEnvironment{
			Platform: &mockPlatform{
				defaultPM: packageManager,
				available: map[string]platform.PackageManager{packageManager.Name(): packageManager},
			},
		}),
	}
}

func res(name string) *Resource {
	base, err := op.NewResourceBase(&op.RuntimeEnvironment{}, "pkg:apt/"+name, reflect.TypeFor[*Resource]())
	assert.NoError("res("+name+")", err)
	return &Resource{
		ResourceBase: base,
		Name:         name,
	}
}

// --- Install Tests ---

func TestInstall(t *testing.T) {
	packageManager := newMockPackageManager()
	p := newTestProvider(packageManager)

	result, state, err := p.Install([]*Resource{res("vim"), res("git")}, "", false)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if len(result) != 2 || result[0].Name != "vim" || result[1].Name != "git" {
		t.Errorf("Install() result = %v, want [vim git]", result)
	}
	for _, r := range result {
		if r.Type != "apt" {
			t.Errorf("Install() result ProviderType = %q, want %q", r.Type, "apt")
		}
	}
	if len(state.Packages) != 2 || state.Packages[0] != "vim" || state.Packages[1] != "git" {
		t.Errorf("Install() state.Packages = %v, want [vim git]", state.Packages)
	}
	if len(state.AlreadyInstalled) != 0 {
		t.Errorf("Install() state.AlreadyInstalled = %v, want empty", state.AlreadyInstalled)
	}
	if !packageManager.installed["vim"] || !packageManager.installed["git"] {
		t.Error("Install() packages not marked installed in package manager")
	}
}

func TestInstallEmptyPackages(t *testing.T) {
	packageManager := newMockPackageManager()
	p := newTestProvider(packageManager)

	_, _, err := p.Install(nil, "", false)
	if err == nil {
		t.Fatal("Install(nil) expected error")
	}
	if err.Error() != "no packages specified" {
		t.Errorf("Install(nil) error = %q, want %q", err, "no packages specified")
	}
}

func TestInstallWithAlreadyInstalled(t *testing.T) {
	packageManager := newMockPackageManager()
	packageManager.installed["vim"] = true
	p := newTestProvider(packageManager)

	_, state, err := p.Install([]*Resource{res("vim"), res("git")}, "", false)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if len(state.AlreadyInstalled) != 1 || state.AlreadyInstalled[0] != "vim" {
		t.Errorf("Install() state.AlreadyInstalled = %v, want [vim]", state.AlreadyInstalled)
	}
}

func TestInstallError(t *testing.T) {
	packageManager := newMockPackageManager()
	packageManager.installErr = "disk full"
	p := newTestProvider(packageManager)

	_, _, err := p.Install([]*Resource{res("vim")}, "", false)
	if err == nil {
		t.Fatal("Install() expected error when package manager fails")
	}
	want := "apt install failed: disk full"
	if err.Error() != want {
		t.Errorf("Install() error = %q, want %q", err, want)
	}
}

// --- CompensateInstall Tests ---

func TestCompensateInstall(t *testing.T) {
	packageManager := newMockPackageManager()
	packageManager.installed["vim"] = true
	packageManager.installed["git"] = true
	p := newTestProvider(packageManager)

	state := Receipt{
		Packages:         []string{"vim", "git"},
		Manager:          "",
		Cask:             false,
		AlreadyInstalled: []string{"vim"},
	}
	err := p.CompensateInstall(&state)
	if err != nil {
		t.Fatalf("CompensateInstall() error = %v", err)
	}
	// vim was already installed, so it should remain.
	if !packageManager.installed["vim"] {
		t.Error("CompensateInstall() removed vim (was already_installed)")
	}
	// git was newly installed, so it should be removed.
	if packageManager.installed["git"] {
		t.Error("CompensateInstall() did not remove git (was newly installed)")
	}
}

func TestCompensateInstallEmptyState(t *testing.T) {
	packageManager := newMockPackageManager()
	p := newTestProvider(packageManager)

	err := p.CompensateInstall(nil)
	if err != nil {
		t.Fatalf("CompensateInstall(empty) error = %v", err)
	}
}

// --- Upgrade Tests ---

func TestUpgrade(t *testing.T) {
	packageManager := newMockPackageManager()
	packageManager.installed["vim"] = true
	packageManager.versions["vim"] = "8.2"
	p := newTestProvider(packageManager)

	result, state, err := p.Upgrade([]*Resource{res("vim")}, "", false)
	if err != nil {
		t.Fatalf("Upgrade() error = %v", err)
	}
	if len(result) != 1 || result[0].Name != "vim" {
		t.Errorf("Upgrade() result = %v, want [vim]", result)
	}
	if result[0].Type != "apt" {
		t.Errorf("Upgrade() result ProviderType = %q, want %q", result[0].Type, "apt")
	}
	if state.PreviousVersions["vim"] != "8.2" {
		t.Errorf("Upgrade() state.PreviousVersions[vim] = %q, want %q", state.PreviousVersions["vim"], "8.2")
	}
}

func TestUpgradeEmptyPackages(t *testing.T) {
	packageManager := newMockPackageManager()
	p := newTestProvider(packageManager)

	_, _, err := p.Upgrade(nil, "", false)
	if err == nil {
		t.Fatal("Upgrade(nil) expected error")
	}
	if err.Error() != "no packages specified" {
		t.Errorf("Upgrade(nil) error = %q, want %q", err, "no packages specified")
	}
}

// --- Remove Tests ---

func TestRemove(t *testing.T) {
	packageManager := newMockPackageManager()
	packageManager.installed["vim"] = true
	packageManager.installed["git"] = true
	p := newTestProvider(packageManager)

	result, state, err := p.Remove([]*Resource{res("vim"), res("git")}, "", false)
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if len(result) != 2 || result[0].Name != "vim" || result[1].Name != "git" {
		t.Errorf("Remove() result = %v, want [vim git]", result)
	}
	for _, r := range result {
		if r.Type != "apt" {
			t.Errorf("Remove() result ProviderType = %q, want %q", r.Type, "apt")
		}
	}
	if len(state.Packages) != 2 || state.Packages[0] != "vim" || state.Packages[1] != "git" {
		t.Errorf("Remove() state.Packages = %v, want [vim git]", state.Packages)
	}
	if packageManager.installed["vim"] || packageManager.installed["git"] {
		t.Error("Remove() packages still marked installed in package manager")
	}
}

// --- CompensateRemove Tests ---

func TestCompensateRemove(t *testing.T) {
	packageManager := newMockPackageManager()
	p := newTestProvider(packageManager)

	state := Receipt{
		Packages: []string{"vim", "git"},
		Manager:  "",
		Cask:     false,
	}
	err := p.CompensateRemove(&state)
	if err != nil {
		t.Fatalf("CompensateRemove() error = %v", err)
	}
	if !packageManager.installed["vim"] || !packageManager.installed["git"] {
		t.Error("CompensateRemove() did not reinstall packages")
	}
}

// --- Update Tests ---

func TestUpdate(t *testing.T) {
	packageManager := newMockPackageManager()
	p := newTestProvider(packageManager)

	name, err := p.Update("")
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if name != "apt" {
		t.Errorf("Update() = %q, want %q", name, "apt")
	}
}

func TestUpdateError(t *testing.T) {
	packageManager := newMockPackageManager()
	packageManager.updateErr = "network error"
	p := newTestProvider(packageManager)

	_, err := p.Update("")
	if err == nil {
		t.Fatal("Update() expected error")
	}
	want := "apt update failed: network error"
	if err.Error() != want {
		t.Errorf("Update() error = %q, want %q", err, want)
	}
}

// --- Predicate Tests ---

func TestPredicates(t *testing.T) {
	packageManager := newMockPackageManager()
	packageManager.installed["vim"] = true
	packageManager.versions["vim"] = "9.0"
	p := newTestProvider(packageManager)

	t.Run("Installed true", func(t *testing.T) {
		got, err := p.Installed(res("vim"))
		if err != nil {
			t.Fatalf("Installed() error = %v", err)
		}
		if !got {
			t.Error("Installed(vim) = false, want true")
		}
	})

	t.Run("Installed false", func(t *testing.T) {
		got, err := p.Installed(res("emacs"))
		if err != nil {
			t.Fatalf("Installed() error = %v", err)
		}
		if got {
			t.Error("Installed(emacs) = true, want false")
		}
	})

	t.Run("NotInstalled true", func(t *testing.T) {
		got, err := p.NotInstalled(res("emacs"))
		if err != nil {
			t.Fatalf("NotInstalled() error = %v", err)
		}
		if !got {
			t.Error("NotInstalled(emacs) = false, want true")
		}
	})

	t.Run("NotInstalled false", func(t *testing.T) {
		got, err := p.NotInstalled(res("vim"))
		if err != nil {
			t.Fatalf("NotInstalled() error = %v", err)
		}
		if got {
			t.Error("NotInstalled(vim) = true, want false")
		}
	})

	t.Run("VersionGTE true", func(t *testing.T) {
		got, err := p.VersionGTE(res("vim"), "8.0")
		if err != nil {
			t.Fatalf("VersionGTE() error = %v", err)
		}
		if !got {
			t.Error("VersionGTE(vim, 8.0) = false, want true")
		}
	})

	t.Run("VersionGTE false", func(t *testing.T) {
		got, err := p.VersionGTE(res("vim"), "9.1")
		if err != nil {
			t.Fatalf("VersionGTE() error = %v", err)
		}
		if got {
			t.Error("VersionGTE(vim, 9.1) = true, want false")
		}
	})

	t.Run("VersionGTE not installed", func(t *testing.T) {
		got, err := p.VersionGTE(res("emacs"), "1.0")
		if err != nil {
			t.Fatalf("VersionGTE() error = %v", err)
		}
		if got {
			t.Error("VersionGTE(emacs, 1.0) = true, want false (not installed)")
		}
	})
}
