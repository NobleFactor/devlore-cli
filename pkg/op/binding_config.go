// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"io"
	"os"
)

// BindingConfig holds configuration for constructing Starlark bindings.
// Use [NewBindingConfig] to create, then chain With* methods:
//
//	cfg := op.NewBindingConfig("lore").
//	    WithGraphBuilder().
//	    WithReceivers("ui", "file").
//	    WithColor()
type BindingConfig struct {
	// ProgramName identifies the running tool (e.g., "lore", "writ").
	ProgramName string

	// Receivers lists the Starlark namespaces to expose as globals.
	// Provider names (e.g., "file", "ui") include their immediate receivers.
	Receivers []string

	// GraphBuilder enables the plan.* graph namespace (PlanRoot).
	GraphBuilder bool

	// Writer is the output destination for immediate receivers (e.g., ui.note).
	// Defaults to os.Stderr.
	Writer io.Writer

	// Color enables ANSI color codes in output.
	Color bool
}

// NewBindingConfig creates a BindingConfig with the given program name.
// Writer defaults to os.Stderr.
//
// Parameters:
//   - programName: the name of the running tool (e.g., "lore", "writ").
//
// Returns:
//   - *BindingConfig: the initialized config.
func NewBindingConfig(programName string) *BindingConfig {

	return &BindingConfig{
		ProgramName: programName,
		Writer:      os.Stderr,
	}
}

// region EXPORTED METHODS

// region State management

// WithGraphBuilder enables the plan.* graph namespace in the runtime.
//
// Returns:
//   - *BindingConfig: the config for method chaining.
func (c *BindingConfig) WithGraphBuilder() *BindingConfig {

	c.GraphBuilder = true
	return c
}

// WithReceivers sets the Starlark namespaces to expose as globals.
//
// Parameters:
//   - names: the receiver names to expose.
//
// Returns:
//   - *BindingConfig: the config for method chaining.
func (c *BindingConfig) WithReceivers(names ...string) *BindingConfig {

	c.Receivers = names
	return c
}

// WithWriter sets the output destination for immediate receivers.
//
// Parameters:
//   - w: the output writer.
//
// Returns:
//   - *BindingConfig: the config for method chaining.
func (c *BindingConfig) WithWriter(w io.Writer) *BindingConfig {

	c.Writer = w
	return c
}

// WithColor enables ANSI color codes in output.
//
// Returns:
//   - *BindingConfig: the config for method chaining.
func (c *BindingConfig) WithColor() *BindingConfig {

	c.Color = true
	return c
}

// endregion

// endregion
