// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// This file is scheduled for deletion. Its contents — the starlark.Value wrapper [Var] — were superseded by
// having plan.Provider.Variable return *op.Variable directly; the generic *goReceiver marshaling path
// handles the starlark round-trip. The commit script should `git rm pkg/op/starlarkbridge/var.go` and
// `git rm pkg/op/starlarkbridge/var_test.go` to clean these up.

package starlarkbridge
