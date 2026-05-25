// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package adopt

// Phase 6.C target: `BuildGraph(env *op.RuntimeEnvironment, cfg AdoptConfig) (*op.Graph, error)` constructs
// the mkdir → move → link three-node graph using `plan.Provider.Variable("dest_dir")`,
// `plan.variable("source_path")`, `plan.variable("dest_path")` slot references for every user-controlled
// input. Stub file lands in Phase 6.B for layout continuity.
