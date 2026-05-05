// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"os"
	"reflect"
	"syscall"
)

// fileModeType is the [reflect.Type] of [os.FileMode], cached so per-arg type-equality checks compare
// pointers rather than reconstructing the type on every dispatch.
var fileModeType = reflect.TypeFor[os.FileMode]()

func init() {
	RegisterDefaultFunc("umask", defaultUmask)
	RegisterDefaultFunc("mode", defaultMode)
	RegisterDefaultFunc("env", defaultEnv)
}

// region UNEXPORTED FUNCTIONS

// region Behaviors

// Fallible actions

// defaultUmask implements `{{ umask base }}` — masks the literal base mode by the process umask.
//
// Mirrors the semantic Linux's cp and mkdir use when no explicit mode is supplied: the file mode is
// `base &^ umask`. The umask is read via a [syscall.Umask] round-trip (Get-and-restore), so the
// process's actual umask is unchanged after the call.
//
// Parameters:
//   - env:  unused.
//   - _:    sibling-slot map; unused (umask has no sibling references).
//   - args: exactly one argument — the base mode as int, uint, or os.FileMode.
//
// Returns:
//   - reflect.Value: the resolved mode as os.FileMode.
//   - error:         non-nil on argument-arity mismatch or argument-type mismatch.
func defaultUmask(_ *RuntimeEnvironment, _ map[string]any, args []reflect.Value) (reflect.Value, error) {

	if len(args) != 1 {
		return reflect.Value{}, fmt.Errorf("umask: expected 1 argument, got %d", len(args))
	}

	base, err := argFileMode("umask", args[0])
	if err != nil {
		return reflect.Value{}, err
	}

	mask := syscall.Umask(0)
	syscall.Umask(mask)

	return reflect.ValueOf(base &^ os.FileMode(mask)), nil
}

// defaultMode implements `{{ mode symbolic }}` — parses a 9-character POSIX permission string into
// os.FileMode.
//
// The accepted form is exactly the 9-char `rwxrwxrwx` template with the dash character marking unset
// bits, e.g., `"rwxr-x---"` (0o750). Position 0..2 is owner, 3..5 is group, 6..8 is other. Each
// position must be the expected letter at that index (`r` / `w` / `x`) or a `-`. The full POSIX
// symbolic-mode dialect (`u+x,g-w,o=r`) is not supported — mode errors with a future-pointing
// message if the input doesn't match the 9-char template.
//
// Parameters:
//   - env:  unused.
//   - _:    sibling-slot map; unused.
//   - args: exactly one argument — the symbolic mode string.
//
// Returns:
//   - reflect.Value: the parsed permission bits as os.FileMode.
//   - error:         non-nil on argument errors or malformed input.
func defaultMode(_ *RuntimeEnvironment, _ map[string]any, args []reflect.Value) (reflect.Value, error) {

	if len(args) != 1 {
		return reflect.Value{}, fmt.Errorf("mode: expected 1 argument, got %d", len(args))
	}

	if args[0].Kind() != reflect.String {
		return reflect.Value{}, fmt.Errorf("mode: expected symbolic mode string, got %s", args[0].Kind())
	}

	parsed, err := parseSymbolicMode9(args[0].String())
	if err != nil {
		return reflect.Value{}, fmt.Errorf("mode: %w", err)
	}

	return reflect.ValueOf(parsed), nil
}

// defaultEnv implements `{{ env key }}` — returns the value of the named environment variable.
//
// A missing variable resolves to the empty string (Go's [os.Getenv] semantics), not an error. Tests
// that depend on a specific value should use `t.Setenv` to set it, and directive authors that need a
// non-empty fallback should use a downstream pipe stage (e.g., `{{ env "MODE" | or "0o644" }}`)
// once such a stage is registered. v1 ships only the bare lookup.
//
// Parameters:
//   - env:  unused (the function reads from os.Environ, not from the runtime environment).
//   - _:    sibling-slot map; unused.
//   - args: exactly one argument — the environment-variable name as a string.
//
// Returns:
//   - reflect.Value: the looked-up value as a string (empty string if unset).
//   - error:         non-nil on argument errors only.
func defaultEnv(_ *RuntimeEnvironment, _ map[string]any, args []reflect.Value) (reflect.Value, error) {

	if len(args) != 1 {
		return reflect.Value{}, fmt.Errorf("env: expected 1 argument, got %d", len(args))
	}

	if args[0].Kind() != reflect.String {
		return reflect.Value{}, fmt.Errorf("env: expected string key, got %s", args[0].Kind())
	}

	return reflect.ValueOf(os.Getenv(args[0].String())), nil
}

// argFileMode extracts an os.FileMode from a reflect.Value. Accepts:
//   - os.FileMode directly (type identity).
//   - signed integer kinds (rejects negatives).
//   - unsigned integer kinds.
//
// The receiving function uses argFileMode to admit both literal numeric directive args (parsed as
// int64 by text/template/parse) and sibling-slot references that already hold an os.FileMode value.
//
// Parameters:
//   - fnName: the calling function's name, used in error messages.
//   - v:      the reflect.Value to interpret.
//
// Returns:
//   - os.FileMode: the extracted mode.
//   - error:       non-nil on type mismatch or negative input.
func argFileMode(fnName string, v reflect.Value) (os.FileMode, error) {

	if v.Type() == fileModeType {
		return os.FileMode(v.Uint()), nil
	}

	if v.CanInt() {
		raw := v.Int()
		if raw < 0 {
			return 0, fmt.Errorf("%s: negative file mode %d", fnName, raw)
		}
		return os.FileMode(raw), nil
	}

	if v.CanUint() {
		return os.FileMode(v.Uint()), nil
	}

	return 0, fmt.Errorf("%s: expected file mode (int or os.FileMode), got %s", fnName, v.Kind())
}

// parseSymbolicMode9 parses a 9-character permission string into os.FileMode.
//
// Accepted shape: `"rwxrwxrwx"` template with `-` for unset bits. Position 0..2 is owner perms (r/w/x),
// 3..5 is group, 6..8 is other. Any position must be either the expected letter at that index or a `-`.
//
// Parameters:
//   - s: the 9-character permission string.
//
// Returns:
//   - os.FileMode: the parsed permission bits.
//   - error:       non-nil on length mismatch or invalid character.
func parseSymbolicMode9(s string) (os.FileMode, error) {

	if len(s) != 9 {
		return 0, fmt.Errorf("expected 9-character permission string (e.g., \"rwxr-x---\"), got %q", s)
	}

	var (
		bits     = [9]os.FileMode{0o400, 0o200, 0o100, 0o040, 0o020, 0o010, 0o004, 0o002, 0o001}
		expected = "rwxrwxrwx"
	)

	var m os.FileMode
	for i := range 9 {
		switch s[i] {
		case expected[i]:
			m |= bits[i]
		case '-':
			// Bit stays unset.
		default:
			return 0, fmt.Errorf("position %d: expected %q or '-', got %q", i, string(expected[i]), string(s[i]))
		}
	}

	return m, nil
}

// endregion

// endregion