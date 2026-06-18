// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package config

import (
	"testing"
)

func TestConfigElement_Path(t *testing.T) {
	elem := &ConfigElement{path: "lint.copyright"}
	if got := elem.Path(); got != "lint.copyright" {
		t.Errorf("Path() = %q, want %q", got, "lint.copyright")
	}
}

func TestConfigElement_SetPath(t *testing.T) {
	elem := &ConfigElement{}
	elem.SetPath("lint.go")
	if got := elem.Path(); got != "lint.go" {
		t.Errorf("Path() = %q, want %q", got, "lint.go")
	}
}

func TestConfigElement_Register(t *testing.T) {
	root := &ConfigElement{path: ""}
	child := &ConfigElement{}

	root.Register("lint", child)

	// Child should be registered
	if got := root.Get("lint"); got != child {
		t.Error("Get(lint) should return registered child")
	}

	// Child path should be set
	if got := child.Path(); got != "lint" {
		t.Errorf("child.Path() = %q, want %q", got, "lint")
	}
}

func TestConfigElement_Register_Nested(t *testing.T) {
	root := &ConfigElement{path: ""}
	lint := &ConfigElement{}
	copyright := &ConfigElement{}

	root.Register("lint", lint)
	lint.Register("copyright", copyright)

	// Paths should be correct
	if got := lint.Path(); got != "lint" {
		t.Errorf("lint.Path() = %q, want %q", got, "lint")
	}
	if got := copyright.Path(); got != "lint.copyright" {
		t.Errorf("copyright.Path() = %q, want %q", got, "lint.copyright")
	}
}

func TestConfigElement_Get_NotFound(t *testing.T) {
	elem := &ConfigElement{}
	if got := elem.Get("missing"); got != nil {
		t.Errorf("Get(missing) = %v, want nil", got)
	}
}

func TestConfigElement_Children(t *testing.T) {
	root := &ConfigElement{}
	child1 := &ConfigElement{}
	child2 := &ConfigElement{}

	root.Register("a", child1)
	root.Register("b", child2)

	children := root.Children()
	if len(children) != 2 {
		t.Errorf("len(Children()) = %d, want 2", len(children))
	}
	if children["a"] != child1 || children["b"] != child2 {
		t.Error("Children() should contain registered children")
	}
}

func TestConfigElement_Navigate_Empty(t *testing.T) {
	elem := &ConfigElement{path: "some-path"}
	if got := elem.Navigate(""); got != elem {
		t.Error("Navigate('') should return the element itself")
	}
}

func TestConfigElement_Navigate_SingleLevel(t *testing.T) {
	root := &ConfigElement{}
	lint := &ConfigElement{}
	root.Register("lint", lint)

	if got := root.Navigate("lint"); got != lint {
		t.Error("Navigate('lint') should return lint child")
	}
}

func TestConfigElement_Navigate_MultiLevel(t *testing.T) {
	root := &ConfigElement{}
	lint := &ConfigElement{}
	copyright := &ConfigElement{}

	root.Register("lint", lint)
	lint.Register("copyright", copyright)

	if got := root.Navigate("lint.copyright"); got != copyright {
		t.Error("Navigate('lint.copyright') should return copyright child")
	}
}

func TestConfigElement_Navigate_NotFound(t *testing.T) {
	root := &ConfigElement{}
	lint := &ConfigElement{}
	root.Register("lint", lint)

	if got := root.Navigate("missing"); got != nil {
		t.Errorf("Navigate('missing') = %v, want nil", got)
	}
	if got := root.Navigate("lint.missing"); got != nil {
		t.Errorf("Navigate('lint.missing') = %v, want nil", got)
	}
}

// TestStruct is a test struct that embeds ConfigElement
type TestStruct struct {
	ConfigElement
	Enabled bool
	Name    string
	Count   int
}

func TestConfigElement_Navigate_StructField(t *testing.T) {
	root := &ConfigElement{}
	testStruct := &TestStruct{
		Enabled: true,
		Name:    "test",
		Count:   42,
	}
	root.Register("test", testStruct)

	// Navigate to struct
	if got := root.Navigate("test"); got != testStruct {
		t.Error("Navigate('test') should return testStruct")
	}

	// Navigate to field
	if got := root.Navigate("test.enabled"); got != true {
		t.Errorf("Navigate('test.enabled') = %v, want true", got)
	}
	if got := root.Navigate("test.name"); got != "test" {
		t.Errorf("Navigate('test.name') = %v, want 'test'", got)
	}
	if got := root.Navigate("test.count"); got != 42 {
		t.Errorf("Navigate('test.count') = %v, want 42", got)
	}
}

func TestConfigElement_Register_StructWithEmbeddedElement(t *testing.T) {
	root := &ConfigElement{}
	testStruct := &TestStruct{}

	root.Register("test", testStruct)

	// Path should be set on embedded ConfigElement
	if got := testStruct.Path(); got != "test" {
		t.Errorf("testStruct.Path() = %q, want %q", got, "test")
	}
}

func TestToPascalCase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"enabled", "Enabled"},
		{"skip_mod_tidy", "SkipModTidy"},
		{"pass_filenames", "PassFilenames"},
		{"id", "Id"},
		{"already_Pascal", "AlreadyPascal"},
		{"kebab-case", "KebabCase"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := toPascalCase(tt.input); got != tt.want {
				t.Errorf("toPascalCase(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestToSnakeCase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"Enabled", "enabled"},
		{"SkipModTidy", "skip_mod_tidy"},
		{"PassFilenames", "pass_filenames"},
		{"ID", "i_d"}, // Known limitation: consecutive capitals
		{"enabled", "enabled"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := toSnakeCase(tt.input); got != tt.want {
				t.Errorf("toSnakeCase(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
