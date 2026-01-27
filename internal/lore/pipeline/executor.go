// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package pipeline

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"go.starlark.net/starlark"

	loreStar "github.com/NobleFactor/devlore-cli/internal/starlark"
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
}

// ExecutionResult captures the result of a full pipeline execution.
type ExecutionResult struct {
	Package   string
	Operation Operation
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

	return &Executor{
		config:   cfg,
		bindings: bindings,
	}
}

// Execute runs the pipeline for a package.
func (e *Executor) Execute(ctx context.Context, lifecycle *Lifecycle, op Operation) (*ExecutionResult, error) {
	result := &ExecutionResult{
		Package:   lifecycle.Name,
		Operation: op,
		Started:   time.Now(),
	}

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
		e.printDryRun(lifecycle, phases)
		result.Success = true
		result.Completed = time.Now()
		result.Duration = result.Completed.Sub(result.Started)
		return result, nil
	}

	// Run each phase
	for _, phaseName := range phases {
		select {
		case <-ctx.Done():
			result.Error = ctx.Err()
			result.Completed = time.Now()
			result.Duration = result.Completed.Sub(result.Started)
			return result, ctx.Err()
		default:
		}

		phaseResult := e.executePhase(lifecycle, phaseName)
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
		return DecommissionPhaseOrder
	default:
		return PhaseOrder
	}
}

func (e *Executor) executePhase(lifecycle *Lifecycle, phaseName string) PhaseResult {
	result := PhaseResult{
		Name:    phaseName,
		Started: time.Now(),
	}

	scriptPath := lifecycle.GetPhaseScript(phaseName)
	if scriptPath == "" {
		if e.config.Verbose {
			fmt.Fprintf(e.config.Output, "Skipping phase %q (not defined)\n", phaseName)
		}
		result.Skipped = true
		result.Success = true
		result.Completed = time.Now()
		result.Duration = result.Completed.Sub(result.Started)
		return result
	}

	e.printPhaseStart(phaseName)

	err := e.runPhaseScript(scriptPath, phaseName)
	result.Completed = time.Now()
	result.Duration = result.Completed.Sub(result.Started)

	if err != nil {
		result.Error = err
		result.Success = false
		fmt.Fprintf(e.config.Output, "\n✗ Phase %q failed: %v\n", phaseName, err)
	} else {
		result.Success = true
		fmt.Fprintf(e.config.Output, "✓ Phase %q completed\n\n", phaseName)
	}

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
			fmt.Fprintf(e.config.Output, "  [print] %s\n", msg)
		},
	}

	// Execute the script
	globals, err := starlark.ExecFile(thread, scriptPath, data, e.bindings.Globals())
	if err != nil {
		return fmt.Errorf("executing script: %w", err)
	}

	// Call the phase function
	fn, ok := globals[phaseName]
	if !ok {
		return fmt.Errorf("function %q not found in script", phaseName)
	}

	callable, ok := fn.(starlark.Callable)
	if !ok {
		return fmt.Errorf("%q is not callable", phaseName)
	}

	_, err = starlark.Call(thread, callable, nil, nil)
	if err != nil {
		return fmt.Errorf("calling %s(): %w", phaseName, err)
	}

	return nil
}

func (e *Executor) printHeader(lifecycle *Lifecycle, features []string, settings map[string]string) {
	fmt.Fprintln(e.config.Output, "╔════════════════════════════════════════════════════════════════╗")
	fmt.Fprintf(e.config.Output, "║  Lore Pipeline: %s\n", lifecycle.Name)
	fmt.Fprintln(e.config.Output, "╚════════════════════════════════════════════════════════════════╝")
	fmt.Fprintln(e.config.Output)
	fmt.Fprintf(e.config.Output, "Package:  %s v%s\n", lifecycle.Name, lifecycle.Version)
	fmt.Fprintf(e.config.Output, "Features: %v\n", features)
	if len(settings) > 0 {
		fmt.Fprintf(e.config.Output, "Settings: %v\n", settings)
	}
	fmt.Fprintln(e.config.Output)
}

func (e *Executor) printDryRun(lifecycle *Lifecycle, phases []string) {
	fmt.Fprintln(e.config.Output, "[DRY RUN] Would execute the following phases:")
	for _, phaseName := range phases {
		if script, ok := lifecycle.Phases[phaseName]; ok {
			fmt.Fprintf(e.config.Output, "  - %s: %s\n", phaseName, script)
		} else {
			fmt.Fprintf(e.config.Output, "  - %s: (not defined, would skip)\n", phaseName)
		}
	}
}

func (e *Executor) printPhaseStart(phaseName string) {
	fmt.Fprintln(e.config.Output, "────────────────────────────────────────────────────────────────")
	fmt.Fprintf(e.config.Output, "Phase: %s\n", phaseName)
	fmt.Fprintln(e.config.Output, "────────────────────────────────────────────────────────────────")
}

func (e *Executor) printSuccess(lifecycle *Lifecycle) {
	fmt.Fprintln(e.config.Output, "════════════════════════════════════════════════════════════════")
	fmt.Fprintf(e.config.Output, "✓ Pipeline completed successfully for %s\n", lifecycle.Name)
	fmt.Fprintln(e.config.Output, "════════════════════════════════════════════════════════════════")
}
