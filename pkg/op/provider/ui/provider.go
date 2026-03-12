// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package ui provides user-facing terminal messaging for the operation graph.
package ui

import (
	"fmt"
	"io"
	"os"

	"github.com/NobleFactor/devlore-cli/pkg/op"
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

// Provider provides terminal status messaging.
//
// +devlore:access=immediate
// +devlore:bind Writer=Writer, ProgramName=ProgramName, Color=Color
type Provider struct {
	op.ProviderBase
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

// Error reports a non-fatal problem to the user.
func (p *Provider) Error(msg string) {
	if p.Silent {
		return
	}
	fmt.Fprintf(p.writer(), "[%s] [%s] %s\n", p.programName(), p.colorize(colorRed, symbolError), msg) //nolint:errcheck // write to stderr
}

// Fail prints an error message and returns an error.
func (p *Provider) Fail(msg string) error {
	if !p.Silent {
		fmt.Fprintf(p.writer(), "[%s] [%s] %s\n", p.programName(), p.colorize(colorRed, symbolError), msg) //nolint:errcheck // write to stderr
	}
	return fmt.Errorf("%s", msg)
}

// Note informs the user of progress.
func (p *Provider) Note(msg string) {
	if p.Silent {
		return
	}
	fmt.Fprintf(p.writer(), "[%s] [%s] %s\n", p.programName(), p.colorize(colorGray, symbolNote), msg) //nolint:errcheck // write to stderr
}

// Success confirms completion to the user.
func (p *Provider) Success(msg string) {
	if p.Silent {
		return
	}
	fmt.Fprintf(p.writer(), "[%s] [%s] %s\n", p.programName(), p.colorize(colorGreen, symbolSuccess), msg) //nolint:errcheck // write to stderr
}

// Warn alerts the user to a potential issue.
func (p *Provider) Warn(msg string) {
	if p.Silent {
		return
	}
	fmt.Fprintf(p.writer(), "[%s] [%s] %s\n", p.programName(), p.colorize(colorYellow, symbolWarn), msg) //nolint:errcheck // write to stderr
}
