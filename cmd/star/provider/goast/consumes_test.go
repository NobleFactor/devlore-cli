// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package goast

import (
	"testing"
)

func TestParseConsumes_ExactlyOne(t *testing.T) {
	c, err := ParseConsumes("Paragraph")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Min != 1 || c.Max != 1 {
		t.Errorf("expected {1, 1}, got {%d, %d}", c.Min, c.Max)
	}
	if len(c.Types) != 1 || c.Types[0] != "Paragraph" {
		t.Errorf("expected [Paragraph], got %v", c.Types)
	}
}

func TestParseConsumes_Alternative(t *testing.T) {
	c, err := ParseConsumes("Paragraph / Heading")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Min != 1 || c.Max != 1 {
		t.Errorf("expected {1, 1}, got {%d, %d}", c.Min, c.Max)
	}
	if len(c.Types) != 2 {
		t.Fatalf("expected 2 types, got %d", len(c.Types))
	}
	if c.Types[0] != "Paragraph" || c.Types[1] != "Heading" {
		t.Errorf("expected [Paragraph, Heading], got %v", c.Types)
	}
}

func TestParseConsumes_ZeroOrMore(t *testing.T) {
	c, err := ParseConsumes("*Paragraph")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Min != 0 || c.Max != -1 {
		t.Errorf("expected {0, -1}, got {%d, %d}", c.Min, c.Max)
	}
	if len(c.Types) != 1 || c.Types[0] != "Paragraph" {
		t.Errorf("expected [Paragraph], got %v", c.Types)
	}
}

func TestParseConsumes_ZeroOrMoreAlternatives(t *testing.T) {
	c, err := ParseConsumes("*(Paragraph / Code)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Min != 0 || c.Max != -1 {
		t.Errorf("expected {0, -1}, got {%d, %d}", c.Min, c.Max)
	}
	if len(c.Types) != 2 {
		t.Fatalf("expected 2 types, got %d", len(c.Types))
	}
	if c.Types[0] != "Paragraph" || c.Types[1] != "Code" {
		t.Errorf("expected [Paragraph, Code], got %v", c.Types)
	}
}

func TestParseConsumes_ZeroOrMoreTriple(t *testing.T) {
	c, err := ParseConsumes("*(Paragraph / Code / Heading)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Min != 0 || c.Max != -1 {
		t.Errorf("expected {0, -1}, got {%d, %d}", c.Min, c.Max)
	}
	if len(c.Types) != 3 {
		t.Fatalf("expected 3 types, got %d", len(c.Types))
	}
}

func TestParseConsumes_OneOrMore(t *testing.T) {
	c, err := ParseConsumes("1*Paragraph")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Min != 1 || c.Max != -1 {
		t.Errorf("expected {1, -1}, got {%d, %d}", c.Min, c.Max)
	}
}

func TestParseConsumes_Optional(t *testing.T) {
	c, err := ParseConsumes("0*1Paragraph")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Min != 0 || c.Max != 1 {
		t.Errorf("expected {0, 1}, got {%d, %d}", c.Min, c.Max)
	}
}

func TestParseConsumes_List(t *testing.T) {
	c, err := ParseConsumes("List")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Min != 1 || c.Max != 1 {
		t.Errorf("expected {1, 1}, got {%d, %d}", c.Min, c.Max)
	}
	if len(c.Types) != 1 || c.Types[0] != "List" {
		t.Errorf("expected [List], got %v", c.Types)
	}
}

func TestParseConsumes_InvalidType(t *testing.T) {
	_, err := ParseConsumes("Bogus")
	if err == nil {
		t.Error("expected error for unknown block type")
	}
}

func TestParseConsumes_Empty(t *testing.T) {
	_, err := ParseConsumes("")
	if err == nil {
		t.Error("expected error for empty string")
	}
}

func TestConsumes_Matches(t *testing.T) {
	c := Consumes{Types: []string{"Paragraph", "Code"}}
	if !c.Matches("Paragraph") {
		t.Error("should match Paragraph")
	}
	if !c.Matches("Code") {
		t.Error("should match Code")
	}
	if c.Matches("Heading") {
		t.Error("should not match Heading")
	}
	if c.Matches("List") {
		t.Error("should not match List")
	}
}
