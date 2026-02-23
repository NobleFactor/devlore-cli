// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package ui provides user-facing terminal messaging for the operation graph.
//
//+devlore:access=immediate
package ui

import (
	"fmt"
	"io"
	"os"
)

// ANSI color codes.
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorGray   = "\033[37m"
)

// Status symbols.
const (
	symbolNote    = "+"
	symbolWarn    = "△"
	symbolError   = "✖"
	symbolSuccess = "✔"
)

// Provider provides user-facing terminal messaging.
type Provider struct {
	// Writer is the output destination. Defaults to os.Stderr.
	Writer io.Writer
	// ProgramName is the prefix for messages. Defaults to "devlore".
	ProgramName string
	// Silent suppresses all output when true.
	Silent bool
	// Color enables ANSI color codes. Defaults to true.
	Color bool
}

func (p *Provider) writer() io.Writer {
	if p.Writer != nil {
		return p.Writer
	}
	return os.Stderr
}

func (p *Provider) programName() string {
	if p.ProgramName != "" {
		return p.ProgramName
	}
	return "devlore"
}

func (p *Provider) colorize(color, text string) string {
	if p.Color {
		return color + text + colorReset
	}
	return text
}

//+devlore:access=immediate

// Note informs the user of progress.
func (p *Provider) Note(msg string) {
	if p.Silent {
		return
	}
	fmt.Fprintf(p.writer(), "[%s] [%s] %s\n", p.programName(), p.colorize(colorGray, symbolNote), msg) //nolint:errcheck // write to stderr
}

//+devlore:access=immediate

// Warn alerts the user to a potential issue.
func (p *Provider) Warn(msg string) {
	if p.Silent {
		return
	}
	fmt.Fprintf(p.writer(), "[%s] [%s] %s\n", p.programName(), p.colorize(colorYellow, symbolWarn), msg) //nolint:errcheck // write to stderr
}

//+devlore:access=immediate

// Error reports a non-fatal problem to the user.
func (p *Provider) Error(msg string) {
	if p.Silent {
		return
	}
	fmt.Fprintf(p.writer(), "[%s] [%s] %s\n", p.programName(), p.colorize(colorRed, symbolError), msg) //nolint:errcheck // write to stderr
}

//+devlore:access=immediate

// Success confirms completion to the user.
func (p *Provider) Success(msg string) {
	if p.Silent {
		return
	}
	fmt.Fprintf(p.writer(), "[%s] [%s] %s\n", p.programName(), p.colorize(colorGreen, symbolSuccess), msg) //nolint:errcheck // write to stderr
}

//+devlore:access=immediate

// Fail prints an error message and returns an error.
func (p *Provider) Fail(msg string) error {
	if !p.Silent {
		fmt.Fprintf(p.writer(), "[%s] [%s] %s\n", p.programName(), p.colorize(colorRed, symbolError), msg) //nolint:errcheck // write to stderr
	}
	return fmt.Errorf("%s", msg)
}
