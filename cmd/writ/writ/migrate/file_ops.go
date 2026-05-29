// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"context"
	"fmt"
	"os"
	"reflect"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/plan"
)

// Mkdir builds a one-node graph for `file.Mkdir` and dispatches it. Replaces the pre-Phase-7 nil-activation
// call site `fp.Mkdir(nil, path, chmod, "")` with the binding-model path: the `path` argument flows as a
// `plan.variable("path")` slot reference resolved at preflight from [op.RuntimeEnvironment.Application.Flags].
//
// Parameters:
//   - `ctx`: parent context; passed verbatim to [op.GraphExecutor.Run].
//   - `targetRoot`: absolute path the [op.Root] is anchored at — `filepath.Dir(path)` for migrate's Mkdir
//     sites is the right anchor since the target may not yet exist.
//   - `path`: the directory to create.
//   - `chmod`: file mode for the new directory.
//
// Returns:
//   - `error`: non-nil on preflight or dispatch failure.
func Mkdir(ctx context.Context, targetRoot string, path string, chmod os.FileMode) error {

	return runFileOp(ctx, targetRoot, "Mkdir", map[string]any{
		"path":  pathVariable,
		"chmod": chmod,
		"chown": "",
	}, map[string]any{
		"path": path,
	})
}

// Move builds a one-node graph for `file.Move` and dispatches it. Replaces the pre-Phase-7 nil-activation
// call site `fp.Move(nil, &file.Resource{SourcePath: ...}, dest)`. Both `source` and `destination_path` flow as
// `plan.variable(...)` slot references; the string → `*file.Resource` coercion at the `source` slot fill is
// handled by `*file.Resource`'s [op.TargetConverter] (landed under Phase 6.0).
//
// Parameters:
//   - `ctx`: parent context.
//   - `targetRoot`: absolute path the [op.Root] is anchored at — must contain both `source` and `dest`.
//   - `source`: the absolute path of the file being moved.
//   - `dest`: the absolute destination path.
//
// Returns:
//   - `error`: non-nil on preflight or dispatch failure.
func Move(ctx context.Context, targetRoot string, source string, dest string) error {

	return runFileOp(ctx, targetRoot, "Move", map[string]any{
		"source":           sourceVariable,
		"destination_path": destinationPathVariable,
	}, map[string]any{
		"source":           source,
		"destination_path": dest,
	})
}

// Link builds a one-node graph for `file.Link` and dispatches it. Replaces the pre-Phase-7 nil-activation
// call site `fp.Link(nil, &file.Resource{SourcePath: ...}, target)`. Both `source` and `target_path` flow as
// `plan.variable(...)` slot references; the string → `*file.Resource` coercion at the `source` slot fill is
// handled by `*file.Resource`'s [op.TargetConverter].
//
// Parameters:
//   - `ctx`: parent context.
//   - `targetRoot`: absolute path the [op.Root] is anchored at.
//   - `source`: the absolute path the symlink points at.
//   - `target`: the absolute path where the symlink lives.
//
// Returns:
//   - `error`: non-nil on preflight or dispatch failure.
func Link(ctx context.Context, targetRoot string, source string, target string) error {

	return runFileOp(ctx, targetRoot, "Link", map[string]any{
		"source":      sourceVariable,
		"target_path": targetPathVariable,
	}, map[string]any{
		"source":      source,
		"target_path": target,
	})
}

// runFileOp builds a single-node graph for the named file-provider method, populates the planning + execution
// specs with the supplied flag values so the resolver can satisfy the [op.VariableValue] slot references, and
// dispatches via the standard [op.Plan] + [op.NewGraphExecutor] + [op.GraphExecutor.Run] sequence. Mirrors
// the dual-spec pattern from [cmd/writ/writ/adopt] (planning and execution share nothing but the resolved
// graph, since [op.RuntimeEnvironment.Close] closes the spec's Root).
//
// Parameters:
//   - `ctx`: parent context.
//   - `targetRoot`: absolute path the per-phase [op.Root] is anchored at.
//   - `methodName`: file-provider method name ("Mkdir" / "Move" / "Link").
//   - `slots`: keyword arguments to the file-provider method. Variable references for user-supplied paths
//     (typically `*op.Variable` from [plan.Provider.Variable]); raw values for tool-set constants.
//   - `flags`: the [application.Application.Flags] map for both phases — supplies the variable resolver
//     with values for any [op.VariableValue] slot reference in `slots`.
//
// Returns:
//   - `error`: non-nil on planning, preflight, or dispatch failure.
func runFileOp(ctx context.Context, targetRoot string, methodName string, slots map[string]any, flags map[string]any) error {

	planningSpec, err := buildMigrateSpec(targetRoot, flags)
	if err != nil {
		return err
	}

	graph, err := op.Plan(ctx, planningSpec, func(env *op.RuntimeEnvironment) (*op.Graph, error) {
		return buildSingleOpGraph(env, methodName, slots)
	})
	if err != nil {
		return err
	}

	executeSpec, err := buildMigrateSpec(targetRoot, flags)
	if err != nil {
		return err
	}

	executor := op.NewGraphExecutor(graph, executeSpec)
	if _, err := executor.Run(ctx, nil); err != nil {
		return fmt.Errorf("migrate %s: %w", methodName, err)
	}

	return nil
}

// buildSingleOpGraph constructs a one-invocation graph for `methodName` against the file provider, using
// `slots` as the kwargs for the method's planner. Routes through [plan.Provider.Assemble] so the orphan
// check passes and the catalog handoff happens.
//
// Parameters:
//   - `env`: the planning runtime environment.
//   - `methodName`: file-provider method to invoke ("Mkdir" / "Move" / "Link").
//   - `slots`: kwargs map; see [runFileOp].
//
// Returns:
//   - *op.Graph: the assembled graph; one top-level child.
//   - `error`: non-nil when the file provider is not registered, the method is not found, or assembly fails.
func buildSingleOpGraph(env *op.RuntimeEnvironment, methodName string, slots map[string]any) (*op.Graph, error) {

	fileType := reflect.TypeFor[file.Provider]()
	rt, ok := env.ReceiverRegistry.TypeByReflection(fileType)
	if !ok {
		rt, ok = env.ReceiverRegistry.TypeByReflection(reflect.PointerTo(fileType))
	}
	if !ok {
		return nil, fmt.Errorf("migrate.runFileOp: file provider not registered in receiver registry")
	}

	fileReceiverType, ok := rt.(op.ProviderReceiverType)
	if !ok {
		return nil, fmt.Errorf("migrate.runFileOp: file provider registered as %T, want op.ProviderReceiverType", rt)
	}

	method, ok := fileReceiverType.MethodByName(methodName)
	if !ok {
		return nil, fmt.Errorf("migrate.runFileOp: file.%s not found in receiver type", methodName)
	}

	planProvider := plan.NewProvider(env)

	unit, err := method.Planner().Plan(planProvider, fileReceiverType, method, nil, slots, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("migrate.runFileOp: plan file.%s: %w", methodName, err)
	}

	label := planProvider.InvocationRegistry().AutoLabel(fileReceiverType.Name() + "." + op.CamelToSnake(methodName))
	invocation := &op.Invocation{
		Target: unit,
		Result: op.NewPromise(unit, ""),
		Label:  label,
	}
	if err := planProvider.InvocationRegistry().Register(label, invocation); err != nil {
		return nil, fmt.Errorf("migrate.runFileOp: register file.%s: %w", methodName, err)
	}

	graph, err := planProvider.Assemble([]*op.Invocation{invocation}, nil, nil, nil, op.Origin{})
	if err != nil {
		return nil, fmt.Errorf("migrate.runFileOp: assemble: %w", err)
	}

	return graph, nil
}

// buildMigrateSpec constructs a fresh [op.RuntimeEnvironmentSpec] anchored at `targetRoot` with `flags`
// supplied to the [application.Application]. Mirrors `cmd/writ/writ/adopt_cmd.go`'s `buildAdoptSpec`: each
// phase (planning + execution) gets its own Root because [op.RuntimeEnvironment.Close] closes the spec's
// Root, and reusing one spec across both phases would invalidate the second.
//
// Parameters:
//   - `targetRoot`: absolute path the [op.Root] is anchored at.
//   - `flags`: the [application.Application.Flags] map.
//
// Returns:
//   - *op.RuntimeEnvironmentSpec: the constructed spec.
//   - `error`: non-nil when [op.NewConfinedRoot] fails.
func buildMigrateSpec(targetRoot string, flags map[string]any) (*op.RuntimeEnvironmentSpec, error) {

	root, err := op.NewConfinedRoot(targetRoot)
	if err != nil {
		return nil, fmt.Errorf("open root %s: %w", targetRoot, err)
	}

	return op.NewRuntimeEnvironmentSpec("writ", op.NewReceiverRegistry()).
		WithRoot(root).
		WithApplication(&application.Application{
			Name:  "writ",
			Flags: flags,
		}), nil
}

// pathVariable, sourceVariable, destinationPathVariable, targetPathVariable are the canonical
// [plan.Provider.Variable] references reused by [Mkdir] / [Move] / [Link] so each helper threads its
// user-supplied paths through the binding model under stable variable names.
//
// These are package-level rather than per-call because [plan.Provider.Variable] returns an [*op.Variable]
// whose only meaningful field is its name; [op.ActionPlanner.Plan] wraps it as [op.VariableValue{Name: …}]
// at slot-stamp time. Per-call construction would just allocate identical structs.
var (
	pathVariable            = &op.Variable{Name: "path"}
	sourceVariable          = &op.Variable{Name: "source"}
	destinationPathVariable = &op.Variable{Name: "destination_path"}
	targetPathVariable      = &op.Variable{Name: "target_path"}
)
