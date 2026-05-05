// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package ui exposes the runtime environment's [status.UI] capability to starlark.
//
// The provider is a thin passthrough — it carries no state of its own. Method bodies forward to
// `p.RuntimeEnvironment().Status.<Method>(msg)`. Configuration (writer, program name, color, silent)
// lives on the [status.UI] instance the client installed at bootstrap; the same instance flows from
// the cli facade into the runtime environment, ensuring `--silent`, color settings, and program-name
// prefixing apply uniformly across cli emissions, provider emissions, and starlark `print()` output.
package ui

import (
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Provider exposes the [status.UI] capability to starlark.
//
// +devlore:access=immediate
type Provider struct {
	op.ProviderBase
}

// NewProvider constructs a *Provider for the registered ProviderConstructor.
//
// The provider holds no state of its own; configuration lives on the [status.UI] instance the runtime
// environment carries. Method bodies retrieve the UI via p.RuntimeEnvironment().Status.
func NewProvider(ctx *op.RuntimeEnvironment) *Provider {
	return &Provider{
		ProviderBase: op.NewProviderBase(ctx),
	}
}

// region EXPORTED METHODS

// region Behaviors

// Note informs the user of progress.
//
// Parameters:
//   - msg: the informational message to display.
func (p *Provider) Note(msg string) {
	p.RuntimeEnvironment().Status.Note(msg)
}

// Warn alerts the user to a potential issue.
//
// Parameters:
//   - msg: the warning message to display.
func (p *Provider) Warn(msg string) {
	p.RuntimeEnvironment().Status.Warn(msg)
}

// Error reports a non-fatal problem to the user.
//
// Parameters:
//   - msg: the error message to display.
func (p *Provider) Error(msg string) {
	p.RuntimeEnvironment().Status.Error(msg)
}

// Succeed confirms completion to the user.
//
// Parameters:
//   - msg: the success message to display.
func (p *Provider) Succeed(msg string) {
	p.RuntimeEnvironment().Status.Succeed(msg)
}

// Fail reports a fatal error and aborts execution.
//
// Parameters:
//   - msg: the fatal error message.
//
// Returns:
//   - error: a non-nil error wrapping msg.
func (p *Provider) Fail(msg string) error {
	return p.RuntimeEnvironment().Status.Fail(msg)
}

// Print emits raw text without categorized-message decoration. Used by starlark `print()` output;
// reads as the script wrote it (no [program] [symbol] prefix).
//
// Parameters:
//   - msg: the raw text to emit.
func (p *Provider) Print(msg string) {
	p.RuntimeEnvironment().Status.Print(msg)
}

// endregion

// endregion
