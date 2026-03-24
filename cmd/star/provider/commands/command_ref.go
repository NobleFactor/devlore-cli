// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package commands

import (
	"fmt"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// CommandRef wraps a command for use in Starlark. It implements starlark.Value
// and starlark.HasAttrs, exposing name, help, flags, and a callable run method.
type CommandRef struct {
	name string
	tree CommandTree
}

// NewCommandRef creates a new CommandRef.
func NewCommandRef(name string, tree CommandTree) *CommandRef {
	return &CommandRef{name: name, tree: tree}
}

// String implements starlark.Value.
func (r *CommandRef) String() string { return fmt.Sprintf("<command %s>", r.name) }

// Type implements starlark.Value.
func (r *CommandRef) Type() string { return "command" }

// Freeze implements starlark.Value.
func (r *CommandRef) Freeze() {}

// Truth implements starlark.Value.
func (r *CommandRef) Truth() starlark.Bool { return true }

// Hash implements starlark.Value.
func (r *CommandRef) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: command") }

// Attr implements starlark.HasAttrs.
func (r *CommandRef) Attr(name string) (starlark.Value, error) {
	switch name {
	case "name":
		return starlark.String(r.name), nil
	case "help":
		return starlark.String(r.tree.CommandHelp(r.name)), nil
	case "flags":
		return r.flagsList(), nil
	case "run":
		return starlark.NewBuiltin("command.run", r.run), nil
	default:
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("command has no .%s attribute", name))
	}
}

// AttrNames implements starlark.HasAttrs.
func (r *CommandRef) AttrNames() []string {
	return []string{"flags", "help", "name", "run"}
}

func (r *CommandRef) flagsList() starlark.Value {
	flags := r.tree.CommandFlags(r.name)
	var list []starlark.Value
	for _, f := range flags {
		list = append(list, starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
			"name":    starlark.String(f.Name),
			"help":    starlark.String(f.Help),
			"default": starlark.String(f.Default),
		}))
	}
	return starlark.NewList(list)
}

func (r *CommandRef) run(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	cmdFlags := make(map[string]string)
	var positional []string

	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		// List kwargs are treated as positional args (e.g., path=["./internal"]).
		if list, ok := kv[1].(*starlark.List); ok {
			for i := 0; i < list.Len(); i++ {
				positional = append(positional, starlarkValueToString(list.Index(i)))
			}
		} else {
			cmdFlags[key] = starlarkValueToString(kv[1])
		}
	}

	err := r.tree.RunCommand(r.name, cmdFlags, positional...)

	errStr := ""
	if err != nil {
		errStr = err.Error()
	}

	return starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
		"passed": starlark.Bool(err == nil),
		"error":  starlark.String(errStr),
	}), nil
}

func starlarkValueToString(v starlark.Value) string {
	switch val := v.(type) {
	case starlark.String:
		return string(val)
	case starlark.Bool:
		if val {
			return "true"
		}
		return "false"
	case starlark.Int:
		return val.String()
	default:
		return v.String()
	}
}

// getParentPath returns the parent path of a dot-separated command name.
func getParentPath(name string) string {
	lastDot := strings.LastIndex(name, ".")
	if lastDot == -1 {
		return ""
	}
	return name[:lastDot]
}

// matchPattern checks if a name matches a pattern with * wildcards.
func matchPattern(name, pattern string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, ".*")
		return strings.HasPrefix(name, prefix+".") || name == prefix
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(name, suffix)
	}
	return name == pattern
}
