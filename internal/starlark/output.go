// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import "github.com/NobleFactor/devlore-cli/pkg/projection"

// FillSlot delegates to projection.FillSlot.
// The code generator (star devlore actions generate) emits unqualified
// FillSlot calls in plan_*_gen.go files via the planFillSlots template
// function. This alias bridges the generator output to the canonical
// implementation in pkg/projection.
var FillSlot = projection.FillSlot
