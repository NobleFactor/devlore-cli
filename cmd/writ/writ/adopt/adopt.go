// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package adopt plans the `writ adopt` batch graph.
//
// The cobra layer (`cmd/writ/writ/adopt_cmd.go`) enumerates the inputs — the locations of the files to adopt — into
// per-scope [Item] batches; [BuildGraph] turns one batch into one execution graph: a deduplicated mkdir pre-stage
// plus a `flow.gather` over the item records whose body guards and performs each adoption via field projections
// (the writ-adopt design, docs/plans/extract-starlark-from-op/phase-8/writ-adopt-command.md; phase-8 step 33
// slice A on the step-45 projection surface).
package adopt
