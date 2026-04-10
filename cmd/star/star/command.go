// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package star

import (
	"errors"
	"fmt"
	"sort"
	"strconv"

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

// Run executes the command with the given flag values and optional positional arguments.
func (c *Command) Run(flags map[string]string, positional ...string) error {
	thread := &starlark.Thread{
		Name:  c.Name,
		Print: func(_ *starlark.Thread, msg string) { fmt.Println(msg) },
	}

	// Build flag type lookup for native starlark types.
	flagTypes := make(map[string]string, len(c.Flags))
	for _, f := range c.Flags {
		flagTypes[f.Name] = f.Type
	}

	// Build context dict with native types based on flag spec.
	argsDict := starlark.NewDict(len(flags) + len(c.Args))
	for k, v := range flags {
		sv := flagToStarlark(flagTypes[k], v)
		if err := argsDict.SetKey(starlark.String(k), sv); err != nil {
			return fmt.Errorf("setting flag %q: %w", k, err)
		}
	}

	// Map positional args to named entries using the arg spec.
	for _, arg := range c.Args {
		if arg.Variadic {
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
		} else if len(positional) > 0 {
			if err := argsDict.SetKey(starlark.String(arg.Name), starlark.String(positional[0])); err != nil {
				return fmt.Errorf("setting arg %q: %w", arg.Name, err)
			}
			positional = positional[1:]
		} else if arg.Default != "" {
			if err := argsDict.SetKey(starlark.String(arg.Name), starlark.String(arg.Default)); err != nil {
				return fmt.Errorf("setting arg %q: %w", arg.Name, err)
			}
		}
	}

	ctx := starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
		"args":    argsDict,
		"dry_run": starlark.Bool(DryRun),
	})

	// Set current command name in context data for the commands provider.
	if c.runtime != nil && c.runtime.data != nil {
		c.runtime.data["current_command"] = c.Name
	}

	// Do run(command, ctx).
	_, err := starlark.Call(thread, c.RunFunc, starlark.Tuple{c, ctx}, nil)
	if err != nil {
		var evalErr *starlark.EvalError
		if errors.As(err, &evalErr) {
			return fmt.Errorf("%s", evalErr.Backtrace())
		}
		return err
	}
	return nil
}

// flagToStarlark converts a string value to the appropriate starlark type based on
// the flag type from the extension spec.
func flagToStarlark(flagType, value string) starlark.Value {
	switch flagType {
	case "bool":
		return starlark.Bool(value == "true")
	case "int":
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
