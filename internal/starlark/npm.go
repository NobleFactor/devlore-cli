// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package starlark

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// NpmBindings provides the npm.* API to Starlark scripts.
//
// Uses kwargs pass-through: any keyword argument is converted to a CLI flag.
// This means all npm flags work automatically without explicit binding code.
//
// Example:
//
//	npm.install("astro", "tailwind", global=True, save_dev=True)
//	# Executes: npm install --global --save-dev astro tailwind
type NpmBindings struct {
	bindings *Bindings
}

// NewNpmBindings creates npm bindings attached to the parent bindings.
func NewNpmBindings(b *Bindings) *NpmBindings {
	return &NpmBindings{bindings: b}
}

// Struct returns the npm.* namespace for Starlark.
func (n *NpmBindings) Struct() *starlarkstruct.Struct {
	return starlarkstruct.FromStringDict(starlark.String("npm"), starlark.StringDict{
		// Core operations (kwargs pass-through)
		"install":   starlark.NewBuiltin("npm.install", n.install),
		"uninstall": starlark.NewBuiltin("npm.uninstall", n.uninstall),
		"update":    starlark.NewBuiltin("npm.update", n.update),
		"run":       starlark.NewBuiltin("npm.run", n.run),
		"exec":      starlark.NewBuiltin("npm.exec", n.execCmd),
		"init":      starlark.NewBuiltin("npm.init", n.init),
		"publish":   starlark.NewBuiltin("npm.publish", n.publish),
		"pack":      starlark.NewBuiltin("npm.pack", n.pack),
		"link":      starlark.NewBuiltin("npm.link", n.link),
		"audit":     starlark.NewBuiltin("npm.audit", n.audit),
		"ci":        starlark.NewBuiltin("npm.ci", n.ci),
		"cache":     starlark.NewBuiltin("npm.cache", n.cache),
		"config":    starlark.NewBuiltin("npm.config", n.config),

		// Query operations (special return types)
		"installed":   starlark.NewBuiltin("npm.installed", n.installed),
		"version":     starlark.NewBuiltin("npm.version", n.version),
		"list_global": starlark.NewBuiltin("npm.list_global", n.listGlobal),
		"prefix":      starlark.NewBuiltin("npm.prefix", n.prefix),
	})
}

// =============================================================================
// Core Operations (kwargs pass-through)
// =============================================================================

// install installs packages.
// npm.install("astro", "tailwind", global=True, save_dev=True)
func (n *NpmBindings) install(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return n.passThrough("install", args, kwargs)
}

// uninstall removes packages.
// npm.uninstall("astro", global=True)
func (n *NpmBindings) uninstall(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return n.passThrough("uninstall", args, kwargs)
}

// update updates packages.
// npm.update("astro", global=True)
func (n *NpmBindings) update(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return n.passThrough("update", args, kwargs)
}

// run executes an npm script from package.json.
// npm.run("build") or npm.run("test", silent=True)
func (n *NpmBindings) run(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return n.passThrough("run", args, kwargs)
}

// execCmd runs npx to execute a package binary.
// npm.exec("create-astro@latest", yes=True)
func (n *NpmBindings) execCmd(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	cmdArgs := []string{}

	// Convert kwargs to flags
	cmdArgs = append(cmdArgs, n.kwargsToFlags(kwargs)...)

	// Append positional args
	for _, arg := range args {
		cmdArgs = append(cmdArgs, argToString(arg))
	}

	fmt.Fprintf(n.bindings.output, "  [npx] %s\n", strings.Join(cmdArgs, " "))

	return n.runNpx(cmdArgs)
}

// init initializes a new package.json.
// npm.init(yes=True, scope="@myorg")
func (n *NpmBindings) init(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return n.passThrough("init", args, kwargs)
}

// publish publishes a package.
// npm.publish(access="public", tag="beta")
func (n *NpmBindings) publish(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return n.passThrough("publish", args, kwargs)
}

// pack creates a tarball from a package.
// npm.pack(dry_run=True)
func (n *NpmBindings) pack(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return n.passThrough("pack", args, kwargs)
}

// link symlinks a package folder.
// npm.link("my-package") or npm.link()
func (n *NpmBindings) link(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return n.passThrough("link", args, kwargs)
}

// audit runs a security audit.
// npm.audit(fix=True, audit_level="high")
func (n *NpmBindings) audit(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return n.passThrough("audit", args, kwargs)
}

// ci installs from package-lock.json.
// npm.ci(ignore_scripts=True)
func (n *NpmBindings) ci(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return n.passThrough("ci", args, kwargs)
}

// cache manages the npm cache.
// npm.cache("clean", force=True) or npm.cache("verify")
func (n *NpmBindings) cache(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return n.passThrough("cache", args, kwargs)
}

// config manages npm configuration.
// npm.config("set", "registry", "https://...") or npm.config("get", "prefix")
func (n *NpmBindings) config(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return n.passThrough("config", args, kwargs)
}

// =============================================================================
// Query Operations (special return types)
// =============================================================================

// installed checks if a package is installed.
// npm.installed("astro") -> True/False
func (n *NpmBindings) installed(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	var global bool = true

	if len(args) >= 1 {
		s, ok := starlark.AsString(args[0])
		if !ok {
			return nil, fmt.Errorf("npm.installed: package name must be a string")
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
		return nil, fmt.Errorf("npm.installed: package name required")
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

// version returns the installed version of a package.
// npm.version("astro") -> "5.1.3" or ""
func (n *NpmBindings) version(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	var global bool = true

	if len(args) >= 1 {
		s, ok := starlark.AsString(args[0])
		if !ok {
			return nil, fmt.Errorf("npm.version: package name must be a string")
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
		return nil, fmt.Errorf("npm.version: package name required")
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

// listGlobal returns a list of globally installed packages.
// npm.list_global() -> ["astro", "create-astro", ...]
func (n *NpmBindings) listGlobal(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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

// prefix returns the global prefix path.
// npm.prefix() -> "/usr/local" or "~/.npm-global"
func (n *NpmBindings) prefix(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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

// passThrough converts Starlark args/kwargs to npm CLI arguments.
func (n *NpmBindings) passThrough(subcommand string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	cmdArgs := []string{subcommand}

	// Convert kwargs to flags
	cmdArgs = append(cmdArgs, n.kwargsToFlags(kwargs)...)

	// Append positional args
	for _, arg := range args {
		cmdArgs = append(cmdArgs, argToString(arg))
	}

	fmt.Fprintf(n.bindings.output, "  [npm] %s\n", strings.Join(cmdArgs, " "))

	return n.runNpm(cmdArgs)
}

// kwargsToFlags converts Starlark kwargs to CLI flags.
func (n *NpmBindings) kwargsToFlags(kwargs []starlark.Tuple) []string {
	var flags []string

	for _, kv := range kwargs {
		key := strings.ReplaceAll(string(kv[0].(starlark.String)), "_", "-")
		val := kv[1]

		switch v := val.(type) {
		case starlark.Bool:
			if bool(v) {
				// npm uses single-char shortcuts: -g, -D, -S
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

// runNpm executes npm with the given arguments.
func (n *NpmBindings) runNpm(args []string) (starlark.Value, error) {
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

// runNpx executes npx with the given arguments.
func (n *NpmBindings) runNpx(args []string) (starlark.Value, error) {
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
