// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/op/sops"
	"github.com/NobleFactor/devlore-cli/pkg/platform"
	"github.com/NobleFactor/devlore-cli/pkg/result"
	"github.com/NobleFactor/devlore-cli/pkg/sink"
	"github.com/NobleFactor/devlore-cli/pkg/status"
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

	// Application is the tool-side handle that carries the variable-resolver source maps (flags / config /
	// overrides) and the tool's program name. Tools set this via [WithApplication]; pkg/op builds the
	// [VariableResolver] from it at [NewRuntimeEnvironment] time.
	Application *application.Application

	// BackupSuffix is appended to back up filenames during conflict resolution.
	//
	// Defaults to ".<ProgramName>-backup" when empty.
	BackupSuffix string

	// ConflictResolution chooses how to handle preflight conflicts.
	// Zero value is ResolutionStop.
	ConflictResolution ConflictResolution

	// Platform classifies the host (OS, arch, distro, version) and gives access to the managers available to providers.
	//
	// Construct via [platform.Linux] / [platform.Darwin] / [platform.Windows] for explicit fixtures or via
	// [platform.Detect] for host detection.
	Platform platform.Platform

	// Result is the primary output sink.
	//
	// Carries structured data destined for the user or downstream tooling (JSON / YAML / CSV / template). The same
	// instance flows from the client's bootstrap into the runtime environment. When nil, [RuntimeEnvironmentSpec.Build]
	// defaults to a [result.Pipeline] writing JSON to [sink.Stdout].
	Result *result.Pipeline

	// Root provides scoped filesystem operations for providers.
	Root Root

	// Sops provides SOPS operations.
	//
	// No Nil when SOPS is not configured.
	Sops *sops.Client

	// Status is the user-facing side-channel narrator.
	//
	// It Carries categorized status messages and starlark `print()` output. The same instance flows from the client's
	// bootstrap into the runtime environment. When nil, [RuntimeEnvironmentSpec.Build] defaults to a [status.Narrator]
	// writing through [sink.Stderr]; pass a Narrator wrapping [sink.Discard] to suppress.
	Status *status.Narrator
}

// NewRuntimeEnvironmentSpec creates a RuntimeEnvironmentSpec with the given program name and registry.
//
// Parameters:
//   - `programName`: the name of the running tool (e.g., "lore", "writ").
//   - `registry`: the receiver type registry.
//
// Returns:
//   - *RuntimeEnvironmentSpec: the initialized config.
func NewRuntimeEnvironmentSpec(programName string, registry *ReceiverRegistry) *RuntimeEnvironmentSpec {
	return &RuntimeEnvironmentSpec{
		ProgramName: programName,
		Registry:    registry,
		Status:      status.NewNarrator(programName, sink.Discard()),
		Result:      result.NewPipeline(nil, result.JSONFormatter{}, sink.Discard()),
	}
}

// region EXPORTED METHODS

// region State management

// WithApplication sets the tool-side [application.Application] handle. The constructed runtime environment
// builds its [VariableResolver] from the Application's Name / Flags / Config / Overrides; framework code
// also reads system flags (e.g., "dry_run") directly from `Application.Flags`.
//
// Parameters:
//   - `app`: the [application.Application] the tool main constructed.
//
// Returns:
//   - *RuntimeEnvironmentSpec: the config for method chaining.
func (c *RuntimeEnvironmentSpec) WithApplication(app *application.Application) *RuntimeEnvironmentSpec {
	c.Application = app
	return c
}

// WithBackupSuffix sets the backup filename suffix.
//
// Parameters:
//   - `suffix`: the suffix (e.g. ".bak").
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
//   - `res`: the resolution strategy.
//
// Returns:
//   - *RuntimeEnvironmentSpec: the config for method chaining.
func (c *RuntimeEnvironmentSpec) WithConflictResolution(res ConflictResolution) *RuntimeEnvironmentSpec {
	c.ConflictResolution = res
	return c
}

// WithModules sets the modules to expose as Starlark globals.
//
// Parameters:
//   - `modules`: the selected provider receiver types.
//
// Returns:
//   - *RuntimeEnvironmentSpec: the config for method chaining.
func (c *RuntimeEnvironmentSpec) WithModules(modules ...ProviderReceiverType) *RuntimeEnvironmentSpec {
	c.Modules = modules
	return c
}

// WithPlatform sets the interface-typed platform capability for the constructed runtime environment.
//
// Parameters:
//   - `p`: the [platform.Platform] instance — construct via [platform.Linux] / [platform.Darwin] /
//     [platform.Windows] for explicit fixtures or via [platform.Detect] for host detection.
//
// Returns:
//   - *RuntimeEnvironmentSpec: the config for method chaining.
func (c *RuntimeEnvironmentSpec) WithPlatform(p platform.Platform) *RuntimeEnvironmentSpec {
	c.Platform = p
	return c
}

// WithResult sets the primary output sink for the constructed runtime environment.
//
// Parameters:
//   - `pipeline`: the [result.Pipeline] instance — typically constructed via [result.NewPipeline].
//
// Returns:
//   - *RuntimeEnvironmentSpec: the config for method chaining.
func (c *RuntimeEnvironmentSpec) WithResult(pipeline *result.Pipeline) *RuntimeEnvironmentSpec {
	c.Result = pipeline
	return c
}

// WithRoot sets the scoped filesystem root for provider I/O.
//
// Parameters:
//   - `root`: the filesystem root.
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
//   - `client`: the SOPS client (nil means SOPS is not configured).
//
// Returns:
//   - *RuntimeEnvironmentSpec: the config for method chaining.
func (c *RuntimeEnvironmentSpec) WithSops(client *sops.Client) *RuntimeEnvironmentSpec {
	c.Sops = client
	return c
}

// WithStatus sets the side-channel narrator for the constructed runtime environment.
//
// Parameters:
//   - `narrator`: the [status.Narrator] instance — typically the same one held by the cli facade via [cli.SetUI].
//
// Returns:
//   - *RuntimeEnvironmentSpec: the config for method chaining.
func (c *RuntimeEnvironmentSpec) WithStatus(narrator *status.Narrator) *RuntimeEnvironmentSpec {
	c.Status = narrator
	return c
}

// endregion

// endregion
