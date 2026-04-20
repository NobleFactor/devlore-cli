// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bind

import (
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Invocation is the handle a plan-mode dispatch constructs for every `plan.*` call.
//
// It carries both representations a binding site may need: Target is the op-level unit the invocation will dispatch
// (an [op.Node] or an [op.Subgraph]); Result is the Promise to the invocation's output. [NodeBuilder.fillSlot] chooses
// which field to use based on the target parameter's type at the binding site — slots typed as [op.ExecutableUnit]
// consume Target; value-typed slots consume Result and create an edge. Starlark callers don't distinguish; the
// binding layer handles the dispatch.
type Invocation struct {
	Target op.ExecutableUnit
	Result *Promise
}
