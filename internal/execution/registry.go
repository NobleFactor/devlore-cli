// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import "github.com/NobleFactor/devlore-cli/pkg/op"

// ActionRegistry is the registry for looking up actions by name, re-exported from pkg/op.
type ActionRegistry = op.ActionRegistry

// NewActionRegistry creates a new empty ActionRegistry, re-exported from pkg/op.
var NewActionRegistry = op.NewActionRegistry
