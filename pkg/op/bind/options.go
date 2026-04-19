// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bind

import (
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Options carries the cross-cutting, plan-time-settable concerns that apply uniformly to every [Invocation].
//
// Zero values mean "use the default": an empty Label triggers auto-labeling via [InvocationRegistry.AutoLabel]; a nil
// RetryPolicy means no retry. Options is constructed by the plan provider's Options method (exposed to starlark as
// plan.options(...)) and passed into a plan-mode dispatch via the reserved `options` kwarg. It is a pure Go data
// struct — flow through starlark is handled by the generic receiver marshaling path.
type Options struct {
	Label       string
	RetryPolicy *op.RetryPolicy
}
