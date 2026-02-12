// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"
	"io"
	"os/exec"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// GitReceiver provides the git.* Starlark namespace.
//
// Backing implementation: os/exec (exec.Command("git", ...)).
// Uses kwargs pass-through: any keyword argument is converted to a CLI flag.
// This means all git flags work automatically without explicit binding code.
//
// Example:
//
//	git.clone("https://github.com/user/repo", depth=1, branch="main")
//	# Executes: git clone --depth 1 --branch main https://github.com/user/repo
type GitReceiver struct {
	Receiver
	output io.Writer
}

// NewGitReceiver creates a new git receiver.
func NewGitReceiver(output io.Writer) *GitReceiver {
	return &GitReceiver{Receiver: NewReceiver("git"), output: output}
}

// Attr implements starlark.HasAttrs.
func (g *GitReceiver) Attr(name string) (starlark.Value, error) {
	switch name {
	case "clone":
		return MakeAttr("git.clone", g.clone), nil
	case "pull":
		return MakeAttr("git.pull", g.pull), nil
	case "push":
		return MakeAttr("git.push", g.push), nil
	case "fetch":
		return MakeAttr("git.fetch", g.fetch), nil
	case "checkout":
		return MakeAttr("git.checkout", g.checkout), nil
	case "branch":
		return MakeAttr("git.branch", g.branch), nil
	case "merge":
		return MakeAttr("git.merge", g.merge), nil
	case "rebase":
		return MakeAttr("git.rebase", g.rebaseCmd), nil
	case "reset":
		return MakeAttr("git.reset", g.reset), nil
	case "stash":
		return MakeAttr("git.stash", g.stash), nil
	case "tag":
		return MakeAttr("git.tag", g.tag), nil
	case "add":
		return MakeAttr("git.add", g.add), nil
	case "commit":
		return MakeAttr("git.commit", g.commit), nil
	case "status":
		return MakeAttr("git.status", g.status), nil
	case "diff":
		return MakeAttr("git.diff", g.diff), nil
	case "log":
		return MakeAttr("git.log", g.log), nil
	case "remote":
		return MakeAttr("git.remote", g.remote), nil
	case "config_get":
		return MakeAttr("git.config_get", g.configGet), nil
	case "config_set":
		return MakeAttr("git.config_set", g.configSet), nil
	case "installed":
		return MakeAttr("git.installed", g.installed), nil
	case "version":
		return MakeAttr("git.version", g.version), nil
	case "repo_root":
		return MakeAttr("git.repo_root", g.repoRoot), nil
	case "current_branch":
		return MakeAttr("git.current_branch", g.currentBranch), nil
	case "remote_url":
		return MakeAttr("git.remote_url", g.remoteURL), nil
	case "is_clean":
		return MakeAttr("git.is_clean", g.isClean), nil
	case "latest_tag":
		return MakeAttr("git.latest_tag", g.latestTag), nil
	case "commit_hash":
		return MakeAttr("git.commit_hash", g.commitHash), nil
	default:
		return nil, NoSuchAttrError("git", name)
	}
}

// AttrNames implements starlark.HasAttrs.
func (g *GitReceiver) AttrNames() []string {
	return []string{
		"add", "branch", "checkout", "clone", "commit", "commit_hash",
		"config_get", "config_set", "current_branch", "diff", "fetch",
		"installed", "is_clean", "latest_tag", "log", "merge", "pull",
		"push", "rebase", "remote", "remote_url", "repo_root", "reset",
		"stash", "status", "tag", "version",
	}
}

// =============================================================================
// Core Operations (kwargs pass-through)
// =============================================================================

func (g *GitReceiver) clone(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("clone", args, kwargs)
}

func (g *GitReceiver) pull(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("pull", args, kwargs)
}

func (g *GitReceiver) push(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("push", args, kwargs)
}

func (g *GitReceiver) fetch(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("fetch", args, kwargs)
}

func (g *GitReceiver) checkout(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("checkout", args, kwargs)
}

func (g *GitReceiver) branch(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("branch", args, kwargs)
}

func (g *GitReceiver) merge(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("merge", args, kwargs)
}

func (g *GitReceiver) rebaseCmd(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("rebase", args, kwargs)
}

func (g *GitReceiver) reset(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("reset", args, kwargs)
}

func (g *GitReceiver) stash(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("stash", args, kwargs)
}

func (g *GitReceiver) tag(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("tag", args, kwargs)
}

func (g *GitReceiver) add(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("add", args, kwargs)
}

func (g *GitReceiver) commit(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("commit", args, kwargs)
}

func (g *GitReceiver) status(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("status", args, kwargs)
}

func (g *GitReceiver) diff(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("diff", args, kwargs)
}

func (g *GitReceiver) log(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("log", args, kwargs)
}

func (g *GitReceiver) remote(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("remote", args, kwargs)
}

// =============================================================================
// Config Operations
// =============================================================================

func (g *GitReceiver) configGet(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var key string
	var global bool

	if len(args) >= 1 {
		s, _ := starlark.AsString(args[0])
		key = s
	}

	for _, kv := range kwargs {
		k := string(kv[0].(starlark.String))
		switch k {
		case "key":
			key = string(kv[1].(starlark.String))
		case "global":
			global = bool(kv[1].(starlark.Bool))
		}
	}

	if key == "" {
		return nil, fmt.Errorf("git.config_get: key required")
	}

	cmdArgs := []string{"config"}
	if global {
		cmdArgs = append(cmdArgs, "--global")
	}
	cmdArgs = append(cmdArgs, "--get", key)

	cmd := exec.Command("git", cmdArgs...)
	output, err := cmd.Output()
	if err != nil {
		return starlark.String(""), nil
	}
	return starlark.String(strings.TrimSpace(string(output))), nil
}

func (g *GitReceiver) configSet(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var key, value string
	var global bool

	if len(args) >= 2 {
		s, _ := starlark.AsString(args[0])
		key = s
		s, _ = starlark.AsString(args[1])
		value = s
	}

	for _, kv := range kwargs {
		k := string(kv[0].(starlark.String))
		switch k {
		case "key":
			key = string(kv[1].(starlark.String))
		case "value":
			value = string(kv[1].(starlark.String))
		case "global":
			global = bool(kv[1].(starlark.Bool))
		}
	}

	if key == "" || value == "" {
		return nil, fmt.Errorf("git.config_set: key and value required")
	}

	cmdArgs := []string{"config"}
	if global {
		cmdArgs = append(cmdArgs, "--global")
	}
	cmdArgs = append(cmdArgs, key, value)

	_, _ = fmt.Fprintf(g.output, "  [git] config %s = %s\n", key, value)

	return g.runGit(cmdArgs)
}

// =============================================================================
// Query Operations
// =============================================================================

func (g *GitReceiver) installed(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	_, err := exec.LookPath("git")
	return starlark.Bool(err == nil), nil
}

func (g *GitReceiver) version(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	cmd := exec.Command("git", "--version")
	output, err := cmd.Output()
	if err != nil {
		return starlark.String(""), nil
	}

	parts := strings.Fields(string(output))
	if len(parts) >= 3 {
		return starlark.String(parts[2]), nil
	}
	return starlark.String(strings.TrimSpace(string(output))), nil
}

func (g *GitReceiver) repoRoot(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return starlark.String(""), nil
	}
	return starlark.String(strings.TrimSpace(string(output))), nil
}

func (g *GitReceiver) currentBranch(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return starlark.String(""), nil
	}
	return starlark.String(strings.TrimSpace(string(output))), nil
}

func (g *GitReceiver) remoteURL(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	remote := "origin"

	if len(args) >= 1 {
		s, _ := starlark.AsString(args[0])
		remote = s
	}

	for _, kv := range kwargs {
		if string(kv[0].(starlark.String)) == "remote" {
			remote = string(kv[1].(starlark.String))
		}
	}

	cmd := exec.Command("git", "remote", "get-url", remote)
	output, err := cmd.Output()
	if err != nil {
		return starlark.String(""), nil
	}
	return starlark.String(strings.TrimSpace(string(output))), nil
}

func (g *GitReceiver) isClean(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return starlark.False, nil
	}
	return starlark.Bool(len(strings.TrimSpace(string(output))) == 0), nil
}

func (g *GitReceiver) latestTag(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	cmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
	output, err := cmd.Output()
	if err != nil {
		return starlark.String(""), nil
	}
	return starlark.String(strings.TrimSpace(string(output))), nil
}

func (g *GitReceiver) commitHash(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var short bool

	for _, kv := range kwargs {
		if string(kv[0].(starlark.String)) == "short" {
			short = bool(kv[1].(starlark.Bool))
		}
	}

	cmdArgs := []string{"rev-parse"}
	if short {
		cmdArgs = append(cmdArgs, "--short")
	}
	cmdArgs = append(cmdArgs, "HEAD")

	cmd := exec.Command("git", cmdArgs...)
	output, err := cmd.Output()
	if err != nil {
		return starlark.String(""), nil
	}
	return starlark.String(strings.TrimSpace(string(output))), nil
}

// =============================================================================
// Kwargs Pass-Through Implementation
// =============================================================================

func (g *GitReceiver) passThrough(subcommand string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	cmdArgs := []string{subcommand}
	cmdArgs = append(cmdArgs, kwargsToFlags(kwargs)...)

	for _, arg := range args {
		cmdArgs = append(cmdArgs, argToString(arg))
	}

	_, _ = fmt.Fprintf(g.output, "  [git] %s\n", strings.Join(cmdArgs, " "))

	return g.runGit(cmdArgs)
}

func (g *GitReceiver) runGit(args []string) (starlark.Value, error) {
	cmd := exec.Command("git", args...)
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
