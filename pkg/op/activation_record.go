// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import "context"

// ActivationRecord is the per-invocation data record threaded through every [Action.Do] call (and every
// [CompensableAction.Undo] call) as the framework-injected first argument to provider methods.
//
// The framework constructs one [ActivationRecord] per dispatch and passes it to the provider method as
// the first parameter. Provider methods access shared session state via [ActivationRecord.Runtime],
// per-call state directly off the record (`SiteID`, `Context`, future per-call fields), and stdlib
// `context.Context` for cancellation-aware operations via [ActivationRecord.Context].
//
// Each goroutine-driven dispatch holds its own [ActivationRecord]; pointer fields on `Runtime` (Catalog,
// Status, RecoverySite, Registry, etc.) share underlying instances with their own internal synchronization.
// Concurrent dispatches cannot race on per-call fields because they hold different records.
//
// Lifecycle: created by the executor before dispatch; consumed during the dispatch; discarded afterward.
// No persistent identity, no registry — each record is unique to one invocation.
type ActivationRecord struct {

	// Runtime is the session-scope execution context. Shared across every concurrent dispatch in the same
	// session; never mutated mid-execution.
	Runtime *RuntimeEnvironment

	// SiteID identifies the dispatch site this activation came from — generalized "where in the system did
	// this dispatch originate?" The granularity is dispatcher-dependent:
	//
	//   - Graph dispatch: per-call-site (1:1 with a node, 1:1 with a source-level call expression).
	//     Example: "file.write_text-7f3a2b" minted by [GenerateNodeID].
	//   - Starlark immediate-mode bridge: per-action (one identifier per dispatched method, regardless of
	//     how many lines invoke it). Example: "starlark:file.write_text".
	//   - Test fixtures: per-test (one identifier per test function). Example: "test:TestWriteText".
	//   - CLI-side runners (writ, devlore-test): per-command (one identifier per command invocation).
	//     Example: "writ:adopt".
	//
	// Distinct from [op.RecoverySite], which is the on-disk archive directory for compensation —
	// "site" here means "site of dispatch origin". The [ResourceCatalog] reads this as the producer
	// stamp on Resources interned during the dispatch (`producerID = activation.SiteID`) so downstream
	// consumers can derive producer→consumer edges via [ExtractResource]. Empty outside dispatch
	// (planning, rehydration, discovery) — the strict GetOrCreate path asserts non-empty.
	SiteID string

	// Context is the cancellation-aware context for this dispatch, derived from `Runtime.Context`. Provider
	// methods pass this to stdlib functions that take `context.Context` (e.g., `exec.CommandContext`,
	// `http.NewRequestWithContext`) so cancellation propagates from the session root down through the
	// dispatch.
	Context context.Context
}