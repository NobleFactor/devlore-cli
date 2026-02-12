// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// NpmReceiver provides the npm.* Starlark namespace.
//
// Backing implementation: os/exec (exec.Command("npm", ...), exec.Command("npx", ...)).
// Uses kwargs pass-through: any keyword argument is converted to a CLI flag.
//
// Example:
//
//	npm.install("astro", "tailwind", global=True, save_dev=True)
//	# Executes: npm install --global --save-dev astro tailwind
type NpmReceiver struct {
	Receiver
	output io.Writer
}

// NewNpmReceiver creates a new npm receiver.
func NewNpmReceiver(output io.Writer) *NpmReceiver {
	return &NpmReceiver{Receiver: NewReceiver("npm"), output: output}
}

// Attr implements starlark.HasAttrs.
func (n *NpmReceiver) Attr(name string) (starlark.Value, error) {
	switch name {
	case "install":
		return MakeAttr("npm.install", n.install), nil
	case "uninstall":
		return MakeAttr("npm.uninstall", n.uninstall), nil
	case "update":
		return MakeAttr("npm.update", n.update), nil
	case "run":
		return MakeAttr("npm.run", n.run), nil
	case "exec":
		return MakeAttr("npm.exec", n.execCmd), nil
	case "init":
		return MakeAttr("npm.init", n.init), nil
	case "publish":
		return MakeAttr("npm.publish", n.publish), nil
	case "pack":
		return MakeAttr("npm.pack", n.pack), nil
	case "link":
		return MakeAttr("npm.link", n.link), nil
	case "audit":
		return MakeAttr("npm.audit", n.audit), nil
	case "ci":
		return MakeAttr("npm.ci", n.ci), nil
	case "cache":
		return MakeAttr("npm.cache", n.cache), nil
	case "config":
		return MakeAttr("npm.config", n.config), nil
	case "installed":
		return MakeAttr("npm.installed", n.installed), nil
	case "version":
		return MakeAttr("npm.version", n.version), nil
	case "list_global":
		return MakeAttr("npm.list_global", n.listGlobal), nil
	case "prefix":
		return MakeAttr("npm.prefix", n.prefix), nil
	default:
		return nil, NoSuchAttrError("npm", name)
	}
}

// AttrNames implements starlark.HasAttrs.
func (n *NpmReceiver) AttrNames() []string {
	return []string{
		"audit", "cache", "ci", "config", "exec", "init", "install",
		"installed", "link", "list_global", "pack", "prefix", "publish",
		"run", "uninstall", "update", "version",
	}
}

// =============================================================================
// Core Operations (kwargs pass-through)
// =============================================================================

func (n *NpmReceiver) install(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return n.passThrough("install", args, kwargs)
}

func (n *NpmReceiver) uninstall(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return n.passThrough("uninstall", args, kwargs)
}

func (n *NpmReceiver) update(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return n.passThrough("update", args, kwargs)
}

func (n *NpmReceiver) run(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return n.passThrough("run", args, kwargs)
}

func (n *NpmReceiver) execCmd(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	cmdArgs := []string{}
	cmdArgs = append(cmdArgs, npmKwargsToFlags(kwargs)...)

	for _, arg := range args {
		cmdArgs = append(cmdArgs, argToString(arg))
	}

	_, _ = fmt.Fprintf(n.output, "  [npx] %s\n", strings.Join(cmdArgs, " "))

	return n.runNpx(cmdArgs)
}

func (n *NpmReceiver) init(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return n.passThrough("init", args, kwargs)
}

func (n *NpmReceiver) publish(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return n.passThrough("publish", args, kwargs)
}

func (n *NpmReceiver) pack(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return n.passThrough("pack", args, kwargs)
}

func (n *NpmReceiver) link(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return n.passThrough("link", args, kwargs)
}

func (n *NpmReceiver) audit(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return n.passThrough("audit", args, kwargs)
}

func (n *NpmReceiver) ci(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return n.passThrough("ci", args, kwargs)
}

func (n *NpmReceiver) cache(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return n.passThrough("cache", args, kwargs)
}

func (n *NpmReceiver) config(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return n.passThrough("config", args, kwargs)
}

// =============================================================================
// Query Operations
// =============================================================================

func (n *NpmReceiver) installed(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	var global = true

	if len(args) >= 1 {
		s, ok := starlark.AsString(args[0])
		if !ok {
			return nil, fmt.Errorf("npm.installed: package name must be a string") //nolint:staticcheck // error from API context
		}
		name = s
	}

	for _, kv := range kwargs {
		switch string(kv[0].(starlark.String)) {
		case "name":
			name = string(kv[1].(starlark.String))
		case "global":
			global = bool(kv[1].(starlark.Bool))
		}
	}

	if name == "" {
		return nil, fmt.Errorf("npm.installed: package name required") //nolint:staticcheck // error from API context
	}

	cmdArgs := []string{"list", "--depth=0", "--json"}
	if global {
		cmdArgs = append(cmdArgs, "-g")
	}

	cmd := exec.Command("npm", cmdArgs...)
	output, _ := cmd.Output()

	var result struct {
		Dependencies map[string]interface{} `json:"dependencies"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return starlark.False, nil
	}

	_, found := result.Dependencies[name]
	return starlark.Bool(found), nil
}

func (n *NpmReceiver) version(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	var global = true

	if len(args) >= 1 {
		s, ok := starlark.AsString(args[0])
		if !ok {
			return nil, fmt.Errorf("npm.version: package name must be a string") //nolint:staticcheck // error from API context
		}
		name = s
	}

	for _, kv := range kwargs {
		switch string(kv[0].(starlark.String)) {
		case "name":
			name = string(kv[1].(starlark.String))
		case "global":
			global = bool(kv[1].(starlark.Bool))
		}
	}

	if name == "" {
		return nil, fmt.Errorf("npm.version: package name required") //nolint:staticcheck // error from API context
	}

	cmdArgs := []string{"list", "--depth=0", "--json"}
	if global {
		cmdArgs = append(cmdArgs, "-g")
	}

	cmd := exec.Command("npm", cmdArgs...)
	output, _ := cmd.Output()

	var result struct {
		Dependencies map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return starlark.String(""), nil
	}

	if dep, found := result.Dependencies[name]; found {
		return starlark.String(dep.Version), nil
	}
	return starlark.String(""), nil
}

func (n *NpmReceiver) listGlobal(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	cmd := exec.Command("npm", "list", "-g", "--depth=0", "--json")
	output, _ := cmd.Output()

	var result struct {
		Dependencies map[string]interface{} `json:"dependencies"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return starlark.NewList(nil), nil
	}

	packages := make([]starlark.Value, 0, len(result.Dependencies))
	for name := range result.Dependencies {
		packages = append(packages, starlark.String(name))
	}

	return starlark.NewList(packages), nil
}

func (n *NpmReceiver) prefix(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	cmd := exec.Command("npm", "config", "get", "prefix")
	output, err := cmd.Output()
	if err != nil {
		return starlark.String(""), nil
	}
	return starlark.String(strings.TrimSpace(string(output))), nil
}

// =============================================================================
// Kwargs Pass-Through Implementation
// =============================================================================

func (n *NpmReceiver) passThrough(subcommand string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	cmdArgs := []string{subcommand}
	cmdArgs = append(cmdArgs, npmKwargsToFlags(kwargs)...)

	for _, arg := range args {
		cmdArgs = append(cmdArgs, argToString(arg))
	}

	_, _ = fmt.Fprintf(n.output, "  [npm] %s\n", strings.Join(cmdArgs, " "))

	return n.runNpm(cmdArgs)
}

// npmKwargsToFlags converts Starlark kwargs to npm CLI flags.
func npmKwargsToFlags(kwargs []starlark.Tuple) []string {
	var flags []string

	for _, kv := range kwargs {
		key := strings.ReplaceAll(string(kv[0].(starlark.String)), "_", "-")
		val := kv[1]

		switch v := val.(type) {
		case starlark.Bool:
			if bool(v) {
				if key == "g" || key == "D" || key == "S" {
					flags = append(flags, "-"+key)
				} else {
					flags = append(flags, "--"+key)
				}
			}
		case starlark.String:
			if s := string(v); s != "" {
				flags = append(flags, "--"+key, s)
			}
		case starlark.Int:
			i, _ := v.Int64()
			flags = append(flags, "--"+key, fmt.Sprintf("%d", i))
		case *starlark.List:
			for i := 0; i < v.Len(); i++ {
				flags = append(flags, "--"+key, argToString(v.Index(i)))
			}
		default:
			flags = append(flags, "--"+key, val.String())
		}
	}

	return flags
}

func (n *NpmReceiver) runNpm(args []string) (starlark.Value, error) {
	cmd := exec.Command("npm", args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			code = -1
		}
	}

	return starlarkstruct.FromStringDict(starlark.String("result"), starlark.StringDict{
		"ok":     starlark.Bool(code == 0),
		"stdout": starlark.String(strings.TrimSpace(stdout.String())),
		"stderr": starlark.String(strings.TrimSpace(stderr.String())),
		"code":   starlark.MakeInt(code),
	}), nil
}

func (n *NpmReceiver) runNpx(args []string) (starlark.Value, error) {
	cmd := exec.Command("npx", args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			code = -1
		}
	}

	return starlarkstruct.FromStringDict(starlark.String("result"), starlark.StringDict{
		"ok":     starlark.Bool(code == 0),
		"stdout": starlark.String(strings.TrimSpace(stdout.String())),
		"stderr": starlark.String(strings.TrimSpace(stderr.String())),
		"code":   starlark.MakeInt(code),
	}), nil
}
