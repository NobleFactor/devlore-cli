// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import (
	"testing"
)

// mockProvider returns a Provider with test hooks that record calls
// instead of executing real package manager commands.
func mockProvider(installed map[string]bool, versions map[string]string) (*Provider, *[]string) {
	var log []string
	return &Provider{
		isInstalledFn: func(pkg, _ string) bool {
			return installed[pkg]
		},
		getVersionFn: func(pkg, _ string) string {
			return versions[pkg]
		},
		installFn: func(packages []string, manager string, cask bool) error {
			for _, p := range packages {
				log = append(log, "install "+p)
			}
			return nil
		},
		upgradeFn: func(packages []string, manager string, cask bool) error {
			for _, p := range packages {
				log = append(log, "upgrade "+p)
			}
			return nil
		},
		removeFn: func(packages []string, manager string, cask bool) error {
			for _, p := range packages {
				log = append(log, "remove "+p)
			}
			return nil
		},
	}, &log
}

// --- Install ---

func TestInstallNoneInstalled(t *testing.T) {
	p, log := mockProvider(map[string]bool{}, nil)
	_, state, err := p.Install([]string{"curl", "wget"}, "brew", false)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	already, _ := state["already_installed"].([]string)
	if len(already) != 0 {
		t.Errorf("expected no already_installed, got %v", already)
	}

	// Compensate: should remove both (neither was installed before)
	if err := p.CompensateInstall(state); err != nil {
		t.Fatalf("CompensateInstall: %v", err)
	}
	if len(*log) != 4 { // 2 install + 2 remove
		t.Fatalf("expected 4 commands, got %d: %v", len(*log), *log)
	}
	if (*log)[2] != "remove curl" || (*log)[3] != "remove wget" {
		t.Errorf("expected remove curl, remove wget; got %v", (*log)[2:])
	}
}

func TestInstallSomeAlreadyInstalled(t *testing.T) {
	p, log := mockProvider(map[string]bool{"curl": true}, nil)
	_, state, err := p.Install([]string{"curl", "wget"}, "brew", false)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	already, _ := state["already_installed"].([]string)
	if len(already) != 1 || already[0] != "curl" {
		t.Errorf("expected already_installed=[curl], got %v", already)
	}

	// Compensate: should only remove wget (curl was already installed)
	if err := p.CompensateInstall(state); err != nil {
		t.Fatalf("CompensateInstall: %v", err)
	}
	if len(*log) != 3 { // 2 install + 1 remove
		t.Fatalf("expected 3 commands, got %d: %v", len(*log), *log)
	}
	if (*log)[2] != "remove wget" {
		t.Errorf("expected remove wget, got %q", (*log)[2])
	}
}

func TestInstallAllAlreadyInstalled(t *testing.T) {
	p, log := mockProvider(map[string]bool{"curl": true, "wget": true}, nil)
	_, state, err := p.Install([]string{"curl", "wget"}, "brew", false)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Compensate: no-op (all were already installed)
	if err := p.CompensateInstall(state); err != nil {
		t.Fatalf("CompensateInstall: %v", err)
	}
	if len(*log) != 2 { // 2 install only, no removes
		t.Errorf("expected 2 commands (install only), got %d: %v", len(*log), *log)
	}
}

func TestInstallEmptyPackages(t *testing.T) {
	p, _ := mockProvider(nil, nil)
	_, _, err := p.Install(nil, "brew", false)
	if err == nil {
		t.Error("expected error for empty packages")
	}
}

// --- Upgrade ---

func TestUpgradeCapturesVersions(t *testing.T) {
	p, _ := mockProvider(nil, map[string]string{"curl": "7.88", "wget": "1.21"})
	_, state, err := p.Upgrade([]string{"curl", "wget"}, "brew", false)
	if err != nil {
		t.Fatalf("Upgrade: %v", err)
	}

	versions, _ := state["previous_versions"].(map[string]string)
	if versions["curl"] != "7.88" || versions["wget"] != "1.21" {
		t.Errorf("expected previous versions, got %v", versions)
	}
}

func TestUpgradeCompensateNoOp(t *testing.T) {
	p, _ := mockProvider(nil, map[string]string{"curl": "7.88"})
	_, state, err := p.Upgrade([]string{"curl"}, "brew", false)
	if err != nil {
		t.Fatalf("Upgrade: %v", err)
	}

	// CompensateUpgrade is always a no-op
	if err := p.CompensateUpgrade(state); err != nil {
		t.Fatalf("CompensateUpgrade: %v", err)
	}
}

func TestUpgradeEmptyPackages(t *testing.T) {
	p, _ := mockProvider(nil, nil)
	_, _, err := p.Upgrade(nil, "brew", false)
	if err == nil {
		t.Error("expected error for empty packages")
	}
}

// --- Remove ---

func TestRemoveCompensateReinstalls(t *testing.T) {
	p, log := mockProvider(nil, nil)
	_, state, err := p.Remove([]string{"curl", "wget"}, "brew", false)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	packages, _ := state["packages"].([]string)
	if len(packages) != 2 {
		t.Errorf("expected 2 packages in state, got %v", packages)
	}

	// Compensate: should reinstall both
	if err := p.CompensateRemove(state); err != nil {
		t.Fatalf("CompensateRemove: %v", err)
	}
	if len(*log) != 4 { // 2 remove + 2 install
		t.Fatalf("expected 4 commands, got %d: %v", len(*log), *log)
	}
	if (*log)[2] != "install curl" || (*log)[3] != "install wget" {
		t.Errorf("expected install curl, install wget; got %v", (*log)[2:])
	}
}

func TestRemoveEmptyPackages(t *testing.T) {
	p, _ := mockProvider(nil, nil)
	_, _, err := p.Remove(nil, "brew", false)
	if err == nil {
		t.Error("expected error for empty packages")
	}
}

// --- Nil state safety ---

func TestCompensateNilState(t *testing.T) {
	p, _ := mockProvider(nil, nil)

	if err := p.CompensateInstall(nil); err != nil {
		t.Errorf("CompensateInstall(nil): %v", err)
	}
	if err := p.CompensateUpgrade(nil); err != nil {
		t.Errorf("CompensateUpgrade(nil): %v", err)
	}
	if err := p.CompensateRemove(nil); err != nil {
		t.Errorf("CompensateRemove(nil): %v", err)
	}
}
