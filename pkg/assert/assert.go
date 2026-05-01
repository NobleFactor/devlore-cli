// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package assert provides a tiny, uniform vocabulary for invariant checks.
//
// Every helper panics with an [*AssertionError] when its condition fails. The error carries the calling function's name,
// file, and line — captured via [runtime.Callers] — so callers do not have to repeat their own location in the message.
// The panic value is typed (not a bare string) so tests and top-level recover handlers can distinguish invariant
// breaches from unrelated runtime panics via [errors.As].
//
// These checks are not stripped from release builds. An invariant worth asserting is worth asserting in production.
package assert

import (
	"fmt"
	"reflect"
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

// NotNil panics with an [*AssertionError] when v is a nil interface or a typed nil pointer / map / slice / channel /
// function. Use for constructor preconditions where a required collaborator must be supplied.
//
// Parameters:
//   - name: short identifier of the value being checked (e.g. "Root", "cfg.Registry").
//   - v:    the value to inspect; nil interfaces and typed-nil reference values both fail.
func NotNil(name string, v any) {

	if !isNil(v) {
		return
	}

	raise(2, name+" is required")
}

// True panics with an [*AssertionError] when cond is false. Use for inline invariants that are not ergonomic to express
// as a NotNil/Unreachable check.
//
// Parameters:
//   - claim: short prose describing the invariant that must hold (e.g. "boundary not empty").
//   - cond:  the condition; failure raises with message "<claim>".
func True(claim string, cond bool) {

	if cond {
		return
	}

	raise(2, claim)
}

// Unreachable panics unconditionally with an [*AssertionError]. Use in default branches of exhaustive switches and on
// "this can't happen" paths.
//
// Parameters:
//   - reason: short prose describing why the branch is unreachable.
func Unreachable(reason string) {

	raise(2, "unreachable: "+reason)
}

// Failf panics with an [*AssertionError] whose Message is fmt.Sprintf(format, args...). Use when the message needs
// interpolation (type names, indices, registry keys, …).
//
// Parameters:
//   - format: a [fmt.Sprintf] format string.
//   - args:   the format arguments.
func Failf(format string, args ...any) {

	raise(2, fmt.Sprintf(format, args...))
}

// endregion

// region UNEXPORTED FUNCTIONS

// raise builds an [*AssertionError] from the caller skip-frames up the stack and panics with it.
//
// Parameters:
//   - skip:    number of frames between raise and the user's call site (2 for the public helpers).
//   - message: the formatted invariant description.
func raise(skip int, message string) {

	fn, file, line := callerFrame(skip + 1)

	panic(&AssertionError{
		Function: fn,
		File:     file,
		Line:     line,
		Message:  message,
	})
}

// callerFrame returns the short function name, file, and line of the frame skip levels above callerFrame itself.
//
// Parameters:
//   - skip: number of frames to skip from the call to [runtime.Callers].
//
// Returns:
//   - string: the short function name (last path segment + function), or "?" if unknown.
//   - string: the source file, or "?" if unknown.
//   - int:    the line number, or 0 if unknown.
func callerFrame(skip int) (string, string, int) {

	var pcs [1]uintptr

	if runtime.Callers(skip+1, pcs[:]) < 1 {
		return "?", "?", 0
	}

	frame, _ := runtime.CallersFrames(pcs[:]).Next()

	return shortFunc(frame.Function), frame.File, frame.Line
}

// shortFunc trims a fully-qualified function name to its last path segment so messages stay readable.
//
// Examples:
//
//	"github.com/NobleFactor/devlore-cli/pkg/op/starlarkbridge.NewRuntime" → "starlarkbridge.NewRuntime"
//	"github.com/.../file.(*Provider).Link"                                → "file.(*Provider).Link"
//	""                                                                    → "?"
//
// Parameters:
//   - name: the fully qualified function name from [runtime.Frame.Function].
//
// Returns:
//   - string: the trimmed function name, or "?" if name is empty.
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

// isNil reports whether v is a nil interface or a typed-nil reference value.
//
// A plain `v == nil` only catches nil interfaces; pointers, maps, slices, channels, and functions held inside an
// interface compare unequal to nil even when their underlying value is zero. This helper handles both shapes — falling
// back to [reflect.Value.IsNil] on the kinds where typed-nil is meaningful. NotNil runs only on the panic-imminent path,
// so the reflect cost is irrelevant.
//
// Parameters:
//   - v: the value to inspect.
//
// Returns:
//   - bool: true if v is nil in any of the senses above.
func isNil(v any) bool {

	if v == nil {
		return true
	}

	rv := reflect.ValueOf(v)

	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		return rv.IsNil()
	}

	return false
}

// endregion