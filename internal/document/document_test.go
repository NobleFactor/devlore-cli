// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package document

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testDoc is a simple struct used across all tests.
type testDoc struct {
	Name  string `yaml:"name" json:"name"`
	Count int    `yaml:"count" json:"count"`
}

// --- Read (io.Reader) ---

func TestRead_YAML(t *testing.T) {

	r := strings.NewReader("name: alice\ncount: 42\n")
	doc, err := Read[testDoc](r)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if doc.Name != "alice" {
		t.Errorf("Name = %q, want %q", doc.Name, "alice")
	}
	if doc.Count != 42 {
		t.Errorf("Count = %d, want %d", doc.Count, 42)
	}
}

func TestRead_JSON(t *testing.T) {

	r := strings.NewReader(`{"name":"bob","count":7}`)
	doc, err := Read[testDoc](r)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if doc.Name != "bob" {
		t.Errorf("Name = %q, want %q", doc.Name, "bob")
	}
	if doc.Count != 7 {
		t.Errorf("Count = %d, want %d", doc.Count, 7)
	}
}

// --- ReadFile ---

func TestReadFile_YAML(t *testing.T) {

	path := filepath.Join(t.TempDir(), "data.yaml")
	if err := os.WriteFile(path, []byte("name: alice\ncount: 42\n"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	doc, err := ReadFile[testDoc](path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if doc.Name != "alice" {
		t.Errorf("Name = %q, want %q", doc.Name, "alice")
	}
	if doc.Count != 42 {
		t.Errorf("Count = %d, want %d", doc.Count, 42)
	}
}

func TestReadFile_JSON(t *testing.T) {

	path := filepath.Join(t.TempDir(), "data.json")
	if err := os.WriteFile(path, []byte(`{"name":"bob","count":7}`), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	doc, err := ReadFile[testDoc](path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if doc.Name != "bob" {
		t.Errorf("Name = %q, want %q", doc.Name, "bob")
	}
	if doc.Count != 7 {
		t.Errorf("Count = %d, want %d", doc.Count, 7)
	}
}

func TestReadFile_MissingFileReturnsError(t *testing.T) {

	_, err := ReadFile[testDoc](filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "read ") {
		t.Errorf("error should contain 'read ' prefix: %v", err)
	}
}

func TestReadFile_MalformedContentReturnsParseError(t *testing.T) {

	path := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(path, []byte(":\n  :\n    - }{"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err := ReadFile[testDoc](path)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "parse ") {
		t.Errorf("error should contain 'parse ' prefix: %v", err)
	}
}

func TestReadFile_ErrorIncludesFilePath(t *testing.T) {

	path := filepath.Join(t.TempDir(), "missing.yaml")
	_, err := ReadFile[testDoc](path)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error should contain path %q: %v", path, err)
	}
}

// --- Write ---

func TestWrite_YAMLCreatesFileWith0o600(t *testing.T) {

	path := filepath.Join(t.TempDir(), "out.yaml")
	doc := testDoc{Name: "dave", Count: 99}

	if err := Write(path, &doc); err != nil {
		t.Fatalf("Write: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("permission = %o, want %o", perm, 0o600)
	}

	readBack, err := ReadFile[testDoc](path)
	if err != nil {
		t.Fatalf("ReadFile back: %v", err)
	}
	if readBack.Name != "dave" || readBack.Count != 99 {
		t.Errorf("round-trip failed: got %+v", readBack)
	}
}

func TestWrite_JSONCreatesFileWith0o600(t *testing.T) {

	path := filepath.Join(t.TempDir(), "out.json")
	doc := testDoc{Name: "eve", Count: 5}

	if err := Write(path, &doc); err != nil {
		t.Fatalf("Write: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("permission = %o, want %o", perm, 0o600)
	}

	readBack, err := ReadFile[testDoc](path)
	if err != nil {
		t.Fatalf("ReadFile back: %v", err)
	}
	if readBack.Name != "eve" || readBack.Count != 5 {
		t.Errorf("round-trip failed: got %+v", readBack)
	}
}

func TestWrite_CreatesParentDirectories(t *testing.T) {

	path := filepath.Join(t.TempDir(), "a", "b", "c", "deep.yaml")
	doc := testDoc{Name: "nested", Count: 1}

	if err := Write(path, &doc); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should exist: %v", err)
	}
}

func TestWrite_WithPermOverridesPermission(t *testing.T) {

	path := filepath.Join(t.TempDir(), "perm.yaml")
	doc := testDoc{Name: "frank", Count: 0}

	if err := Write(path, &doc, WithPerm(0o644)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("permission = %o, want %o", perm, 0o644)
	}
}

func TestWrite_WithHeaderPrependsText(t *testing.T) {

	path := filepath.Join(t.TempDir(), "header.yaml")
	doc := testDoc{Name: "grace", Count: 10}
	header := "# Auto-generated — do not edit\n"

	if err := Write(path, &doc, WithHeader(header)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	if !strings.HasPrefix(content, header) {
		t.Errorf("content should start with header:\n%s", content)
	}
	if !strings.Contains(content, "grace") {
		t.Errorf("content should contain serialized data:\n%s", content)
	}
}

func TestWrite_WithHeaderAppendsNewlineIfMissing(t *testing.T) {

	path := filepath.Join(t.TempDir(), "header2.yaml")
	doc := testDoc{Name: "heidi", Count: 2}

	if err := Write(path, &doc, WithHeader("# no trailing newline")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.HasPrefix(string(data), "# no trailing newline\n") {
		t.Errorf("header should have newline appended:\n%s", string(data))
	}
}

func TestWrite_JSONTrailingNewline(t *testing.T) {

	path := filepath.Join(t.TempDir(), "out.json")
	if err := Write(path, &testDoc{Name: "ivan", Count: 1}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Error("JSON output should end with a trailing newline")
	}
}

// --- formatFromExt ---

func TestFormatFromExt_JSON(t *testing.T) {

	if f := formatFromExt("config.json"); f != "json" {
		t.Errorf("formatFromExt(config.json) = %q, want json", f)
	}
}

func TestFormatFromExt_YAML(t *testing.T) {

	if f := formatFromExt("config.yaml"); f != "yaml" {
		t.Errorf("formatFromExt(config.yaml) = %q, want yaml", f)
	}
}

func TestFormatFromExt_YML(t *testing.T) {

	if f := formatFromExt("config.yml"); f != "yaml" {
		t.Errorf("formatFromExt(config.yml) = %q, want yaml", f)
	}
}

func TestFormatFromExt_UnknownDefaultsToYAML(t *testing.T) {

	if f := formatFromExt("config.toml"); f != "yaml" {
		t.Errorf("formatFromExt(config.toml) = %q, want yaml", f)
	}
}

func TestFormatFromExt_CaseInsensitive(t *testing.T) {

	if f := formatFromExt("config.JSON"); f != "json" {
		t.Errorf("formatFromExt(config.JSON) = %q, want json", f)
	}
}

// --- Round-trip ---

func TestRoundTrip_YAMLReadWritePreservesData(t *testing.T) {

	dir := t.TempDir()
	original := testDoc{Name: "round", Count: 77}

	path := filepath.Join(dir, "trip.yaml")
	if err := Write(path, &original); err != nil {
		t.Fatalf("Write: %v", err)
	}

	restored, err := ReadFile[testDoc](path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if *restored != original {
		t.Errorf("round-trip mismatch: got %+v, want %+v", *restored, original)
	}
}

func TestRoundTrip_JSONReadWritePreservesData(t *testing.T) {

	dir := t.TempDir()
	original := testDoc{Name: "trip", Count: 88}

	path := filepath.Join(dir, "trip.json")
	if err := Write(path, &original); err != nil {
		t.Fatalf("Write: %v", err)
	}

	restored, err := ReadFile[testDoc](path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if *restored != original {
		t.Errorf("round-trip mismatch: got %+v, want %+v", *restored, original)
	}
}
