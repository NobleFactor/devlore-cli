// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"testing"
)

func TestPackageContextHasFeature(t *testing.T) {
	ctx := &PackageContext{
		Name:     "mypackage",
		Version:  "1.0.0",
		Features: []string{"vim-mode", "dark-theme", "experimental"},
	}

	tests := []struct {
		feature  string
		expected bool
	}{
		{"vim-mode", true},
		{"dark-theme", true},
		{"experimental", true},
		{"nonexistent", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.feature, func(t *testing.T) {
			result := ctx.HasFeature(tt.feature)
			if result != tt.expected {
				t.Errorf("HasFeature(%q) = %v, want %v", tt.feature, result, tt.expected)
			}
		})
	}
}

func TestPackageContextHasFeatureEmpty(t *testing.T) {
	ctx := &PackageContext{
		Name:     "mypackage",
		Features: nil, // No features
	}

	if ctx.HasFeature("anything") {
		t.Error("expected HasFeature to return false for nil features")
	}
}

func TestPackageContextSetting(t *testing.T) {
	ctx := &PackageContext{
		Name: "mypackage",
		Settings: map[string]string{
			"theme":    "dark",
			"editor":   "vim",
			"tabwidth": "4",
		},
	}

	tests := []struct {
		key      string
		expected string
	}{
		{"theme", "dark"},
		{"editor", "vim"},
		{"tabwidth", "4"},
		{"nonexistent", ""},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := ctx.Setting(tt.key)
			if result != tt.expected {
				t.Errorf("Setting(%q) = %q, want %q", tt.key, result, tt.expected)
			}
		})
	}
}

func TestPackageContextSettingNil(t *testing.T) {
	ctx := &PackageContext{
		Name:     "mypackage",
		Settings: nil, // No settings
	}

	result := ctx.Setting("anything")
	if result != "" {
		t.Errorf("expected empty string for nil settings, got %q", result)
	}
}

func TestPackageContextFields(t *testing.T) {
	ctx := &PackageContext{
		Name:       "mypackage",
		Version:    "1.2.3",
		Features:   []string{"feature1", "feature2"},
		Settings:   map[string]string{"key": "value"},
		DryRun:     true,
		SourceRoot: "/path/to/source",
		TargetRoot: "/path/to/target",
	}

	if ctx.Name != "mypackage" {
		t.Errorf("expected Name 'mypackage', got %q", ctx.Name)
	}
	if ctx.Version != "1.2.3" {
		t.Errorf("expected Version '1.2.3', got %q", ctx.Version)
	}
	if len(ctx.Features) != 2 {
		t.Errorf("expected 2 features, got %d", len(ctx.Features))
	}
	if !ctx.DryRun {
		t.Error("expected DryRun to be true")
	}
	if ctx.SourceRoot != "/path/to/source" {
		t.Errorf("expected SourceRoot '/path/to/source', got %q", ctx.SourceRoot)
	}
	if ctx.TargetRoot != "/path/to/target" {
		t.Errorf("expected TargetRoot '/path/to/target', got %q", ctx.TargetRoot)
	}
}
