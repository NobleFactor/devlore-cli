// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"reflect"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// DefaultFunc is the signature every entry in the deferred-default registry conforms to.
//
// Each `{{ funcname arg1 arg2 ... }}` command in a directive expression resolves at slot-fill time to exactly one
// DefaultFunc call. The evaluator gathers the command's evaluated arguments into args, looks up funcname in
// `env.DefaultFuncs` (a snapshot of the package-level announcements), and invokes the matched DefaultFunc.
//
// Parameters:
//   - env:      live runtime environment from the dispatching call. Always non-nil at slot-fill; registered
//     functions may rely on env.Status, env.Root, env.Catalog, etc. without a nil check.
//   - siblings: already-filled slot values from the same dispatch, keyed by parameter name. Nil-safe lookups
//     via the standard `v, ok := siblings[name]` form. Functions that don't consult siblings (umask, chmod,
//     env) ignore this argument.
//   - args:     argument values produced by recursively evaluating each child node of the CommandNode in
//     announce-declared order. Functions validate arity and per-argument Kind before extracting concrete
//     values. Empty slice for a zero-arg call (e.g., `{{ umask }}`).
//
// Returns:
//   - reflect.Value: the function's natural Go result. The evaluator carries this up the pipeline; the last
//     command's reflect.Value is what treeDefault.Resolve hands to op.Convert.
//   - error: non-nil on argument-arity mismatch, argument-type mismatch, or function-internal failure.
type DefaultFunc func(env *RuntimeEnvironment, siblings map[string]any, args []reflect.Value) (reflect.Value, error)

// region EXPORTED FUNCTIONS

// region Behaviors

// RegisterDefaultFunc adds a function to the package-level deferred-default registry under the given name.
//
// Intended caller is package init; the registry is conceptually static for the process lifetime. Re-registration
// of the same name panics — registration is a one-time act, and accidental duplicate registration almost always
// indicates an init-order bug or a copy-paste error.
//
// Parameters:
//   - name: the identifier as it appears in directive expressions (`{{ name args }}`). Conventionally lowercase
//     ASCII; the parser is case-sensitive.
//   - fn:   the function to invoke at slot-fill time. Must be non-nil.
//
// Panics:
//   - If name is empty.
//   - If fn is nil.
//   - If name is already registered.
func RegisterDefaultFunc(name string, fn DefaultFunc) {

	err := announced.registerDefaultFunc(name, fn)
	assert.NoError("op.RegisterDefaultFunc", err)
}

// endregion

// endregion