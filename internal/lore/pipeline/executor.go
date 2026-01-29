// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pipeline

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/engine"
	"github.com/NobleFactor/devlore-cli/internal/host"
	"github.com/NobleFactor/devlore-cli/internal/registry"
	loreStar "github.com/NobleFactor/devlore-cli/internal/starlark"
	"github.com/NobleFactor/devlore-cli/internal/starlark/platform"
)

// Operation represents the type of pipeline operation.
type Operation int

const (
	// OpDeploy runs the standard deploy pipeline: prepare → install → provision → verify
	OpDeploy Operation = iota
	// OpUpgrade runs the upgrade pipeline (same as deploy with version context)
	OpUpgrade
	// OpDecommission runs the reverse pipeline: unprovision → uninstall → cleanup
	OpDecommission
)

// String returns the operation name.
func (o Operation) String() string {
	switch o {
	case OpDeploy:
		return "deploy"
	case OpUpgrade:
		return "upgrade"
	case OpDecommission:
		return "decommission"
	default:
		return "unknown"
	}
}

// toRegistryOp converts pipeline.Operation to registry.Operation.
func (o Operation) toRegistryOp() registry.Operation {
	switch o {
	case OpDeploy:
		return registry.OpDeploy
	case OpUpgrade:
		return registry.OpUpgrade
	case OpDecommission:
		return registry.OpDecommission
	default:
		return registry.OpDeploy
	}
}

// ExecutorConfig configures the pipeline executor.
type ExecutorConfig struct {
	// Features to enable for this execution.
	Features []string

	// Settings to pass to phase scripts.
	Settings map[string]string

	// DryRun shows what would happen without executing.
	DryRun bool

	// Verbose enables detailed output.
	Verbose bool

	// Output is where execution output is written.
	Output io.Writer

	// SinglePhase runs only this phase (empty = all phases).
	SinglePhase string

	// Platform override (empty = auto-detect).
	Platform string
}

// PhaseResult captures the result of executing a single phase.
type PhaseResult struct {
	Name      string
	Started   time.Time
	Completed time.Time
	Duration  time.Duration
	Success   bool
	Error     error
	Skipped   bool
	Scripts   []string // Scripts that were executed for this phase
}

// ExecutionResult captures the result of a full pipeline execution.
type ExecutionResult struct {
	Package   string
	Operation Operation
	Platform  string
	Started   time.Time
	Completed time.Time
	Duration  time.Duration
	Success   bool
	Phases    []PhaseResult
	Error     error
}

// Executor runs lore pipeline phases.
type Executor struct {
	config   ExecutorConfig
	bindings *loreStar.Bindings
	host     host.Host
	platform string // Resolved platform string for registry

	// Per-execution state (set during execute)
	currentLifecycle *registry.Lifecycle
	currentGraph     *engine.Graph
}

// NewExecutor creates a new pipeline executor.
func NewExecutor(cfg ExecutorConfig) *Executor {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}
	if cfg.Settings == nil {
		cfg.Settings = make(map[string]string)
	}

	bindings := loreStar.NewBindings(cfg.Features, cfg.Settings, cfg.Output)
	h := host.NewHost()

	// Resolve platform
	plat := cfg.Platform
	if plat == "" {
		plat = DetectPlatform()
	}

	return &Executor{
		config:   cfg,
		bindings: bindings,
		host:     h,
		platform: plat,
	}
}

// DetectPlatform converts host.Platform to registry platform string.
// Examples: "Darwin", "Linux", "Linux.Debian", "Linux.Fedora", "Windows"
func DetectPlatform() string {
	p := host.DetectPlatform()
	switch p.OS {
	case "darwin":
		return "Darwin"
	case "windows":
		return "Windows"
	case "linux":
		// Qualify with distro if available
		switch strings.ToLower(p.Distro) {
		case "debian", "ubuntu":
			return "Linux.Debian"
		case "fedora", "rhel", "centos", "rocky", "alma":
			return "Linux.Fedora"
		default:
			return "Linux"
		}
	default:
		return "Linux"
	}
}

// ExecutePackage runs the pipeline for a LorePackage.
func (e *Executor) ExecutePackage(ctx context.Context, pkg *registry.LorePackage, op Operation) (*ExecutionResult, error) {
	lifecycle := pkg.Lifecycle()
	return e.execute(ctx, lifecycle, pkg.Dir, op)
}

// Execute runs the pipeline for a package lifecycle.
// Deprecated: Use ExecutePackage with a LorePackage for proper directory resolution.
func (e *Executor) Execute(ctx context.Context, lifecycle *registry.Lifecycle, packageDir string, op Operation) (*ExecutionResult, error) {
	return e.execute(ctx, lifecycle, packageDir, op)
}

func (e *Executor) execute(ctx context.Context, lifecycle *registry.Lifecycle, packageDir string, op Operation) (*ExecutionResult, error) {
	result := &ExecutionResult{
		Package:   lifecycle.Name,
		Operation: op,
		Platform:  e.platform,
		Started:   time.Now(),
	}

	// Store execution state
	e.currentLifecycle = lifecycle
	e.currentGraph = &engine.Graph{}

	// Resolve features and settings
	features := lifecycle.EnabledFeatures(e.config.Features)
	settings := lifecycle.ResolvedSettings(e.config.Settings)

	// Update bindings with resolved values
	e.bindings.UpdateFeatures(features)
	e.bindings.UpdateSettings(settings)

	// Print header
	e.printHeader(lifecycle, features, settings)

	// Determine phases to run
	phases := e.phasesToRun(op)

	if e.config.DryRun {
		e.printDryRun(lifecycle, packageDir, op, phases)
		result.Success = true
		result.Completed = time.Now()
		result.Duration = result.Completed.Sub(result.Started)
		return result, nil
	}

	// Run each phase
	regOp := op.toRegistryOp()
	for _, phaseName := range phases {
		select {
		case <-ctx.Done():
			result.Error = ctx.Err()
			result.Completed = time.Now()
			result.Duration = result.Completed.Sub(result.Started)
			return result, ctx.Err()
		default:
		}

		phaseResult := e.executePhase(lifecycle, packageDir, regOp, phaseName)
		result.Phases = append(result.Phases, phaseResult)

		if !phaseResult.Success && !phaseResult.Skipped {
			result.Error = phaseResult.Error
			result.Completed = time.Now()
			result.Duration = result.Completed.Sub(result.Started)
			return result, phaseResult.Error
		}
	}

	result.Success = true
	result.Completed = time.Now()
	result.Duration = result.Completed.Sub(result.Started)

	e.printSuccess(lifecycle)
	return result, nil
}

func (e *Executor) phasesToRun(op Operation) []string {
	if e.config.SinglePhase != "" {
		return []string{e.config.SinglePhase}
	}

	switch op {
	case OpDecommission:
		return registry.DecommissionPhaseOrder
	case OpUpgrade:
		return registry.UpgradePhaseOrder
	default:
		return registry.DeployPhaseOrder
	}
}

func (e *Executor) executePhase(lifecycle *registry.Lifecycle, packageDir string, op registry.Operation, phaseName string) PhaseResult {
	result := PhaseResult{
		Name:    phaseName,
		Started: time.Now(),
	}

	// Discover all scripts for this phase (chained execution)
	scripts := lifecycle.DiscoverPhaseScripts(packageDir, e.platform, op, phaseName)
	result.Scripts = scripts

	if len(scripts) == 0 {
		if e.config.Verbose {
			_, _ = fmt.Fprintf(e.config.Output, "Skipping phase %q (no scripts found)\n", phaseName)
		}
		result.Skipped = true
		result.Success = true
		result.Completed = time.Now()
		result.Duration = result.Completed.Sub(result.Started)
		return result
	}

	e.printPhaseStart(phaseName, scripts)

	// Execute scripts in order (general → specific)
	for _, scriptPath := range scripts {
		err := e.runPhaseScript(scriptPath, phaseName)
		if err != nil {
			result.Error = err
			result.Success = false
			result.Completed = time.Now()
			result.Duration = result.Completed.Sub(result.Started)
			_, _ = fmt.Fprintf(e.config.Output, "\n✗ Phase %q failed in %s: %v\n", phaseName, scriptPath, err)
			return result
		}
	}

	result.Success = true
	result.Completed = time.Now()
	result.Duration = result.Completed.Sub(result.Started)
	_, _ = fmt.Fprintf(e.config.Output, "✓ Phase %q completed\n\n", phaseName)

	return result
}

func (e *Executor) runPhaseScript(scriptPath, phaseName string) error {
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		return fmt.Errorf("reading script: %w", err)
	}

	thread := &starlark.Thread{
		Name: phaseName,
		Print: func(_ *starlark.Thread, msg string) {
			_, _ = fmt.Fprintf(e.config.Output, "  [print] %s\n", msg)
		},
	}

	// Create the three binding arguments
	systemBindings := loreStar.NewSystemBindings(e.host)
	planBindings := platform.NewPlanBindings(e.currentGraph, e.host, e.currentLifecycle.Name)

	// Create package context
	features := e.currentLifecycle.EnabledFeatures(e.config.Features)
	settings := e.currentLifecycle.ResolvedSettings(e.config.Settings)

	pkgContext := &loreStar.PackageContext{
		Name:       e.currentLifecycle.Name,
		Version:    e.currentLifecycle.Version,
		Features:   features,
		Settings:   settings,
		DryRun:     e.config.DryRun,
		SourceRoot: "", // Set by caller if needed
		TargetRoot: e.host.HomeDir(),
	}

	// Build globals with legacy bindings plus new three-argument bindings
	globals := e.bindings.Globals()
	globals["system"] = systemBindings.ToStarlark()
	globals["package"] = pkgContext.ToStarlark()
	globals["plan"] = planBindings.ToStarlark()

	// Execute the script
	scriptGlobals, err := starlark.ExecFile(thread, scriptPath, data, globals)
	if err != nil {
		return fmt.Errorf("executing script: %w", err)
	}

	// Call the phase function
	fn, ok := scriptGlobals[phaseName]
	if !ok {
		return fmt.Errorf("function %q not found in script", phaseName)
	}

	callable, ok := fn.(starlark.Callable)
	if !ok {
		return fmt.Errorf("%q is not callable", phaseName)
	}

	// Determine if this is a new-style script (accepts 3 args) or legacy (accepts 0 args)
	// by checking the function signature
	// Argument order: package, system, plan
	args := starlark.Tuple{
		pkgContext.ToStarlark(),
		systemBindings.ToStarlark(),
		planBindings.ToStarlark(),
	}

	// Try calling with three arguments first (new style)
	_, err = starlark.Call(thread, callable, args, nil)
	if err != nil {
		errStr := err.Error()
		// Check if the error is about wrong number of arguments
		// Starlark error messages include patterns like:
		// - "accepts no arguments"
		// - "missing 1 required positional argument"
		// - "accepts 1 positional argument"
		if strings.Contains(errStr, "accepts no argument") ||
			strings.Contains(errStr, "takes no argument") ||
			strings.Contains(errStr, "expected 0 argument") {
			// Legacy script: call with no arguments
			_, err = starlark.Call(thread, callable, nil, nil)
			if err != nil {
				return fmt.Errorf("calling %s(): %w", phaseName, err)
			}
			return nil
		}
		return fmt.Errorf("calling %s(): %w", phaseName, err)
	}

	return nil
}

func (e *Executor) printHeader(lifecycle *registry.Lifecycle, features []string, settings map[string]string) {
	_, _ = fmt.Fprintln(e.config.Output, "╔════════════════════════════════════════════════════════════════╗")
	_, _ = fmt.Fprintf(e.config.Output, "║  Lore Pipeline: %s\n", lifecycle.Name)
	_, _ = fmt.Fprintln(e.config.Output, "╚════════════════════════════════════════════════════════════════╝")
	_, _ = fmt.Fprintln(e.config.Output)
	_, _ = fmt.Fprintf(e.config.Output, "Package:  %s v%s\n", lifecycle.Name, lifecycle.Version)
	_, _ = fmt.Fprintf(e.config.Output, "Platform: %s\n", e.platform)
	_, _ = fmt.Fprintf(e.config.Output, "Features: %v\n", features)
	if len(settings) > 0 {
		_, _ = fmt.Fprintf(e.config.Output, "Settings: %v\n", settings)
	}
	_, _ = fmt.Fprintln(e.config.Output)
}

func (e *Executor) printDryRun(lifecycle *registry.Lifecycle, packageDir string, op Operation, phases []string) {
	regOp := op.toRegistryOp()
	_, _ = fmt.Fprintln(e.config.Output, "[DRY RUN] Would execute the following phases:")
	for _, phaseName := range phases {
		scripts := lifecycle.DiscoverPhaseScripts(packageDir, e.platform, regOp, phaseName)
		if len(scripts) > 0 {
			_, _ = fmt.Fprintf(e.config.Output, "  - %s:\n", phaseName)
			for _, s := range scripts {
				_, _ = fmt.Fprintf(e.config.Output, "      %s\n", s)
			}
		} else {
			_, _ = fmt.Fprintf(e.config.Output, "  - %s: (no scripts, would skip)\n", phaseName)
		}
	}
}

func (e *Executor) printPhaseStart(phaseName string, scripts []string) {
	_, _ = fmt.Fprintln(e.config.Output, "────────────────────────────────────────────────────────────────")
	_, _ = fmt.Fprintf(e.config.Output, "Phase: %s\n", phaseName)
	if len(scripts) > 1 {
		_, _ = fmt.Fprintf(e.config.Output, "Scripts: %d (chained)\n", len(scripts))
	}
	_, _ = fmt.Fprintln(e.config.Output, "────────────────────────────────────────────────────────────────")
}

func (e *Executor) printSuccess(lifecycle *registry.Lifecycle) {
	_, _ = fmt.Fprintln(e.config.Output, "════════════════════════════════════════════════════════════════")
	_, _ = fmt.Fprintf(e.config.Output, "✓ Pipeline completed successfully for %s\n", lifecycle.Name)
	_, _ = fmt.Fprintln(e.config.Output, "════════════════════════════════════════════════════════════════")
}
