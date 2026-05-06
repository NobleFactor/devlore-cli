// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"github.com/NobleFactor/devlore-cli/pkg/op/sops"
	"github.com/NobleFactor/devlore-cli/pkg/platform"
	"github.com/NobleFactor/devlore-cli/pkg/process"
	"github.com/NobleFactor/devlore-cli/pkg/result"
	"github.com/NobleFactor/devlore-cli/pkg/sink"
	"github.com/NobleFactor/devlore-cli/pkg/status"
	"go.starlark.net/starlark"
)

// RuntimeEnvironment provides the execution environment for providers, resources, and graphs.
type RuntimeEnvironment struct {

	// ProgramName identifies the running tool (e.g., "lore", "writ").
	ProgramName string

	// BackupSuffix is appended to back-up filenames during conflict resolution.
	BackupSuffix string

	// Catalog is the resource catalog for the current execution session.
	//
	// The do layer uses it to shadow Resource results after dispatch. Nil when running without catalog integration
	// (e.g., tests).
	Catalog *ResourceCatalog

	// ConflictResolution chooses how to handle preflight conflicts.
	ConflictResolution ConflictResolution

	// Context carries a deadline, a cancellation signal, and other values across API boundaries.
	//
	// See  https://pkg.go.dev/context.
	Context context.Context

	// Data holds tool-provided context: template variables, identities, segment maps, etc. Each tool populates this
	// before calling GraphExecutor.Run().
	Data map[string]any

	// DryRun prevents filesystem modifications when true.
	DryRun bool

	// Platform provides platform abstractions (package manager, service manager) to do providers.
	//
	// Nil when running in environments where host access is not needed (e.g., pure data transforms).
	Platform platform.Platform

	// RecoverySite is the shared recovery service for archiving and restoring resources during compensation.
	//
	// Instantiated by the executor from Root.
	RecoverySite *RecoverySite

	// Registry is the receiver type registry for the current session.
	//
	// Providers, graphs, and the starlark runtime use it to look up receiver types by name.
	Registry *ReceiverRegistry

	// Result is the primary output pipeline carried from the [RuntimeEnvironmentSpec].
	//
	// Populated by [RuntimeEnvironmentSpec.Build]. Defaults to a [result.Pipeline] writing JSON to
	// [sink.Stdout] when the spec field is nil.
	Result *result.Pipeline

	// Results holds the accumulated node results from the current execution.
	//
	// Flow actions (choose, gather) use this to resolve cross-phase promise references in branch nodes.
	Results map[string]any

	// Root provides scoped filesystem operations.
	//
	// All provider I/O goes through this interface. Three implementations: confinedRoot (execution), RootReader
	// (planning), RootReaderWriter (testing). Created by the executor or test runner; closed after execution completes.
	Root Root

	// Sops provides SOPS operations (decryption, signing, verification).
	//
	// Nil when SOPS is not configured (no .sops.yaml found). Receivers access this via
	// p.RuntimeEnvironment().SopsClient.
	Sops *sops.Client

	// Status is the user-facing side-channel narrator carried from the [RuntimeEnvironmentSpec].
	//
	// Same instance that flows to `cli.UI()` and through every status emission point. Populated by
	// [RuntimeEnvironmentSpec.Build] (defaults to a [status.Narrator] writing through [sink.Stderr]
	// when the spec field is nil; pass a Narrator wrapping [sink.Discard] to suppress).
	Status *status.Narrator

	// Thread is a Starlark execution thread for callable initialization.
	//
	// Created by the executor at execution time. Actions that need to invoke mem.Function resources call
	// Init(ctx.Thread) before Fn().
	Thread starlark.Thread

	// mutex guards the providers map for concurrent access.
	mutex sync.Mutex

	// providers caches lazily-constructed provider instances by name.
	providers map[string]any
}

// region EXPORTED METHODS

// NewRuntimeEnvironment constructs a fully-populated [RuntimeEnvironment] from this spec.
//
// It performs defaulting (BackupSuffix → ".<ProgramName>-backup", Status → [status.Narrator]
// over [sink.Stderr], Result → [result.Pipeline] writing JSON to [sink.Stdout]) and wires the
// [RecoverySite] if a Root is present.
//
// Returns:
//   - *RuntimeEnvironment: the constructed context.
func NewRuntimeEnvironment(ctx context.Context, spec *RuntimeEnvironmentSpec) *RuntimeEnvironment {

	assert.NotNil("spec.Registry", spec.Registry)

	backupSuffix := spec.BackupSuffix
	if backupSuffix == "" {
		backupSuffix = "." + spec.ProgramName + "-backup"
	}

	statusUI := spec.Status
	if statusUI == nil {
		statusUI = status.NewNarrator(spec.ProgramName, sink.Stderr())
	}

	resultSink := spec.Result
	if resultSink == nil {
		resultSink = result.NewPipeline(nil, result.JSONFormatter{}, sink.Stdout())
	}

	env := &RuntimeEnvironment{
		ProgramName:        spec.ProgramName,
		Catalog:            NewResourceCatalog(),
		Context:            ctx,
		Data:               spec.Data,
		DryRun:             spec.DryRun,
		Platform:           spec.Platform,
		Registry:           spec.Registry,
		Results:            make(map[string]any),
		Root:               spec.Root,
		Sops:               spec.Sops,
		Status:             statusUI,
		Result:             resultSink,
		BackupSuffix:       backupSuffix,
		ConflictResolution: spec.ConflictResolution,
	}

	if spec.Root != nil {
		env.RecoverySite = NewRecoverySite(env)
	}

	return env
}

// region Behaviors

// ActionByName returns a resolved Action for the given dotted action name (e.g., "file.write_text").
//
// The name is split into provider name and method name. The provider must play the action role. The provider instance
// is cached — subsequent calls for the same provider reuse the instance.
//
// Parameters:
//   - name: the dotted action name (e.g., "file.write_text").
//
// Returns:
//   - Action: the resolved action wrapping the provider instance and method.
//   - error: non-nil if the provider is not a registered action, the method doesn't exist, or construction fails.
func (ctx *RuntimeEnvironment) ActionByName(name string) (Action, error) {

	dot := strings.LastIndex(name, ".")
	if dot < 0 {
		return nil, fmt.Errorf("invalid action name %q: no dot", name)
	}

	receiverName := name[:dot]
	methodSnake := name[dot+1:]

	prt, ok := ctx.Registry.ActionByName(receiverName)
	if !ok {
		return nil, fmt.Errorf("unknown action provider: %s", receiverName)
	}

	var method *Method
	for m := range prt.Methods() {
		if CamelToSnake(m.Name()) == methodSnake {
			method = m
			break
		}
	}

	if method == nil {
		return nil, fmt.Errorf("action %q: method %q not found on %q", name, methodSnake, receiverName)
	}

	if _, err := ctx.cachedProvider(prt); err != nil {
		return nil, err
	}

	return newAction(prt, method, name), nil
}

// Capture executes cmd, returning stdout bytes verbatim and streaming stderr through the runtime environment's
// status UI.
//
// In dry-run, the command is narrated and nil bytes are returned with a nil error.
//
// Parameters:
//   - cmd: the prepared exec.Cmd; its Stdout, Stderr, and Cancel fields are overwritten by the runner.
//
// Returns:
//   - []byte: the captured stdout (nil in dry-run).
//   - error: a wrapped exit-error including the stderr tail on non-zero exit.
func (ctx *RuntimeEnvironment) Capture(cmd *exec.Cmd) ([]byte, error) {

	return ctx.runner().Capture(cmd)
}

// Emit captures stdout, applies parse to produce a typed value, then forwards the value to the runtime environment's
// result sink.
//
// Stderr streams through the status UI. In dry-run, the command is narrated and nil is returned without invoking
// parse.
//
// Parameters:
//   - cmd:   the prepared exec.Cmd.
//   - parse: converts captured stdout bytes into the typed value forwarded to the result sink.
//
// Returns:
//   - error: a wrapped exit-error, parse error, or sink error; nil on success.
func (ctx *RuntimeEnvironment) Emit(cmd *exec.Cmd, parse func([]byte) (any, error)) error {

	return ctx.runner().Emit(cmd, parse)
}

// ModuleByName returns a cached provider instance for the named module, constructing it on first access.
//
// Parameters:
//   - name: the module name (e.g., "file", "ui").
//
// Returns:
//   - any: the provider instance.
//   - error: non-nil if the name is not a registered module or construction fails.
func (ctx *RuntimeEnvironment) ModuleByName(name string) (any, error) {

	prt, ok := ctx.Registry.ModuleByName(name)
	if !ok {
		return nil, fmt.Errorf("unknown module: %s", name)
	}

	return ctx.cachedProvider(prt)
}

// Property returns a value from the tool-provided context data.
func (ctx *RuntimeEnvironment) Property(key string) (any, bool) {

	if ctx.Data == nil {
		return nil, false
	}
	v, ok := ctx.Data[key]
	return v, ok
}

// Run executes cmd, streaming stdout and stderr line-by-line through the runtime environment's status UI.
//
// In dry-run, the command is narrated and nil is returned without launching it.
//
// Parameters:
//   - cmd: the prepared exec.Cmd; its Stdout, Stderr, and Cancel fields are overwritten by the runner.
//
// Returns:
//   - error: a wrapped exit-error including the stderr tail on non-zero exit.
func (ctx *RuntimeEnvironment) Run(cmd *exec.Cmd) error {

	return ctx.runner().Run(cmd)
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// runner builds a process.Runner from this environment's status, result, dryrun, and context.
//
// Returns:
//   - *process.Runner: the runner bound to the current configuration.
func (ctx *RuntimeEnvironment) runner() *process.Runner {

	return process.NewRunner(ctx.Context, ctx.DryRun, ctx.Result, ctx.Status)
}

// cachedProvider returns a cached provider instance for the given type descriptor, constructing it on first access.
//
// The lock is released before calling Construct to avoid deadlock when a provider's constructor calls cachedProvider for
// a sibling. Double-check after construction handles concurrent callers.
//
// Parameters:
//   - prt: the provider receiver type descriptor.
//
// Returns:
//   - any: the provider instance.
//   - error: non-nil if construction fails.
func (ctx *RuntimeEnvironment) cachedProvider(prt ProviderReceiverType) (any, error) {

	name := prt.Name()

	ctx.mutex.Lock()
	if p, ok := ctx.providers[name]; ok {
		ctx.mutex.Unlock()
		return p, nil
	}
	ctx.mutex.Unlock()

	p, err := prt.Construct()(ctx)
	if err != nil {
		return nil, fmt.Errorf("construct provider %s: %w", name, err)
	}

	ctx.mutex.Lock()
	if existing, ok := ctx.providers[name]; ok {
		ctx.mutex.Unlock()
		return existing, nil
	}
	if ctx.providers == nil {
		ctx.providers = make(map[string]any)
	}
	ctx.providers[name] = p
	ctx.mutex.Unlock()

	return p, nil
}

// endregion

// endregion
