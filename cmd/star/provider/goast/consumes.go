// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package goast

import (
	"fmt"
	"strings"
)

// Consumes defines what block types a production accepts and how many.
// Parsed from ABNF-like notation in schema config.
type Consumes struct {
	Min   int      // minimum count (0 for optional, 1 for required)
	Max   int      // maximum count (-1 for unbounded)
	Types []string // allowed block types: "Paragraph", "Heading", "Code", "List"
}

// Matches returns true if the given block type is in the allowed set.
func (c Consumes) Matches(blockType string) bool {
	for _, t := range c.Types {
		if t == blockType {
			return true
		}
	}
	return false
}

// ParseConsumes parses an ABNF-like consumes string into a Consumes struct.
//
// Grammar:
//
//	consumes = [repeat] types
//	repeat   = "*"            → min=0, max=-1
//	         | number "*"     → min=number, max=-1
//	         | number "*" number → min=first, max=second
//	types    = type *("/" type)
//	         | "(" type *("/" type) ")"
//	type     = "Paragraph" | "Heading" | "Code" | "List"
//
// Examples:
//
//	"Paragraph"              → {1, 1, [Paragraph]}
//	"Paragraph / Heading"    → {1, 1, [Paragraph, Heading]}
//	"*Paragraph"             → {0, -1, [Paragraph]}
//	"*(Paragraph / Code)"    → {0, -1, [Paragraph, Code]}
//	"1*Paragraph"            → {1, -1, [Paragraph]}
//	"0*1Paragraph"           → {0, 1, [Paragraph]}
//	"0*1Paragraph List"      → sequence (not yet supported — use separate elements)
func ParseConsumes(s string) (Consumes, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Consumes{}, fmt.Errorf("empty consumes string")
	}

	min, max, rest := parseRepeat(s)
	types, err := parseTypes(rest)
	if err != nil {
		return Consumes{}, fmt.Errorf("parse consumes %q: %w", s, err)
	}

	return Consumes{Min: min, Max: max, Types: types}, nil
}

// parseRepeat extracts the optional repeat prefix from an ABNF string.
// Returns min, max, and the remaining string after the repeat.
func parseRepeat(s string) (int, int, string) {
	if len(s) == 0 {
		return 1, 1, s
	}

	// Leading "*" — zero or more.
	if s[0] == '*' {
		return 0, -1, s[1:]
	}

	// Leading digit — could be "N*", "N*M", or just a type name.
	if s[0] >= '0' && s[0] <= '9' {
		i := 0
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i < len(s) && s[i] == '*' {
			first := atoi(s[:i])
			rest := s[i+1:]

			// Check for "N*M" pattern.
			j := 0
			for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
				j++
			}
			if j > 0 {
				second := atoi(rest[:j])
				return first, second, rest[j:]
			}

			// "N*" — min=N, max=unbounded.
			return first, -1, rest
		}
	}

	// No repeat prefix — exactly one.
	return 1, 1, s
}

// parseTypes extracts the type list from the remaining ABNF string.
func parseTypes(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("no block types specified")
	}

	// Strip outer parens if present.
	if s[0] == '(' && s[len(s)-1] == ')' {
		s = s[1 : len(s)-1]
	}

	// Split on "/" for alternatives.
	parts := strings.Split(s, "/")
	var types []string
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t == "" {
			continue
		}
		if !isValidBlockType(t) {
			return nil, fmt.Errorf("unknown block type %q", t)
		}
		types = append(types, t)
	}

	if len(types) == 0 {
		return nil, fmt.Errorf("no block types specified")
	}
	return types, nil
}

func isValidBlockType(t string) bool {
	switch t {
	case "Paragraph", "Heading", "Code", "List":
		return true
	}
	return false
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}
