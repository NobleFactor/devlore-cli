// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"context"
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"sync"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/op/sops"
	"github.com/NobleFactor/devlore-cli/pkg/platform"
	"github.com/NobleFactor/devlore-cli/pkg/process"
	"github.com/NobleFactor/devlore-cli/pkg/result"
	"github.com/NobleFactor/devlore-cli/pkg/sink"
	"github.com/NobleFactor/devlore-cli/pkg/status"
	"go.starlark.net/starlark"
)

// RuntimeEnvironment is the session-scoped execution context for providers, resources, and graphs.
//
// One env per session. A session is bounded by a single CLI command invocation, a single test in the test
// harness, or any other unit of work that owns a graph's plan-then-execute lifecycle. Long-running processes
// (test runners, daemons, repls) construct a fresh env per session; one process may produce many envs over
// its lifetime.
//
// The env owns the [Root] handle and is the single point of Close responsibility — see [RuntimeEnvironment.Close].
// Every other type in this package that holds a *RuntimeEnvironment ([starlarkbridge.Runtime], [GraphExecutor],
// [Graph]) is a co-user of the session, not an owner; callers construct the env, defer Close once, and pass it by
// pointer to whatever needs it.
//
// Session-shared state (Catalog, Registry, variable map, provider cache, RecoverySite, …) lives on this struct, so
// plan-time and execute-time machinery operate on the same instances.
type RuntimeEnvironment struct {

	// Application is the tool-side handle carrying the variable-resolver source maps (flags / config /
	// overrides) and the tool's program name (formerly env.ProgramName — now [application.Application.Name]
	// is the single source of truth). Framework code reads system flags such as "dry-run" directly from
	// `Application.Flags` via [application.Application.DryRun].
	Application *application.Application

	// BackupSuffix is appended to back up filenames during conflict resolution.
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
	// See https://pkg.go.dev/context.
	Context context.Context

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

	// declaredParameters records every parameter registered via [RegisterParameter], keyed by name.
	//
	// Used to detect type-mismatch on re-registration and to enumerate the session-resolved parameter surface for
	// diagnostics.
	declaredParameters map[string]Parameter

	// mutex guards the provider's map for concurrent access.
	mutex sync.Mutex

	// providers is a map of lazily constructed provider instances by name.
	providers map[string]any

	// variables is the binding-layer resolved variable map.
	//
	// It is populated by [VariableResolver.Resolve] during the executor's preflight pass. Phase 1 ships this
	// nil-initialized; Phase 4 wires the preflight pass.
	variables map[string]Variable

	// variableResolver assembles binding-layer variable values from the spec's flag / config / override
	// sources plus the process environment.
	//
	// Read by the executor's preflight pass to populate variables.
	variableResolver *VariableResolver

	// closeOnce guards [RuntimeEnvironment.Close], so the close path runs exactly once per env, regardless of how many
	// times defer fires.
	closeOnce sync.Once

	// closeErr captures the joined error from the close-once execution and is returned by every Close call
	// after the first.
	closeErr error
}

// region EXPORTED METHODS

// NewRuntimeEnvironment constructs a fully populated [RuntimeEnvironment] from this spec.
//
// It performs defaulting (BackupSuffix → ".<ProgramName>-backup", Status → [status.Narrator] over [sink.Stderr], Result
// → [result.Pipeline] writing JSON to [sink.Stdout]) and wires the [RecoverySite] if a Root is present.
//
// Returns:
//   - *RuntimeEnvironment: the constructed context.
func NewRuntimeEnvironment(ctx context.Context, spec *RuntimeEnvironmentSpec) *RuntimeEnvironment {

	assert.NonZero("spec", spec)
	assert.NonZero("spec.Registry", spec.Registry)
	assert.NonZero("spec.Application", spec.Application)

	backupSuffix := spec.BackupSuffix

	if backupSuffix == "" {
		backupSuffix = "." + spec.ProgramName + "-backup"
	}

	statusNarrator := spec.Status

	if statusNarrator == nil {
		statusNarrator = status.NewNarrator(spec.ProgramName, sink.Stderr())
	}

	resultPipeline := spec.Result

	if resultPipeline == nil {
		resultPipeline = result.NewPipeline(nil, result.JSONFormatter{}, sink.Stdout())
	}

	env := &RuntimeEnvironment{
		Application:        spec.Application,
		Catalog:            NewResourceCatalog(),
		Context:            ctx,
		Platform:           spec.Platform,
		Registry:           spec.Registry,
		Results:            make(map[string]any),
		Root:               spec.Root,
		Sops:               spec.Sops,
		Status:             statusNarrator,
		Result:             resultPipeline,
		BackupSuffix:       backupSuffix,
		ConflictResolution: spec.ConflictResolution,
		variableResolver:   NewVariableResolver(spec.Application),
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
// is cached. Subsequent calls for the same provider reuse the instance.
//
// Parameters:
//   - `name`: the dotted action name (e.g., "file.write_text").
//
// Returns:
//   - `Action`: the resolved action wrapping the provider instance and method.
//   - `error`: non-nil if the provider is not a registered action, the method doesn't exist, or construction fails.
func (re *RuntimeEnvironment) ActionByName(name string) (Action, error) {

	dot := strings.LastIndex(name, ".")
	if dot < 0 {
		return nil, fmt.Errorf("invalid action name %q: no dot", name)
	}

	receiverName := name[:dot]
	methodSnake := name[dot+1:]

	prt, ok := re.Registry.ActionByName(receiverName)
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

	if _, err := re.cachedProvider(prt); err != nil {
		return nil, err
	}

	return newAction(prt, method, name), nil
}

// Close releases the session's owned resources — currently the [Root] handle.
//
// Idempotent: The close path runs exactly once per env regardless of how many times Close is called. The first call
// performs the close and stores any joined error; later calls return the stored error without re-closing.
//
// Callers construct the env, defer Close once, then hand the env by pointer to whatever uses it
// ([starlarkbridge.Runtime], [GraphExecutor], providers, …). Holders do not implement their own Close. The
// [RuntimeEnvironment] is the single owner.
//
// Returns:
//   - `error`: the joined error from closing the env's owned resources, or nil on success.
func (re *RuntimeEnvironment) Close() error {

	re.closeOnce.Do(func() {
		iox.Close(&re.closeErr, re.Root)
	})

	return re.closeErr
}

// RegisterParameter declares interest in a binding-layer variable.
//
// The cascade resolves the parameter immediately against the [Application]'s source maps (override → flag → env →
// config → default) and stores the resolved [Variable] in env.variables. Subsequent reads via
// [RuntimeEnvironment.VariableByName] return the typed value with [VariableSource] provenance.
//
// Reregistration of the same name is idempotent when the declared [Parameter.Type] matches the prior declaration; a
// type-mismatch returns an error without overwriting state.
//
// Type checking on source-supplied values: every non-env source's raw value is asserted against the declared Type via
// [reflect.Type.AssignableTo]. Mismatches return an error before any storage. The env source (which always yields
// strings) currently skips parameters whose Type is not string-parsable — full env-string parsing lands with the
// binding-resolver real implementation in a later phase.
//
// Parameters:
//   - `p`: the parameter declaration. Name and Type must be set; Default is optional.
//
// Returns:
//   - `error`: non-nil on re-registration type-mismatch or source-value type-mismatch.
func (re *RuntimeEnvironment) RegisterParameter(p Parameter) error {

	if re.declaredParameters == nil {
		re.declaredParameters = make(map[string]Parameter)
	}
	if existing, ok := re.declaredParameters[p.Name]; ok {
		if existing.Type != p.Type {
			return fmt.Errorf(
				"parameter %q: redeclared with type %s but previously declared %s",
				p.Name, p.Type, existing.Type)
		}
		return nil
	}
	re.declaredParameters[p.Name] = p

	if re.variables == nil {
		re.variables = make(map[string]Variable)
	}

	if re.Application == nil {
		return nil
	}

	if raw, ok := re.Application.Overrides[p.Name]; ok {
		v, err := assignToType(re, p.Name, "override", raw, p.Type)
		if err != nil {
			return err
		}
		re.variables[p.Name] = Variable{
			Name:   p.Name,
			Value:  v,
			Source: VariableSource{Kind: VariableSourceKindOverride, Name: p.Name},
		}
		return nil
	}

	if raw, ok := re.Application.Flags[p.Name]; ok {
		v, err := assignToType(re, p.Name, "flag", raw, p.Type)
		if err != nil {
			return err
		}
		re.variables[p.Name] = Variable{
			Name:   p.Name,
			Value:  v,
			Source: VariableSource{Kind: VariableSourceKindFlag, Name: p.Name},
		}
		return nil
	}

	// Env source: full string-parsing lands with the resolver's real implementation. Skip silently for now.

	if raw, ok := re.Application.Config[p.Name]; ok {
		v, err := assignToType(re, p.Name, "config", raw, p.Type)
		if err != nil {
			return err
		}
		re.variables[p.Name] = Variable{
			Name:   p.Name,
			Value:  v,
			Source: VariableSource{Kind: VariableSourceKindConfig, Name: p.Name},
		}
		return nil
	}

	if p.Default != nil {
		v, err := assignToType(re, p.Name, "default", p.Default, p.Type)
		if err != nil {
			return err
		}
		re.variables[p.Name] = Variable{
			Name:   p.Name,
			Value:  v,
			Source: VariableSource{Kind: VariableSourceKindDefault, Name: p.Name},
		}
	}

	return nil
}

// assignToType projects raw into a value of the declared type via the [Convert] cascade.
//
// Used by both [RuntimeEnvironment.DeclareParameter] (pre-Phase-4 path) and [VariableResolver.resolveOne]
// (Phase-4 path) to project source-supplied raw values into the parameter's declared Go type. Routes through
// [Convert] so the same conversion contract — identity, assignability, slice/map element-wise, source-side
// [SourceConverter], target-side [TargetConverter], registered Resource construction — applies uniformly to
// variable resolution and to slot fill at dispatch.
//
// Parameters:
//   - `env`: the runtime environment. Used by [Convert] step 7 to look up registered Resource constructors;
//     may be nil for callers whose target types never reach Resource construction (steps 1–6 are pure
//     reflection plus interface dispatch).
//   - `paramName`: parameter name for the error message.
//   - `sourceKind`: source-kind label for the error message ("override", "flag", "config", "default").
//   - `raw`: the source-supplied value.
//   - `declared`: the parameter's declared [reflect.Type].
//
// Returns:
//   - `any`: the projected value, ready to assign to a parameter of type declared.
//   - `error`: non-nil when no conversion path produces a value of declared.
func assignToType(env *RuntimeEnvironment, paramName, sourceKind string, raw any, declared reflect.Type) (any, error) {

	if declared == nil {
		return raw, nil
	}
	if raw == nil {
		if declared.Kind() == reflect.Ptr ||
			declared.Kind() == reflect.Interface ||
			declared.Kind() == reflect.Slice ||
			declared.Kind() == reflect.Map ||
			declared.Kind() == reflect.Chan ||
			declared.Kind() == reflect.Func {
			return raw, nil
		}
		return nil, fmt.Errorf("parameter %q: %s value is nil but declared type %s is not nilable",
			paramName, sourceKind, declared)
	}

	converted, err := Convert(env, raw, declared)
	if err != nil {
		return nil, fmt.Errorf("parameter %q: %s value of type %T not assignable to declared type %s",
			paramName, sourceKind, raw, declared)
	}

	return converted, nil
}

// Capture executes cmd, returning stdout bytes verbatim and streaming stderr through the environment's status UI.
//
// In dry-run, the command is narrated and nil bytes are returned with a nil error.
//
// Parameters:
//   - `cmd`: the prepared exec.Cmd; its Stdout, Stderr, and Cancel fields are overwritten by the runner.
//
// Returns:
//   - []byte: the captured stdout (nil in dry-run).
//   - `error`: a wrapped exit-error including the stderr tail on non-zero exit.
func (re *RuntimeEnvironment) Capture(cmd *exec.Cmd) ([]byte, error) {
	return re.runner().Capture(cmd)
}

// Emit captures stdout, applies parse to produce a typed value, and emits that value to the environment's result sink.
//
// Stderr streams through the status UI. In dry-run, the command is narrated and nil is returned without invoking parse.
//
// Parameters:
//   - `cmd`: the prepared exec.Cmd.
//   - `parse`: converts captured stdout bytes into the typed value forwarded to the result sink.
//
// Returns:
//   - `error`: a wrapped exit-error, parse error, or sink error; nil on success.
func (re *RuntimeEnvironment) Emit(cmd *exec.Cmd, parse func([]byte) (any, error)) error {
	return re.runner().Emit(cmd, parse)
}

// ModuleByName returns a cached provider instance for the named module, constructing it on first access.
//
// Parameters:
//   - `name`: the module name (e.g., "file", "ui").
//
// Returns:
//   - `any`: the provider instance.
//   - `error`: non-nil if the name is not a registered module or construction fails.
func (re *RuntimeEnvironment) ModuleByName(name string) (any, error) {

	prt, ok := re.Registry.ModuleByName(name)
	if !ok {
		return nil, fmt.Errorf("unknown module: %s", name)
	}

	return re.cachedProvider(prt)
}

// VariableByName returns the binding-layer [Variable] resolved for the named parameter.
//
// Returns the zero [Variable] and false until the executor's preflight pass calls [VariableResolver.Resolve].
//
// Parameters:
//   - `name`: the parameter name.
//
// Returns:
//   - Variable: the resolved variable, or the zero value when absent.
//   - `bool`: true if a variable was resolved for this name; false otherwise.
func (re *RuntimeEnvironment) VariableByName(name string) (Variable, bool) {

	if re.variables == nil {
		return Variable{}, false
	}

	v, ok := re.variables[name]
	return v, ok
}

// Run executes cmd, streaming stdout and stderr line-by-line through the runtime environment's status UI.
//
// In dry-run, the command is narrated and nil is returned without launching it.
//
// Parameters:
//   - `cmd`: the prepared exec.Cmd; its Stdout, Stderr, and Cancel fields are overwritten by the runner.
//
// Returns:
//   - `error`: a wrapped exit-error including the stderr tail on non-zero exit.
func (re *RuntimeEnvironment) Run(cmd *exec.Cmd) error {

	return re.runner().Run(cmd)
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// runner builds a process.Runner
//
// Returns:
//   - *process.Runner: the runner bound to the current configuration.
func (re *RuntimeEnvironment) runner() *process.Runner {
	return process.NewRunner(re.Context, re.Application.DryRun(), re.Result, re.Status)
}

// cachedProvider returns a cached provider instance for the given type descriptor, constructing it on first access.
//
// The lock is released before calling Construct to avoid deadlock when a provider's constructor calls cachedProvider for
// a sibling. Double-check after construction handles concurrent callers.
//
// Parameters:
//   - `prt`: the provider receiver type descriptor.
//
// Returns:
//   - `any`: the provider instance.
//   - `error`: non-nil if construction fails.
func (re *RuntimeEnvironment) cachedProvider(prt ProviderReceiverType) (any, error) {

	name := prt.Name()

	re.mutex.Lock()
	if p, ok := re.providers[name]; ok {
		re.mutex.Unlock()
		return p, nil
	}
	re.mutex.Unlock()

	p, err := prt.Construct()(re)
	if err != nil {
		return nil, fmt.Errorf("construct provider %s: %w", name, err)
	}

	re.mutex.Lock()
	if existing, ok := re.providers[name]; ok {
		re.mutex.Unlock()
		return existing, nil
	}
	if re.providers == nil {
		re.providers = make(map[string]any)
	}
	re.providers[name] = p
	re.mutex.Unlock()

	return p, nil
}

// endregion

// endregion
