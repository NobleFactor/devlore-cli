// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import "context"

// Plan runs a planning session bounded by spec and fn. The session-shape is:
//
//  1. Build a planning [RuntimeEnvironment] from spec.
//  2. Construct a fresh [Graph] via [NewGraph].
//  3. Bind the env to the graph via [Graph.Rebind].
//  4. Call fn with the graph; the caller does the work of populating it (loading a starlark script,
//     adding nodes, populating the catalog, etc.).
//  5. Unbind the graph from the planning env.
//  6. Close the planning env.
//
// Steps 5 and 6 fire via defer, so a panic inside fn still leaves the graph unbound and the env closed.
//
// The returned graph leaves the planning session unbound — its `ctx` field is nil. The next session-owner
// (typically a [GraphExecutor]) Rebinds during its own Run.
//
// Parameters:
//   - `ctx`: the parent context whose cancellation / values flow into the planning env.
//   - `spec`: the planning-environment configuration.
//   - `fn`: the caller-supplied planning routine; receives the freshly-bound graph.
//
// Returns:
//   - *Graph: the planned graph, unbound from the planning env.
//   - `error`: non-nil if fn returned an error or the planning env's [RuntimeEnvironment.Close] failed.
func Plan(ctx context.Context, spec *RuntimeEnvironmentSpec, fn func(*Graph) error) (*Graph, error) {

	env := NewRuntimeEnvironment(ctx, spec)
	defer func() { _ = env.Close() }()

	graph := NewGraph()
	graph.Rebind(env)
	defer graph.Unbind()

	if err := fn(graph); err != nil {
		return nil, err
	}

	return graph, nil
}
