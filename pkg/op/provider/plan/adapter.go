// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package plan

import (
	"fmt"
	"sort"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/starlarkbridge"
)

var (
	_ starlark.Value    = (*adapter)(nil) // Interface Guard: ensures *adapter implements starlark.Value.
	_ starlark.HasAttrs = (*adapter)(nil) // Interface Guard: ensures *adapter implements starlark.HasAttrs.
)

// adapter implements [starlark.HasAttrs] for one plan sub-namespace (`plan.file`, `plan.shell`, ...).
//
// One adapter per [op.ProviderReceiverType], minted lazily by [Provider.ResolveAttr] and cached on the Provider.
// Resolution flow for `plan.<X>.<method>(arg, kw=value)`:
//
//  1. starlark resolves `plan` → goReceiver wrapping plan.Provider.
//  2. `.X` → plan.Provider.ResolveAttr("X") returns the adapter for the X sub-namespace.
//  3. `.method` → adapter.Attr("method") returns a [*starlark.Builtin].
//  4. Builtin(args, kwargs) → splits reserved kwargs via [splitReservedKwargs], converts args / kwargs to Go via
//     [starlarkbridge.StarlarkToGoTyped], and calls [Provider.invocation].
//  5. The resulting [*op.Invocation] is wrapped via [starlarkbridge.NewGoReceiver] for return to starlark, giving
//     authors a uniform receiver surface.
//
// The adapter holds no business logic. Every dispatch decision lives in [Provider.invocation] and the method's
// declared [op.Planner]; the adapter exists solely to give starlark a HasAttrs-shaped handle on each sub-namespace.
type adapter struct {
	provider     *Provider
	receiverType op.ProviderReceiverType
}

// newAdapter constructs an adapter bound to the given Provider and receiver type.
//
// Parameters:
//   - `provider`: the plan.Provider that owns this adapter (used to reach the runtime environment for arg
//     conversion and the dispatch method [Provider.invocation]).
//   - `receiverType`: the sub-namespace's provider receiver type whose methods this adapter routes for.
//
// Returns:
//   - *adapter: the constructed adapter; immutable after construction (no mutex required for concurrent reads).
func newAdapter(provider *Provider, receiverType op.ProviderReceiverType) *adapter {
	return &adapter{
		provider:     provider,
		receiverType: receiverType,
	}
}

// region EXPORTED METHODS

// region State management

// Freeze implements [starlark.Value].
//
// No-op — adapters carry no mutable state observable from starlark.
func (a *adapter) Freeze() {}

// Hash implements [starlark.Value]. Adapters are not hashable.
//
// Returns:
//   - `uint32`: always zero.
//   - `error`: always non-nil with message "unhashable type: plan.adapter".
func (a *adapter) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: %s", a.Type())
}

// String implements [starlark.Value].
//
// Returns:
//   - `string`: the qualified namespace label, e.g., `"plan.file"`.
func (a *adapter) String() string { return "plan." + a.receiverType.Name() }

// Truth implements [starlark.Value]. Always true.
//
// Returns:
//   - starlark.Bool: true.
func (a *adapter) Truth() starlark.Bool { return true }

// Type implements [starlark.Value].
//
// Returns:
//   - `string`: the constant `"plan.adapter"`.
func (a *adapter) Type() string { return "plan.adapter" }

// endregion

// region Behaviors

// Fallible actions

// Attr implements [starlark.HasAttrs] by resolving `name` (snake_case) against the receiverType's methods (CamelCase)
// via [op.CamelToSnake].
//
// On match, returns a [*starlark.Builtin] whose body routes the call through [Provider.invocation] (see
// [dispatchBuiltinBody]). On miss, returns a [starlarkbridge.NoSuchAttrError].
//
// Parameters:
//   - `name`: the snake-cased method name supplied by starlark.
//
// Returns:
//   - starlark.Value: the bound builtin, never nil on success.
//   - `error`: non-nil when no method matches.
func (a *adapter) Attr(name string) (starlark.Value, error) {

	for method := range a.receiverType.Methods() {
		if op.CamelToSnake(method.Name()) != name {
			continue
		}
		actionName := a.receiverType.Name() + "." + name
		body := dispatchBuiltinBody(a.provider, a.receiverType, method.Name(), actionName)
		return starlark.NewBuiltin(actionName, body), nil
	}

	return nil, starlarkbridge.NoSuchAttrError(a.Type(), name)
}

// Actions

// AttrNames implements [starlark.HasAttrs].
//
// Returns:
//   - []string: the sorted set of snake-cased method names exposed by the receiver type.
func (a *adapter) AttrNames() []string {

	var names []string
	for method := range a.receiverType.Methods() {
		names = append(names, op.CamelToSnake(method.Name()))
	}
	sort.Strings(names)
	return names
}

// endregion

// endregion
