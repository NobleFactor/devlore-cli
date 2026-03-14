// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package lorepackage

import (
	"testing"
)

func TestPMCommand_String(t *testing.T) {
	tests := []struct {
		cmd  PMCommand
		want string
	}{
		{PMInstall, "install"},
		{PMRemove, "remove"},
		{PMUpdate, "update"},
		{PMUpgrade, "upgrade"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.cmd.String(); got != tt.want {
				t.Errorf("PMCommand.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestScriptAction(t *testing.T) {
	action := &ScriptAction{
		Path:      "/pkg/Common/Deploy/install.star",
		PhaseName: "install",
		Platform:  "Common",
	}

	if action.Type() != ActionScript {
		t.Errorf("ProviderType() = %v, want ActionScript", action.Type())
	}
	if action.Phase() != "install" {
		t.Errorf("Phase() = %q, want \"install\"", action.Phase())
	}
}

func TestNativePMAction(t *testing.T) {
	action := &NativePMAction{
		Manager:   SourceApt,
		Command:   PMInstall,
		Packages:  []string{"curl", "wget"},
		PhaseName: "install",
	}

	if action.Type() != ActionNativePM {
		t.Errorf("ProviderType() = %v, want ActionNativePM", action.Type())
	}
	if action.Phase() != "install" {
		t.Errorf("Phase() = %q, want \"install\"", action.Phase())
	}
}

func TestNativePMAction_Batchable(t *testing.T) {
	tests := []struct {
		cmd  PMCommand
		want bool
	}{
		{PMInstall, true},
		{PMRemove, true},
		{PMUpdate, false},
		{PMUpgrade, true},
	}

	for _, tt := range tests {
		t.Run(tt.cmd.String(), func(t *testing.T) {
			action := &NativePMAction{Command: tt.cmd}
			if got := action.Batchable(); got != tt.want {
				t.Errorf("Batchable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNativePMAction_CanBatchWith(t *testing.T) {
	base := &NativePMAction{
		Manager:   SourceApt,
		Command:   PMInstall,
		Packages:  []string{"curl"},
		PhaseName: "install",
	}

	tests := []struct {
		name  string
		other *NativePMAction
		want  bool
	}{
		{
			name: "same manager and command",
			other: &NativePMAction{
				Manager:   SourceApt,
				Command:   PMInstall,
				Packages:  []string{"wget"},
				PhaseName: "install",
			},
			want: true,
		},
		{
			name: "different manager",
			other: &NativePMAction{
				Manager:   SourceBrew,
				Command:   PMInstall,
				Packages:  []string{"wget"},
				PhaseName: "install",
			},
			want: false,
		},
		{
			name: "different command",
			other: &NativePMAction{
				Manager:   SourceApt,
				Command:   PMRemove,
				Packages:  []string{"wget"},
				PhaseName: "install",
			},
			want: false,
		},
		{
			name: "different phase",
			other: &NativePMAction{
				Manager:   SourceApt,
				Command:   PMInstall,
				Packages:  []string{"wget"},
				PhaseName: "upgrade",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := base.CanBatchWith(tt.other); got != tt.want {
				t.Errorf("CanBatchWith() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNativePMAction_Merge(t *testing.T) {
	a := &NativePMAction{
		Manager:   SourceApt,
		Command:   PMInstall,
		Packages:  []string{"curl", "wget"},
		PhaseName: "install",
	}

	b := &NativePMAction{
		Manager:   SourceApt,
		Command:   PMInstall,
		Packages:  []string{"jq", "vim"},
		PhaseName: "install",
	}

	merged := a.Merge(b)
	if merged == nil {
		t.Fatal("Merge() returned nil")
	}

	if merged.Manager != SourceApt {
		t.Errorf("Manager = %v, want SourceApt", merged.Manager)
	}
	if merged.Command != PMInstall {
		t.Errorf("Command = %v, want PMInstall", merged.Command)
	}
	if len(merged.Packages) != 4 {
		t.Errorf("len(Packages) = %d, want 4", len(merged.Packages))
	}

	expected := []string{"curl", "wget", "jq", "vim"}
	for i, pkg := range expected {
		if merged.Packages[i] != pkg {
			t.Errorf("Packages[%d] = %q, want %q", i, merged.Packages[i], pkg)
		}
	}

	// Test incompatible merge returns nil
	incompatible := &NativePMAction{
		Manager:   SourceBrew,
		Command:   PMInstall,
		Packages:  []string{"htop"},
		PhaseName: "install",
	}
	if result := a.Merge(incompatible); result != nil {
		t.Errorf("Merge(incompatible) = %v, want nil", result)
	}
}

func TestUpgradePhaseOrder_IncludesMigrate(t *testing.T) {
	// Verify the upgrade phase order includes migrate
	found := false
	for _, phase := range UpgradePhaseOrder {
		if phase == "migrate" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("UpgradePhaseOrder = %v, should include \"migrate\"", UpgradePhaseOrder)
	}

	// Verify order: prepare, upgrade, migrate, verify
	expected := []string{"prepare", "upgrade", "migrate", "verify"}
	if len(UpgradePhaseOrder) != len(expected) {
		t.Errorf("len(UpgradePhaseOrder) = %d, want %d", len(UpgradePhaseOrder), len(expected))
	}
	for i, phase := range expected {
		if i < len(UpgradePhaseOrder) && UpgradePhaseOrder[i] != phase {
			t.Errorf("UpgradePhaseOrder[%d] = %q, want %q", i, UpgradePhaseOrder[i], phase)
		}
	}
}
