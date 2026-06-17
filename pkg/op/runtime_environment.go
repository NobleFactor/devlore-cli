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
	"github.com/NobleFactor/devlore-cli/pkg/devconfig"
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/platform"
	"github.com/NobleFactor/devlore-cli/pkg/process"
	"github.com/NobleFactor/devlore-cli/pkg/result"
	"github.com/NobleFactor/devlore-cli/pkg/sink"
	"github.com/NobleFactor/devlore-cli/pkg/status"
)

var _ devconfig.Section = (*RuntimeEnvironmentConfig)(nil) // Interface Guard: ensures *Provider implements op.Provider.

func init() {
	devconfig.AnnounceSection(reflect.TypeFor[RuntimeEnvironmentConfig](), func() devconfig.Section {
		return NewRuntimeEnvironmentConfig()
	})
}

// RuntimeEnvironment is the session-scoped execution context for providers, resources, and graphs.
//
// One runtime environment per session. A session is bounded by a single CLI command invocation, a single test in the
// test harness, or any other unit of work that owns a graph's plan-then-execute lifecycle. Long-running processes (test
// runners, daemons, repls) construct a fresh runtime environment per session; one process may produce many runtime
// environments over its lifetime.
//
// The runtime environment owns the [Root] handle and is the single point of Close responsibility. See
// [RuntimeEnvironment.Close]. Every other type in this package that holds a *RuntimeEnvironment
// ([starlarkbridge.Runtime], [GraphExecutor], [Graph]) is a co-user of the session, not an owner; callers construct the
// runtime environment, defer Close once, and pass it by pointer to whatever needs it.
//
// Session-shared state (ResourceCatalog, variable map, provider cache, RecoverySite, …) lives on this
// struct, so plan-time and execute-time machinery operate on the same instances.
type RuntimeEnvironment struct {

	// Application is the tool-side handle carrying the variable-resolver source maps (flags / config / overrides) and
	// the tool's program name (formerly a ProgramName field — now [application.Application.Name] is the single source
	// of truth). Framework code reads system flags such as "dry-run" directly from `Application.Flags` via
	// [application.Application.DryRun].
	Application *application.Application

	// BackupSuffix is appended to back up filenames during conflict resolution.
	BackupSuffix string

	// ConflictPolicy chooses how to handle preflight conflicts.
	ConflictPolicy ConflictPolicy

	// Context carries a deadline, a cancellation signal, and other values across API boundaries.
	//
	// See https://pkg.go.dev/context.
	Context context.Context

	// Platform provides platform abstractions (package manager, service manager) to do providers.
	//
	// Nil when running in environments where host access is not needed (e.g., pure data transforms).
	Platform platform.Platform

	// Root provides scoped filesystem operations.
	//
	// All provider I/O goes through this interface. Three implementations: confinedRoot (execution), RootReader
	// (planning), RootReaderWriter (testing). Created by the executor or test runner; closed after execution completes.
	Root fsroot.Root

	// Result is the primary output pipeline carried from the [RuntimeEnvironmentSpec].
	//
	// Populated by [RuntimeEnvironmentSpec.Build]. Defaults to a [result.Pipeline] writing JSON to
	// [sink.Stdout] when the spec field is nil.
	Result *result.Pipeline

	// Status is the user-facing side-channel narrator carried from the [RuntimeEnvironmentSpec].
	//
	// Same instance that flows to `cli.UI()` and through every status emission point. Populated by
	// [RuntimeEnvironmentSpec.Build] (defaults to a [status.Narrator] writing through [sink.Stderr] when the spec field
	// is nil; pass a Narrator wrapping [sink.Discard] to suppress).
	Status *status.Narrator

	// RecoverySite is the shared recovery service for archiving and restoring resources during compensation.
	//
	// Instantiated by the executor from Root.
	RecoverySite *RecoverySite

	// ResourceCatalog is the resource catalog for the current execution session.
	//
	// The do layer uses it to shadow Resource results after dispatch. Nil when running without catalog integration
	// (e.g., tests).
	ResourceCatalog *ResourceCatalog

	// declaredParameters records every parameter registered via [RegisterParameter], keyed by name.
	//
	// Used to detect type-mismatch on re-registration and to enumerate the session-resolved parameter surface for
	// diagnostics.
	declaredParameters map[string]Parameter

	// variableResolver assembles binding-layer variable values from the spec's flag / config / override
	// sources plus the process environment.
	//
	// Read by the executor's preflight pass to populate variables.
	variableResolver *VariableResolver

	// variables is the binding-layer resolved variable map.
	//
	// It is populated by [VariableResolver.Resolve] during the executor's preflight pass. Phase 1 ships this
	// nil-initialized; Phase 4 wires the preflight pass.
	variables map[string]Variable

	// resolvers memoizes a lazy resolver per declared parameter (name → func() (Variable, bool), built with
	// [sync.OnceValues]). [VariableByName] falls back to it when `variables` has no eagerly-resolved entry, so a
	// source value set after the parameter was registered — e.g. the application's "config" override, wired after
	// the runtime is built — is still found regardless of registration-vs-set order. Resolution runs once, on first
	// read, and the (Variable, found) result is cached; both the map and each resolver are concurrency-safe.
	//
	// INVARIANT: the first read of a parameter must follow population of its source. Because OnceValues memoizes the
	// first result — including a not-found — a read *before* the source is set caches the absence permanently and a
	// later set is not picked up. This holds today because no provider reads a variable at construction (before the
	// application wires its source maps); every read happens at dispatch. Reading a variable in a provider
	// constructor would silently reintroduce the config-resolution bug — see TestVariableByName_ReadBeforeSourceSet.
	resolvers sync.Map

	// mutex guards the providers and services maps for concurrent access.
	mutex sync.Mutex

	// providers is a map of lazily constructed provider instances by name.
	providers map[string]any

	// closeErr captures the joined error from the close-once execution and is returned by every Close call
	// after the first.
	closeErr error

	// closeOnce guards [RuntimeEnvironment.Close], so the close path runs exactly once per runtime environment,
	// regardless of how many times defer fires.
	closeOnce sync.Once
}

// NewRuntimeEnvironment constructs a fully populated [RuntimeEnvironment] from this spec.
//
// It performs defaulting (BackupSuffix → ".<ProgramName>-backup", Status → [status.Narrator] over [sink.Stderr], Result
// → [result.Pipeline] writing JSON to [sink.Stdout]) and wires the [RecoverySite] if a Root is present.
//
// Returns:
//   - `*RuntimeEnvironment`: the constructed runtime environment.
func NewRuntimeEnvironment(ctx context.Context, spec *RuntimeEnvironmentSpec) *RuntimeEnvironment {

	assert.NonZero("spec", spec)
	assert.NonZero("spec.Application", spec.Application)

	statusNarrator := spec.Status

	if statusNarrator == nil {
		statusNarrator = status.NewNarrator(spec.ProgramName, sink.Stderr())
	}

	resultPipeline := spec.Result

	if resultPipeline == nil {
		resultPipeline = result.NewPipeline(nil, result.JSONFormatter{}, sink.Stdout())
	}

	resourceCatalog := spec.ResourceCatalog

	if resourceCatalog == nil {
		resourceCatalog = NewResourceCatalog()
	}

	platformCapability := spec.Platform

	if platformCapability == nil {
		if detected, err := platform.Detect(); err == nil {
			platformCapability, _ = platform.New(detected)
		}
	}

	runtimeEnvironment := &RuntimeEnvironment{
		Application:      spec.Application,
		Context:          ctx,
		Platform:         platformCapability,
		ResourceCatalog:  resourceCatalog,
		Root:             spec.Root,
		Status:           statusNarrator,
		Result:           resultPipeline,
		variableResolver: NewVariableResolver(spec.Application),
	}

	if spec.Root != nil {
		runtimeEnvironment.RecoverySite = NewRecoverySite(runtimeEnvironment)
	}

	return runtimeEnvironment
}

// region EXPORTED METHODS

// region State management

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

	providerReceiverType, ok := ReceiverRegistry().ActionByName(receiverName)
	if !ok {
		return nil, fmt.Errorf("unknown action provider: %s", receiverName)
	}

	var method *Method

	for m := range providerReceiverType.Methods() {
		if CamelToSnake(m.Name()) == methodSnake {
			method = m
			break
		}
	}

	if method == nil {
		return nil, fmt.Errorf("action %q: method %q not found on %q", name, methodSnake, receiverName)
	}

	if _, err := re.cachedProvider(providerReceiverType); err != nil {
		return nil, err
	}

	return newAction(providerReceiverType, method, name), nil
}

func (re *RuntimeEnvironment) Config() *RuntimeEnvironmentConfig {
	return nil
}

// endregion

// region Behaviors

// Capture executes cmd, returning stdout bytes verbatim and streaming stderr through the environment's status UI.
//
// In dry-run, the command is narrated and nil bytes are returned with a nil error.
//
// Parameters:
//   - `cmd`: the prepared exec.Cmd; its Stdout, Stderr, and Cancel fields are overwritten by the runner.
//
// Returns:
//   - `[]byte`: the captured stdout (nil in dry-run).
//   - `error`: a wrapped exit-error including the stderr tail on non-zero exit.
func (re *RuntimeEnvironment) Capture(cmd *exec.Cmd) ([]byte, error) {

	return re.runner().Capture(cmd)
}

// Close releases the session's owned resources — currently the [Root] handle.
//
// Idempotent: The close path runs exactly once per runtime environment regardless of how many times Close is called.
// The first call performs the close and stores any joined error; later calls return the stored error without
// re-closing.
//
// Callers construct the runtime environment, defer Close once, then hand the runtime environment by pointer to whatever
// uses it ([starlarkbridge.Runtime], [GraphExecutor], providers, …). Holders do not implement their own Close. The
// [RuntimeEnvironment] is the single owner.
//
// Returns:
//   - `error`: the joined error from closing the runtime environment's owned resources, or nil on success.
func (re *RuntimeEnvironment) Close() error {

	re.closeOnce.Do(func() {
		iox.Close(&re.closeErr, re.Root)
	})

	return re.closeErr
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

	providerReceiverType, ok := ReceiverRegistry().ModuleByName(name)
	if !ok {
		return nil, fmt.Errorf("unknown module: %s", name)
	}

	return re.cachedProvider(providerReceiverType)
}

// ProviderByType returns a cached provider instance for the given Go type, constructing it on first access.
//
// Resolves `reflectType` to its [ProviderReceiverType] via [ReceiverRegistry.TypeByReflection], then delegates to the
// shared provider cache. Use this when one provider needs to invoke another's methods (e.g.,
// archive.Provider.CompensateExtract delegating to file.Provider.CompensateWriteText) — the returned instance is the
// same one [action_types.go] resolves on the dispatch path for the same `(runtimeEnvironment, type)` pair, so the
// GC-amortization invariant from D16(c) in `docs/plans/extract-starlark-from-op/phase-8.md` holds across this access
// path too.
//
// `reflectType` must match the form passed to [AnnounceProvider] at registration time — the struct type (e.g.,
// `reflect.TypeFor[file.Provider]()`), not the pointer type. The returned `any` is the constructor's return value,
// which is conventionally `*Provider`; callers type-assert as `provider.(*file.Provider)`.
//
// Parameters:
//   - `reflectType`: the provider's Go type, in the struct form used at registration.
//
// Returns:
//   - `any`: the provider instance.
//   - `error`: non-nil if the type is not registered, the registered receiver is not a provider, or
//     construction fails.
func (re *RuntimeEnvironment) ProviderByType(reflectType reflect.Type) (any, error) {

	receiverType, ok := ReceiverRegistry().TypeByReflection(reflectType)
	if !ok {
		return nil, fmt.Errorf("unknown provider type: %s", reflectType)
	}

	providerReceiverType, ok := receiverType.(ProviderReceiverType)
	if !ok {
		return nil, fmt.Errorf("type %s is not a provider", reflectType)
	}

	return re.cachedProvider(providerReceiverType)
}

// RegisterParameter declares interest in a binding-layer variable.
//
// The cascade resolves the parameter immediately against the [Application]'s source maps (override → flag → environment
// variable → config → default) and stores the resolved [Variable] in the runtime environment's `variables` map.
// Subsequent reads via [RuntimeEnvironment.VariableByName] return the typed value with [VariableSource] provenance.
//
// Reregistration of the same name is idempotent when the declared [Parameter.Type] matches the prior declaration; a
// type mismatch returns an error without overwriting state.
//
// Type checking on source-supplied values: every non-environment-variable source's raw value is asserted against the
// declared Type via [reflect.Type.AssignableTo]. Mismatches return an error before any storage. The environment
// variable source (which always yields strings) currently skips parameters whose Type is not string-parsable — full
// environment-variable-string parsing lands with the binding-resolver real implementation in a later phase.
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

	// Install a memoized resolver so [VariableByName] can resolve this parameter lazily on first read. A source value
	// set after registration (e.g. the application's "config" override, wired after the runtime is built) is then still
	// found, regardless of call order. [sync.OnceValues] runs the cascade once and caches (Variable, found),
	// concurrency-safe. A conversion error during the lazy resolve is treated as unresolved; the eager pass below
	// surfaces conversion errors when the source is already present at registration.

	re.resolvers.LoadOrStore(p.Name, sync.OnceValues(func() (Variable, bool) {
		v, found, err := re.resolveParameter(p)
		if err != nil {
			return Variable{}, false
		}
		return v, found
	}))

	// Eager pass: if a source already supplies the value at registration, resolve it now. This preserves early
	// conversion error reporting and populates `variables` for the common case. A miss defers to the lazy resolver.

	v, found, err := re.resolveParameter(p)
	if err != nil {
		return err
	}

	if found {
		re.variables[p.Name] = v
	}

	return nil
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

// VariableByName returns the binding-layer [Variable] resolved for the named parameter.
//
// It reads the eagerly-resolved `variables` map first (populated by [RegisterParameter] when a source was present at
// registration, and by the executor's preflight pass). On a miss it falls back to the parameter's lazy resolver
// (installed by [RegisterParameter], memoized with [sync.OnceValues]), which resolves on first read and caches — so a
// source value set after registration is still found, independent of call order. Returns the zero [Variable] and false
// only when the name was never declared or no source supplies a value.
//
// Parameters:
//   - `name`: the parameter name.
//
// Returns:
//   - `Variable`: the resolved variable, or the zero value when absent.
//   - `bool`: true if a variable was resolved for this name; false otherwise.
func (re *RuntimeEnvironment) VariableByName(name string) (Variable, bool) {

	if re.variables != nil {
		if v, ok := re.variables[name]; ok {
			return v, true
		}
	}

	if resolve, ok := re.resolvers.Load(name); ok {
		return resolve.(func() (Variable, bool))()
	}

	return Variable{}, false
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// cachedProvider returns a cached provider instance for the given type descriptor, constructing it on first access.
//
// The lock is released before calling Construct to avoid deadlock when a provider's constructor calls cachedProvider
// for a sibling. Double-check after construction handles concurrent callers.
//
// Parameters:
//   - `providerReceiverType`: the provider receiver type descriptor.
//
// Returns:
//   - `any`: the provider instance.
//   - `error`: non-nil if construction fails.
func (re *RuntimeEnvironment) cachedProvider(providerReceiverType ProviderReceiverType) (any, error) {

	name := providerReceiverType.Name()
	re.mutex.Lock()

	if p, ok := re.providers[name]; ok {
		re.mutex.Unlock()
		return p, nil
	}

	re.mutex.Unlock()

	p, err := providerReceiverType.Construct()(re)
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

// resolveParameter walks the source cascade (override → flag → config → default) for one declared parameter.
//
// It is the single resolution path shared by the eager pass in [RuntimeEnvironment.RegisterParameter] and the lazy
// resolver consulted by [RuntimeEnvironment.VariableByName]. With no [application.Application] it resolves nothing, so
// a parameter stays unresolved until an Application supplies a source. The environment-variable source is not yet
// wired.
//
// Parameters:
//   - `p`: the declared parameter; `p.Name` is the lookup key and `p.Type` the conversion target.
//
// Returns:
//   - `Variable`: the resolved variable on a source hit.
//   - `bool`: true on a hit; false when no source supplies a value.
//   - `error`: non-nil when a source value cannot be converted to `p.Type`.
func (re *RuntimeEnvironment) resolveParameter(p Parameter) (Variable, bool, error) {

	if re.Application == nil {
		return Variable{}, false, nil
	}

	if raw, ok := re.Application.Overrides[p.Name]; ok {
		v, err := assignToType(re, p.Name, "override", raw, p.Type)
		if err != nil {
			return Variable{}, false, err
		}
		return Variable{
			Name:   p.Name,
			Value:  v,
			Source: VariableSource{Kind: VariableSourceKindOverride, Name: p.Name},
		}, true, nil
	}

	if raw, ok := re.Application.Flags[p.Name]; ok {
		v, err := assignToType(re, p.Name, "flag", raw, p.Type)
		if err != nil {
			return Variable{}, false, err
		}
		return Variable{
			Name:   p.Name,
			Value:  v,
			Source: VariableSource{Kind: VariableSourceKindFlag, Name: p.Name},
		}, true, nil
	}

	// Environment-variable source: string-parsing lands with the resolver's real implementation; skip for now.

	if raw, ok := re.Application.Config[p.Name]; ok {
		v, err := assignToType(re, p.Name, "config", raw, p.Type)
		if err != nil {
			return Variable{}, false, err
		}
		return Variable{
			Name:   p.Name,
			Value:  v,
			Source: VariableSource{Kind: VariableSourceKindConfig, Name: p.Name},
		}, true, nil
	}

	if p.Default != nil {
		v, err := assignToType(re, p.Name, "default", p.Default, p.Type)
		if err != nil {
			return Variable{}, false, err
		}
		return Variable{
			Name:   p.Name,
			Value:  v,
			Source: VariableSource{Kind: VariableSourceKindDefault, Name: p.Name},
		}, true, nil
	}

	return Variable{}, false, nil
}

// runner builds a process.Runner for the current configuration.
//
// Returns:
//   - `*process.Runner`: the runner bound to the current configuration.
func (re *RuntimeEnvironment) runner() *process.Runner {
	return process.NewRunner(re.Context, re.Application.DryRun(), re.Result, re.Status)
}

// endregion

// endregion

// region SUPPORTING TYPES

// ConflictPolicy specifies how to handle conflicts during execution.
type ConflictPolicy int

const (
	// ConflictStop aborts execution on first conflict.
	ConflictStop ConflictPolicy = iota
	// ConflictBackup moves conflicting files to timestamped backups.
	ConflictBackup
	// ConflictOverwrite removes conflicting files without backup.
	ConflictOverwrite
	// ConflictSkip skips conflicting files and continues.
	ConflictSkip
)

// RuntimeEnvironmentConfig is pkg/op's own configuration section: the execution-runtime settings the framework reads.
//
// It carries dry-run, the conflict policy, and the backup suffix — the settings [RuntimeEnvironment] applies during
// execution. As the framework's own owner it is announced at init(), and consumers read it live from
// [application.Application.Config]: the builtin floor now, the same lookup enriched with file / env / cli once the
// loader resolves those sources.
//
// # TODO(david-noble): migrate the consumers (file.Provider.Backup, the dry-run readers) off the spec and Application
// flags onto Application.Config.
type RuntimeEnvironmentConfig struct {
	devconfig.SectionBase

	// BackupSuffix is appended to back up filenames during conflict resolution.
	BackupSuffix string

	// ConflictPolicy chooses how preflight conflicts are handled.
	ConflictPolicy ConflictPolicy

	// DryRun narrates actions instead of performing them when true.
	DryRun bool
}

// NewRuntimeEnvironmentConfig returns the runtime section at its builtin floor.
//
// Floor: dry-run off, [ConflictStop], and `BackupSuffix` ".devlore-backup".
//
// Returns:
//   - `*RuntimeEnvironmentConfig`: the runtime section at its builtin floor.
func NewRuntimeEnvironmentConfig() *RuntimeEnvironmentConfig {

	return &RuntimeEnvironmentConfig{
		SectionBase:    devconfig.NewSectionBase("runtime"),
		BackupSuffix:   ".devlore-backup",
		ConflictPolicy: ConflictStop,
		DryRun:         false,
	}
}

// RuntimeEnvironmentSpec holds configuration for constructing Starlark bindings.
//
// Use [NewRuntimeEnvironmentSpec] to create, then chain With* methods:
//
//	cfg := op.NewRuntimeEnvironmentSpec("lore").
//	    WithModules(op.ReceiverRegistry().ModuleByName("file"), op.ReceiverRegistry().ModuleByName("json")).
//	    WithRoot(fsroot.OpenConfined(wd)).
//	    WithBackupSuffix(".bak").
//	    WithConflictPolicy(op.ConflictBackup)
type RuntimeEnvironmentSpec struct {

	// ProgramName identifies the running tool (e.g., "lore", "writ").
	ProgramName string

	// Modules lists the selected modules to expose as Starlark globals.
	Modules []ProviderReceiverType

	// Application is the tool-side handle that carries the variable-resolver source maps (flags / config /
	// overrides) and the tool's program name. Tools set this via [WithApplication]; pkg/op builds the
	// [VariableResolver] from it at [NewRuntimeEnvironment] time.
	Application *application.Application

	// ResourceCatalog is the resource catalog the constructed runtime environment will hold.
	//
	// When nil, [NewRuntimeEnvironment] creates a fresh empty [ResourceCatalog]. Callers that need to seed the
	// environment with a pre-built catalog — typically [GraphExecutor.Run] cloning the graph's planning catalog
	// onto the per-run environment — set this via [WithCatalog].
	ResourceCatalog *ResourceCatalog

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
	Root fsroot.Root

	// Status is the user-facing side-channel narrator.
	//
	// It Carries categorized status messages and starlark `print()` output. The same instance flows from the client's
	// bootstrap into the runtime environment. When nil, [RuntimeEnvironmentSpec.Build] defaults to a [status.Narrator]
	// writing through [sink.Stderr]; pass a Narrator wrapping [sink.Discard] to suppress.
	Status *status.Narrator
}

// NewRuntimeEnvironmentSpec creates a RuntimeEnvironmentSpec with the given program name.
//
// Parameters:
//   - `programName`: the name of the running tool (e.g., "lore", "wri	t").
//
// Returns:
//   - *RuntimeEnvironmentSpec: the initialized config.
func NewRuntimeEnvironmentSpec(programName string) *RuntimeEnvironmentSpec {

	return &RuntimeEnvironmentSpec{
		ProgramName: programName,
		Status:      status.NewNarrator(programName, sink.Discard()),
		Result:      result.NewPipeline(nil, result.JSONFormatter{}, sink.Discard()),
	}
}

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

// WithCatalog seeds the constructed runtime environment with the supplied [*ResourceCatalog] instead of
// having [NewRuntimeEnvironment] create a fresh one.
//
// Used by [GraphExecutor.Run] to clone the planning graph's catalog onto the per-run environment, so the
// per-run env is born with the right catalog instead of having one created and immediately replaced.
//
// Parameters:
//   - `catalog`: the catalog to seed the environment with. Nil means "fall back to the default of a fresh
//     empty catalog created at [NewRuntimeEnvironment] time."
//
// Returns:
//   - *RuntimeEnvironmentSpec: the config for method chaining.
func (c *RuntimeEnvironmentSpec) WithCatalog(catalog *ResourceCatalog) *RuntimeEnvironmentSpec {

	c.ResourceCatalog = catalog
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
//   - `fsroot`: the filesystem root.
//
// Returns:
//   - *RuntimeEnvironmentSpec: the config for method chaining.
func (c *RuntimeEnvironmentSpec) WithRoot(root fsroot.Root) *RuntimeEnvironmentSpec {

	c.Root = root
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
