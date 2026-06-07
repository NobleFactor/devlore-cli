// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/platform"
)

// --- Mocks ---

// mockPackageManager is an in-memory [platform.PackageManager] for the provider tests. It tracks installed state and
// versions by package name, and (since the mock platform routes a single leaf) ignores purl-type routing — it acts on
// whatever purls it is handed, tagging every receipt with its own `purlType`.
type mockPackageManager struct {
	purlType   string
	installed  map[string]bool
	versions   map[string]string
	installErr string
	removeErr  string
	updateErr  string
}

func (m *mockPackageManager) Install(packages []platform.PURL, _ map[string]any) ([]platform.Receipt, error) {

	receipts := make([]platform.Receipt, len(packages))

	for i, p := range packages {

		prior := m.versions[p.Name]

		receipts[i] = platform.Receipt{Purl: platform.PURL{Type: m.purlType, Name: p.Name}, PriorVersion: prior}

		if m.installErr != "" {
			receipts[i].Err = fmt.Errorf("%s install failed: %s", m.purlType, m.installErr)
			continue
		}

		m.installed[p.Name] = true
		receipts[i].Version = m.versions[p.Name]
	}

	if m.installErr != "" {
		return receipts, fmt.Errorf("%s install failed: %s", m.purlType, m.installErr)
	}

	return receipts, nil
}

func (m *mockPackageManager) Remove(packages []platform.PURL, _ map[string]any) ([]platform.Receipt, error) {

	receipts := make([]platform.Receipt, len(packages))

	for i, p := range packages {

		prior := m.versions[p.Name]

		receipts[i] = platform.Receipt{Purl: platform.PURL{Type: m.purlType, Name: p.Name}, PriorVersion: prior}

		if m.removeErr != "" {
			receipts[i].Err = fmt.Errorf("%s remove failed: %s", m.purlType, m.removeErr)
			continue
		}

		delete(m.installed, p.Name)
	}

	if m.removeErr != "" {
		return receipts, fmt.Errorf("%s remove failed: %s", m.purlType, m.removeErr)
	}

	return receipts, nil
}

func (m *mockPackageManager) Upgrade(packages []platform.PURL, kwargs map[string]any) ([]platform.Receipt, error) {
	return m.Install(packages, kwargs)
}

func (m *mockPackageManager) Update() error {
	if m.updateErr != "" {
		return fmt.Errorf("%s update failed: %s", m.purlType, m.updateErr)
	}
	return nil
}

func (m *mockPackageManager) Installed(p platform.PURL) bool { return m.installed[p.Name] }
func (m *mockPackageManager) Version(p platform.PURL) string { return m.versions[p.Name] }
func (m *mockPackageManager) Available(platform.PURL) bool   { return true }

func (m *mockPackageManager) Search(string, int) []platform.SearchResult { return nil }

// mockPlatform is a minimal [platform.Platform] used by the package-provider tests. It routes every purl to a single
// leaf and resolves every prefix to that leaf's purl type (identity routing).
type mockPlatform struct {
	manager *mockPackageManager
}

func (m *mockPlatform) OS() string              { return "" }
func (m *mockPlatform) Arch() string            { return "" }
func (m *mockPlatform) Distro() string          { return "" }
func (m *mockPlatform) Version() string         { return "" }
func (m *mockPlatform) Hostname() string        { return "" }
func (m *mockPlatform) DefaultConcurrency() int { return 1 }
func (m *mockPlatform) DefaultPurlType() string { return m.manager.purlType }
func (m *mockPlatform) ResolvePurlType(prefix string) (string, bool) {
	if prefix == m.manager.purlType {
		return m.manager.purlType, true
	}
	return "", false
}
func (m *mockPlatform) PackageManager() platform.PackageManager { return m.manager }
func (m *mockPlatform) ServiceManager() platform.ServiceManager { return nil }

// --- Helpers ---

func newMockPackageManager() *mockPackageManager {
	return &mockPackageManager{
		purlType:  "apt",
		installed: make(map[string]bool),
		versions:  make(map[string]string),
	}
}

func newTestProvider(packageManager *mockPackageManager) *Provider {
	return &Provider{
		ProviderBase: op.NewProviderBase(&op.RuntimeEnvironment{
			Platform: &mockPlatform{manager: packageManager},
		}),
	}
}

func res(name string) *Resource {
	base, err := op.NewResourceBase(&op.RuntimeEnvironment{}, "pkg:apt/"+name, reflect.TypeFor[*Resource]())
	assert.NoError("res("+name+")", err)
	return &Resource{
		ResourceBase: base,
		Name:         name,
		Type:         "apt",
	}
}

// --- Install Tests ---

func TestInstall(t *testing.T) {
	packageManager := newMockPackageManager()
	p := newTestProvider(packageManager)

	result, state, err := p.Install([]*Resource{res("vim"), res("git")}, nil)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if len(result) != 2 || result[0].Name != "vim" || result[1].Name != "git" {
		t.Errorf("Install() result = %v, want [vim git]", result)
	}
	for _, r := range result {
		if r.Type != "apt" {
			t.Errorf("Install() result Type = %q, want %q", r.Type, "apt")
		}
	}
	if len(state) != 2 {
		t.Fatalf("Install() state len = %d, want 2", len(state))
	}
	for _, s := range state {
		if s.InstalledBefore {
			t.Errorf("Install() receipt InstalledBefore = true, want false")
		}
		if s.Manager != "apt" {
			t.Errorf("Install() receipt Manager = %q, want apt", s.Manager)
		}
	}
	if !packageManager.installed["vim"] || !packageManager.installed["git"] {
		t.Error("Install() packages not marked installed in package manager")
	}
}

func TestInstallEmptyPackages(t *testing.T) {
	packageManager := newMockPackageManager()
	p := newTestProvider(packageManager)

	_, _, err := p.Install(nil, nil)
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
	packageManager.versions["vim"] = "8.2"
	p := newTestProvider(packageManager)

	_, state, err := p.Install([]*Resource{res("vim"), res("git")}, nil)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if !state[0].InstalledBefore {
		t.Errorf("Install() receipt[vim].InstalledBefore = false, want true")
	}
	if state[0].PreviousVersion != "8.2" {
		t.Errorf("Install() receipt[vim].PreviousVersion = %q, want 8.2", state[0].PreviousVersion)
	}
	if state[1].InstalledBefore {
		t.Errorf("Install() receipt[git].InstalledBefore = true, want false")
	}
}

func TestInstallError(t *testing.T) {
	packageManager := newMockPackageManager()
	packageManager.installErr = "disk full"
	p := newTestProvider(packageManager)

	_, _, err := p.Install([]*Resource{res("vim")}, nil)
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
	packageManager.versions["vim"] = "8.2"
	p := newTestProvider(packageManager)

	// vim was installed before, git was newly installed.
	state := []*Receipt{
		{ReceiptBase: op.NewReceiptBase(res("vim")), Manager: "apt", InstalledBefore: true, PreviousVersion: "8.2"},
		{ReceiptBase: op.NewReceiptBase(res("git")), Manager: "apt", InstalledBefore: false},
	}
	err := p.CompensateInstall(state)
	if err != nil {
		t.Fatalf("CompensateInstall() error = %v", err)
	}
	// vim was already installed, so it should remain.
	if !packageManager.installed["vim"] {
		t.Error("CompensateInstall() removed vim (was installed before)")
	}
	// git was newly installed, so it should be removed.
	if packageManager.installed["git"] {
		t.Error("CompensateInstall() did not remove git (was newly installed)")
	}
}

func TestCompensateInstallEmptyState(t *testing.T) {
	packageManager := newMockPackageManager()
	p := newTestProvider(packageManager)

	if err := p.CompensateInstall(nil); err != nil {
		t.Fatalf("CompensateInstall(nil) error = %v", err)
	}
}

func TestCompensateInstallRestoresDriftedVersion(t *testing.T) {
	packageManager := newMockPackageManager()
	// vim was present before at 8.2; the install drifted it to 9.0.
	packageManager.installed["vim"] = true
	packageManager.versions["vim"] = "9.0"
	p := newTestProvider(packageManager)

	state := []*Receipt{
		{ReceiptBase: op.NewReceiptBase(res("vim")), Manager: "apt", InstalledBefore: true, PreviousVersion: "8.2"},
	}
	if err := p.CompensateInstall(state); err != nil {
		t.Fatalf("CompensateInstall() error = %v", err)
	}
	// The drifted package is reinstalled at its prior version, so it remains installed (not removed).
	if !packageManager.installed["vim"] {
		t.Error("CompensateInstall() removed a pre-existing drifted package; want restore in place")
	}
}

func TestCompensateInstallLeavesUnchangedPreExisting(t *testing.T) {
	packageManager := newMockPackageManager()
	// vim was present before at 8.2 and the install did not change it.
	packageManager.installed["vim"] = true
	packageManager.versions["vim"] = "8.2"
	p := newTestProvider(packageManager)

	state := []*Receipt{
		{ReceiptBase: op.NewReceiptBase(res("vim")), Manager: "apt", InstalledBefore: true, PreviousVersion: "8.2"},
	}
	if err := p.CompensateInstall(state); err != nil {
		t.Fatalf("CompensateInstall() error = %v", err)
	}
	if !packageManager.installed["vim"] {
		t.Error("CompensateInstall() disturbed an unchanged pre-existing package")
	}
}

// --- Upgrade Tests ---

func TestUpgrade(t *testing.T) {
	packageManager := newMockPackageManager()
	packageManager.installed["vim"] = true
	packageManager.versions["vim"] = "8.2"
	p := newTestProvider(packageManager)

	result, state, err := p.Upgrade([]*Resource{res("vim")}, nil)
	if err != nil {
		t.Fatalf("Upgrade() error = %v", err)
	}
	if len(result) != 1 || result[0].Name != "vim" {
		t.Errorf("Upgrade() result = %v, want [vim]", result)
	}
	if result[0].Type != "apt" {
		t.Errorf("Upgrade() result Type = %q, want %q", result[0].Type, "apt")
	}
	if state[0].PreviousVersion != "8.2" {
		t.Errorf("Upgrade() state[vim].PreviousVersion = %q, want %q", state[0].PreviousVersion, "8.2")
	}
}

func TestUpgradeEmptyPackages(t *testing.T) {
	packageManager := newMockPackageManager()
	p := newTestProvider(packageManager)

	_, _, err := p.Upgrade(nil, nil)
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

	result, state, err := p.Remove([]*Resource{res("vim"), res("git")}, nil)
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if len(result) != 2 || result[0].Name != "vim" || result[1].Name != "git" {
		t.Errorf("Remove() result = %v, want [vim git]", result)
	}
	for _, r := range result {
		if r.Type != "apt" {
			t.Errorf("Remove() result Type = %q, want %q", r.Type, "apt")
		}
	}
	if len(state) != 2 {
		t.Fatalf("Remove() state len = %d, want 2", len(state))
	}
	if packageManager.installed["vim"] || packageManager.installed["git"] {
		t.Error("Remove() packages still marked installed in package manager")
	}
}

// --- CompensateRemove Tests ---

func TestCompensateRemove(t *testing.T) {
	packageManager := newMockPackageManager()
	p := newTestProvider(packageManager)

	// Both packages were present before the removal.
	state := []*Receipt{
		{ReceiptBase: op.NewReceiptBase(res("vim")), Manager: "apt", InstalledBefore: true},
		{ReceiptBase: op.NewReceiptBase(res("git")), Manager: "apt", InstalledBefore: true},
	}
	err := p.CompensateRemove(state)
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

	if err := p.Update(); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
}

func TestUpdateError(t *testing.T) {
	packageManager := newMockPackageManager()
	packageManager.updateErr = "network error"
	p := newTestProvider(packageManager)

	err := p.Update()
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
