// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package ui exposes the runtime environment's [status.Narrator] capability to starlark.
//
// The provider is a thin passthrough — it carries no state of its own. Method bodies forward to
// `p.RuntimeEnvironment().Status.<Method>(msg)`. Configuration (writer, program name, color, silent)
// lives on the [status.Narrator] instance the client installed at bootstrap; the same instance flows from
// the cli facade into the runtime environment, ensuring `--silent`, color settings, and program-name
// prefixing apply uniformly across cli emissions, provider emissions, and starlark `print()` output.
package ui

import (
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Provider exposes the [status.Narrator] capability to starlark.
//
// Root-placed: the six methods surface as top-level globals -- note(), warn(), print() -- rather than under
// ui.*. Two of those names, print and fail, belong to starlark's universe, and the resolver checks predeclared
// before universal, so this REPLACES the builtins rather than shadowing them. That is the point: the builtin
// print writes straight to stderr through starlark-go, escaping --silent, color, and program-name prefixing,
// and would escape the diagnostics stream of docs/architecture/2.8-eventing-infrastructure.md. Routing it here
// is what makes the uniformity this package claims actually hold for a bare print(...).
//
// The methods take one string rather than the builtin's variadic-with-separator. That is deliberate and
// documented in docs/architecture/3.5.16-ui-provider.md: starlark's % is an operator that runs BEFORE the call,
// so a format-string signature would re-scan an already-rendered string and corrupt any data containing a %,
// while a variadic one would receive Go natives after conversion and render True as true and None as <nil>.
// The script renders with str() and %, which is starlark's own rendering, and hands over a finished string.
//
// +devlore:root=true
type Provider struct {
	op.ProviderBase
}

// NewProvider constructs a *Provider for the registered ProviderConstructor.
//
// The provider holds no state of its own; configuration lives on the [status.Narrator] instance the runtime
// environment carries. Method bodies retrieve the narrator via p.RuntimeEnvironment().Status.
func NewProvider(runtimeEnvironment *op.RuntimeEnvironment) *Provider {
	return &Provider{
		ProviderBase: op.NewProviderBase(runtimeEnvironment),
	}
}

// region EXPORTED METHODS

// region Behaviors

// Fallible actions

// Fail reports a fatal error and aborts execution.
//
// Parameters:
//   - `msg`: the fatal error message.
//
// Returns:
//   - `error`: a non-nil error wrapping msg.
//
// +devlore:claim=deterministic
func (p *Provider) Fail(msg string) error {
	return p.RuntimeEnvironment().Status.Fail(msg)
}

// Actions

// Error reports a non-fatal problem to the user.
//
// Parameters:
//   - `msg`: the error message to display.
//
// +devlore:claim=deterministic
func (p *Provider) Error(msg string) {
	p.RuntimeEnvironment().Status.Error(msg)
}

// Note informs the user of progress.
//
// Parameters:
//   - `msg`: the informational message to display.
//
// +devlore:claim=deterministic
func (p *Provider) Note(msg string) {
	p.RuntimeEnvironment().Status.Note(msg)
}

// Print emits raw text without categorized-message decoration.
//
// Used by starlark `print()` output; reads as the script wrote it (no [program] [symbol] prefix).
//
// Parameters:
//   - `msg`: the raw text to emit.
//
// +devlore:claim=deterministic
func (p *Provider) Print(msg string) {
	p.RuntimeEnvironment().Status.Print(msg)
}

// Succeed confirms completion to the user.
//
// Parameters:
//   - `msg`: the success message to display.
//
// +devlore:claim=deterministic
func (p *Provider) Succeed(msg string) {
	p.RuntimeEnvironment().Status.Succeed(msg)
}

// Warn alerts the user to a potential issue.
//
// Parameters:
//   - `msg`: the warning message to display.
//
// +devlore:claim=deterministic
func (p *Provider) Warn(msg string) {
	p.RuntimeEnvironment().Status.Warn(msg)
}

// endregion

// endregion
