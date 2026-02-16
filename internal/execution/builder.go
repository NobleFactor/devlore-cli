// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"context"
)

// GraphBuilder is the interface for building execution graphs.
// Implementations are provided by tools (writ, lore) and create graphs
// from their respective inputs (file trees, package manifests, etc.).
type GraphBuilder interface {
	// Build creates an execution graph.
	// Implementations hold their configuration internally (set at construction).
	Build(ctx context.Context) (*Graph, error)
}
