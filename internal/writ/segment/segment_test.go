// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package segment

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOSFamily(t *testing.T) {
	tests := []struct {
		os     string
		family string
	}{
		{"Darwin", "Unix"},
		{"Linux", "Unix"},
		{"Windows", ""},
		{"FreeBSD", ""},
	}

	for _, tt := range tests {
		t.Run(tt.os, func(t *testing.T) {
			got := OSFamily(tt.os)
			if got != tt.family {
				t.Errorf("OSFamily(%q) = %q, want %q", tt.os, got, tt.family)
			}
		})
	}
}

func TestParseDirName(t *testing.T) {
	tests := []struct {
		dirname  string
		project  string
		suffixes []string
	}{
		{"all", "all", nil},
		{"all.Darwin", "all", []string{"Darwin"}},
		{"all.Darwin.arm64", "all", []string{"Darwin", "arm64"}},
		{"noblefactor.Unix", "noblefactor", []string{"Unix"}},
		{"microsoft.Windows.amd64", "microsoft", []string{"Windows", "amd64"}},
	}

	for _, tt := range tests {
		t.Run(tt.dirname, func(t *testing.T) {
			project, suffixes := ParseDirName(tt.dirname)
			if project != tt.project {
				t.Errorf("project = %q, want %q", project, tt.project)
			}
			if len(suffixes) != len(tt.suffixes) {
				t.Errorf("suffixes = %v, want %v", suffixes, tt.suffixes)
			} else {
				for i := range suffixes {
					if suffixes[i] != tt.suffixes[i] {
						t.Errorf("suffixes[%d] = %q, want %q", i, suffixes[i], tt.suffixes[i])
					}
				}
			}
		})
	}
}

func TestSegmentsMatch(t *testing.T) {
	// Simulate macOS arm64
	darwinSegs := Segments{
		{Name: "OS", Value: "Darwin"},
		{Name: "DISTRO", Value: ""},
		{Name: "ARCH", Value: "arm64"},
	}

	// Simulate Ubuntu amd64
	ubuntuSegs := Segments{
		{Name: "OS", Value: "Linux"},
		{Name: "DISTRO", Value: "Ubuntu"},
		{Name: "ARCH", Value: "amd64"},
	}

	tests := []struct {
		name    string
		segs    Segments
		dirname string
		match   bool
	}{
		// macOS tests
		{"darwin-base", darwinSegs, "all", true},
		{"darwin-os", darwinSegs, "all.Darwin", true},
		{"darwin-unix", darwinSegs, "all.Unix", true},
		{"darwin-arch", darwinSegs, "all.arm64", true},
		{"darwin-os-arch", darwinSegs, "all.Darwin.arm64", true},
		{"darwin-unix-arch", darwinSegs, "all.Unix.arm64", true},
		{"darwin-wrong-os", darwinSegs, "all.Linux", false},
		{"darwin-wrong-arch", darwinSegs, "all.amd64", false},
		{"darwin-distro", darwinSegs, "all.Ubuntu", false},

		// Ubuntu tests
		{"ubuntu-base", ubuntuSegs, "all", true},
		{"ubuntu-os", ubuntuSegs, "all.Linux", true},
		{"ubuntu-unix", ubuntuSegs, "all.Unix", true},
		{"ubuntu-distro", ubuntuSegs, "all.Ubuntu", true},
		{"ubuntu-arch", ubuntuSegs, "all.amd64", true},
		{"ubuntu-os-distro", ubuntuSegs, "all.Linux.Ubuntu", true},
		{"ubuntu-unix-distro", ubuntuSegs, "all.Unix.Ubuntu", true},
		{"ubuntu-wrong-os", ubuntuSegs, "all.Darwin", false},
		{"ubuntu-wrong-distro", ubuntuSegs, "all.Debian", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.segs.Match(tt.dirname)
			if got != tt.match {
				t.Errorf("Match(%q) = %v, want %v", tt.dirname, got, tt.match)
			}
		})
	}
}

func TestSegmentsAllValues(t *testing.T) {
	segs := Segments{
		{Name: "OS", Value: "Darwin"},
		{Name: "DISTRO", Value: ""},
		{Name: "ARCH", Value: "arm64"},
	}

	values := segs.AllValues()

	// Should include: Darwin, Unix (family), arm64
	expected := []string{"Darwin", "Unix", "arm64"}

	if len(values) != len(expected) {
		t.Errorf("AllValues() = %v, want %v", values, expected)
		return
	}

	for i, v := range expected {
		if values[i] != v {
			t.Errorf("AllValues()[%d] = %q, want %q", i, values[i], v)
		}
	}
}

func TestMatchDirectories(t *testing.T) {
	// Create temp directory with test structure
	tmpDir := t.TempDir()

	dirs := []string{
		"all",
		"all.Darwin",
		"all.Linux",
		"all.Unix",
		"noblefactor",
		"noblefactor.Unix",
		"microsoft",
		"microsoft.Windows",
	}

	for _, d := range dirs {
		if err := os.Mkdir(filepath.Join(tmpDir, d), 0o755); err != nil {
			t.Fatalf("failed to create dir %s: %v", d, err)
		}
	}

	darwinSegs := Segments{
		{Name: "OS", Value: "Darwin"},
		{Name: "DISTRO", Value: ""},
		{Name: "ARCH", Value: "arm64"},
	}

	// Test matching specific projects
	results, err := MatchDirectories(tmpDir, []string{"all", "noblefactor"}, darwinSegs)
	if err != nil {
		t.Fatalf("MatchDirectories failed: %v", err)
	}

	// Should match: all, all.Darwin, all.Unix, noblefactor, noblefactor.Unix
	expectedDirs := []string{"all", "all.Darwin", "all.Unix", "noblefactor", "noblefactor.Unix"}
	if len(results) != len(expectedDirs) {
		var got []string
		for _, r := range results {
			got = append(got, filepath.Base(r.Path))
		}
		t.Errorf("got %d results %v, want %d %v", len(results), got, len(expectedDirs), expectedDirs)
	}

	// Verify projects are correct
	for _, r := range results {
		if r.Project != "all" && r.Project != "noblefactor" {
			t.Errorf("unexpected project: %s", r.Project)
		}
	}
}

func TestMatchResultSpecificity(t *testing.T) {
	tests := []struct {
		suffixes    []string
		specificity int
	}{
		{nil, 0},
		{[]string{"Darwin"}, 1},
		{[]string{"Darwin", "arm64"}, 2},
		{[]string{"Unix", "Ubuntu", "amd64"}, 3},
	}

	for _, tt := range tests {
		r := MatchResult{Suffixes: tt.suffixes}
		if got := r.Specificity(); got != tt.specificity {
			t.Errorf("Specificity() with %v = %d, want %d", tt.suffixes, got, tt.specificity)
		}
	}
}

func TestDetectSegments(t *testing.T) {
	segs := DetectSegments()

	// Should always have OS, DISTRO, ARCH
	if len(segs) != 3 {
		t.Errorf("DetectSegments() returned %d segments, want 3", len(segs))
	}

	// OS should be set
	if osVal := segs.Get("OS"); osVal == "" {
		t.Error("OS segment is empty")
	}

	// ARCH should be set
	if arch := segs.Get("ARCH"); arch == "" {
		t.Error("ARCH segment is empty")
	}
}

func TestDetectSegmentsWithNames(t *testing.T) {
	// Config defines segment names (no values)
	segs := DetectSegmentsWithNames([]string{"ROLE", "SITE"})

	// Should have OS, DISTRO, ARCH + ROLE, SITE
	if len(segs) != 5 {
		t.Errorf("got %d segments, want 5", len(segs))
	}

	// Custom segments should have empty values (like DISTRO on macOS)
	if segs.Get("ROLE") != "" {
		t.Errorf("ROLE = %q, want empty", segs.Get("ROLE"))
	}
	if segs.Get("SITE") != "" {
		t.Errorf("SITE = %q, want empty", segs.Get("SITE"))
	}
}

func TestSetValues(t *testing.T) {
	// Config defines segment names, values are empty
	segs := Segments{
		{Name: "OS", Value: "Darwin"},
		{Name: "DISTRO", Value: ""},
		{Name: "ARCH", Value: "arm64"},
		{Name: "ROLE", Value: ""}, // defined in config, no value yet
		{Name: "SITE", Value: ""}, // defined in config, no value yet
	}

	// Set value for ROLE via CLI --segment ROLE=server
	result, err := segs.SetValues(map[string]string{"ROLE": "server"})
	if err != nil {
		t.Errorf("SetValues with valid segment failed: %v", err)
	}
	if result.Get("ROLE") != "server" {
		t.Errorf("ROLE = %q, want %q", result.Get("ROLE"), "server")
	}
	// SITE remains empty (unassigned, like DISTRO on macOS)
	if result.Get("SITE") != "" {
		t.Errorf("SITE = %q, want empty", result.Get("SITE"))
	}
	// Original should be unchanged
	if segs.Get("ROLE") != "" {
		t.Error("original segs was modified")
	}

	// Invalid: ENV is not defined in config
	_, err = segs.SetValues(map[string]string{"ENV": "prod"})
	if err == nil {
		t.Error("SetValues with undefined segment should fail")
	}
	var undefinedErr *UndefinedSegmentError
	if !errors.As(err, &undefinedErr) {
		t.Errorf("error should be UndefinedSegmentError, got %T", err)
	}
	if undefinedErr != nil && undefinedErr.Name != "ENV" {
		t.Errorf("UndefinedSegmentError.Name = %q, want %q", undefinedErr.Name, "ENV")
	}
}

func TestUnassignedSegmentMatching(t *testing.T) {
	// Segments with ROLE defined but unassigned (empty value)
	segs := Segments{
		{Name: "OS", Value: "Darwin"},
		{Name: "DISTRO", Value: ""}, // empty on macOS
		{Name: "ARCH", Value: "arm64"},
		{Name: "ROLE", Value: ""}, // defined but unassigned
	}

	// Directory with ROLE suffix should NOT match (ROLE is empty)
	if segs.Match("noblefactor.desktop") {
		t.Error("noblefactor.desktop should not match when ROLE is empty")
	}

	// ProviderBase directory should match
	if !segs.Match("noblefactor") {
		t.Error("noblefactor should match")
	}

	// Now assign ROLE
	segs, _ = segs.SetValues(map[string]string{"ROLE": "desktop"})

	// Directory with ROLE suffix should now match
	if !segs.Match("noblefactor.desktop") {
		t.Error("noblefactor.desktop should match when ROLE=desktop")
	}

	// Wrong ROLE value should not match
	if segs.Match("noblefactor.server") {
		t.Error("noblefactor.server should not match when ROLE=desktop")
	}
}

func TestLoadFromEnv(t *testing.T) {
	// Set up test environment variables
	t.Setenv("WRIT_SEGMENT_ROLE", "server")
	t.Setenv("WRIT_SEGMENT_SITE", "aws")

	// Segments with custom names defined
	segs := Segments{
		{Name: "OS", Value: "Darwin"},
		{Name: "DISTRO", Value: ""},
		{Name: "ARCH", Value: "arm64"},
		{Name: "ROLE", Value: ""},
		{Name: "SITE", Value: ""},
	}

	// Load values from environment
	result := segs.LoadFromEnv()

	// Check values were loaded
	if result.Get("ROLE") != "server" {
		t.Errorf("ROLE = %q, want %q", result.Get("ROLE"), "server")
	}
	if result.Get("SITE") != "aws" {
		t.Errorf("SITE = %q, want %q", result.Get("SITE"), "aws")
	}

	// Original should be unchanged
	if segs.Get("ROLE") != "" {
		t.Error("original segs was modified")
	}

	// System segments should be preserved
	if result.Get("OS") != "Darwin" {
		t.Errorf("OS = %q, want %q", result.Get("OS"), "Darwin")
	}
}

func TestLoadFromEnvIgnoresUndefined(t *testing.T) {
	// Set env var for segment not in our list
	t.Setenv("WRIT_SEGMENT_UNKNOWN", "value")

	segs := Segments{
		{Name: "OS", Value: "Darwin"},
		{Name: "ROLE", Value: ""},
	}

	// Should not error, just ignores undefined env vars
	result := segs.LoadFromEnv()

	// ROLE should remain empty (no env var set for it)
	if result.Get("ROLE") != "" {
		t.Errorf("ROLE = %q, want empty", result.Get("ROLE"))
	}
}

func TestLoadFromEnvOverridesExisting(t *testing.T) {
	// Set env var that overrides an existing value
	t.Setenv("WRIT_SEGMENT_ROLE", "server")

	segs := Segments{
		{Name: "OS", Value: "Darwin"},
		{Name: "ROLE", Value: "desktop"}, // has a value
	}

	result := segs.LoadFromEnv()

	// Env var should override existing value
	if result.Get("ROLE") != "server" {
		t.Errorf("ROLE = %q, want %q", result.Get("ROLE"), "server")
	}
}

func TestEnvVarPrefix(t *testing.T) {
	// Verify the constant is what we expect
	if EnvVarPrefix != "WRIT_SEGMENT_" {
		t.Errorf("EnvVarPrefix = %q, want %q", EnvVarPrefix, "WRIT_SEGMENT_")
	}
}
