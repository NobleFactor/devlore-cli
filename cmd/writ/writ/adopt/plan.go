// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package adopt

import (
	"fmt"
	"os"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/plan"
)

// BuildGraph constructs the mkdir → move → link three-node adopt graph.
//
// Realizes the writ-adopt unit operation through the binding model: every user-controlled input (destination
// directory, source path, destination path) enters as a `plan.variable("<name>")` reference. The executor's preflight
// resolver fills those variables from the [op.RuntimeEnvironment.Application] (cobra flags supplied by the writ adopt
// command) at execute time; slot fill at dispatch converts `string` flag values to `*file.Resource` where the method
// signature requires it, via the framework's [op.TargetConverter] cascade.
//
// Graph shape (three top-level children of the assembled graph's root):
//
//  1. file.mkdir — destination directory; `path = plan.variable("dest_dir")`, `chmod = 0o755`, `chown = ""`.
//  2. file.move — bytes from source to destination; `source = plan.variable("source_path")` (string flag value coerced
//     to *file.Resource by the framework's TargetConverter at slot fill), `destination_path = plan.variable("dest_path")`.
//  3. file.link — symlink at the original location pointing at the moved file; `source = plan.variable("dest_path")`
//     (coerced to *file.Resource), `target_path = plan.variable("source_path")`.
//
// Variables `dest_path` and `source_path` each bind two slots whose declared types differ (`string` on one,
// `*file.Resource` on the other); the convertibility-aware bubble-up honors the interconvertibility without raising a
// collision, and the variable's source-side type (`string`, the cobra flag's natural shape) wins.
//
// Parameters:
//   - `env`: the planning runtime environment; supplies the receiver registry for provider-method lookup.
//
// Returns:
//   - *op.Graph: the assembled graph, ready for execution.
//   - `error`: non-nil when a method lookup fails or the assembly reports orphan invocations or graph-validation errors.
func BuildGraph(env *op.RuntimeEnvironment) (*op.Graph, error) {

	planProvider := plan.NewProvider(env)

	destDirVariable := planProvider.Variable("dest_dir", nil)
	sourcePathVariable := planProvider.Variable("source_path", nil)
	destPathVariable := planProvider.Variable("dest_path", nil)

	mkdirInvocation, err := planProvider.Plan("file.mkdir", nil, map[string]any{
		"path":  destDirVariable,
		"chmod": os.FileMode(0o755),
		"chown": "",
	})
	if err != nil {
		return nil, fmt.Errorf("adopt.BuildGraph: plan file.mkdir: %w", err)
	}

	moveInvocation, err := planProvider.Plan("file.move", nil, map[string]any{
		"source":           sourcePathVariable,
		"destination_path": destPathVariable,
	})
	if err != nil {
		return nil, fmt.Errorf("adopt.BuildGraph: plan file.move: %w", err)
	}

	linkInvocation, err := planProvider.Plan("file.link", nil, map[string]any{
		"source":      destPathVariable,
		"target_path": sourcePathVariable,
	})
	if err != nil {
		return nil, fmt.Errorf("adopt.BuildGraph: plan file.link: %w", err)
	}

	graph, err := planProvider.Assemble(
		[]*op.Invocation{mkdirInvocation, moveInvocation, linkInvocation},
		nil, nil, nil,
		planProvider.Origin("adopt"),
	)
	if err != nil {
		return nil, fmt.Errorf("adopt.BuildGraph: assemble: %w", err)
	}

	return graph, nil
}
