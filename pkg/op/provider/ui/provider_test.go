// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestNote(t *testing.T) {
	var buf bytes.Buffer
	p := Provider{Writer: &buf}
	p.Note("hello")

	got := buf.String()
	if !strings.Contains(got, "[devlore]") {
		t.Errorf("output missing program name: %q", got)
	}
	if !strings.Contains(got, "["+symbolNote+"]") {
		t.Errorf("output missing note symbol: %q", got)
	}
	if !strings.Contains(got, "hello") {
		t.Errorf("output missing message: %q", got)
	}
}

func TestWarn(t *testing.T) {
	var buf bytes.Buffer
	p := Provider{Writer: &buf, Color: true}
	p.Warn("alert")

	got := buf.String()
	if !strings.Contains(got, "[devlore]") {
		t.Errorf("output missing program name: %q", got)
	}
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

func TestError(t *testing.T) {
	var buf bytes.Buffer
	p := Provider{Writer: &buf, Color: true}
	p.Error("oops")

	got := buf.String()
	if !strings.Contains(got, "[devlore]") {
		t.Errorf("output missing program name: %q", got)
	}
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

func TestSuccess(t *testing.T) {
	var buf bytes.Buffer
	p := Provider{Writer: &buf, Color: true}
	p.Success("done")

	got := buf.String()
	if !strings.Contains(got, "[devlore]") {
		t.Errorf("output missing program name: %q", got)
	}
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

func TestFail(t *testing.T) {
	var buf bytes.Buffer
	p := Provider{Writer: &buf, Color: true}

	err := p.Fail("broken")
	if err == nil {
		t.Fatal("Fail() returned nil error, want non-nil")
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

func TestSilent(t *testing.T) {
	methods := []struct {
		name string
		call func(p *Provider)
	}{
		{"Note", func(p *Provider) { p.Note("hidden") }},
		{"Warn", func(p *Provider) { p.Warn("hidden") }},
		{"Error", func(p *Provider) { p.Error("hidden") }},
		{"Success", func(p *Provider) { p.Success("hidden") }},
	}

	for _, m := range methods {
		t.Run(m.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := Provider{Writer: &buf, Silent: true}
			m.call(&p)

			if buf.Len() != 0 {
				t.Errorf("Silent %s produced output: %q", m.name, buf.String())
			}
		})
	}

	t.Run("Fail", func(t *testing.T) {
		var buf bytes.Buffer
		p := Provider{Writer: &buf, Silent: true}
		err := p.Fail("hidden")

		if buf.Len() != 0 {
			t.Errorf("Silent Fail produced output: %q", buf.String())
		}
		if err == nil {
			t.Fatal("Silent Fail returned nil error, want non-nil")
		}
		if err.Error() != "hidden" {
			t.Errorf("error text = %q, want %q", err.Error(), "hidden")
		}
	})
}

func TestColorDisabled(t *testing.T) {
	methods := []struct {
		name string
		call func(p *Provider)
	}{
		{"Note", func(p *Provider) { p.Note("plain") }},
		{"Warn", func(p *Provider) { p.Warn("plain") }},
		{"Error", func(p *Provider) { p.Error("plain") }},
		{"Success", func(p *Provider) { p.Success("plain") }},
		{"Fail", func(p *Provider) { _ = p.Fail("plain") }},
	}

	for _, m := range methods {
		t.Run(m.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := Provider{Writer: &buf, Color: false}
			m.call(&p)

			got := buf.String()
			if strings.Contains(got, "\033[") {
				t.Errorf("%s with Color=false contains ANSI escape: %q", m.name, got)
			}
			if !strings.Contains(got, "plain") {
				t.Errorf("%s output missing message: %q", m.name, got)
			}
		})
	}
}

func TestCustomProgramName(t *testing.T) {
	var buf bytes.Buffer
	p := Provider{Writer: &buf, ProgramName: "myapp"}
	p.Note("msg")

	got := buf.String()
	if !strings.Contains(got, "[myapp]") {
		t.Errorf("output missing custom program name: %q", got)
	}
	if strings.Contains(got, "[devlore]") {
		t.Errorf("output contains default program name when custom set: %q", got)
	}
}
