// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package lorepackage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/selector"
)

func TestRequiredPhase(t *testing.T) {
	tests := []struct {
		action Action
		want   string
	}{
		{Deploy, "install"},
		{Upgrade, "upgrade"},
		{Decommission, "uninstall"},
		{Reconcile, "repair"},
	}

	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			got := RequiredPhase(tt.action)
			if got != tt.want {
				t.Errorf("RequiredPhase(%s) = %q, want %q", tt.action, got, tt.want)
			}
		})
	}
}

func TestPhaseToNativePMAction(t *testing.T) {
	tests := []struct {
		action Action
		phase  string
		wantPM PMCommand
		wantOK bool
	}{
		// Required phases should map
		{Deploy, "install", PMInstall, true},
		{Upgrade, "upgrade", PMUpgrade, true},
		{Decommission, "uninstall", PMRemove, true},

		// Reconcile has no native PM equivalent
		{Reconcile, "repair", 0, false},

		// Non-required phases should not map
		{Deploy, "prepare", 0, false},
		{Deploy, "provision", 0, false},
		{Deploy, "verify", 0, false},
		{Upgrade, "prepare", 0, false},
		{Upgrade, "migrate", 0, false},
		{Decommission, "unprovision", 0, false},
		{Decommission, "cleanup", 0, false},
	}

	for _, tt := range tests {
		name := string(tt.action) + "/" + tt.phase
		t.Run(name, func(t *testing.T) {
			gotPM, gotOK := phaseToNativePMCmd(tt.action, tt.phase)
			if gotOK != tt.wantOK {
				t.Errorf("phaseToNativePMCmd(%s, %s) ok = %v, want %v", tt.action, tt.phase, gotOK, tt.wantOK)
			}
			if gotOK && gotPM != tt.wantPM {
				t.Errorf("phaseToNativePMCmd(%s, %s) = %v, want %v", tt.action, tt.phase, gotPM, tt.wantPM)
			}
		})
	}
}

func TestRelease_PhaseActions_NativePM(t *testing.T) {
	// Native PM package should return NativePMAction only for required phases
	pkg := &Release{
		Name:       "curl",
		Version:    "latest",
		Source:     SourceApt,
		NativeName: "curl",
	}

	tests := []struct {
		action    Action
		phase     string
		wantCount int
		wantPMCmd PMCommand
	}{
		// Required phases return one action
		{Deploy, "install", 1, PMInstall},
		{Upgrade, "upgrade", 1, PMUpgrade},
		{Decommission, "uninstall", 1, PMRemove},

		// Reconcile has no native PM equivalent
		{Reconcile, "repair", 0, 0},

		// Non-required phases return empty
		{Deploy, "prepare", 0, 0},
		{Deploy, "provision", 0, 0},
		{Deploy, "verify", 0, 0},
		{Upgrade, "migrate", 0, 0},
	}

	for _, tt := range tests {
		name := string(tt.action) + "/" + tt.phase
		t.Run(name, func(t *testing.T) {
			actions, err := pkg.PhaseActions(selector.NewHostFromWords("Linux", "Ubuntu", []string{"Debian"}, "amd64"),
				tt.action, tt.phase)
			if err != nil {
				t.Fatalf("PhaseActions: %v", err)
			}

			if len(actions) != tt.wantCount {
				t.Errorf("PhaseActions() returned %d actions, want %d", len(actions), tt.wantCount)
				return
			}

			if tt.wantCount > 0 {
				action := actions[0]
				if action.Type() != ActionNativePM {
					t.Errorf("action.ProviderType() = %v, want ActionNativePM", action.Type())
				}

				pmAction, ok := action.(*NativePMAction)
				if !ok {
					t.Fatal("action is not *NativePMAction")
				}

				if pmAction.Manager != SourceApt {
					t.Errorf("Manager = %v, want SourceApt", pmAction.Manager)
				}
				if pmAction.Command != tt.wantPMCmd {
					t.Errorf("Command = %v, want %v", pmAction.Command, tt.wantPMCmd)
				}
				if len(pmAction.Packages) != 1 || pmAction.Packages[0] != "curl" {
					t.Errorf("Packages = %v, want [curl]", pmAction.Packages)
				}
			}
		})
	}
}

func TestRelease_PhaseActions_LorePackage(t *testing.T) {
	// Create a temporary lore package with scripts
	tmpDir := t.TempDir()
	pkgDir := filepath.Join(tmpDir, "testpkg")

	// Create Common/Deploy/install.star
	commonDeployDir := filepath.Join(pkgDir, "Common", "Deploy")
	if err := os.MkdirAll(commonDeployDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(commonDeployDir, "install.star"), []byte("def install(): pass"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create Darwin/Deploy/install.star
	darwinDeployDir := filepath.Join(pkgDir, "Darwin", "Deploy")
	if err := os.MkdirAll(darwinDeployDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(darwinDeployDir, "install.star"), []byte("def install(): pass"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create lifecycle.yaml
	lifecycleYAML := `name: testpkg
version: "1.0"
description: "Test package"
platforms:
  - Darwin
`
	if err := os.WriteFile(filepath.Join(pkgDir, "lifecycle.yaml"), []byte(lifecycleYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	pkg := &Release{
		Name:    "testpkg",
		Version: "1.0",
		Source:  SourceLore,
		Dir:     pkgDir,
	}

	// Test install phase returns ScriptActions
	darwin := selector.NewHostFromWords("Darwin", "", nil, "arm64")
	actions, err := pkg.PhaseActions(darwin, Deploy, "install")
	if err != nil {
		t.Fatalf("PhaseActions: %v", err)
	}

	// Should have 2 scripts: Common and Darwin (Unix doesn't exist)
	if len(actions) != 2 {
		t.Errorf("PhaseActions() returned %d actions, want 2", len(actions))
	}

	for i, action := range actions {
		if action.Type() != ActionScript {
			t.Errorf("action[%d].ProviderType() = %v, want ActionScript", i, action.Type())
		}
		scriptAction, ok := action.(*ScriptAction)
		if !ok {
			t.Errorf("action[%d] is not *ScriptAction", i)
			continue
		}
		if scriptAction.PhaseName != "install" {
			t.Errorf("action[%d].PhaseName = %q, want \"install\"", i, scriptAction.PhaseName)
		}
	}

	// Test non-existent phase returns empty
	actions, err = pkg.PhaseActions(darwin, Deploy, "provision")
	if err != nil {
		t.Fatalf("PhaseActions(provision): %v", err)
	}
	if len(actions) != 0 {
		t.Errorf("PhaseActions(provision) returned %d actions, want 0", len(actions))
	}
}

func TestRelease_IsNative(t *testing.T) {
	tests := []struct {
		source PackageSource
		want   bool
	}{
		{SourceLore, false},
		{SourceApt, true},
		{SourceDnf, true},
		{SourceBrew, true},
		{SourceWinget, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.source), func(t *testing.T) {
			pkg := &Release{Source: tt.source}
			if got := pkg.IsNative(); got != tt.want {
				t.Errorf("IsNative() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSourceForPurlType: a native manager reports its purl type, and lore names the package by the manager's source;
// lore resolving a package and lore search share the one mapping.
func TestSourceForPurlType(t *testing.T) {

	tests := []struct {
		purlType string
		want     PackageSource
		ok       bool
	}{
		{"deb", SourceApt, true},
		{"rpm", SourceDnf, true},
		{"alpm", SourcePacman, true},
		{"brew", SourceBrew, true},
		{"port", SourcePort, true},
		{"winget", SourceWinget, true},
		{"flatpak", "", false},
		{"apt", "", false},
	}

	for _, tt := range tests {
		if got, ok := sourceForPurlType(tt.purlType); got != tt.want || ok != tt.ok {
			t.Errorf("sourceForPurlType(%q) = %q, %v; want %q, %v", tt.purlType, got, ok, tt.want, tt.ok)
		}
	}
}
