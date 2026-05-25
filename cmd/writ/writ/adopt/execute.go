// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package adopt

// Phase 6.C target: `Run(ctx context.Context, executor *op.GraphExecutor, graph *op.Graph) error` wraps the
// executor with adopt-specific error mapping so the existing CLI prefix style (`creating directory %s: %w`,
// `moving file: %w`, `creating symlink: %w`) is preserved when [op.Convert] envelope errors surface. Stub
// file lands in Phase 6.B for layout continuity.
