// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package goast

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// =============================================================================
// TEST HELPERS
// =============================================================================

// newTestProvider creates a Provider suitable for unit tests.
func newTestProvider() *Provider {
	return NewProvider(&op.RuntimeEnvironment{})
}

// writeTempGoFile writes content to a temporary.go file and returns its path.
//
// The caller must remove the file.
func writeTempGoFile(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	return path
}

// =============================================================================
// SortDeclarations TESTS
// =============================================================================

func TestSortDeclarations_AlreadySorted(t *testing.T) {

	src := `package foo

func Alpha() {}

func Beta() {}

func Gamma() {}
`
	path := writeTempGoFile(t, src)
	p := newTestProvider()

	got, err := p.SortDeclarations(path, "file", "alphabetical")
	if err != nil {
		t.Fatalf("SortDeclarations: %v", err)
	}

	if got != src {
		t.Errorf("expected no change for already-sorted, got:\n%s", got)
	}
}

func TestSortDeclarations_Reorders(t *testing.T) {

	src := `package foo

func Gamma() {}

func Alpha() {}

func Beta() {}
`
	path := writeTempGoFile(t, src)
	p := newTestProvider()

	got, err := p.SortDeclarations(path, "file", "alphabetical")
	if err != nil {
		t.Fatalf("SortDeclarations: %v", err)
	}

	alphaIdx := strings.Index(got, "func Alpha()")
	betaIdx := strings.Index(got, "func Beta()")
	gammaIdx := strings.Index(got, "func Gamma()")

	if alphaIdx < 0 || betaIdx < 0 || gammaIdx < 0 {
		t.Fatalf("missing function declarations in output:\n%s", got)
	}

	if alphaIdx >= betaIdx || betaIdx >= gammaIdx {
		t.Errorf("functions not in alphabetical order: Alpha@%d Beta@%d Gamma@%d", alphaIdx, betaIdx, gammaIdx)
	}
}

func TestSortDeclarations_PreservesDocComments(t *testing.T) {

	src := `package foo

// Gamma does gamma things.
func Gamma() {}

// Alpha does alpha things.
func Alpha() {}
`
	path := writeTempGoFile(t, src)
	p := newTestProvider()

	got, err := p.SortDeclarations(path, "file", "alphabetical")
	if err != nil {
		t.Fatalf("SortDeclarations: %v", err)
	}

	// Alpha's doc comment should appear before Alpha's function.
	alphaDocIdx := strings.Index(got, "// Alpha does alpha things.")
	alphaFuncIdx := strings.Index(got, "func Alpha()")

	if alphaDocIdx < 0 || alphaFuncIdx < 0 {
		t.Fatalf("missing Alpha in output:\n%s", got)
	}

	if alphaDocIdx >= alphaFuncIdx {
		t.Errorf("Alpha doc comment not before func declaration")
	}

	// Alpha should come before Gamma.
	gammaFuncIdx := strings.Index(got, "func Gamma()")
	if alphaFuncIdx >= gammaFuncIdx {
		t.Errorf("Alpha should come before Gamma")
	}
}

func TestSortDeclarations_LineScope(t *testing.T) {

	src := `package foo

func First() {}

func Gamma() {}

func Alpha() {}

func Last() {}
`
	path := writeTempGoFile(t, src)
	p := newTestProvider()

	// Sort only lines 5-7 (Gamma and Alpha), leaving First and Last in place.
	got, err := p.SortDeclarations(path, "lines:5-7", "alphabetical")
	if err != nil {
		t.Fatalf("SortDeclarations: %v", err)
	}

	firstIdx := strings.Index(got, "func First()")
	alphaIdx := strings.Index(got, "func Alpha()")
	gammaIdx := strings.Index(got, "func Gamma()")
	lastIdx := strings.Index(got, "func Last()")

	if firstIdx < 0 || alphaIdx < 0 || gammaIdx < 0 || lastIdx < 0 {
		t.Fatalf("missing function declarations in output:\n%s", got)
	}

	// First should still be first, Last should still be last.
	if firstIdx >= alphaIdx {
		t.Errorf("First should come before Alpha")
	}

	// Within the scope, Alpha should now come before Gamma.
	if alphaIdx >= gammaIdx {
		t.Errorf("Alpha should come before Gamma within scope")
	}

	if gammaIdx >= lastIdx {
		t.Errorf("Gamma should come before Last")
	}
}

func TestSortDeclarations_SingleDecl(t *testing.T) {

	src := `package foo

func Only() {}
`
	path := writeTempGoFile(t, src)
	p := newTestProvider()

	got, err := p.SortDeclarations(path, "file", "alphabetical")
	if err != nil {
		t.Fatalf("SortDeclarations: %v", err)
	}

	if got != src {
		t.Errorf("expected no change for single decl, got:\n%s", got)
	}
}

func TestSortDeclarations_UnknownOrder(t *testing.T) {

	src := `package foo

func A() {}

func B() {}
`
	path := writeTempGoFile(t, src)
	p := newTestProvider()

	_, err := p.SortDeclarations(path, "file", "reverse")
	if err == nil {
		t.Fatal("expected error for unknown order")
	}
}

func TestSortDeclarations_InvalidScope(t *testing.T) {

	src := `package foo

func A() {}
`
	path := writeTempGoFile(t, src)
	p := newTestProvider()

	_, err := p.SortDeclarations(path, "invalid", "alphabetical")
	if err == nil {
		t.Fatal("expected error for invalid scope")
	}
}

func TestSortDeclarations_FileNotFound(t *testing.T) {

	p := newTestProvider()

	_, err := p.SortDeclarations("/nonexistent/file.go", "file", "alphabetical")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}
