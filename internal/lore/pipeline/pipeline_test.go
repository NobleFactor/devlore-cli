// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pipeline

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/internal/registry"
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
  - Darwin
  - Linux
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
`
	if err := os.WriteFile(filepath.Join(pkgDir, "lifecycle.yaml"), []byte(lifecycleYAML), 0644); err != nil {
		t.Fatal(err)
	}

	lifecycle, err := registry.LoadLifecycle(pkgDir)
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
}

func TestLifecycleEnabledFeatures(t *testing.T) {
	lifecycle := &registry.Lifecycle{
		Features: map[string]registry.Feature{
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
	lifecycle := &registry.Lifecycle{
		Settings: map[string]registry.Setting{
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
	// Create a temporary package with phase scripts
	tmpDir := t.TempDir()
	pkgDir := filepath.Join(tmpDir, "testpkg")

	// Create directory structure: Common/Deploy/prepare.star, Common/Deploy/install.star
	deployDir := filepath.Join(pkgDir, "Common", "Deploy")
	if err := os.MkdirAll(deployDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(deployDir, "prepare.star"), []byte("def prepare(): pass"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deployDir, "install.star"), []byte("def install(): pass"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create lifecycle.yaml
	lifecycleYAML := `name: testpkg
version: "1.0"
description: "Test"
platforms:
  - Darwin
  - Linux
`
	if err := os.WriteFile(filepath.Join(pkgDir, "lifecycle.yaml"), []byte(lifecycleYAML), 0644); err != nil {
		t.Fatal(err)
	}

	lifecycle, err := registry.LoadLifecycle(pkgDir)
	if err != nil {
		t.Fatal(err)
	}

	if !lifecycle.HasPhase(pkgDir, "Darwin", registry.OpDeploy, "prepare") {
		t.Error("HasPhase(prepare) = false, want true")
	}
	if !lifecycle.HasPhase(pkgDir, "Darwin", registry.OpDeploy, "install") {
		t.Error("HasPhase(install) = false, want true")
	}
	if lifecycle.HasPhase(pkgDir, "Darwin", registry.OpDeploy, "provision") {
		t.Error("HasPhase(provision) = true, want false")
	}
}

func TestLifecycleDiscoverPhaseScripts(t *testing.T) {
	// Create a temporary package with chained phase scripts
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

	// Create Unix/Deploy/install.star
	unixDeployDir := filepath.Join(pkgDir, "Unix", "Deploy")
	if err := os.MkdirAll(unixDeployDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unixDeployDir, "install.star"), []byte("def install(): pass"), 0644); err != nil {
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
description: "Test"
platforms:
  - Darwin
`
	if err := os.WriteFile(filepath.Join(pkgDir, "lifecycle.yaml"), []byte(lifecycleYAML), 0644); err != nil {
		t.Fatal(err)
	}

	lifecycle, err := registry.LoadLifecycle(pkgDir)
	if err != nil {
		t.Fatal(err)
	}

	// Should discover all three scripts in order: Common → Unix → Darwin
	scripts := lifecycle.DiscoverPhaseScripts(pkgDir, "Darwin", registry.OpDeploy, "install")
	if len(scripts) != 3 {
		t.Errorf("len(scripts) = %d, want 3", len(scripts))
	}

	// Verify order: Common first, Unix second, Darwin last
	if len(scripts) >= 1 && filepath.Base(filepath.Dir(filepath.Dir(scripts[0]))) != "Common" {
		t.Errorf("first script should be from Common, got %s", scripts[0])
	}
	if len(scripts) >= 2 && filepath.Base(filepath.Dir(filepath.Dir(scripts[1]))) != "Unix" {
		t.Errorf("second script should be from Unix, got %s", scripts[1])
	}
	if len(scripts) >= 3 && filepath.Base(filepath.Dir(filepath.Dir(scripts[2]))) != "Darwin" {
		t.Errorf("third script should be from Darwin, got %s", scripts[2])
	}
}

func TestLifecycleSupportsPlatform(t *testing.T) {
	lifecycle := &registry.Lifecycle{
		Platforms: []string{"Darwin", "Linux"},
	}

	if !lifecycle.SupportsPlatform("Darwin") {
		t.Error("SupportsPlatform(Darwin) = false, want true")
	}
	if lifecycle.SupportsPlatform("Windows") {
		t.Error("SupportsPlatform(Windows) = true, want false")
	}
	// Test distro matching
	lifecycle.Platforms = []string{"Linux"}
	if !lifecycle.SupportsPlatform("Linux.Debian") {
		t.Error("SupportsPlatform(Linux.Debian) = false, want true (matches Linux)")
	}
}

func TestExecutorDryRun(t *testing.T) {
	// Create a temporary package
	tmpDir := t.TempDir()
	pkgDir := filepath.Join(tmpDir, "dryrunpkg")

	// Create Common/Deploy/prepare.star and Common/Deploy/install.star
	deployDir := filepath.Join(pkgDir, "Common", "Deploy")
	if err := os.MkdirAll(deployDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deployDir, "prepare.star"), []byte("def prepare(): pass"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deployDir, "install.star"), []byte("def install(): pass"), 0644); err != nil {
		t.Fatal(err)
	}

	lifecycleYAML := `name: dryrunpkg
version: "1.0"
description: "Dry run test"
platforms:
  - Darwin
  - Linux
`
	if err := os.WriteFile(filepath.Join(pkgDir, "lifecycle.yaml"), []byte(lifecycleYAML), 0644); err != nil {
		t.Fatal(err)
	}

	lifecycle, err := registry.LoadLifecycle(pkgDir)
	if err != nil {
		t.Fatalf("LoadLifecycle failed: %v", err)
	}

	var buf bytes.Buffer
	executor := NewExecutor(ExecutorConfig{
		DryRun:   true,
		Output:   &buf,
		Platform: "Darwin",
	})

	result, err := executor.Execute(context.Background(), lifecycle, pkgDir, OpDeploy)
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

	// Create Common/Deploy/ directory
	deployDir := filepath.Join(pkgDir, "Common", "Deploy")
	if err := os.MkdirAll(deployDir, 0755); err != nil {
		t.Fatal(err)
	}

	lifecycleYAML := `name: realpkg
version: "1.0"
description: "Real package test"
platforms:
  - Darwin
  - Linux
`
	if err := os.WriteFile(filepath.Join(pkgDir, "lifecycle.yaml"), []byte(lifecycleYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Create install.star
	installStar := `def install():
    note("Installing realpkg")
`
	if err := os.WriteFile(filepath.Join(deployDir, "install.star"), []byte(installStar), 0644); err != nil {
		t.Fatal(err)
	}

	// Create verify.star
	verifyStar := `def verify():
    note("Verifying realpkg")
`
	if err := os.WriteFile(filepath.Join(deployDir, "verify.star"), []byte(verifyStar), 0644); err != nil {
		t.Fatal(err)
	}

	lifecycle, err := registry.LoadLifecycle(pkgDir)
	if err != nil {
		t.Fatalf("LoadLifecycle failed: %v", err)
	}

	var buf bytes.Buffer
	executor := NewExecutor(ExecutorConfig{
		Output:   &buf,
		Verbose:  true,
		Platform: "Darwin",
	})

	result, err := executor.Execute(context.Background(), lifecycle, pkgDir, OpDeploy)
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

func TestPlatformResolutionOrder(t *testing.T) {
	tests := []struct {
		platform string
		want     []string
	}{
		{"Darwin", []string{"Common", "Unix", "Darwin"}},
		{"Linux", []string{"Common", "Unix", "Linux"}},
		{"Linux.Debian", []string{"Common", "Unix", "Linux", "Linux.Debian"}},
		{"Linux.Fedora", []string{"Common", "Unix", "Linux", "Linux.Fedora"}},
		{"Windows", []string{"Common", "Windows"}},
	}

	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			got := registry.PlatformResolutionOrder(tt.platform)
			if len(got) != len(tt.want) {
				t.Errorf("len = %d, want %d; got %v", len(got), len(tt.want), got)
				return
			}
			for i, p := range tt.want {
				if got[i] != p {
					t.Errorf("order[%d] = %q, want %q", i, got[i], p)
				}
			}
		})
	}
}
