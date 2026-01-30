// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package lorepackage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRequiredPhase(t *testing.T) {
	tests := []struct {
		op   Operation
		want string
	}{
		{OpDeploy, "install"},
		{OpUpgrade, "upgrade"},
		{OpDecommission, "uninstall"},
	}

	for _, tt := range tests {
		t.Run(string(tt.op), func(t *testing.T) {
			got := RequiredPhase(tt.op)
			if got != tt.want {
				t.Errorf("RequiredPhase(%s) = %q, want %q", tt.op, got, tt.want)
			}
		})
	}
}

func TestPhaseToNativePMOp(t *testing.T) {
	tests := []struct {
		op     Operation
		phase  string
		wantPM PMOperation
		wantOK bool
	}{
		// Required phases should map
		{OpDeploy, "install", PMInstall, true},
		{OpUpgrade, "upgrade", PMUpgrade, true},
		{OpDecommission, "uninstall", PMRemove, true},

		// Non-required phases should not map
		{OpDeploy, "prepare", 0, false},
		{OpDeploy, "provision", 0, false},
		{OpDeploy, "verify", 0, false},
		{OpUpgrade, "prepare", 0, false},
		{OpUpgrade, "migrate", 0, false},
		{OpDecommission, "unprovision", 0, false},
		{OpDecommission, "cleanup", 0, false},
	}

	for _, tt := range tests {
		name := string(tt.op) + "/" + tt.phase
		t.Run(name, func(t *testing.T) {
			gotPM, gotOK := phaseToNativePMOp(tt.op, tt.phase)
			if gotOK != tt.wantOK {
				t.Errorf("phaseToNativePMOp(%s, %s) ok = %v, want %v", tt.op, tt.phase, gotOK, tt.wantOK)
			}
			if gotOK && gotPM != tt.wantPM {
				t.Errorf("phaseToNativePMOp(%s, %s) = %v, want %v", tt.op, tt.phase, gotPM, tt.wantPM)
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
		op        Operation
		phase     string
		wantCount int
		wantPMOp  PMOperation
	}{
		// Required phases return one action
		{OpDeploy, "install", 1, PMInstall},
		{OpUpgrade, "upgrade", 1, PMUpgrade},
		{OpDecommission, "uninstall", 1, PMRemove},

		// Non-required phases return empty
		{OpDeploy, "prepare", 0, 0},
		{OpDeploy, "provision", 0, 0},
		{OpDeploy, "verify", 0, 0},
		{OpUpgrade, "migrate", 0, 0},
	}

	for _, tt := range tests {
		name := string(tt.op) + "/" + tt.phase
		t.Run(name, func(t *testing.T) {
			actions := pkg.PhaseActions("Linux.Debian", tt.op, tt.phase)

			if len(actions) != tt.wantCount {
				t.Errorf("PhaseActions() returned %d actions, want %d", len(actions), tt.wantCount)
				return
			}

			if tt.wantCount > 0 {
				action := actions[0]
				if action.Type() != ActionNativePM {
					t.Errorf("action.Type() = %v, want ActionNativePM", action.Type())
				}

				pmAction, ok := action.(*NativePMAction)
				if !ok {
					t.Fatal("action is not *NativePMAction")
				}

				if pmAction.Manager != SourceApt {
					t.Errorf("Manager = %v, want SourceApt", pmAction.Manager)
				}
				if pmAction.Operation != tt.wantPMOp {
					t.Errorf("Operation = %v, want %v", pmAction.Operation, tt.wantPMOp)
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
	if err := os.MkdirAll(commonDeployDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(commonDeployDir, "install.star"), []byte("def install(): pass"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create Darwin/Deploy/install.star
	darwinDeployDir := filepath.Join(pkgDir, "Darwin", "Deploy")
	if err := os.MkdirAll(darwinDeployDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(darwinDeployDir, "install.star"), []byte("def install(): pass"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create lifecycle.yaml
	lifecycleYAML := `name: testpkg
version: "1.0"
description: "Test package"
platforms:
  - Darwin
`
	if err := os.WriteFile(filepath.Join(pkgDir, "lifecycle.yaml"), []byte(lifecycleYAML), 0644); err != nil {
		t.Fatal(err)
	}

	pkg := &Release{
		Name:    "testpkg",
		Version: "1.0",
		Source:  SourceLore,
		Dir:     pkgDir,
	}

	// Test install phase returns ScriptActions
	actions := pkg.PhaseActions("Darwin", OpDeploy, "install")

	// Should have 2 scripts: Common and Darwin (Unix doesn't exist)
	if len(actions) != 2 {
		t.Errorf("PhaseActions() returned %d actions, want 2", len(actions))
	}

	for i, action := range actions {
		if action.Type() != ActionScript {
			t.Errorf("action[%d].Type() = %v, want ActionScript", i, action.Type())
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
	actions = pkg.PhaseActions("Darwin", OpDeploy, "provision")
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
