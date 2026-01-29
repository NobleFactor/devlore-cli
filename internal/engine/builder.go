// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package engine

import "context"

// GraphBuilder builds an execution graph from a manifest file.
// Tools implement this interface to translate their manifest format
// into an execution graph that the engine can process.
//
// When writ encounters a delegate node, it calls BuildGraph with the
// manifest path to get a subgraph. That subgraph is then executed by
// the same engine — no separate tool invocation needed.
type GraphBuilder interface {
	BuildGraph(ctx context.Context, manifestPath string, opts BuildOptions) (*Graph, error)
}

// BuildOptions configures graph building behavior.
type BuildOptions struct {
	// DryRun prevents the builder from making any filesystem queries
	// that have side effects.
	DryRun bool

	// Features lists enabled features (e.g., "rootless", "compose").
	Features []string

	// Data holds tool context: platform info, environment, segments.
	Data map[string]any
}
