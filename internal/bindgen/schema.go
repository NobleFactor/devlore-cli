// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

// Package bindgen provides tools for generating Starlark bindings from CLI metadata.
package bindgen

// BindingDef defines a CLI tool's Starlark binding.
type BindingDef struct {
	Name        string              `yaml:"name"`        // e.g., "docker"
	Description string              `yaml:"description"` // Brief description
	Commands    map[string]*Command `yaml:"commands"`    // Subcommands/methods
}

// Command defines a single CLI command or subcommand.
type Command struct {
	Name        string  `yaml:"-"`           // Populated from map key
	Description string  `yaml:"description"` // From --help or manual
	Args        []*Arg  `yaml:"args"`        // Positional arguments
	Flags       []*Flag `yaml:"flags"`       // Named flags
	Returns     *Return `yaml:"returns"`     // Return type specification
}

// Arg defines a positional argument.
type Arg struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"`        // string, int, bool, string_list
	Required    bool   `yaml:"required"`
	Position    int    `yaml:"position"`    // 0-indexed
	Description string `yaml:"description"`
}

// Flag defines a named flag/option.
type Flag struct {
	Name        string   `yaml:"name"`        // Long name without --
	Short       string   `yaml:"short"`       // Single char without -
	Type        string   `yaml:"type"`        // string, int, bool, string_list, string_map
	Default     string   `yaml:"default"`     // Default value as string
	Description string   `yaml:"description"`
	Values      []string `yaml:"values"`      // Enum values if applicable
}

// Return defines what a command returns.
type Return struct {
	Type   string   `yaml:"type"`   // result, string, bool, list, dict
	Fields []string `yaml:"fields"` // For result type: ok, stdout, stderr, code
	Parse  string   `yaml:"parse"`  // none, json, lines
}

// GoType returns the Go type for a flag/arg type.
func GoType(t string) string {
	switch t {
	case "string":
		return "string"
	case "int":
		return "int"
	case "bool":
		return "bool"
	case "string_list":
		return "[]string"
	case "string_map":
		return "map[string]string"
	default:
		return "string"
	}
}

// StarlarkType returns the Starlark type annotation.
func StarlarkType(t string) string {
	switch t {
	case "string":
		return "str"
	case "int":
		return "int"
	case "bool":
		return "bool"
	case "string_list":
		return "list"
	case "string_map":
		return "dict"
	default:
		return "str"
	}
}
