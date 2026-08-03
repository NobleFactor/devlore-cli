// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package application defines the per-tool [Application] handle the workflow framework carries on its runtime
// environment.
//
// Each tool (lore, star, writ, devlore-test) constructs an [Application] from its own CLI / config plumbing and hands
// it to the framework's runtime-environment spec at construction. The Application carries the variable resolver's
// input sources (flag / config / override maps) plus the tool's program name; the framework reads them uniformly
// without knowing tool specifics.
//
// Flag projection uses cobra's pflag merged view: a single call to [cobra.Command.Flags] surfaces both the command's
// local flags and every persistent flag inherited from its ancestors. [NewApplication] walks that merged view via
// pflag.FlagSet.Visit, which yields only flags the user explicitly supplied on the command line — defaults are not
// stamped into [Application.Flags].
package application

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// Application is the tool-side handle the workflow framework reads through its runtime environment.
//
// It carries the variable-resolver source maps and the tool's program name. Each tool owns its own instance and
// projects its native CLI / config plumbing into the three maps.
//
// Flags, Config, and Overrides are passed verbatim to the framework's variable resolver when the runtime
// environment is constructed. Framework code that needs a specific flag (e.g., "dry-run") reads from
// [Application.Flags] directly without invoking the resolver.
type Application struct {

	// Name is the tool's program name (e.g., "lore", "star", "writ", "devlore-test").
	//
	// The framework's variable resolver derives its env-var prefix from this value as `strings.ToUpper(Name) + "_"`.
	Name string

	// Flags carries values parsed from command-line arguments.
	//
	// Consumed by the variable resolver as its flag source. Keys are snake-case (normalized from
	// cobra/pflag's kebab-case at [NewApplication] time so `--dry-run` lands under `Flags["dry_run"]`); values are the
	// typed Go value pflag parsed.
	//
	// Populated by [NewApplication] via [pflag.FlagSet.Visit] — only flags the user explicitly supplied are present. A
	// lookup for a flag the user did not pass returns the zero value via map-zero semantics.
	Flags map[string]any

	// Config carries values loaded from configuration files.
	//
	// Consumed by the variable resolver as its config source.
	Config map[string]any

	// Overrides carries programmatic-force values applied at highest precedence.
	//
	// Consumed by the variable resolver as its override source.
	Overrides map[string]any
}

// DryRun reports whether the user supplied `--dry-run` on the active command.
//
// Reads [Application.Flags] under the canonical snake-case key "dry_run" (normalized from cobra's "dry-run" at
// [NewApplication] time). Returns false when the flag was not supplied, when its value is not a bool, or when
// [Application.Flags] is nil.
//
// Returns:
//   - `bool`: true when `--dry-run` was supplied; false otherwise.
func (a *Application) DryRun() bool {

	if v, ok := a.Flags["dry_run"].(bool); ok {
		return v
	}
	return false
}

// NewApplication constructs an [Application] from a cobra command's parsed flag state.
//
// Walks the command's merged pflag set (cobra merges persistent + local automatically when [cobra.Command.Flags] is
// called on the leaf) and stamps each user-supplied flag into [Application.Flags] under its snake-case form (cobra's
// kebab-case flag name with hyphens replaced by underscores). Defaults are not stamped.
//
// The kebab→snake normalization makes [Application.Flags] uniform with the variable resolver's parameter-name
// conventions (snake_case throughout); the resolver's flag-source step matches the parameter name verbatim against the
// snake-case keys.
//
// Config and Overrides are left nil. Tools that source either layer populate them via direct field assignment after
// construction.
//
// Parameters:
//   - `name`: the tool's program name (e.g., "lore", "writ").
//   - `cmd`: the cobra command whose merged flag set drives Flags. Must be non-nil.
//
// Returns:
//   - *Application: the constructed Application with Name and Flags set.
func NewApplication(name string, cmd *cobra.Command) *Application {

	flags := make(map[string]any)

	cmd.Flags().Visit(func(f *pflag.Flag) {
		flags[kebabToSnake(f.Name)] = flagValue(cmd, f)
	})

	return &Application{
		Name:  name,
		Flags: flags,
	}
}

// kebabToSnake converts a cobra/pflag kebab-case flag name to snake_case by replacing every '-' with '_'.
//
// Idempotent on already-snake names.
//
// Parameters:
//   - `name`: the kebab-case flag name (e.g., "dry-run", "target-root").
//
// Returns:
//   - `string`: the snake-case form (e.g., "dry_run", "target_root").
func kebabToSnake(name string) string {

	return strings.ReplaceAll(name, "-", "_")
}

// flagValue extracts the typed Go value of a [pflag.Flag] by switching on its declared type.
//
// The pflag typed accessors are called on the [cobra.Command]'s merged FlagSet because each accessor handles the
// not-found case via its second return value. We already know the flag exists (because Visit yielded it), so the error
// is discarded.
//
// Unknown / custom flag types fall back to the flag's string representation via [pflag.Value.String].
//
// Parameters:
//   - `cmd`: the cobra command (provides typed flag accessors).
//   - `f`: the pflag.Flag whose typed value is being extracted.
//
// Returns:
//   - `any`: the typed Go value of the flag, or its string representation when the type is unknown.
func flagValue(cmd *cobra.Command, f *pflag.Flag) any {

	if v, ok := flagIntegerValue(cmd, f); ok {
		return v
	}
	if v, ok := flagScalarValue(cmd, f); ok {
		return v
	}
	if v, ok := flagCollectionValue(cmd, f); ok {
		return v
	}

	return f.Value.String()
}

// flagIntegerValue extracts a signed or unsigned integer flag value.
//
// Parameters:
//   - `cmd`: the cobra command (provides typed flag accessors).
//   - `f`: the pflag.Flag whose typed value is being extracted.
//
// Returns:
//   - `any`: the typed integer value.
//   - `bool`: false when the flag's type is not an integer family member.
func flagIntegerValue(cmd *cobra.Command, f *pflag.Flag) (any, bool) {

	switch f.Value.Type() {
	case "int":
		return assert.Must(cmd.Flags().GetInt(f.Name)), true
	case "int8":
		return assert.Must(cmd.Flags().GetInt8(f.Name)), true
	case "int16":
		return assert.Must(cmd.Flags().GetInt16(f.Name)), true
	case "int32":
		return assert.Must(cmd.Flags().GetInt32(f.Name)), true
	case "int64":
		return assert.Must(cmd.Flags().GetInt64(f.Name)), true
	case "uint":
		return assert.Must(cmd.Flags().GetUint(f.Name)), true
	case "uint8":
		return assert.Must(cmd.Flags().GetUint8(f.Name)), true
	case "uint16":
		return assert.Must(cmd.Flags().GetUint16(f.Name)), true
	case "uint32":
		return assert.Must(cmd.Flags().GetUint32(f.Name)), true
	case "uint64":
		return assert.Must(cmd.Flags().GetUint64(f.Name)), true
	}

	return nil, false
}

// flagScalarValue extracts a non-integer scalar flag value.
//
// Parameters:
//   - `cmd`: the cobra command (provides typed flag accessors).
//   - `f`: the pflag.Flag whose typed value is being extracted.
//
// Returns:
//   - `any`: the typed scalar value.
//   - `bool`: false when the flag's type is not a non-integer scalar.
func flagScalarValue(cmd *cobra.Command, f *pflag.Flag) (any, bool) {

	switch f.Value.Type() {
	case "bool":
		return assert.Must(cmd.Flags().GetBool(f.Name)), true
	case "string":
		return assert.Must(cmd.Flags().GetString(f.Name)), true
	case "float32":
		return assert.Must(cmd.Flags().GetFloat32(f.Name)), true
	case "float64":
		return assert.Must(cmd.Flags().GetFloat64(f.Name)), true
	case "duration":
		return assert.Must(cmd.Flags().GetDuration(f.Name)), true
	case "count":
		return assert.Must(cmd.Flags().GetCount(f.Name)), true
	}

	return nil, false
}

// flagCollectionValue extracts a slice or map flag value.
//
// Parameters:
//   - `cmd`: the cobra command (provides typed flag accessors).
//   - `f`: the pflag.Flag whose typed value is being extracted.
//
// Returns:
//   - `any`: the typed collection value.
//   - `bool`: false when the flag's type is not a collection.
func flagCollectionValue(cmd *cobra.Command, f *pflag.Flag) (any, bool) {

	switch f.Value.Type() {
	case "stringSlice":
		return assert.Must(cmd.Flags().GetStringSlice(f.Name)), true
	case "stringArray":
		return assert.Must(cmd.Flags().GetStringArray(f.Name)), true
	case "intSlice":
		return assert.Must(cmd.Flags().GetIntSlice(f.Name)), true
	case "int32Slice":
		return assert.Must(cmd.Flags().GetInt32Slice(f.Name)), true
	case "int64Slice":
		return assert.Must(cmd.Flags().GetInt64Slice(f.Name)), true
	case "boolSlice":
		return assert.Must(cmd.Flags().GetBoolSlice(f.Name)), true
	case "stringToString":
		return assert.Must(cmd.Flags().GetStringToString(f.Name)), true
	case "stringToInt":
		return assert.Must(cmd.Flags().GetStringToInt(f.Name)), true
	case "stringToInt64":
		return assert.Must(cmd.Flags().GetStringToInt64(f.Name)), true
	}

	return nil, false
}
