// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package adopt

import (
	"fmt"
	"os"
	"reflect"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/plan"
)

// BuildGraph constructs the mkdir → move → link three-node adopt graph.
//
// Realizes the writ-adopt unit operation through the 13.0(n) binding model: every user-controlled input (destination
// directory, source path, destination path) enters as an [op.VariableValue] slot bound to a `plan.variable("<name>")`
// reference. The executor's preflight resolver fills those variables from the [op.RuntimeEnvironment.Application]
// (cobra flags supplied by the writ adopt command) at execute time; slot fill at dispatch converts `string` flag
// values to `*file.Resource` where the method signature requires it, via the framework's [op.TargetConverter]
// cascade landed under Phase 6.0.
//
// Graph shape (three top-level children of `graph.Root`):
//
//  1. file.Mkdir — destination directory; `path = plan.variable("dest_dir")`, `chmod = 0o755`, `chown = ""`.
//  2. file.Move — bytes from source to destination; `source = plan.variable("source_path")` (string flag value
//     coerced to *file.Resource by the framework's TargetConverter at slot fill), `destination_path =
//     plan.variable("dest_path")`.
//  3. file.Link — symlink at the original location pointing at the moved file; `source = plan.variable("dest_path")`
//     (coerced to *file.Resource), `target_path = plan.variable("source_path")`.
//
// Variables `dest_path` and `source_path` each bind two slots whose declared types differ (`string` on one,
// `*file.Resource` on the other). Phase 6.0's convertibility-aware [op.Subgraph.mergeBubbled] honors the
// interconvertibility via [op.typesAreInterconvertible]; no collision is raised. The variable's source-side type
// (`string`, the cobra flag's natural shape) wins via [preferSourceSide].
//
// Parameters:
//   - `env`: the planning runtime environment; supplies the receiver registry for file-provider method lookup and
//     becomes the constructed graph's bound environment via [op.Graph.Rebind].
//
// Returns:
//   - *op.Graph: the assembled graph, ready for [op.GraphExecutor.Run]. Unbound by the calling [op.Plan] closure.
//   - `error`: non-nil when the file provider is not registered, any method lookup fails, or [plan.Provider.Assemble]
//     reports orphan invocations or graph-validation errors.
func BuildGraph(env *op.RuntimeEnvironment) (*op.Graph, error) {

	fileReceiverType, err := lookupFileReceiverType(env)
	if err != nil {
		return nil, err
	}

	planProvider := plan.NewProvider(env)

	destDirVariable := planProvider.Variable("dest_dir", nil)
	sourcePathVariable := planProvider.Variable("source_path", nil)
	destPathVariable := planProvider.Variable("dest_path", nil)

	mkdirInv, err := buildFileInvocation(planProvider, fileReceiverType, "Mkdir", nil, map[string]any{
		"path":  destDirVariable,
		"chmod": os.FileMode(0o755),
		"chown": "",
	})
	if err != nil {
		return nil, err
	}

	moveInv, err := buildFileInvocation(planProvider, fileReceiverType, "Move", nil, map[string]any{
		"source":           sourcePathVariable,
		"destination_path": destPathVariable,
	})
	if err != nil {
		return nil, err
	}

	linkInv, err := buildFileInvocation(planProvider, fileReceiverType, "Link", nil, map[string]any{
		"source":      destPathVariable,
		"target_path": sourcePathVariable,
	})
	if err != nil {
		return nil, err
	}

	graph, err := planProvider.Assemble([]*op.Invocation{mkdirInv, moveInv, linkInv}, nil, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("adopt.BuildGraph: assemble: %w", err)
	}

	return graph, nil
}

// lookupFileReceiverType resolves the [op.ProviderReceiverType] for [file.Provider] out of the env's registry.
//
// The file provider's announcement under `pkg/op/provider/file/gen/provider.gen.go` registers it under the value type
// (`file.Provider`); the registry's value-or-pointer fallback handles the lookup.
//
// Parameters:
//   - `env`: the runtime environment whose [op.ReceiverRegistry] is consulted.
//
// Returns:
//   - op.ProviderReceiverType: the resolved receiver type.
//   - `error`: non-nil when the file provider is not registered, or the registered entry is not a provider.
func lookupFileReceiverType(env *op.RuntimeEnvironment) (op.ProviderReceiverType, error) {

	fileType := reflect.TypeFor[file.Provider]()

	rt, ok := env.ReceiverRegistry.TypeByReflection(fileType)
	if !ok {
		rt, ok = env.ReceiverRegistry.TypeByReflection(reflect.PointerTo(fileType))
	}
	if !ok {
		return nil, fmt.Errorf("adopt.BuildGraph: file provider not registered in receiver registry")
	}

	prt, ok := rt.(op.ProviderReceiverType)
	if !ok {
		return nil, fmt.Errorf("adopt.BuildGraph: file provider registered as %T, want op.ProviderReceiverType", rt)
	}

	return prt, nil
}

// buildFileInvocation constructs and registers one invocation against a file-provider method.
//
// Mirrors the unexported `plan.Provider.invocation` path that the .star bridge takes: looks up the method, runs the
// method's planner against the supplied `args` / `slots`, wraps the resulting [op.ExecutableUnit] in a fresh
// [*op.Invocation] with an auto-generated label, and registers it in the planning provider's invocation registry so
// [plan.Provider.Assemble]'s orphan check is satisfied.
//
// Parameters:
//   - `planProvider`: the planning provider; consulted for [op.PlanInvocator.InvocationRegistry] access and used as
//     the planner's invocator argument.
//   - `fileReceiverType`: the file provider's receiver type.
//   - `methodName`: the file-provider method to invoke (e.g., "Mkdir", "Move", "Link").
//   - `args`: positional arguments converted to Go; nil when the method has no positional surface.
//   - `slots`: keyword arguments shaped as `slot-name → SlotValue` ([op.VariableValue], [op.ImmediateValue], or
//     [op.PromiseValue]). Values that are not [op.SlotValue] implementations are wrapped in [op.ImmediateValue].
//
// Returns:
//   - *op.Invocation: the constructed and registered invocation.
//   - `error`: non-nil on method-lookup failure, planner failure, or registry-side label collision.
func buildFileInvocation(
	planProvider *plan.Provider,
	fileReceiverType op.ProviderReceiverType,
	methodName string,
	args []any,
	slots map[string]any,
) (*op.Invocation, error) {

	method, ok := fileReceiverType.MethodByName(methodName)
	if !ok {
		return nil, fmt.Errorf("adopt.BuildGraph: file.%s not found in receiver type", methodName)
	}

	unit, err := method.Planner().Plan(planProvider, fileReceiverType, method, args, slots, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("adopt.BuildGraph: plan file.%s: %w", methodName, err)
	}

	label := planProvider.InvocationRegistry().AutoLabel(fileReceiverType.Name() + "." + op.CamelToSnake(methodName))
	invocation := &op.Invocation{
		Target: unit,
		Result: op.NewPromise(unit, ""),
		Label:  label,
	}

	if err := planProvider.InvocationRegistry().Register(label, invocation); err != nil {
		return nil, fmt.Errorf("adopt.BuildGraph: register file.%s: %w", methodName, err)
	}

	return invocation, nil
}
