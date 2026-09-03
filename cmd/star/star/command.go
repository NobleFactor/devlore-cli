// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package star

import (
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// Command is an immutable object representing a single command within an extension.
// YAML fields are deserialized from the commands: section of extension.yaml.
// Runtime fields are set during extension loading.
type Command struct {
	// YAML fields.
	Name           string `yaml:"name"`
	Help           string `yaml:"help"`
	Implementation string `yaml:"implementation"`
	Args           []Arg  `yaml:"args"`
	Flags          []Flag `yaml:"flags"`

	// Runtime fields — set after unmarshaling.
	Extension   *Extension          `yaml:"-"`
	RunFunc     starlark.Callable   `yaml:"-"`
	globals     starlark.StringDict `yaml:"-"`
	predeclared starlark.StringDict `yaml:"-"`
	runtime     *Application        `yaml:"-"`
}

// Arg represents a positional argument.
type Arg struct {
	Name     string `yaml:"name"`
	Help     string `yaml:"help"`
	Default  string `yaml:"default"`
	Variadic bool   `yaml:"variadic"`
}

// Flag represents a command flag.
type Flag struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	Help     string `yaml:"help"`
	Default  string `yaml:"default"`
	Required bool   `yaml:"required"`
}

// Run executes the command with the given flag values and optional positional arguments, and returns
// what the script returned.
//
// A script's return value is the command's result: it reaches stdout through the shared output
// pipeline, rendered by `--output`, so a script never prints its result itself. None is no result.
//
// Parameters:
//   - `flags`: flag name to string value, as collected from cobra.
//   - `positional`: the positional arguments, consumed in arg-spec order.
//
// Returns:
//   - `any`: the script's return value as the JSON-shaped Go value the pipeline renders; nil for None.
//   - `error`: non-nil when the arguments cannot be bound, the script fails, or its return value has
//     no JSON shape.
func (c *Command) Run(flags map[string]string, positional ...string) (any, error) {
	thread := &starlark.Thread{
		Name:  c.Name,
		Print: func(_ *starlark.Thread, msg string) { cli.Note("  [print] %s", msg) },
	}

	argsDict, err := c.buildArgsDict(flags, positional)
	if err != nil {
		return nil, err
	}

	ctx := starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
		"args":    argsDict,
		"dry_run": starlark.Bool(DryRun),
	})

	c.setCurrentCommand()

	// Do run(command, ctx).
	value, err := starlark.Call(thread, c.RunFunc, starlark.Tuple{c, ctx}, nil)
	if err != nil {
		var evalErr *starlark.EvalError
		if errors.As(err, &evalErr) {
			return nil, fmt.Errorf("%s", evalErr.Backtrace())
		}
		return nil, err
	}

	return goValue(value)
}

// buildArgsDict builds the context dict with native starlark types from the flag values and
// positional arguments, per the command's specs.
//
// Parameters:
//   - `flags`: flag name to string value, as collected from cobra.
//   - `positional`: the positional arguments, consumed in arg-spec order.
//
// Returns:
//   - `*starlark.Dict`: the populated args dict.
//   - `error`: non-nil when a dict entry cannot be set.
func (c *Command) buildArgsDict(flags map[string]string, positional []string) (*starlark.Dict, error) {

	// Build flag type lookup for native starlark types.
	flagTypes := make(map[string]string, len(c.Flags))
	for _, f := range c.Flags {
		flagTypes[f.Name] = f.Type
	}

	argsDict := starlark.NewDict(len(flags) + len(c.Args))
	for k, v := range flags {
		sv := flagToStarlark(flagTypes[k], v)
		if err := argsDict.SetKey(starlark.String(k), sv); err != nil {
			return nil, fmt.Errorf("setting flag %q: %w", k, err)
		}
	}

	if err := applyPositionalArgs(argsDict, c.Args, positional); err != nil {
		return nil, err
	}

	return argsDict, nil
}

// applyPositionalArgs maps the positional arguments onto named dict entries per the arg specs: a
// variadic arg absorbs the remainder as a list (falling back to its default when empty), a plain arg
// consumes one value, and an unfilled arg takes its default when it has one.
//
// Parameters:
//   - `argsDict`: the args dict receiving the entries.
//   - `args`: the command's positional arg specs, in declaration order.
//   - `positional`: the positional arguments, consumed left to right.
//
// Returns:
//   - `error`: non-nil when a dict entry cannot be set.
func applyPositionalArgs(argsDict *starlark.Dict, args []Arg, positional []string) error {

	for _, arg := range args {
		switch {
		case arg.Variadic:
			vals := make([]starlark.Value, len(positional))
			for i, v := range positional {
				vals[i] = starlark.String(v)
			}
			if len(vals) == 0 && arg.Default != "" {
				vals = []starlark.Value{starlark.String(arg.Default)}
			}
			if err := argsDict.SetKey(starlark.String(arg.Name), starlark.NewList(vals)); err != nil {
				return fmt.Errorf("setting arg %q: %w", arg.Name, err)
			}
		case len(positional) > 0:
			if err := argsDict.SetKey(starlark.String(arg.Name), starlark.String(positional[0])); err != nil {
				return fmt.Errorf("setting arg %q: %w", arg.Name, err)
			}
			positional = positional[1:]
		case arg.Default != "":
			if err := argsDict.SetKey(starlark.String(arg.Name), starlark.String(arg.Default)); err != nil {
				return fmt.Errorf("setting arg %q: %w", arg.Name, err)
			}
		}
	}

	return nil
}

// setCurrentCommand records this command's name in the application overrides so the commands provider
// can read it via Application.Overrides during this dispatch.
//
// Per-dispatch mutation — doesn't fit RegisterParameter's construction-time resolution model.
func (c *Command) setCurrentCommand() {

	if c.runtime == nil || c.runtime.app == nil {
		return
	}

	if c.runtime.app.Overrides == nil {
		c.runtime.app.Overrides = make(map[string]any)
	}
	c.runtime.app.Overrides["current_command"] = c.Name
}

// flagToStarlark converts a string value to the appropriate starlark type based on
// the flag type from the extension spec.
func flagToStarlark(flagType, value string) starlark.Value {
	switch flagType {
	case "bool":
		return starlark.Bool(value == "true")
	case "int":
		//nolint:errcheck // diagnose-ignored-error: falls back to 0; see docs/architecture/2.8-eventing-infrastructure.md
		n, _ := strconv.Atoi(value)
		return starlark.MakeInt(n)
	default:
		return starlark.String(value)
	}
}

// region starlark.Value interface

// String implements starlark.Value.
func (c *Command) String() string {
	return fmt.Sprintf("<command %s>", c.Name)
}

// Type implements starlark.Value.
func (c *Command) Type() string { return "command" }

// Freeze implements starlark.Value.
func (c *Command) Freeze() {} // immutable

// Truth implements starlark.Value.
func (c *Command) Truth() starlark.Bool { return starlark.True }

// Hash implements starlark.Value.
func (c *Command) Hash() (uint32, error) {
	return starlark.String(c.Name).Hash()
}

// Attr implements starlark.HasAttrs.
func (c *Command) Attr(name string) (starlark.Value, error) {
	switch name {
	case "name":
		return starlark.String(c.Name), nil
	case "extension":
		return c.Extension, nil
	default:
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("command has no .%s attribute", name))
	}
}

// AttrNames implements starlark.HasAttrs.
func (c *Command) AttrNames() []string {
	names := []string{"extension", "name"}
	sort.Strings(names)
	return names
}

// Ensure interfaces are satisfied.
var (
	_ starlark.Value    = (*Command)(nil)
	_ starlark.HasAttrs = (*Command)(nil)
)

// endregion
