// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"github.com/NobleFactor/devlore-cli/pkg/op/sops"
	"github.com/NobleFactor/devlore-cli/pkg/platform"
	"github.com/NobleFactor/devlore-cli/pkg/result"
	"github.com/NobleFactor/devlore-cli/pkg/status"
	"go.starlark.net/starlark"
)

// ConflictResolution specifies how to handle conflicts during execution.
type ConflictResolution int

const (
	// ResolutionStop aborts execution on first conflict.
	ResolutionStop ConflictResolution = iota
	// ResolutionBackup moves conflicting files to timestamped backups.
	ResolutionBackup
	// ResolutionOverwrite removes conflicting files without backup.
	ResolutionOverwrite
	// ResolutionSkip skips conflicting files and continues.
	ResolutionSkip
)

// RuntimeEnvironmentSpec holds configuration for constructing Starlark bindings.
// Use [NewRuntimeEnvironmentSpec] to create, then chain With* methods:
//
//	registry := op.NewReceiverRegistry()
//	cfg := op.NewRuntimeEnvironmentSpec("lore", registry).
//	    WithModules(registry.ModuleByName("file"), registry.ModuleByName("json")).
//	    WithRoot(op.NewConfinedRoot(wd)).
//	    WithBackupSuffix(".bak").
//	    WithConflictResolution(op.ResolutionBackup)
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

	// Sops provides SOPS operations.
	//
	// No Nil when SOPS is not configured.
	Sops *sops.Client

	// Writer is the output destination for immediate receivers (e.g., ui.note).
	//
	// Defaults to os.Stderr.
	Writer io.Writer

	// BackupSuffix is appended to backup filenames during conflict resolution.
	// Defaults to ".<ProgramName>-backup" when empty.
	BackupSuffix string

	// ConflictResolution chooses how to handle preflight conflicts.
	// Zero value is ResolutionStop.
	ConflictResolution ConflictResolution

	// Status is the user-facing side-channel UI. Carries categorized status messages and starlark
	// `print()` output. Same instance flows from the client's bootstrap into the runtime
	// environment. When zero, [RuntimeEnvironmentSpec.Build] defaults to [status.NoOp].
	Status status.UI

	// Result is the primary output sink. Carries structured data destined for the user or
	// downstream tooling (JSON / YAML / CSV / template). Same instance flows from the client's
	// bootstrap into the runtime environment. When zero, [RuntimeEnvironmentSpec.Build] defaults to
	// [result.UnconfiguredSink] which errors loudly on every Emit.
	Result result.Sink

	// Platform is the interface-typed platform capability — host classification (OS, arch, distro,
	// version) plus the package and service managers available to providers. Construct via
	// [platform.Linux] / [platform.Darwin] / [platform.Windows] for explicit fixtures or via
	// [platform.Detect] for host detection. Replaces the legacy `*op.Platform` struct on the
	// runtime environment as callers migrate.
	Platform platform.Platform
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
		Status:      status.NoOp{},
		Result:      result.UnconfiguredSink{},
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
	c.Sops = client
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

// WithBackupSuffix sets the backup filename suffix.
//
// Parameters:
//   - suffix: the suffix (e.g. ".bak").
//
// Returns:
//   - *RuntimeEnvironmentSpec: the config for method chaining.
func (c *RuntimeEnvironmentSpec) WithBackupSuffix(suffix string) *RuntimeEnvironmentSpec {
	c.BackupSuffix = suffix
	return c
}

// WithConflictResolution sets the conflict resolution strategy.
//
// Parameters:
//   - res: the resolution strategy.
//
// Returns:
//   - *RuntimeEnvironmentSpec: the config for method chaining.
func (c *RuntimeEnvironmentSpec) WithConflictResolution(res ConflictResolution) *RuntimeEnvironmentSpec {
	c.ConflictResolution = res
	return c
}

// WithStatus sets the side-channel UI for the constructed runtime environment.
//
// Parameters:
//   - ui: the [status.UI] instance — typically the same one held by the cli facade via [cli.SetUI].
//
// Returns:
//   - *RuntimeEnvironmentSpec: the config for method chaining.
func (c *RuntimeEnvironmentSpec) WithStatus(ui status.UI) *RuntimeEnvironmentSpec {
	c.Status = ui
	return c
}

// WithResult sets the primary output sink for the constructed runtime environment.
//
// Parameters:
//   - sink: the [result.Sink] instance — typically constructed via [result.NewPipeline].
//
// Returns:
//   - *RuntimeEnvironmentSpec: the config for method chaining.
func (c *RuntimeEnvironmentSpec) WithResult(sink result.Sink) *RuntimeEnvironmentSpec {
	c.Result = sink
	return c
}

// WithPlatform sets the interface-typed platform capability for the constructed runtime environment.
//
// Parameters:
//   - p: the [platform.Platform] instance — construct via [platform.Linux] / [platform.Darwin] /
//     [platform.Windows] for explicit fixtures or via [platform.Detect] for host detection.
//
// Returns:
//   - *RuntimeEnvironmentSpec: the config for method chaining.
func (c *RuntimeEnvironmentSpec) WithPlatform(p platform.Platform) *RuntimeEnvironmentSpec {
	c.Platform = p
	return c
}

// NewRuntimeEnvironment constructs a fully-populated [RuntimeEnvironment] from this spec.
//
// It performs defaulting (Writer → os.Stderr, BackupSuffix → ".<ProgramName>-backup"), constructs the
// [starlark.Thread], initializes the [Platform], and wires the [RecoverySite] if a Root is present.
//
// Returns:
//   - *RuntimeEnvironment: the constructed context.
func (c *RuntimeEnvironmentSpec) Build(ctx context.Context) *RuntimeEnvironment {

	assert.NotNil("spec.Registry", c.Registry)

	writer := c.Writer
	if writer == nil {
		writer = os.Stderr
	}

	backupSuffix := c.BackupSuffix
	if backupSuffix == "" {
		backupSuffix = "." + c.ProgramName + "-backup"
	}

	statusUI := c.Status
	if statusUI == nil {
		statusUI = status.NoOp{}
	}

	resultSink := c.Result
	if resultSink == nil {
		resultSink = result.UnconfiguredSink{}
	}

	env := &RuntimeEnvironment{
		Context:            ctx,
		ProgramName:        c.ProgramName,
		Data:               c.Data,
		DryRun:             c.DryRun,
		Platform:           NewPlatform(),
		Plat:               c.Platform,
		Registry:           c.Registry,
		Results:            make(map[string]any),
		Root:               c.Root,
		Sops:               c.Sops,
		Status:             statusUI,
		Result:             resultSink,
		Writer:             writer,
		BackupSuffix:       backupSuffix,
		ConflictResolution: c.ConflictResolution,
	}

	env.Thread = starlark.Thread{
		Name: c.ProgramName,
		Print: func(_ *starlark.Thread, msg string) {
			_, _ = fmt.Fprintln(writer, msg)
		},
	}

	if c.Root != nil {
		env.RecoverySite = NewRecoverySite(env)
	}

	return env
}

// endregion

// endregion
