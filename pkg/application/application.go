// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package application defines the per-tool [Application] handle carried on every [op.RuntimeEnvironment].
// Each tool (lore, star, writ, devlore-test) constructs an [Application] from its own CLI / config plumbing
// and hands it to [op.NewRuntimeEnvironmentSpec.WithApplication]. The Application carries the variable-
// resolver's input sources (flag / config / override maps) plus the tool's program name; pkg/op reads them
// uniformly without knowing tool specifics.
//
// Flag projection uses cobra's pflag merged view: a single call to [cobra.Command.Flags] surfaces both the
// command's local flags and every persistent flag inherited from its ancestors. [NewApplication] walks that
// merged view via pflag.FlagSet.Visit, which yields only flags the user explicitly supplied on the command
// line — defaults are not stamped into [Application.Flags].
package application

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Application is the tool-side handle the workflow framework reads through [op.RuntimeEnvironment]. It
// carries the variable-resolver source maps and the tool's program name. Each tool owns its own instance
// and projects its native CLI / config plumbing into the three maps.
//
// Flags, Config, and Overrides are passed verbatim to the [op.VariableResolver] when the runtime
// environment is constructed. Framework code that needs a specific flag (e.g., "dry-run") reads from
// [Application.Flags] directly without invoking the resolver.
type Application struct {

	// Name is the tool's program name (e.g., "lore", "star", "writ", "devlore-test"). The
	// [op.VariableResolver] derives its env-var prefix from this value as `strings.ToUpper(Name) + "_"`.
	Name string

	// Flags carries values parsed from command-line arguments. Consumed by [op.VariableResolver] under
	// [op.VariableSourceKindFlag]. Keys are the cobra/pflag flag names (kebab-case as the user typed them);
	// values are the typed Go value pflag parsed.
	//
	// Populated by [NewApplication] via [pflag.FlagSet.Visit] — only flags the user explicitly supplied are
	// present. A lookup for a flag the user did not pass returns the zero value via map-zero semantics.
	Flags map[string]any

	// Config carries values loaded from configuration files. Consumed by [op.VariableResolver] under
	// [op.VariableSourceKindConfig].
	Config map[string]any

	// Overrides carries programmatic-force values applied at highest precedence. Consumed by
	// [op.VariableResolver] under [op.VariableSourceKindOverride].
	Overrides map[string]any
}

// DryRun reports whether the user supplied `--dry-run` on the active command. Reads
// [Application.Flags] under the canonical key "dry-run" (the cobra flag name verbatim). Returns false when
// the flag was not supplied, when its value is not a bool, or when [Application.Flags] is nil.
//
// Returns:
//   - `bool`: true when `--dry-run` was supplied; false otherwise.
func (a *Application) DryRun() bool {

	v, _ := a.Flags["dry-run"].(bool)
	return v
}

// NewApplication constructs an [Application] from a cobra command's parsed flag state. Walks the command's
// merged pflag set (cobra merges persistent + local automatically when [cobra.Command.Flags] is called on
// the leaf) and stamps each user-supplied flag into [Application.Flags]. Defaults are not stamped.
//
// Config and Overrides are left nil. Tools that source either layer populate them via direct field
// assignment after construction.
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
		flags[f.Name] = flagValue(cmd, f)
	})

	return &Application{
		Name:  name,
		Flags: flags,
	}
}

// flagValue extracts the typed Go value of a [pflag.Flag] by switching on its declared type. The pflag
// typed accessors are called on the [cobra.Command]'s merged FlagSet because each accessor handles the
// not-found case via its second return value — we already know the flag exists (Visit yielded it), so the
// error is discarded.
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

	switch f.Value.Type() {
	case "bool":
		v, _ := cmd.Flags().GetBool(f.Name)
		return v
	case "string":
		v, _ := cmd.Flags().GetString(f.Name)
		return v
	case "int":
		v, _ := cmd.Flags().GetInt(f.Name)
		return v
	case "int8":
		v, _ := cmd.Flags().GetInt8(f.Name)
		return v
	case "int16":
		v, _ := cmd.Flags().GetInt16(f.Name)
		return v
	case "int32":
		v, _ := cmd.Flags().GetInt32(f.Name)
		return v
	case "int64":
		v, _ := cmd.Flags().GetInt64(f.Name)
		return v
	case "uint":
		v, _ := cmd.Flags().GetUint(f.Name)
		return v
	case "uint8":
		v, _ := cmd.Flags().GetUint8(f.Name)
		return v
	case "uint16":
		v, _ := cmd.Flags().GetUint16(f.Name)
		return v
	case "uint32":
		v, _ := cmd.Flags().GetUint32(f.Name)
		return v
	case "uint64":
		v, _ := cmd.Flags().GetUint64(f.Name)
		return v
	case "float32":
		v, _ := cmd.Flags().GetFloat32(f.Name)
		return v
	case "float64":
		v, _ := cmd.Flags().GetFloat64(f.Name)
		return v
	case "duration":
		v, _ := cmd.Flags().GetDuration(f.Name)
		return v
	case "stringSlice":
		v, _ := cmd.Flags().GetStringSlice(f.Name)
		return v
	case "stringArray":
		v, _ := cmd.Flags().GetStringArray(f.Name)
		return v
	case "intSlice":
		v, _ := cmd.Flags().GetIntSlice(f.Name)
		return v
	case "int32Slice":
		v, _ := cmd.Flags().GetInt32Slice(f.Name)
		return v
	case "int64Slice":
		v, _ := cmd.Flags().GetInt64Slice(f.Name)
		return v
	case "stringToString":
		v, _ := cmd.Flags().GetStringToString(f.Name)
		return v
	case "stringToInt":
		v, _ := cmd.Flags().GetStringToInt(f.Name)
		return v
	case "stringToInt64":
		v, _ := cmd.Flags().GetStringToInt64(f.Name)
		return v
	case "boolSlice":
		v, _ := cmd.Flags().GetBoolSlice(f.Name)
		return v
	case "count":
		v, _ := cmd.Flags().GetCount(f.Name)
		return v
	default:
		return f.Value.String()
	}
}
