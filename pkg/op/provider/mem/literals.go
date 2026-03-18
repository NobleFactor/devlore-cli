// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package mem

import (
	"fmt"
	"sort"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// FormatLiteral serializes a frozen Starlark value as a valid Starlark
// source literal. Used to inline closure bindings in synthetic files.
//
// Supports: String, Int, Float, Bool, NoneType, List, Dict, Tuple, Struct.
// Struct values (e.g., marshaled Resources) are serialized as dict literals
// with sorted keys for deterministic output.
// Returns an error for types that cannot be represented as source literals.
func FormatLiteral(v starlark.Value) (string, error) {
	return formatValue(v, 0)
}

func formatValue(v starlark.Value, depth int) (string, error) {
	if depth > 20 {
		return "", fmt.Errorf("FormatLiteral: nesting too deep (circular reference?)")
	}

	switch v := v.(type) {
	case starlark.NoneType:
		return "None", nil

	case starlark.Bool:
		if v {
			return "True", nil
		}
		return "False", nil

	case starlark.Int:
		return v.String(), nil

	case starlark.Float:
		return fmt.Sprintf("%g", float64(v)), nil

	case starlark.String:
		return quote(string(v)), nil

	case *starlark.List:
		return formatSequence("[", "]", v.Len(), v.Index, depth)

	case starlark.Tuple:
		if v.Len() == 1 {
			elem, err := formatValue(v.Index(0), depth+1)
			if err != nil {
				return "", err
			}
			return "(" + elem + ",)", nil
		}
		return formatSequence("(", ")", v.Len(), v.Index, depth)

	case *starlark.Dict:
		return formatDict(v, depth)

	case *op.StructValue:
		return formatAttrs(v, depth)

	case *starlarkstruct.Struct:
		return formatAttrs(v, depth)

	case *starlark.Set:
		return "", fmt.Errorf("FormatLiteral: set type not supported (use list)")

	default:
		return "", fmt.Errorf("FormatLiteral: unsupported type %s", v.Type())
	}
}

func formatSequence(open, close string, n int, index func(int) starlark.Value, depth int) (string, error) {
	var b strings.Builder
	b.WriteString(open)
	for i := range n {
		if i > 0 {
			b.WriteString(", ")
		}
		elem, err := formatValue(index(i), depth+1)
		if err != nil {
			return "", err
		}
		b.WriteString(elem)
	}
	b.WriteString(close)
	return b.String(), nil
}

func formatDict(d *starlark.Dict, depth int) (string, error) {
	var b strings.Builder
	b.WriteString("{")
	items := d.Items()
	for i, item := range items {
		if i > 0 {
			b.WriteString(", ")
		}
		k, err := formatValue(item[0], depth+1)
		if err != nil {
			return "", err
		}
		v, err := formatValue(item[1], depth+1)
		if err != nil {
			return "", err
		}
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(v)
	}
	b.WriteString("}")
	return b.String(), nil
}

func formatAttrs(s starlark.HasAttrs, depth int) (string, error) {
	names := s.AttrNames()
	sort.Strings(names) // deterministic ordering

	var b strings.Builder
	b.WriteString("{")
	first := true
	for _, name := range names {
		val, err := s.Attr(name)
		if err != nil {
			return "", fmt.Errorf("FormatLiteral: struct attr %q: %w", name, err)
		}
		if !first {
			b.WriteString(", ")
		}
		first = false
		b.WriteString(quote(name))
		b.WriteString(": ")
		formatted, err := formatValue(val, depth+1)
		if err != nil {
			return "", err
		}
		b.WriteString(formatted)
	}
	b.WriteString("}")
	return b.String(), nil
}

// quote produces a Starlark string literal with proper escaping.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
