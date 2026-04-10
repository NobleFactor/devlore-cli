// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"io"
	"os"

	"github.com/NobleFactor/devlore-cli/pkg/op/sops"
)

// RuntimeEnvironmentSpec holds configuration for constructing Starlark bindings.
// Use [NewRuntimeEnvironmentSpec] to create, then chain With* methods:
//
//	registry := op.NewReceiverRegistry()
//	cfg := op.NewBindingConfig("lore", registry).
//	    WithModules(registry.ModuleByName("file"), registry.ModuleByName("json")).
//	    WithRoot(op.NewRootReaderWriter(wd)).
//	    WithColor()
type RuntimeEnvironmentSpec struct {

	// ProgramName identifies the running tool (e.g., "lore", "writ").
	ProgramName string

	// Modules lists the selected modules to expose as Starlark globals.
	Modules []ProviderReceiverType

	// Registry is the receiver type registry.
	Registry *ReceiverRegistry

	// Color enables ANSI color codes in output.
	Color bool

	// Data holds tool-provided context: template variables, identities, segment maps, etc.
	Data map[string]any

	// DryRun prevents filesystem modifications when true.
	DryRun bool

	// Root provides scoped filesystem operations for providers.
	Root Root

	// SopsClient provides SOPS operations.
	//
	// Nil when SOPS is not configured.
	SopsClient *sops.Client

	// Writer is the output destination for immediate receivers (e.g., ui.note).
	//
	// Defaults to os.Stderr.
	Writer io.Writer
}

// NewRuntimeEnvironmentSpec creates a RuntimeEnvironmentSpec with the given program name and registry.
//
// Parameters:
//   - programName: the name of the running tool (e.g., "lore", "writ").
//   - registry: the receiver type registry.
//
// Returns:
//   - *RuntimeEnvironmentSpec: the initialized config.
func NewRuntimeEnvironmentSpec(programName string, registry *ReceiverRegistry) *RuntimeEnvironmentSpec {

	return &RuntimeEnvironmentSpec{
		ProgramName: programName,
		Registry:    registry,
		Writer:      os.Stderr,
	}
}

// region EXPORTED METHODS

// region State management

// WithModules sets the modules to expose as Starlark globals.
//
// Parameters:
//   - modules: the selected provider receiver types.
//
// Returns:
//   - *RuntimeEnvironmentSpec: the config for method chaining.
func (c *RuntimeEnvironmentSpec) WithModules(modules ...ProviderReceiverType) *RuntimeEnvironmentSpec {
	c.Modules = modules
	return c
}

// WithColor enables ANSI color codes in output.
//
// Returns:
//   - *RuntimeEnvironmentSpec: the config for method chaining.
func (c *RuntimeEnvironmentSpec) WithColor() *RuntimeEnvironmentSpec {
	c.Color = true
	return c
}

// WithData sets the tool-provided context data.
//
// Parameters:
//   - data: the context data map.
//
// Returns:
//   - *RuntimeEnvironmentSpec: the config for method chaining.
func (c *RuntimeEnvironmentSpec) WithData(data map[string]any) *RuntimeEnvironmentSpec {
	c.Data = data
	return c
}

// WithDryRun sets the dry-run mode (no filesystem modifications when true).
//
// Parameters:
//   - dryRun: true to prevent filesystem modifications.
//
// Returns:
//   - *RuntimeEnvironmentSpec: the config for method chaining.
func (c *RuntimeEnvironmentSpec) WithDryRun(value bool) *RuntimeEnvironmentSpec {
	c.DryRun = value
	return c
}

// WithRoot sets the scoped filesystem root for provider I/O.
//
// Parameters:
//   - root: the filesystem root.
//
// Returns:
//   - *RuntimeEnvironmentSpec: the config for method chaining.
func (c *RuntimeEnvironmentSpec) WithRoot(root Root) *RuntimeEnvironmentSpec {
	c.Root = root
	return c
}

// WithSops sets the SOPS client for decryption and signing.
//
// Parameters:
//   - client: the SOPS client (nil means SOPS is not configured).
//
// Returns:
//   - *RuntimeEnvironmentSpec: the config for method chaining.
func (c *RuntimeEnvironmentSpec) WithSops(client *sops.Client) *RuntimeEnvironmentSpec {
	c.SopsClient = client
	return c
}

// WithWriter sets the output destination for immediate receivers.
//
// Parameters:
//   - w: the output writer.
//
// Returns:
//   - *RuntimeEnvironmentSpec: the config for method chaining.
func (c *RuntimeEnvironmentSpec) WithWriter(w io.Writer) *RuntimeEnvironmentSpec {
	c.Writer = w
	return c
}

// endregion

// endregion
