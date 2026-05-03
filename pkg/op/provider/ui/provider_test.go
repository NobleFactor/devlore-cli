// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package ui

import (
	"errors"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// captureUI is a [status.UI] test double that records every method call. After the [Provider] thin
// adapter rewrite, the provider holds no rendering state of its own — every method forwards directly
// to the [status.UI] on the runtime environment. These tests verify the forwarding wiring; rendering
// behavior (program-name prefix, color codes, silent gating) is exercised in pkg/status/console
// tests.
type captureUI struct {
	notes     []string
	warns     []string
	errors    []string
	successes []string
	fails     []string
	prints    []string
}

func (c *captureUI) Note(msg string)    { c.notes = append(c.notes, msg) }
func (c *captureUI) Warn(msg string)    { c.warns = append(c.warns, msg) }
func (c *captureUI) Error(msg string)   { c.errors = append(c.errors, msg) }
func (c *captureUI) Success(msg string) { c.successes = append(c.successes, msg) }
func (c *captureUI) Fail(msg string) error {
	c.fails = append(c.fails, msg)
	return errors.New(msg)
}
func (c *captureUI) Print(msg string) { c.prints = append(c.prints, msg) }

// newProviderWithCapture returns a *Provider whose runtime environment carries the given captureUI as
// its Status. The captureUI records every forwarded call.
func newProviderWithCapture(t *testing.T) (*Provider, *captureUI) {
	t.Helper()
	capture := &captureUI{}
	env := &op.RuntimeEnvironment{Status: capture}
	return NewProvider(env), capture
}

func TestProviderNoteForwards(t *testing.T) {
	p, capture := newProviderWithCapture(t)

	p.Note("hello")

	if len(capture.notes) != 1 || capture.notes[0] != "hello" {
		t.Errorf("Note forward: got %v, want [hello]", capture.notes)
	}
}

func TestProviderWarnForwards(t *testing.T) {
	p, capture := newProviderWithCapture(t)

	p.Warn("alert")

	if len(capture.warns) != 1 || capture.warns[0] != "alert" {
		t.Errorf("Warn forward: got %v, want [alert]", capture.warns)
	}
}

func TestProviderErrorForwards(t *testing.T) {
	p, capture := newProviderWithCapture(t)

	p.Error("oops")

	if len(capture.errors) != 1 || capture.errors[0] != "oops" {
		t.Errorf("Error forward: got %v, want [oops]", capture.errors)
	}
}

func TestProviderSuccessForwards(t *testing.T) {
	p, capture := newProviderWithCapture(t)

	p.Success("done")

	if len(capture.successes) != 1 || capture.successes[0] != "done" {
		t.Errorf("Success forward: got %v, want [done]", capture.successes)
	}
}

func TestProviderFailForwards(t *testing.T) {
	p, capture := newProviderWithCapture(t)

	err := p.Fail("broken")

	if err == nil {
		t.Fatal("Fail() returned nil error, want non-nil")
	}
	if err.Error() != "broken" {
		t.Errorf("error text = %q, want %q", err.Error(), "broken")
	}
	if len(capture.fails) != 1 || capture.fails[0] != "broken" {
		t.Errorf("Fail forward: got %v, want [broken]", capture.fails)
	}
}

func TestProviderPrintForwards(t *testing.T) {
	p, capture := newProviderWithCapture(t)

	p.Print("raw text")

	if len(capture.prints) != 1 || capture.prints[0] != "raw text" {
		t.Errorf("Print forward: got %v, want [raw text]", capture.prints)
	}
}
