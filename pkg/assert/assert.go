// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package assert provides uniform vocabulary for invariant checks.
//
// Every helper panics with an [*AssertionError] when its condition fails. The error carries the calling function's
// name, file, and line — captured via [runtime.Callers] — so callers do not have to repeat their own location in the
// message. The panic value is typed (not a bare string) so tests and top-level recover handlers can distinguish
// invariant breaches from unrelated runtime panics via [errors.As].
//
// These checks are not stripped from release builds. An invariant worth asserting is worth asserting in production.
package assert

import (
	"fmt"
	"runtime"
)

// AssertionError is the typed panic value produced by every helper in this package.
//
// Function holds the short form of the calling function (last path segment, e.g. "starlarkbridge.NewRuntime") rather
// than the fully qualified import path; File and Line point at the assert call site itself.
type AssertionError struct {
	Function string
	File     string
	Line     int
	Message  string
}

// Error returns the formatted invariant description prefixed by the calling function.
//
// Returns:
//   - string: "<Function>: <Message>".
func (e *AssertionError) Error() string {

	return e.Function + ": " + e.Message
}

// region EXPORTED FUNCTIONS

// Nil panics with an [*AssertionError] when `value` is non-nil.
//
// Constrained to pointer types so the nil check is type-safe; the compiler rejects non-nillable inputs
// (strings, ints, structs, …) at the call site rather than letting the assertion silently succeed.
//
// For interface or function nil-checks (where the value is not addressable as `*T`), use [True] with an
// explicit predicate: `assert.True("err nil", err == nil)`.
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

// NotNil panics with an [*AssertionError] when `value` is nil.
//
// Constrained to pointer types so the nil check is type-safe; the compiler rejects non-nillable inputs
// at the call site. For interface or function nil-checks, use [True] with an explicit predicate.
//
// Parameters:
//   - `name`: a short identifier of the value being checked (e.g. "Root", "cfg.Registry").
//   - `value`: the pointer to inspect.
func NotNil[T any](name string, value *T) {

	if value != nil {
		return
	}

	raise(2, name+" is required")
}

// True panics with an [*AssertionError] when the given condition is false.
//
// Use for inline invariants that are not ergonomic to express as a NotNil/Unreachable check.
//
// Parameters:
//   - `claim`: short prose describing the invariant that must hold (e.g. "boundary not empty").
//   - `cond`: the condition; failure raises with a message "<claim>".
func True(claim string, condition bool) {
	if condition {
		return
	}
	raise(2, claim)
}

// Truef panics with an [*AssertionError] whose Message is fmt.Sprintf(format, args...) when condition is false.
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

// Unreachable panics unconditionally with an [*AssertionError].
//
// Use in default branches of exhaustive switches and on "this can't happen" paths.
//
// Parameters:
//   - `reason`: short prose describing why the branch is unreachable.
func Unreachable(reason string) {

	raise(2, "unreachable: "+reason)
}

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

// NoError panics with an [*AssertionError] when `err` is non-nil.
//
// Use for downstream errors that indicate a bug — not a recoverable runtime condition. The panic
// message has the form "<context>: <err>". For sites where `context` itself needs interpolation,
// build it with [fmt.Sprintf] at the call site or use [Failf] directly.
//
// Parameters:
//   - `context`: short label identifying the operation that produced `err` (e.g. "op.Defer").
//   - `err`: the error to inspect.
func NoError(context string, err error) {

	if err == nil {
		return
	}

	raise(2, fmt.Sprintf("%s: %v", context, err))
}

// endregion

// region UNEXPORTED FUNCTIONS

// callerFrame returns the short function name, file, and line of the frame skip levels above callerFrame itself.
//
// Parameters:
//   - `skip`: number of frames to skip from the call to [runtime.Callers].
//
// Returns:
//   - `string`: the short function name (last path segment + function), or "?" if unknown.
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

// raise builds an [*AssertionError] from the caller skip frames up the stack and panics with it.
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
//	"github.com/NobleFactor/devlore-cli/pkg/op/starlarkbridge.NewRuntime" → "starlarkbridge.NewRuntime"
//	"github.com/.../file.(*Provider).Link"                                → "file.(*Provider).Link"
//	""                                                                    → "?"
//
// Parameters:
//   - `name`: the fully qualified function name from [runtime.Frame.Function].
//
// Returns:
//   - `string`: the trimmed function name, or "?" if name is empty.
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
