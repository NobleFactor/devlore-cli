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
	writer      io.Writer
	programName string
	silent      bool
	color       bool
}

// NewProvider constructs a *Provider for the registered ProviderConstructor.
func NewProvider(ctx *op.ExecutionContext) *Provider {
	return &Provider{
		ProviderBase: op.NewProviderBase(ctx),
		writer:       ctx.Writer,
		programName:  ctx.ProgramName,
		color:        true,
		silent:       false,
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

	if p.silent {
		return
	}
	_, err := fmt.Fprintf(p.getWriter(), "[%s] [%s] %s\n", p.getProgramName(), p.colorize(colorRed, symbolError), msg)
	if err != nil {
		return
	}
}

// Note informs the user of progress.
//
// Parameters:
//   - msg: the informational message to display.
func (p *Provider) Note(msg string) {

	if p.silent {
		return
	}
	_, err := fmt.Fprintf(p.getWriter(), "[%s] [%s] %s\n", p.getProgramName(), p.colorize(colorGray, symbolNote), msg)
	if err != nil {
		return
	}
}

// Success confirms completion to the user.
//
// Parameters:
//   - msg: the success message to display.
func (p *Provider) Success(msg string) {

	if p.silent {
		return
	}
	_, err := fmt.Fprintf(p.getWriter(), "[%s] [%s] %s\n", p.getProgramName(), p.colorize(colorGreen, symbolSuccess), msg)
	if err != nil {
		return
	}
}

// Warn alerts the user to a potential issue.
//
// Parameters:
//   - msg: the warning message to display.
func (p *Provider) Warn(msg string) {

	if p.silent {
		return
	}
	_, err := fmt.Fprintf(p.getWriter(), "[%s] [%s] %s\n", p.getProgramName(), p.colorize(colorYellow, symbolWarn), msg)
	if err != nil {
		return
	}
}

// Fail reports a fatal error and aborts execution.
//
// Parameters:
//   - msg: the fatal error message.
//
// Returns:
//   - error: always returns a non-nil error wrapping the message.
func (p *Provider) Fail(msg string) error {

	p.Error(msg)
	return fmt.Errorf("fatal: %s", msg)
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region State management

// colorize wraps text in ANSI color codes when Color is enabled.
//
// Parameters:
//   - color: the ANSI color escape sequence.
//   - text: the text to colorize.
//
// Returns:
//   - string: the colorized text, or plain text if Color is disabled.
func (p *Provider) colorize(color, text string) string {

	if p.color {
		return color + text + colorReset
	}
	return text
}

// getProgramName returns the configured program name, defaulting to "devlore".
//
// Returns:
//   - string: the program name prefix for messages.
func (p *Provider) getProgramName() string {

	if p.programName != "" {
		return p.programName
	}
	return "devlore"
}

// getWriter returns the configured writer, defaulting to os.Stderr.
//
// Returns:
//   - io.Writer: the output destination.
func (p *Provider) getWriter() io.Writer {

	if p.writer != nil {
		return p.writer
	}
	return os.Stderr
}

// endregion

// endregion
