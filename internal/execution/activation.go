// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"github.com/NobleFactor/devlore-cli/pkg/projection"
)

// ActivationState captures per-execution mutable state for a node.
//
// For non-gather execution, activation state is inlined on the Node struct
// (Status). For gather's concurrent execution, ActivationState lives in a
// per-iteration map so that shared nodes are never mutated.
//
// ActivationState is transient — it is discarded after results and undo state
// are captured.
type ActivationState struct {
	Status    projection.NodeStatus
	Timestamp string
	Error     string
}
