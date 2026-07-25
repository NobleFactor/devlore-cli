// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"encoding"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"time"
)

// Compile-time interface check.
var _ SourceConverter = envValue("")

var (
	// envValueTimeDurationType is the `[reflect.Type]` of `[time.Duration]`.
	envValueTimeDurationType = reflect.TypeOf(time.Duration(0))

	// envValueFileModeType is the `[reflect.Type]` of `[os.FileMode]`.
	envValueFileModeType = reflect.TypeOf(os.FileMode(0))

	// envValueResourceType is the `[reflect.Type]` of the `[Resource]` interface for the Resource-target `opt-out`.
	envValueResourceType = reflect.TypeOf((*Resource)(nil)).Elem()
)

// envValue wraps a raw env-sourced string so the [Convert] cascade routes string parsing through [SourceConverter].
//
// The wrapper exists because plain `string` cannot implement [SourceConverter] (Go forbids methods on built-in types),
// and routing string-source parsing through [Convert] directly would corrupt the cascade. See the layering note at the
// head of `parameter.go`'s [parseDefaultExpression] for the rationale that keeps source-syntax parsing out of [Convert].
//
// envValue keeps env-source string parsing localized: env strings get the lenient cascade (primitives → stdlib
// special-cases → [encoding.TextUnmarshaler] → [json.Unmarshal]), while plain Go strings from starlark slot fills and
// immediate-mode Go calls continue to fail on primitive targets as they always have. [envValue.CanConvertTo] returns
// false for [Resource] targets, so [Convert] step 7 (registered Resource construction) wins routing for env strings
// naming a Resource.
type envValue string

// region EXPORTED METHODS

// region Behaviors

// CanConvertTo reports whether [envValue.ConvertTo] can produce a value of the given target.
//
// Returns false for nil targets and for targets implementing [Resource] (the cascade falls through to [Convert] step 7,
// which routes through the registered Resource constructor with the original env value). Returns true for every other
// target; [envValue.ConvertTo] performs the actual parsing and may return its own error if no strategy succeeds.
//
// Parameters:
//   - `target`: the parsed-value type.
//
// Returns:
//   - `bool`: false when `target` is nil or implements [Resource]; true otherwise.
func (envValue) CanConvertTo(target reflect.Type) bool {

	if target == nil {
		return false
	}

	if target.Implements(envValueResourceType) {
		return false
	}

	return true
}

// ConvertTo parses the wrapped env string into a value of target. Tries (in order):
//
//  1. `string` identity when `target.Kind() == reflect.String`.
//  2. [time.Duration] via [time.ParseDuration] handles "30s", "1h15m", etc.
//  3. [os.FileMode] via [strconv.ParseUint] base 0 handles "0o755", "0x1ff", "493".
//  4. `primitive` [reflect.Kind] switch for bool/int*/uint*/float*/complex* via strconv with base 0.
//  5. [encoding.TextUnmarshaler] on a fresh `*target` probe.
//  6. [json.Unmarshal] universal fallback.
//
// Parameters:
//   - `target`: the parsed-value type.
//
// Returns:
//   - `any`: the parsed value, dynamic-type-equal to target.
//   - `error`: non-nil when no parsing strategy succeeds.
func (e envValue) ConvertTo(target reflect.Type) (any, error) {

	raw := string(e)

	if target == nil {
		return nil, fmt.Errorf("envValue.ConvertTo: nil target")
	}

	if target.Kind() == reflect.String {
		return reflect.ValueOf(raw).Convert(target).Interface(), nil
	}

	if target == envValueTimeDurationType {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return nil, fmt.Errorf("parse %q as time.Duration: %w", raw, err)
		}
		return d, nil
	}

	if target == envValueFileModeType {
		v, err := strconv.ParseUint(raw, 0, 32)
		if err != nil {
			return nil, fmt.Errorf("parse %q as os.FileMode: %w", raw, err)
		}
		return os.FileMode(v), nil
	}

	switch target.Kind() {

	case reflect.Bool:
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("parse %q as %s: %w", raw, target, err)
		}
		return reflect.ValueOf(v).Convert(target).Interface(), nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v, err := strconv.ParseInt(raw, 0, target.Bits())
		if err != nil {
			return nil, fmt.Errorf("parse %q as %s: %w", raw, target, err)
		}
		return reflect.ValueOf(v).Convert(target).Interface(), nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		v, err := strconv.ParseUint(raw, 0, target.Bits())
		if err != nil {
			return nil, fmt.Errorf("parse %q as %s: %w", raw, target, err)
		}
		return reflect.ValueOf(v).Convert(target).Interface(), nil

	case reflect.Float32, reflect.Float64:
		v, err := strconv.ParseFloat(raw, target.Bits())
		if err != nil {
			return nil, fmt.Errorf("parse %q as %s: %w", raw, target, err)
		}
		return reflect.ValueOf(v).Convert(target).Interface(), nil

	case reflect.Complex64, reflect.Complex128:
		v, err := strconv.ParseComplex(raw, target.Bits())
		if err != nil {
			return nil, fmt.Errorf("parse %q as %s: %w", raw, target, err)
		}
		return reflect.ValueOf(v).Convert(target).Interface(), nil

	default:

		probe := reflect.New(target).Interface()

		if tu, ok := probe.(encoding.TextUnmarshaler); ok {
			if err := tu.UnmarshalText([]byte(raw)); err != nil {
				return nil, fmt.Errorf("UnmarshalText %q into %s: %w", raw, target, err)
			}
			return reflect.ValueOf(probe).Elem().Interface(), nil
		}

		out := reflect.New(target).Interface()

		if err := json.Unmarshal([]byte(raw), out); err != nil {
			return nil, fmt.Errorf("unmarshal %q as %s: %w", raw, target, err)
		}

		return reflect.ValueOf(out).Elem().Interface(), nil
	}
}

// endregion

// endregion
