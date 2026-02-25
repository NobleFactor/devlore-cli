// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import (
	"fmt"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// --- Mocks ---

type mockPM struct {
	name       string
	installed  map[string]bool
	versions   map[string]string
	installErr error
	removeErr  error
	updateErr  error
}

func (m *mockPM) Name() string            { return m.name }
func (m *mockPM) Installed(n string) bool { return m.installed[n] }
func (m *mockPM) Version(n string) string { return m.versions[n] }
func (m *mockPM) Available(_ string) bool { return true }

func (m *mockPM) Install(packages ...string) error {
	if m.installErr != nil {
		return m.installErr
	}
	for _, p := range packages {
		m.installed[p] = true
	}
	return nil
}

func (m *mockPM) Remove(name string) error {
	if m.removeErr != nil {
		return m.removeErr
	}
	delete(m.installed, name)
	return nil
}

func (m *mockPM) Update() error   { return m.updateErr }
func (m *mockPM) NeedsSudo() bool { return false }

type mockHost struct {
	pm  *mockPM
	svc op.ServiceManagerProvider
}

func (h *mockHost) PackageManager() op.PackageManagerProvider { return h.pm }

func (h *mockHost) InstalledBy(name string) op.PackageManagerProvider {
	if h.pm.Installed(name) {
		return h.pm
	}
	return nil
}

func (h *mockHost) AllInstalledBy(name string) []op.PackageManagerProvider {
	if h.pm.Installed(name) {
		return []op.PackageManagerProvider{h.pm}
	}
	return nil
}

func (h *mockHost) GetPackageManager(name string) op.PackageManagerProvider {
	if h.pm.Name() == name {
		return h.pm
	}
	return nil
}

func (h *mockHost) ServiceManager() op.ServiceManagerProvider { return h.svc }

// --- Helpers ---

func newMockPM() *mockPM {
	return &mockPM{
		name:      "apt",
		installed: make(map[string]bool),
		versions:  make(map[string]string),
	}
}

func newMockHost(pm *mockPM) *mockHost {
	return &mockHost{pm: pm}
}

// --- Install Tests ---

func TestInstall(t *testing.T) {
	pm := newMockPM()
	host := newMockHost(pm)
	p := &Provider{}

	result, state, err := p.Install(host, []string{"vim", "git"}, "", false)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if len(result) != 2 || result[0] != "vim" || result[1] != "git" {
		t.Errorf("Install() result = %v, want [vim git]", result)
	}
	if state == nil {
		t.Fatal("Install() state is nil")
	}
	packages := op.StateStringSlice(state, "packages")
	if len(packages) != 2 || packages[0] != "vim" || packages[1] != "git" {
		t.Errorf("Install() state.packages = %v, want [vim git]", packages)
	}
	alreadyInstalled := op.StateStringSlice(state, "already_installed")
	if len(alreadyInstalled) != 0 {
		t.Errorf("Install() state.already_installed = %v, want empty", alreadyInstalled)
	}
	if !pm.installed["vim"] || !pm.installed["git"] {
		t.Error("Install() packages not marked installed in pm")
	}
}

func TestInstallEmptyPackages(t *testing.T) {
	pm := newMockPM()
	host := newMockHost(pm)
	p := &Provider{}

	_, _, err := p.Install(host, nil, "", false)
	if err == nil {
		t.Fatal("Install(nil) expected error")
	}
	if err.Error() != "no packages specified" {
		t.Errorf("Install(nil) error = %q, want %q", err, "no packages specified")
	}
}

func TestInstallWithAlreadyInstalled(t *testing.T) {
	pm := newMockPM()
	pm.installed["vim"] = true
	host := newMockHost(pm)
	p := &Provider{}

	_, state, err := p.Install(host, []string{"vim", "git"}, "", false)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	alreadyInstalled := op.StateStringSlice(state, "already_installed")
	if len(alreadyInstalled) != 1 || alreadyInstalled[0] != "vim" {
		t.Errorf("Install() state.already_installed = %v, want [vim]", alreadyInstalled)
	}
}

func TestInstallError(t *testing.T) {
	pm := newMockPM()
	pm.installErr = fmt.Errorf("disk full")
	host := newMockHost(pm)
	p := &Provider{}

	_, _, err := p.Install(host, []string{"vim"}, "", false)
	if err == nil {
		t.Fatal("Install() expected error when pm.Install fails")
	}
	want := "apt install failed: disk full"
	if err.Error() != want {
		t.Errorf("Install() error = %q, want %q", err, want)
	}
}

// --- CompensateInstall Tests ---

func TestCompensateInstall(t *testing.T) {
	pm := newMockPM()
	pm.installed["vim"] = true
	pm.installed["git"] = true
	host := newMockHost(pm)
	p := &Provider{}

	state := map[string]any{
		"packages":          []string{"vim", "git"},
		"manager":           "",
		"cask":              false,
		"already_installed": []string{"vim"},
	}
	err := p.CompensateInstall(host, state)
	if err != nil {
		t.Fatalf("CompensateInstall() error = %v", err)
	}
	// vim was already installed, so it should remain.
	if !pm.installed["vim"] {
		t.Error("CompensateInstall() removed vim (was already_installed)")
	}
	// git was newly installed, so it should be removed.
	if pm.installed["git"] {
		t.Error("CompensateInstall() did not remove git (was newly installed)")
	}
}

func TestCompensateInstallNilState(t *testing.T) {
	host := newMockHost(newMockPM())
	p := &Provider{}

	err := p.CompensateInstall(host, nil)
	if err != nil {
		t.Fatalf("CompensateInstall(nil) error = %v", err)
	}
}

// --- Upgrade Tests ---

func TestUpgrade(t *testing.T) {
	pm := newMockPM()
	pm.installed["vim"] = true
	pm.versions["vim"] = "8.2"
	host := newMockHost(pm)
	p := &Provider{}

	result, state, err := p.Upgrade(host, []string{"vim"}, "", false)
	if err != nil {
		t.Fatalf("Upgrade() error = %v", err)
	}
	if len(result) != 1 || result[0] != "vim" {
		t.Errorf("Upgrade() result = %v, want [vim]", result)
	}
	if state == nil {
		t.Fatal("Upgrade() state is nil")
	}
	prevRaw, ok := state["previous_versions"]
	if !ok {
		t.Fatal("Upgrade() state missing previous_versions")
	}
	prev, ok := prevRaw.(map[string]string)
	if !ok {
		t.Fatalf("Upgrade() previous_versions type = %T, want map[string]string", prevRaw)
	}
	if prev["vim"] != "8.2" {
		t.Errorf("Upgrade() previous_versions[vim] = %q, want %q", prev["vim"], "8.2")
	}
}

func TestUpgradeEmptyPackages(t *testing.T) {
	pm := newMockPM()
	host := newMockHost(pm)
	p := &Provider{}

	_, _, err := p.Upgrade(host, nil, "", false)
	if err == nil {
		t.Fatal("Upgrade(nil) expected error")
	}
	if err.Error() != "no packages specified" {
		t.Errorf("Upgrade(nil) error = %q, want %q", err, "no packages specified")
	}
}

// --- Remove Tests ---

func TestRemove(t *testing.T) {
	pm := newMockPM()
	pm.installed["vim"] = true
	pm.installed["git"] = true
	host := newMockHost(pm)
	p := &Provider{}

	result, state, err := p.Remove(host, []string{"vim", "git"}, "", false)
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if len(result) != 2 || result[0] != "vim" || result[1] != "git" {
		t.Errorf("Remove() result = %v, want [vim git]", result)
	}
	if state == nil {
		t.Fatal("Remove() state is nil")
	}
	packages := op.StateStringSlice(state, "packages")
	if len(packages) != 2 || packages[0] != "vim" || packages[1] != "git" {
		t.Errorf("Remove() state.packages = %v, want [vim git]", packages)
	}
	if pm.installed["vim"] || pm.installed["git"] {
		t.Error("Remove() packages still marked installed in pm")
	}
}

// --- CompensateRemove Tests ---

func TestCompensateRemove(t *testing.T) {
	pm := newMockPM()
	host := newMockHost(pm)
	p := &Provider{}

	state := map[string]any{
		"packages": []string{"vim", "git"},
		"manager":  "",
		"cask":     false,
	}
	err := p.CompensateRemove(host, state)
	if err != nil {
		t.Fatalf("CompensateRemove() error = %v", err)
	}
	if !pm.installed["vim"] || !pm.installed["git"] {
		t.Error("CompensateRemove() did not reinstall packages")
	}
}

// --- Update Tests ---

func TestUpdate(t *testing.T) {
	pm := newMockPM()
	host := newMockHost(pm)
	p := &Provider{}

	name, err := p.Update(host, "")
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if name != "apt" {
		t.Errorf("Update() = %q, want %q", name, "apt")
	}
}

// --- Predicate Tests ---

func TestPredicates(t *testing.T) {
	pm := newMockPM()
	pm.installed["vim"] = true
	pm.versions["vim"] = "9.0"
	host := newMockHost(pm)
	p := &Provider{}

	t.Run("Installed true", func(t *testing.T) {
		got, err := p.Installed(host, "vim")
		if err != nil {
			t.Fatalf("Installed() error = %v", err)
		}
		if !got {
			t.Error("Installed(vim) = false, want true")
		}
	})

	t.Run("Installed false", func(t *testing.T) {
		got, err := p.Installed(host, "emacs")
		if err != nil {
			t.Fatalf("Installed() error = %v", err)
		}
		if got {
			t.Error("Installed(emacs) = true, want false")
		}
	})

	t.Run("NotInstalled true", func(t *testing.T) {
		got, err := p.NotInstalled(host, "emacs")
		if err != nil {
			t.Fatalf("NotInstalled() error = %v", err)
		}
		if !got {
			t.Error("NotInstalled(emacs) = false, want true")
		}
	})

	t.Run("NotInstalled false", func(t *testing.T) {
		got, err := p.NotInstalled(host, "vim")
		if err != nil {
			t.Fatalf("NotInstalled() error = %v", err)
		}
		if got {
			t.Error("NotInstalled(vim) = true, want false")
		}
	})

	t.Run("VersionGTE true", func(t *testing.T) {
		got, err := p.VersionGTE(host, "vim", "8.0")
		if err != nil {
			t.Fatalf("VersionGTE() error = %v", err)
		}
		if !got {
			t.Error("VersionGTE(vim, 8.0) = false, want true")
		}
	})

	t.Run("VersionGTE false", func(t *testing.T) {
		got, err := p.VersionGTE(host, "vim", "9.1")
		if err != nil {
			t.Fatalf("VersionGTE() error = %v", err)
		}
		if got {
			t.Error("VersionGTE(vim, 9.1) = true, want false")
		}
	})

	t.Run("VersionGTE not installed", func(t *testing.T) {
		got, err := p.VersionGTE(host, "emacs", "1.0")
		if err != nil {
			t.Fatalf("VersionGTE() error = %v", err)
		}
		if got {
			t.Error("VersionGTE(emacs, 1.0) = true, want false (not installed)")
		}
	})
}
