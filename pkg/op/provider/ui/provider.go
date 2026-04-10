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

func NewProvider(ctx *op.ExecutionContext) *Provider {
	return &Provider{
		ProviderBase: op.NewProviderBase(ctx),
		Writer:       ctx.Writer,
		ProgramName:  ctx.ProgramName,
		Color:        true,
		Silent:       false,
	}
}

// region EXPORTED METHODS

// region Behaviors

// Actions

// Error reports a non-fatal problem to the user.
//
// Parameters:
//   - msg: the error message to display.
func (p *Provider) Error(msg string) {

	if p.Silent {
		return
	}
	_, err := fmt.Fprintf(p.writer(), "[%s] [%s] %s\n", p.programName(), p.colorize(colorRed, symbolError), msg)
	if err != nil {
		return
	}
}

// Note informs the user of progress.
//
// Parameters:
//   - msg: the informational message to display.
func (p *Provider) Note(msg string) {

	if p.Silent {
		return
	}
	_, err := fmt.Fprintf(p.writer(), "[%s] [%s] %s\n", p.programName(), p.colorize(colorGray, symbolNote), msg)
	if err != nil {
		return
	}
}

// Success confirms completion to the user.
//
// Parameters:
//   - msg: the success message to display.
func (p *Provider) Success(msg string) {

	if p.Silent {
		return
	}
	_, err := fmt.Fprintf(p.writer(), "[%s] [%s] %s\n", p.programName(), p.colorize(colorGreen, symbolSuccess), msg)
	if err != nil {
		return
	}
}

// Warn alerts the user to a potential issue.
//
// Parameters:
//   - msg: the warning message to display.
func (p *Provider) Warn(msg string) {

	if p.Silent {
		return
	}
	_, err := fmt.Fprintf(p.writer(), "[%s] [%s] %s\n", p.programName(), p.colorize(colorYellow, symbolWarn), msg)
	if err != nil {
		return
	}
}

// Fallible actions

// Fail prints an error message and returns an error.
//
// Parameters:
//   - msg: the error message to display and return.
//
// Returns:
//   - error: an error wrapping msg.
func (p *Provider) Fail(msg string) error {

	if !p.Silent {
		_, err := fmt.Fprintf(p.writer(), "[%s] [%s] %s\n", p.programName(), p.colorize(colorRed, symbolError), msg)
		if err != nil {
			return err
		}
	}
	return fmt.Errorf("%s", msg)
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// colorize wraps text in ANSI color codes when Color is enabled.
//
// Parameters:
//   - color: the ANSI color escape sequence.
//   - text: the text to colorize.
//
// Returns:
//   - string: the colorized text, or plain text if Color is disabled.
func (p *Provider) colorize(color, text string) string {

	if p.Color {
		return color + text + colorReset
	}
	return text
}

// programName returns the configured program name, defaulting to "devlore".
//
// Returns:
//   - string: the program name prefix for messages.
func (p *Provider) programName() string {

	if p.ProgramName != "" {
		return p.ProgramName
	}
	return "devlore"
}

// writer returns the configured writer, defaulting to os.Stderr.
//
// Returns:
//   - io.Writer: the output destination.
func (p *Provider) writer() io.Writer {

	if p.Writer != nil {
		return p.Writer
	}
	return os.Stderr
}

// endregion

// endregion
