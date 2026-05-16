// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package application

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestDryRun_Snake(t *testing.T) {

	t.Run("true under snake key", func(t *testing.T) {
		app := &Application{Flags: map[string]any{"dry_run": true}}
		if !app.DryRun() {
			t.Error("DryRun() = false, want true")
		}
	})

	t.Run("missing returns false", func(t *testing.T) {
		app := &Application{Flags: map[string]any{}}
		if app.DryRun() {
			t.Error("DryRun() with missing key should be false")
		}
	})

	t.Run("nil Flags returns false", func(t *testing.T) {
		app := &Application{}
		if app.DryRun() {
			t.Error("DryRun() with nil Flags should be false")
		}
	})

	t.Run("wrong type returns false", func(t *testing.T) {
		app := &Application{Flags: map[string]any{"dry_run": "true"}}
		if app.DryRun() {
			t.Error("DryRun() with non-bool value should be false")
		}
	})

	t.Run("kebab key is NOT honored (post-normalization world)", func(t *testing.T) {
		app := &Application{Flags: map[string]any{"dry-run": true}}
		if app.DryRun() {
			t.Error("DryRun() should not read kebab-case key; normalization happens at NewApplication time")
		}
	})
}

func TestNewApplication_KebabToSnake(t *testing.T) {

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().String("target-root", "", "")
	cmd.Flags().String("layer", "", "")

	if err := cmd.ParseFlags([]string{"--dry-run", "--target-root", "/tmp/x", "--layer", "personal"}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}

	app := NewApplication("writ", cmd)

	if app.Name != "writ" {
		t.Errorf("Name = %q, want writ", app.Name)
	}

	for _, key := range []string{"dry_run", "target_root", "layer"} {
		if _, ok := app.Flags[key]; !ok {
			t.Errorf("Flags missing snake-case key %q (have %v)", key, app.Flags)
		}
	}

	for _, key := range []string{"dry-run", "target-root"} {
		if _, ok := app.Flags[key]; ok {
			t.Errorf("Flags should NOT have kebab-case key %q after normalization", key)
		}
	}

	if v, _ := app.Flags["dry_run"].(bool); !v {
		t.Errorf("dry_run = %v, want true", v)
	}
	if v, _ := app.Flags["target_root"].(string); v != "/tmp/x" {
		t.Errorf("target_root = %q, want /tmp/x", v)
	}
}

func TestNewApplication_UnsetFlagsNotStamped(t *testing.T) {

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().String("target-root", "default", "")

	// User passes only --dry-run; --target-root is left at default.
	if err := cmd.ParseFlags([]string{"--dry-run"}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}

	app := NewApplication("writ", cmd)

	if _, ok := app.Flags["dry_run"]; !ok {
		t.Error("user-supplied flag should be stamped")
	}
	if _, ok := app.Flags["target_root"]; ok {
		t.Error("unset flag (default value) should NOT be stamped")
	}
}

func TestKebabToSnake(t *testing.T) {

	tests := []struct {
		in, want string
	}{
		{"dry-run", "dry_run"},
		{"target-root", "target_root"},
		{"layer", "layer"},                 // no hyphens: unchanged.
		{"already_snake", "already_snake"}, // idempotent.
		{"a-b-c-d", "a_b_c_d"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := kebabToSnake(tt.in); got != tt.want {
				t.Errorf("kebabToSnake(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
