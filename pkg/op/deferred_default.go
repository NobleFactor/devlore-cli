// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"
	"text/template/parse"
)

// DeferredDefault is a [Parameter.Default] that resolves at slot-fill time against the live runtime
// environment instead of being a typed Go value parsed at announce time.
//
// [parseDefaultExpression] returns a DeferredDefault for any directive value wrapped in `{{ ... }}` outer
// braces. Slot-fill checks Parameter.Default with a type assertion; if the assertion succeeds, slot-fill
// calls Resolve and writes the returned value onto the slot via [ImmediateValue]. Plain literal defaults
// (Parameter.Default holding os.FileMode, int, string, etc.) bypass this path entirely.
//
// The siblings map carries already-filled slot values from the same dispatch — keys are parameter names,
// values are the natural Go values that landed in those slots. Sibling references in the directive
// expression — text/template `.fieldname` syntax — read from this map at evaluation time.
type DeferredDefault interface {

	// Resolve evaluates the deferred default and returns a Go value at target's exact type.
	//
	// Parameters:
	//   - env:      live runtime environment; passed to every registered DefaultFunc.
	//   - siblings: already-filled slot values from the same dispatch, keyed by parameter name.
	//   - target:   the parameter's reflect.Type — Resolve widens its result to this type via [Convert].
	//
	// Returns:
	//   - any:   the resolved value, dynamic type matches target exactly.
	//   - error: non-nil if any function call errors, an identifier doesn't resolve in the funcmap, or a
	//     sibling-slot reference points at an unfilled slot.
	Resolve(env *RuntimeEnvironment, siblings map[string]any, target reflect.Type) (any, error)
}

// treeDefault is the canonical [DeferredDefault] implementation produced by [parseDeferred].
//
// Holds only the parsed text/template AST. The funcmap is the package-level [announcements.defaultFuncs]
// — defaults belong to the provider/resource definition (process-singleton authored by the package
// developer), not to any per-runtime state, so the AST evaluator looks up function names through
// [announcements.defaultFunc] directly at slot-fill.
type treeDefault struct {
	tree *parse.Tree
}

// region Interface guards

var _ DeferredDefault = (*treeDefault)(nil)

// endregion

// region EXPORTED METHODS

// region Behaviors

// Fallible actions

// Resolve walks the parsed AST against the supplied environment and siblings, then widens the result to
// target via [Convert].
//
// Parameters:
//   - env:      live runtime environment.
//   - siblings: parameter-name keyed map of already-filled slot values.
//   - target:   parameter's reflect.Type — final widening target.
//
// Returns:
//   - any:   resolved value at target's exact type.
//   - error: non-nil on eval failure or Convert failure.
func (d *treeDefault) Resolve(env *RuntimeEnvironment, siblings map[string]any, target reflect.Type) (any, error) {

	raw, err := evalTree(d.tree, env, siblings)
	if err != nil {
		return nil, err
	}

	return Convert(env, raw, target)
}

// endregion

// endregion

// region HELPER FUNCTIONS

// region Behaviors

// parseDeferred parses a deferred-default expression into a [treeDefault].
//
// The text argument must include the outer `{{` and `}}` braces. The body is passed through
// [text/template/parse.Parse] with the validator stub from [announcements.validatorStub] so that unknown
// function identifiers are rejected at announce time rather than at slot-fill.
//
// Parameters:
//   - text: the full directive value including outer braces (e.g., `{{ umask 0o666 }}`).
//
// Returns:
//   - *treeDefault: the wrapped parsed tree, ready to embed in [Parameter.Default].
//   - error:        non-nil if the expression doesn't parse or names an unknown function.
func parseDeferred(text string) (*treeDefault, error) {

	trees, err := parse.Parse("deferred", text, "{{", "}}", announced.validatorStub())
	if err != nil {
		return nil, err
	}

	tree, ok := trees["deferred"]
	if !ok {
		return nil, fmt.Errorf("internal: parser produced no tree for %q", text)
	}

	return &treeDefault{tree: tree}, nil
}

// endregion

// endregion
