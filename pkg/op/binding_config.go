// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"io"
	"os"

	"github.com/NobleFactor/devlore-cli/pkg/op/sops"
)

// BindingConfig holds configuration for constructing Starlark bindings.
// Use [NewBindingConfig] to create, then chain With* methods:
//
//	cfg := op.NewBindingConfig("lore").
//	    WithGraphBuilder().
//	    WithReceivers(json.receiver, yaml.receiver).
//	    WithColor()
type BindingConfig struct {
	// ProgramName identifies the running tool (e.g., "lore", "writ").
	ProgramName string

	// Receivers lists the receiver factories to expose as Starlark globals.
	Receivers []ReceiverFactory

	// GraphBuilder enables the plan.* graph namespace (PlanRoot).
	GraphBuilder bool

	// Writer is the output destination for immediate receivers (e.g., ui.note).
	// Defaults to os.Stderr.
	Writer io.Writer

	// Color enables ANSI color codes in output.
	Color bool

	// SopsClient provides SOPS operations. Nil when SOPS is not configured.
	SopsClient *sops.Client
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

// WithReceivers sets the receiver factories to expose as Starlark globals.
//
// Parameters:
//   - receivers: the receiver factories to include.
//
// Returns:
//   - *BindingConfig: the config for method chaining.
func (c *BindingConfig) WithReceivers(receivers ...ReceiverFactory) *BindingConfig {

	c.Receivers = receivers
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

// WithSopsClient sets the SOPS client for decryption and signing.
//
// Parameters:
//   - client: the SOPS client (nil means SOPS is not configured).
//
// Returns:
//   - *BindingConfig: the config for method chaining.
func (c *BindingConfig) WithSopsClient(client *sops.Client) *BindingConfig {

	c.SopsClient = client
	return c
}

// endregion

// endregion
