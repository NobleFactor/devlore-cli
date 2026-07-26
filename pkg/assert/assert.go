// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package assert provides uniform vocabulary for invariant checks.
//
// Every helper panics with an [*AssertionError] when its condition fails. The error carries the calling function's
// name, file, and line — captured via [runtime.Callers] — so callers do not have to repeat their own location in the
// message. The panic value is typed, so tests and top-level recover handlers can distinguish invariant breaches from
// unrelated runtime panics via [errors.As].
//
// These checks are not stripped from release builds. An invariant worth asserting is worth asserting in production.
package assert

import (
	"fmt"
	"runtime"
)

// AssertionError is the typed panic value produced by every helper in this package.
//
// Function holds the short form of the calling function (last path segment, e.g. "fsroot.OpenConfined") rather
// than the fully qualified import path; File and Line point at the assert function's call site.
type AssertionError struct {
	Function string
	File     string
	Line     int
	Message  string
}

// Error returns the formatted invariant description prefixed by the calling function and suffixed by the call site.
//
// Returns:
//   - `string`: "<Function>: <Message> (<File>:<Line>)".
func (e *AssertionError) Error() string {

	return fmt.Sprintf("%s: %s (%s:%d)", e.Function, e.Message, e.File, e.Line)
}

// region EXPORTED FUNCTIONS

// Failf panics with an [*AssertionError] whose Message is fmt.Sprintf(format, args...).
//
// Use when the message needs interpolation (type names, indices, registry keys, …).
//
// Parameters:
//   - `format`: a [fmt.Sprintf] format string.
//   - `args`: the format arguments.
func Failf(format string, args ...any) {
	raise(2, fmt.Sprintf(format, args...))
}

// Must returns `value` unless `err` is non-nil, in which case it panics with an [*AssertionError].
//
// Use to unwrap a (value, error) call whose failure indicates a bug — not a recoverable runtime
// condition: `silent := assert.Must(cmd.Flags().GetBool("silent"))`. Go forbids mixing a context
// argument with a multi-value call, so Must carries no label; the [*AssertionError]'s captured call
// site supplies the location.
//
// Parameters:
//   - `value`: the value to return when `err` is nil.
//   - `err`: the error to inspect.
//
// Returns:
//   - `T`: `value`, unchanged.
func Must[T any](value T, err error) T {

	if err != nil {
		raise(2, fmt.Sprintf("must: %v", err))
	}

	return value
}

// Nil panics with an [*AssertionError] when `value` is non-nil.
//
// Constrained to pointer types, so the nil check is type-safe; the compiler rejects non-nillable inputs (strings, ints,
// structs, …) at the call site rather than letting the assertion silently succeed.
//
// For interface and function nil-checks (where the value is not addressable as `*T`), use [True] with an explicit
// predicate: `assert.True("err nil", err == nil)`, or
//
// Parameters:
//   - `name`: a short identifier of the value being checked (e.g. "cache entry").
//   - `value`: the pointer to inspect.
func Nil[T any](name string, value *T) {
	if value == nil {
		return
	}
	raise(2, fmt.Sprintf("%s: expected nil, got %v", name, value))
}

// NoError panics with an [*AssertionError] when `err` is non-nil.
//
// Use for downstream errors that indicate a bug — not a recoverable runtime condition. The panic
// message has the form "<context>: <err>". For sites where `context` itself needs interpolation,
// build it with [fmt.Sprintf] at the call site or use [Failf] directly.
//
// Parameters:
//   - `context`: a short label identifying the operation that produced `err` (e.g. "iox.Close").
//   - `err`: the error to inspect.
func NoError(context string, err error) {
	if err == nil {
		return
	}
	raise(2, fmt.Sprintf("%s: %v", context, err))
}

// NonEmpty panics with an [*AssertionError] when `value` is empty.
//
// Constrained to collection types (slices, maps, strings), so the check is type-safe;
// the compiler rejects non-indexable inputs at the call site.
//
// Parameters:
//   - `name`: a short identifier of the value being checked (e.g. "items", "cfg.Headers").
//   - `value`: the collection or string to inspect.
//
//goland:noinspection GoUnusedExportedFunction
func NonEmpty[T ~[]E | ~map[K]V | ~string, E any, K comparable, V any](name string, value T) T {
	if len(value) != 0 {
		return value
	}
	raise(2, "non-empty value for "+name+" is required")
	return value
}

// NonZero panics with an [*AssertionError] when `value` is nil.
//
// Constrained to comparable types, so the check for non-zero is type-safe; the compiler rejects non-comparable inputs
// at the call site. For function pointers--which are non-comparable--you must:
//
//	var functionPointer
//	assert.NonZero(*(*uintptr)(unsafe.Pointer(&functionPointer))))
//
// Parameters:
//   - `name`: a short identifier of the value being checked (e.g. "Root", "cfg.Registry").
//   - `value`: the value to inspect.
func NonZero[T comparable](name string, value T) T {

	var zero T

	if value == zero {
		raise(2, "non-zero value for "+name+" is required")
	}

	return value
}

// True panics with an [*AssertionError] when the given condition is false.
//
// Use for inline invariants that are not ergonomic to express as a NonZero/Unreachable check.
//
// Parameters:
//   - `claim`: short prose describing the invariant that must hold (e.g. "boundary is not empty").
//   - `cond`: the condition; failure raises with a message "<claim>".
func True(claim string, condition bool) {
	if condition {
		return
	}
	raise(2, claim)
}

// Truef panics with an [*AssertionError] whose Message is fmt.Sprintf(format, args...) when the condition is false.
//
// Use for inline invariants whose failure message needs interpolation (type names, indices, registry keys, …).
//
// Parameters:
//   - `format`: a [fmt.Sprintf] format string describing the invariant.
//   - `condition`: the condition; failure raises with the formatted message.
//   - `args`: the format arguments.
func Truef(condition bool, format string, args ...any) {
	if condition {
		return
	}
	raise(2, fmt.Sprintf(format, args...))
}

// Type returns `value` as type `T`, panicking with an [*AssertionError] when the dynamic type differs.
//
// Use where a value's type is guaranteed by construction (a just-parsed document field, a registry
// invariant) and a mismatch is a bug: `id := assert.Type[string]("unit id", b.value)`. The labeled
// panic replaces the unlabeled runtime panic of a bare single-value type assertion.
//
// Parameters:
//   - `name`: a short identifier of the value being checked (e.g. "unit id").
//   - `value`: the value whose dynamic type must be `T`.
//
// Returns:
//   - `T`: `value` as `T`.
func Type[T any](name string, value any) T {

	typed, ok := value.(T)
	if !ok {
		raise(2, fmt.Sprintf("%s: expected %T, got %T", name, typed, value))
	}

	return typed
}

// Unimplemented panics unconditionally with an [*AssertionError].
//
// Use in a method that satisfies an interface but is intentionally not yet implemented — a loud stub that fails fast if
// reached, rather than a silent no-op or a soft error return.
//
// Parameters:
//   - `what`: short prose naming the unimplemented operation (e.g. "git.Resource.Exists").
func Unimplemented(what string) {

	raise(2, "unimplemented: "+what)
}

// Unreachable panics unconditionally with an [*AssertionError].
//
// Use in default branches of exhaustive switches and on "this can't happen" paths.
//
// Parameters:
//   - `reason`: short prose describing why the branch is unreachable.
func Unreachable(reason string) {

	raise(2, "unreachable: "+reason)
}

// Unreachablef panics unconditionally with an [*AssertionError].
//
// Use in default branches of exhaustive switches and on "this can't happen" paths.
//
// Parameters:
//   - `reason`: short prose describing why the branch is unreachable.
func Unreachablef(format string, args ...any) {

	raise(2, "unreachable: "+fmt.Sprintf(format, args...))
}

// endregion

// region HELPER FUNCTIONS

// callerFrame returns the short function name, file, and line of the frame skip levels above callerFrame itself.
//
// Parameters:
//   - `skip`: number of frames to skip from the call to [runtime.Callers].
//
// Returns:
//   - `string`: the short function name (last path segment plus function), or "?" if unknown.
//   - `string`: the source file, or "?" if unknown.
//   - `int`: the line number, or 0 if unknown.
func callerFrame(skip int) (string, string, int) {

	var pcs [1]uintptr

	if runtime.Callers(skip+1, pcs[:]) < 1 {
		return "?", "?", 0
	}

	frame, _ := runtime.CallersFrames(pcs[:]).Next()

	return shortFunc(frame.Function), frame.File, frame.Line
}

// raise builds an [*AssertionError] from the caller's skip frames up the stack and panics with it.
//
// Parameters:
//   - `skip`: number of frames between raise and the user's call site (2 for the public helpers).
//   - `message`: the formatted invariant description.
func raise(skip int, message string) {

	fn, file, line := callerFrame(skip + 1)

	panic(&AssertionError{
		Function: fn,
		File:     file,
		Line:     line,
		Message:  message,
	})
}

// shortFunc trims a fully qualified function name to its last path segment so messages stay readable.
//
// Examples:
//
//	"github.com/NobleFactor/devlore-cli/pkg/fsroot.OpenConfined" → "fsroot.OpenConfined"
//	"github.com/.../file.(*Provider).Link"                       → "file.(*Provider).Link"
//	""                                                           → "?"
//
// Parameters:
//   - `name`: the fully qualified function name from [runtime.Frame.Function].
//
// Returns:
//   - `string`: the trimmed function name, or "?" if `name` is empty.
func shortFunc(name string) string {

	if name == "" {
		return "?"
	}

	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '/' {
			return name[i+1:]
		}
	}

	return name
}

// endregion
