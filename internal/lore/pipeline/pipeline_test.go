// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package pipeline

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLifecycle(t *testing.T) {
	// Create a temporary directory with a lifecycle.yaml
	tmpDir := t.TempDir()
	pkgDir := filepath.Join(tmpDir, "testpkg")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}

	lifecycleYAML := `name: testpkg
version: "1.0.0"
description: "Test package"
platforms:
  - darwin
  - linux
features:
  completions:
    description: "Install shell completions"
    default: true
  debug:
    description: "Enable debug mode"
    default: false
settings:
  shell:
    description: "Shell to configure"
    type: string
    default: zsh
phases:
  prepare: prepare.star
  install: install.star
  verify: verify.star
`
	if err := os.WriteFile(filepath.Join(pkgDir, "lifecycle.yaml"), []byte(lifecycleYAML), 0644); err != nil {
		t.Fatal(err)
	}

	lifecycle, err := LoadLifecycle(pkgDir)
	if err != nil {
		t.Fatalf("LoadLifecycle failed: %v", err)
	}

	// Verify basic fields
	if lifecycle.Name != "testpkg" {
		t.Errorf("Name = %q, want %q", lifecycle.Name, "testpkg")
	}
	if lifecycle.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", lifecycle.Version, "1.0.0")
	}
	if len(lifecycle.Platforms) != 2 {
		t.Errorf("len(Platforms) = %d, want 2", len(lifecycle.Platforms))
	}
	if lifecycle.PackageDir != pkgDir {
		t.Errorf("PackageDir = %q, want %q", lifecycle.PackageDir, pkgDir)
	}
}

func TestLifecycleEnabledFeatures(t *testing.T) {
	lifecycle := &Lifecycle{
		Features: map[string]Feature{
			"completions": {Default: true},
			"debug":       {Default: false},
			"telemetry":   {Default: true},
		},
	}

	tests := []struct {
		name     string
		explicit []string
		want     []string
	}{
		{
			name:     "no explicit",
			explicit: nil,
			want:     []string{"completions", "telemetry"},
		},
		{
			name:     "explicit enable non-default",
			explicit: []string{"debug"},
			want:     []string{"debug", "completions", "telemetry"},
		},
		{
			name:     "explicit disable default",
			explicit: []string{"-completions"},
			want:     []string{"telemetry"},
		},
		{
			name:     "mixed",
			explicit: []string{"debug", "-telemetry"},
			want:     []string{"debug", "completions"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lifecycle.EnabledFeatures(tt.explicit)
			gotSet := make(map[string]bool)
			for _, f := range got {
				gotSet[f] = true
			}
			wantSet := make(map[string]bool)
			for _, f := range tt.want {
				wantSet[f] = true
			}

			// Check all wanted features are present
			for f := range wantSet {
				if !gotSet[f] {
					t.Errorf("missing feature %q", f)
				}
			}
			// Check no extra features
			for f := range gotSet {
				if !wantSet[f] {
					t.Errorf("unexpected feature %q", f)
				}
			}
		})
	}
}

func TestLifecycleResolvedSettings(t *testing.T) {
	lifecycle := &Lifecycle{
		Settings: map[string]Setting{
			"shell":  {Default: "zsh"},
			"editor": {Default: "vim"},
		},
	}

	explicit := map[string]string{
		"editor": "nvim",
	}

	got := lifecycle.ResolvedSettings(explicit)

	if got["shell"] != "zsh" {
		t.Errorf("shell = %q, want %q", got["shell"], "zsh")
	}
	if got["editor"] != "nvim" {
		t.Errorf("editor = %q, want %q", got["editor"], "nvim")
	}
}

func TestLifecycleHasPhase(t *testing.T) {
	lifecycle := &Lifecycle{
		Phases: map[string]string{
			"prepare": "prepare.star",
			"install": "install.star",
		},
	}

	if !lifecycle.HasPhase("prepare") {
		t.Error("HasPhase(prepare) = false, want true")
	}
	if lifecycle.HasPhase("provision") {
		t.Error("HasPhase(provision) = true, want false")
	}
}

func TestLifecycleGetPhaseScript(t *testing.T) {
	lifecycle := &Lifecycle{
		PackageDir: "/registry/testpkg",
		Phases: map[string]string{
			"install": "install.star",
		},
	}

	got := lifecycle.GetPhaseScript("install")
	want := "/registry/testpkg/install.star"
	if got != want {
		t.Errorf("GetPhaseScript(install) = %q, want %q", got, want)
	}

	got = lifecycle.GetPhaseScript("provision")
	if got != "" {
		t.Errorf("GetPhaseScript(provision) = %q, want empty", got)
	}
}

func TestLifecycleSupportsPlatform(t *testing.T) {
	lifecycle := &Lifecycle{
		Platforms: []string{"darwin", "linux"},
	}

	if !lifecycle.SupportsPlatform("darwin") {
		t.Error("SupportsPlatform(darwin) = false, want true")
	}
	if lifecycle.SupportsPlatform("windows") {
		t.Error("SupportsPlatform(windows) = true, want false")
	}
}

func TestExecutorDryRun(t *testing.T) {
	// Create a temporary package
	tmpDir := t.TempDir()
	pkgDir := filepath.Join(tmpDir, "dryrunpkg")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}

	lifecycleYAML := `name: dryrunpkg
version: "1.0"
platforms:
  - darwin
  - linux
phases:
  prepare: prepare.star
  install: install.star
`
	if err := os.WriteFile(filepath.Join(pkgDir, "lifecycle.yaml"), []byte(lifecycleYAML), 0644); err != nil {
		t.Fatal(err)
	}

	lifecycle, err := LoadLifecycle(pkgDir)
	if err != nil {
		t.Fatalf("LoadLifecycle failed: %v", err)
	}

	var buf bytes.Buffer
	executor := NewExecutor(ExecutorConfig{
		DryRun: true,
		Output: &buf,
	})

	result, err := executor.Execute(context.Background(), lifecycle, OpDeploy)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !result.Success {
		t.Error("result.Success = false, want true")
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("[DRY RUN]")) {
		t.Error("output should contain [DRY RUN]")
	}
}

func TestExecutorWithPhaseScripts(t *testing.T) {
	// Create a temporary package with real Starlark scripts
	tmpDir := t.TempDir()
	pkgDir := filepath.Join(tmpDir, "realpkg")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}

	lifecycleYAML := `name: realpkg
version: "1.0"
platforms:
  - darwin
  - linux
phases:
  install: install.star
  verify: verify.star
`
	if err := os.WriteFile(filepath.Join(pkgDir, "lifecycle.yaml"), []byte(lifecycleYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Create install.star
	installStar := `def install():
    note("Installing realpkg")
`
	if err := os.WriteFile(filepath.Join(pkgDir, "install.star"), []byte(installStar), 0644); err != nil {
		t.Fatal(err)
	}

	// Create verify.star
	verifyStar := `def verify():
    note("Verifying realpkg")
`
	if err := os.WriteFile(filepath.Join(pkgDir, "verify.star"), []byte(verifyStar), 0644); err != nil {
		t.Fatal(err)
	}

	lifecycle, err := LoadLifecycle(pkgDir)
	if err != nil {
		t.Fatalf("LoadLifecycle failed: %v", err)
	}

	var buf bytes.Buffer
	executor := NewExecutor(ExecutorConfig{
		Output:  &buf,
		Verbose: true,
	})

	result, err := executor.Execute(context.Background(), lifecycle, OpDeploy)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !result.Success {
		t.Errorf("result.Success = false, want true; error = %v", result.Error)
	}

	// Should have run install and verify (prepare and provision skipped)
	if len(result.Phases) != 4 {
		t.Errorf("len(result.Phases) = %d, want 4", len(result.Phases))
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("Installing realpkg")) {
		t.Error("output should contain 'Installing realpkg'")
	}
	if !bytes.Contains([]byte(output), []byte("Verifying realpkg")) {
		t.Error("output should contain 'Verifying realpkg'")
	}
}

func TestOperationString(t *testing.T) {
	tests := []struct {
		op   Operation
		want string
	}{
		{OpDeploy, "deploy"},
		{OpUpgrade, "upgrade"},
		{OpDecommission, "decommission"},
	}

	for _, tt := range tests {
		if got := tt.op.String(); got != tt.want {
			t.Errorf("%d.String() = %q, want %q", tt.op, got, tt.want)
		}
	}
}
