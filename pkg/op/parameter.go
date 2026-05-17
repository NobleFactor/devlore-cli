// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// parseParameterToken cracks one wire-form parameter token into a fully typed Parameter.
//
// The token grammar emitted by codegen is:
//
//	token := variadic | kwargs | named
//	variadic := "*" name
//	kwargs := "**" name
//	named := name ( "?" ( "=" defaultExpr )? )?
//
// Variadic ("*") and kwargs ("**") tokens MAY NOT carry an optional marker ("?") or a default expression ("=value");
// both are inherently "zero or more" in shape and have no coherent default semantic. Named params MAY carry "?" (marks
// the parameter optional — caller may omit the kwarg) and MAY additionally carry "=value" (typed default —
// caller-omitted slots are filled with this value). The "=value" segment requires a leading "?".
//
// When a default expression is present, parseParameterToken delegates to parseDefaultExpression to parse the expression
// text against paramType. The returned Parameter has a Default whose dynamic type is exactly paramType (preserving the
// Q2 invariant: Parameter.Default always holds a Go-native value at Parameter.Type). Defaults are rejected upfront for
// parameters whose target implements Resource — Resource defaults would require slot-fill-time URI conversion through
// op.Convert Step 7 with a live RuntimeEnvironment, which is unavailable at announcement time; the rejection error
// documents the deferred path.
//
// Parameters:
//   - `raw`: the wire token to parse (e.g., "destination_path", "mode?", "mode?=0o666", "*parts", "**kwargs").
//   - `paramType`: the Go-method parameter type the token corresponds to. Used to type-check the default
//     expression and to detect Resource-typed parameters.
//
// Returns:
//   - `Parameter`: the fully typed Parameter, with Name (clean — no decoration), Type, Optional, Variadic, Kwargs, and
//     Default populated.
//   - `error`: non-nil if the token is malformed, if a default expression accompanies a variadic or kwargs token, if a
//     default expression accompanies a non-optional named token, if the default expression cannot be parsed against
//     paramType, or if paramType implements Resource.
func parseParameterToken(raw string, paramType reflect.Type) (Parameter, error) {

	if raw == "" {
		return Parameter{}, fmt.Errorf("empty parameter token")
	}

	// Step 1: kwargs prefix takes priority over variadic — "**" must be checked before "*".

	if strings.HasPrefix(raw, "**") {

		name := raw[2:]

		if name == "" {
			return Parameter{}, fmt.Errorf("kwargs marker %q requires a name", raw)
		}

		if strings.ContainsAny(name, "?=") {
			return Parameter{}, fmt.Errorf("kwargs token %q cannot carry '?' or '=value'", raw)
		}

		// Kwargs are inherently optional — the caller may always omit extra kwargs. Optional is set so consumers
		// that ask "may caller omit this slot?" get a single-source-of-truth answer without special-casing the
		// Variadic / Kwargs flags.
		return Parameter{Name: name, Type: paramType, Optional: true, Kwargs: true}, nil
	}

	if strings.HasPrefix(raw, "*") {

		name := raw[1:]

		if name == "" {
			return Parameter{}, fmt.Errorf("variadic marker %q requires a name", raw)
		}

		if strings.ContainsAny(name, "?=") {
			return Parameter{}, fmt.Errorf("variadic token %q cannot carry '?' or '=value'", raw)
		}

		// Variadic params are inherently optional — the caller may always omit positional overflow. Optional is
		// set so consumers that ask "may caller omit this slot?" get a single-source-of-truth answer without
		// special-casing the Variadic / Kwargs flags.
		return Parameter{Name: name, Type: paramType, Optional: true, Variadic: true}, nil
	}

	// Step 2: named token. Split on the optional marker; everything before is the name, everything after is the
	// optional+default segment.

	name, rest, hasMarker := strings.Cut(raw, "?")

	if !hasMarker {

		// No '?' — must not contain '=' either (default value requires the optional marker).

		if strings.ContainsRune(raw, '=') {
			return Parameter{}, fmt.Errorf(
				"token %q has '=value' without optional marker '?'; defaults require '?='", raw,
			)
		}

		return Parameter{Name: raw, Type: paramType}, nil
	}

	if name == "" {
		return Parameter{}, fmt.Errorf("token %q is missing a parameter name before '?'", raw)
	}

	// `rest` is what follows the '?'. Three cases: empty (just optional, no default), "=value" (optional with default),
	// or anything else (malformed).

	if rest == "" {
		return Parameter{Name: name, Type: paramType, Optional: true}, nil
	}

	if rest[0] != '=' {
		return Parameter{}, fmt.Errorf(
			"token %q has unexpected text after '?'; only '=value' is allowed", raw,
		)
	}

	defaultExpr := rest[1:]

	if defaultExpr == "" {
		return Parameter{}, fmt.Errorf("token %q has empty default expression after '?='", raw)
	}

	if strings.ContainsRune(defaultExpr, '?') {
		return Parameter{}, fmt.Errorf("token %q has stray '?' inside default expression", raw)
	}

	if paramType.Implements(resourceInterfaceType) {
		return Parameter{}, fmt.Errorf(
			"parameter %q: defaults for Resource-typed parameters are not supported yet "+
				"(would require slot-fill-time URI conversion through op.Convert Step 7 with a live ctx)",
			name,
		)
	}

	defaultValue, err := parseDefaultExpression(defaultExpr, paramType)
	if err != nil {
		return Parameter{}, fmt.Errorf("parameter %q: %w", name, err)
	}

	return Parameter{Name: name, Type: paramType, Optional: true, Default: defaultValue}, nil
}

// parseDefaultExpression converts a directive's literal default value text to a Go value of target's named type.
//
// The text comes from a +devlore:defaults directive (e.g., the "0o666" in `+devlore:defaults mode=0o666`). It is a
// Go-literal dialect — strconv-grade parsing is enough; full Go expressions are out of scope. Dispatch is by
// target.Kind: bool via strconv.ParseBool, signed integer kinds via strconv.ParseInt with base 0 (auto-detects 0x / 0o
// / 0b / decimal), unsigned integer kinds via strconv.ParseUint with base 0, float kinds via strconv.ParseFloat, and
// string via direct pass-through with optional surrounding double-quotes stripped. The parsed primitive is then widened
// to `target`'s named type via reflect.Value.Convert so that the returned any boxes a value whose dynamic type matches
// `target` exactly (e.g., os.FileMode(0o666), not uint32(0o666)).
//
// op.Convert is intentionally not used here. op.Convert is the runtime type-projection cascade used at every dispatch
// site; it is type-driven, not source-syntax-driven, and cannot parse "0o666" against os.FileMode. Adding a string
// source step to op.Convert would mix layers — a stray string in any caller's slot would silently parse instead of
// erroring. parseDefaultExpression keeps the directive-dialect parser local to defaults.
//
// Parameters:
//   - `expr`: the literal default-value text, as it appeared after '=' in the directive.
//   - `target`: the parameter's `[reflect.Type]`. Determines the strconv routine and the named-type widening.
//
// Returns:
//   - `any`: the parsed value, boxed in any with dynamic type equal to target.
//   - `error`: non-nil if expr is malformed for target's kind, if a string default has unbalanced quotes, or if
//     `target`'s kind is not one of the supported defaultable kinds (bool, int*, uint*, float*, string).
func parseDefaultExpression(expr string, target reflect.Type) (any, error) {

	// Deferred-default discriminator: directive values wrapped in `{{ ... }}` braces are runtime-resolved expressions
	// parsed at announcement time but evaluated at slot-fill against the live runtime environment. Detection is purely
	// textual; parseDeferred handles parser dispatch and validator-stub wiring. The target type is unused here. Type
	// widening happens at slot-fill via [Convert] inside [treeDefault.Resolve], so deferred defaults can produce any
	// type the func map supports.

	if strings.HasPrefix(expr, "{{") && strings.HasSuffix(expr, "}}") {
		return parseDeferred(expr)
	}

	switch target.Kind() {

	case reflect.Bool:

		v, err := strconv.ParseBool(expr)
		if err != nil {
			return nil, fmt.Errorf("parse default %q as %s: %w", expr, target, err)
		}

		return reflect.ValueOf(v).Convert(target).Interface(), nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:

		v, err := strconv.ParseInt(expr, 0, target.Bits())
		if err != nil {
			return nil, fmt.Errorf("parse default %q as %s: %w", expr, target, err)
		}

		return reflect.ValueOf(v).Convert(target).Interface(), nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:

		v, err := strconv.ParseUint(expr, 0, target.Bits())
		if err != nil {
			return nil, fmt.Errorf("parse default %q as %s: %w", expr, target, err)
		}

		return reflect.ValueOf(v).Convert(target).Interface(), nil

	case reflect.Float32, reflect.Float64:

		v, err := strconv.ParseFloat(expr, target.Bits())
		if err != nil {
			return nil, fmt.Errorf("parse default %q as %s: %w", expr, target, err)
		}

		return reflect.ValueOf(v).Convert(target).Interface(), nil

	case reflect.Complex64, reflect.Complex128:

		v, err := strconv.ParseComplex(expr, target.Bits())
		if err != nil {
			return nil, fmt.Errorf("parse default %q as %s: %w", expr, target, err)
		}

		return reflect.ValueOf(v).Convert(target).Interface(), nil

	case reflect.String:

		s, err := stripOptionalQuotes(expr)
		if err != nil {
			return nil, fmt.Errorf("parse default %q as %s: %w", expr, target, err)
		}

		return reflect.ValueOf(s).Convert(target).Interface(), nil

	default:
		return nil, fmt.Errorf(
			"default values for kind %s are not supported (only bool, int*, uint*, float*, complex*, string)",
			target.Kind(),
		)
	}
}

// stripOptionalQuotes returns the inner content of a double-quoted string, or `s` itself if there are no quotes.
//
// Returns an error if exactly one of the leading or trailing quote is present — a typo by the directive author, not a
// valid form.
//
// Parameters:
//   - `s`: the candidate default-expression text, possibly wrapped in matching double quotes.
//
// Returns:
//   - `string`: the unquoted inner content if `s` was fully quoted, otherwise `s` unchanged.
//   - `error`: non-nil if exactly one of the leading or trailing quote is present.
func stripOptionalQuotes(s string) (string, error) {

	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1], nil
	}

	if strings.HasPrefix(s, `"`) || strings.HasSuffix(s, `"`) {
		return "", fmt.Errorf("unbalanced quotes in %q", s)
	}

	return s, nil
}
