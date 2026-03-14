// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bindgen

import (
	"testing"
)

func TestGoType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"string", "string"},
		{"int", "int"},
		{"bool", "bool"},
		{"string_list", "[]string"},
		{"string_map", "map[string]string"},
		{"unknown", "string"}, // defaults to string
		{"", "string"},        // empty defaults to string
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := GoType(tt.input)
			if result != tt.expected {
				t.Errorf("GoType(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestStarlarkType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"string", "str"},
		{"int", "int"},
		{"bool", "bool"},
		{"string_list", "list"},
		{"string_map", "dict"},
		{"unknown", "str"}, // defaults to str
		{"", "str"},        // empty defaults to str
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := StarlarkType(tt.input)
			if result != tt.expected {
				t.Errorf("StarlarkType(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestBindingDef(t *testing.T) {
	def := &BindingDef{
		Name:        "docker",
		Description: "Docker container management",
		Commands: map[string]*Command{
			"run": {
				Description: "Run a container",
				Args: []*Arg{
					{
						Name:        "image",
						Type:        "string",
						Required:    true,
						Position:    0,
						Description: "Image to run",
					},
				},
				Flags: []*Flag{
					{
						Name:        "detach",
						Short:       "d",
						Type:        "bool",
						Default:     "false",
						Description: "Run container in background",
					},
				},
				Returns: &Return{
					Type:   "result",
					Fields: []string{"ok", "stdout", "stderr", "code"},
				},
			},
		},
	}

	if def.Name != "docker" {
		t.Errorf("expected ReceiverName 'docker', got %q", def.Name)
	}
	if def.Commands == nil {
		t.Fatal("expected Commands to be non-nil")
	}
	if _, ok := def.Commands["run"]; !ok {
		t.Error("expected 'run' command to exist")
	}
}

func TestCommand(t *testing.T) {
	cmd := &Command{
		Name:        "build",
		Description: "Build an image",
		Args: []*Arg{
			{Name: "path", Type: "string", Required: true, Position: 0},
		},
		Flags: []*Flag{
			{Name: "tag", Short: "t", Type: "string", Description: "Image tag"},
			{Name: "no-cache", Type: "bool", Default: "false"},
		},
		Returns: &Return{
			Type:   "result",
			Fields: []string{"ok", "stdout", "stderr"},
			Parse:  "none",
		},
	}

	if cmd.Name != "build" {
		t.Errorf("expected ReceiverName 'build', got %q", cmd.Name)
	}
	if len(cmd.Args) != 1 {
		t.Errorf("expected 1 arg, got %d", len(cmd.Args))
	}
	if len(cmd.Flags) != 2 {
		t.Errorf("expected 2 flags, got %d", len(cmd.Flags))
	}
	if cmd.Returns == nil {
		t.Error("expected Returns to be non-nil")
	}
}

func TestArg(t *testing.T) {
	arg := &Arg{
		Name:        "path",
		Type:        "string",
		Required:    true,
		Position:    0,
		Description: "Path to build context",
	}

	if arg.Name != "path" {
		t.Errorf("expected ReceiverName 'path', got %q", arg.Name)
	}
	if arg.Type != "string" {
		t.Errorf("expected ProviderType 'string', got %q", arg.Type)
	}
	if !arg.Required {
		t.Error("expected Required to be true")
	}
	if arg.Position != 0 {
		t.Errorf("expected Position 0, got %d", arg.Position)
	}
}

func TestFlag(t *testing.T) {
	flag := &Flag{
		Name:        "output",
		Short:       "o",
		Type:        "string",
		Default:     "json",
		Description: "Output format",
		Values:      []string{"json", "yaml", "table"},
	}

	if flag.Name != "output" {
		t.Errorf("expected ReceiverName 'output', got %q", flag.Name)
	}
	if flag.Short != "o" {
		t.Errorf("expected Short 'o', got %q", flag.Short)
	}
	if flag.Default != "json" {
		t.Errorf("expected Default 'json', got %q", flag.Default)
	}
	if len(flag.Values) != 3 {
		t.Errorf("expected 3 enum values, got %d", len(flag.Values))
	}
}

func TestReturn(t *testing.T) {
	ret := &Return{
		Type:   "result",
		Fields: []string{"ok", "stdout", "stderr", "code"},
		Parse:  "json",
	}

	if ret.Type != "result" {
		t.Errorf("expected ProviderType 'result', got %q", ret.Type)
	}
	if len(ret.Fields) != 4 {
		t.Errorf("expected 4 fields, got %d", len(ret.Fields))
	}
	if ret.Parse != "json" {
		t.Errorf("expected Parse 'json', got %q", ret.Parse)
	}
}

func TestAllFlagTypes(t *testing.T) {
	flags := []*Flag{
		{Name: "str", Type: "string"},
		{Name: "num", Type: "int"},
		{Name: "flag", Type: "bool"},
		{Name: "list", Type: "string_list"},
		{Name: "map", Type: "string_map"},
	}

	for _, f := range flags {
		goType := GoType(f.Type)
		starlarkType := StarlarkType(f.Type)

		if goType == "" {
			t.Errorf("GoType(%q) should not be empty", f.Type)
		}
		if starlarkType == "" {
			t.Errorf("StarlarkType(%q) should not be empty", f.Type)
		}
	}
}

func TestReturnTypes(t *testing.T) {
	types := []string{"result", "string", "bool", "list", "dict"}

	for _, retType := range types {
		ret := &Return{Type: retType}
		if ret.Type != retType {
			t.Errorf("expected return type %q, got %q", retType, ret.Type)
		}
	}
}

func TestParseTypes(t *testing.T) {
	parseTypes := []string{"none", "json", "lines"}

	for _, parseType := range parseTypes {
		ret := &Return{Type: "result", Parse: parseType}
		if ret.Parse != parseType {
			t.Errorf("expected parse type %q, got %q", parseType, ret.Parse)
		}
	}
}
