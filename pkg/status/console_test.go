// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package status

import (
	"bytes"
	"strings"
	"testing"
)

// region Console rendering

func TestConsoleNoteRenders(t *testing.T) {

	var buf bytes.Buffer
	c := NewConsole(&buf, "lore", true, false)

	c.Note("hello")

	got := buf.String()
	if !strings.Contains(got, "[lore]") {
		t.Errorf("output missing program name: %q", got)
	}
	if !strings.Contains(got, symbolNote) {
		t.Errorf("output missing note symbol: %q", got)
	}
	if !strings.Contains(got, "hello") {
		t.Errorf("output missing message: %q", got)
	}
	if !strings.Contains(got, colorGray) {
		t.Errorf("output missing gray color code: %q", got)
	}
}

func TestConsoleWarnRenders(t *testing.T) {

	var buf bytes.Buffer
	c := NewConsole(&buf, "lore", true, false)

	c.Warn("alert")

	got := buf.String()
	if !strings.Contains(got, symbolWarn) {
		t.Errorf("output missing warn symbol: %q", got)
	}
	if !strings.Contains(got, "alert") {
		t.Errorf("output missing message: %q", got)
	}
	if !strings.Contains(got, colorYellow) {
		t.Errorf("output missing yellow color code: %q", got)
	}
}

func TestConsoleErrorRenders(t *testing.T) {

	var buf bytes.Buffer
	c := NewConsole(&buf, "lore", true, false)

	c.Error("oops")

	got := buf.String()
	if !strings.Contains(got, symbolError) {
		t.Errorf("output missing error symbol: %q", got)
	}
	if !strings.Contains(got, "oops") {
		t.Errorf("output missing message: %q", got)
	}
	if !strings.Contains(got, colorRed) {
		t.Errorf("output missing red color code: %q", got)
	}
}

func TestConsoleSuccessRenders(t *testing.T) {

	var buf bytes.Buffer
	c := NewConsole(&buf, "lore", true, false)

	c.Succeed("done")

	got := buf.String()
	if !strings.Contains(got, symbolSuccess) {
		t.Errorf("output missing success symbol: %q", got)
	}
	if !strings.Contains(got, "done") {
		t.Errorf("output missing message: %q", got)
	}
	if !strings.Contains(got, colorGreen) {
		t.Errorf("output missing green color code: %q", got)
	}
}

func TestConsoleFailRendersAndReturnsError(t *testing.T) {

	var buf bytes.Buffer
	c := NewConsole(&buf, "lore", true, false)

	err := c.Fail("broken")

	if err == nil {
		t.Fatal("Fail returned nil error, want non-nil")
	}
	if err.Error() != "broken" {
		t.Errorf("error text = %q, want %q", err.Error(), "broken")
	}

	got := buf.String()
	if !strings.Contains(got, symbolError) {
		t.Errorf("output missing error symbol: %q", got)
	}
	if !strings.Contains(got, "broken") {
		t.Errorf("output missing message: %q", got)
	}
	if !strings.Contains(got, colorRed) {
		t.Errorf("output missing red color code: %q", got)
	}
}

func TestConsolePrintEmitsRaw(t *testing.T) {

	var buf bytes.Buffer
	c := NewConsole(&buf, "lore", true, false)

	c.Print("script said this")

	got := buf.String()
	if got != "script said this\n" {
		t.Errorf("Print output = %q, want %q", got, "script said this\n")
	}
	if strings.Contains(got, "[lore]") {
		t.Errorf("Print should not emit program-name prefix: %q", got)
	}
	if strings.Contains(got, "\033[") {
		t.Errorf("Print should not emit ANSI codes: %q", got)
	}
}

// endregion

// region Console color and silent toggles

func TestConsoleColorDisabled(t *testing.T) {

	for _, tc := range []struct {
		name string
		call func(*Console)
	}{
		{"Note", func(c *Console) { c.Note("plain") }},
		{"Warn", func(c *Console) { c.Warn("plain") }},
		{"Error", func(c *Console) { c.Error("plain") }},
		{"Success", func(c *Console) { c.Succeed("plain") }},
		{"Fail", func(c *Console) { _ = c.Fail("plain") }},
	} {
		t.Run(tc.name, func(t *testing.T) {

			var buf bytes.Buffer
			c := NewConsole(&buf, "lore", false, false)

			tc.call(c)

			got := buf.String()
			if strings.Contains(got, "\033[") {
				t.Errorf("%s with color=false contains ANSI escape: %q", tc.name, got)
			}
			if !strings.Contains(got, "plain") {
				t.Errorf("%s output missing message: %q", tc.name, got)
			}
		})
	}
}

func TestConsoleSilentSuppressesAllMethods(t *testing.T) {

	for _, tc := range []struct {
		name string
		call func(*Console)
	}{
		{"Note", func(c *Console) { c.Note("hidden") }},
		{"Warn", func(c *Console) { c.Warn("hidden") }},
		{"Error", func(c *Console) { c.Error("hidden") }},
		{"Success", func(c *Console) { c.Succeed("hidden") }},
		{"Print", func(c *Console) { c.Print("hidden") }},
	} {
		t.Run(tc.name, func(t *testing.T) {

			var buf bytes.Buffer
			c := NewConsole(&buf, "lore", true, true)

			tc.call(c)

			if buf.Len() != 0 {
				t.Errorf("Silent %s emitted: %q", tc.name, buf.String())
			}
		})
	}

	t.Run("Fail emits nothing but still returns error", func(t *testing.T) {

		var buf bytes.Buffer
		c := NewConsole(&buf, "lore", true, true)

		err := c.Fail("hidden")

		if buf.Len() != 0 {
			t.Errorf("Silent Fail emitted: %q", buf.String())
		}
		if err == nil {
			t.Fatal("Silent Fail returned nil error, want non-nil")
		}
		if err.Error() != "hidden" {
			t.Errorf("Silent Fail error text = %q, want %q", err.Error(), "hidden")
		}
	})
}

// endregion

// region NoOp

func TestNoOpAllMethodsSilent(t *testing.T) {

	// NoOp methods take no buffer; the test verifies the methods don't panic and Fail returns
	// a non-nil error. There's no observable side effect to check.
	noop := Discard{}

	noop.Note("ignored")
	noop.Warn("ignored")
	noop.Error("ignored")
	noop.Succeed("ignored")
	noop.Print("ignored")

	err := noop.Fail("propagates")
	if err == nil {
		t.Fatal("NoOp.Fail returned nil error, want non-nil")
	}
	if err.Error() != "propagates" {
		t.Errorf("NoOp.Fail error text = %q, want %q", err.Error(), "propagates")
	}
}

// endregion
