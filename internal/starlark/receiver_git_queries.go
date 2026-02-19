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

	"github.com/NobleFactor/devlore-cli/internal/execution/provider/git"
)

// GitReceiver provides the git.* Starlark namespace.
// Forward operations (clone, checkout, pull) delegate to git.Provider.
// All other operations use kwargs pass-through via exec.Command("git", ...).
type GitReceiver struct {
	Receiver
	provider *git.Provider
	output   io.Writer
}

// NewGitReceiver creates a new git receiver.
func NewGitReceiver(provider *git.Provider, output io.Writer) *GitReceiver {
	return &GitReceiver{
		Receiver: NewReceiver("git"),
		provider: provider,
		output:   output,
	}
}

func (r *GitReceiver) queryAttr(name string) (starlark.Value, error) {
	switch name {
	// Pass-through operations (kwargs → CLI flags)
	case "push":
		return MakeAttr("git.push", r.push), nil
	case "fetch":
		return MakeAttr("git.fetch", r.fetch), nil
	case "branch":
		return MakeAttr("git.branch", r.branch), nil
	case "merge":
		return MakeAttr("git.merge", r.merge), nil
	case "rebase":
		return MakeAttr("git.rebase", r.rebaseCmd), nil
	case "reset":
		return MakeAttr("git.reset", r.reset), nil
	case "stash":
		return MakeAttr("git.stash", r.stash), nil
	case "tag":
		return MakeAttr("git.tag", r.tag), nil
	case "add":
		return MakeAttr("git.add", r.add), nil
	case "commit":
		return MakeAttr("git.commit", r.commit), nil
	case "status":
		return MakeAttr("git.status", r.gitStatus), nil
	case "diff":
		return MakeAttr("git.diff", r.diff), nil
	case "log":
		return MakeAttr("git.log", r.gitLog), nil
	case "remote":
		return MakeAttr("git.remote", r.remote), nil
	// Config operations
	case "config_get":
		return MakeAttr("git.config_get", r.configGet), nil
	case "config_set":
		return MakeAttr("git.config_set", r.configSet), nil
	// Query operations
	case "installed":
		return MakeAttr("git.installed", r.installed), nil
	case "version":
		return MakeAttr("git.version", r.gitVersion), nil
	case "repo_root":
		return MakeAttr("git.repo_root", r.repoRoot), nil
	case "current_branch":
		return MakeAttr("git.current_branch", r.currentBranch), nil
	case "remote_url":
		return MakeAttr("git.remote_url", r.remoteURL), nil
	case "is_clean":
		return MakeAttr("git.is_clean", r.isClean), nil
	case "latest_tag":
		return MakeAttr("git.latest_tag", r.latestTag), nil
	case "commit_hash":
		return MakeAttr("git.commit_hash", r.commitHash), nil
	default:
		return nil, NoSuchAttrError("git", name)
	}
}

// =============================================================================
// Pass-Through Operations (kwargs → CLI flags)
// =============================================================================

func (r *GitReceiver) push(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return r.passThrough("push", args, kwargs)
}

func (r *GitReceiver) fetch(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return r.passThrough("fetch", args, kwargs)
}

func (r *GitReceiver) branch(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return r.passThrough("branch", args, kwargs)
}

func (r *GitReceiver) merge(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return r.passThrough("merge", args, kwargs)
}

func (r *GitReceiver) rebaseCmd(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return r.passThrough("rebase", args, kwargs)
}

func (r *GitReceiver) reset(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return r.passThrough("reset", args, kwargs)
}

func (r *GitReceiver) stash(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return r.passThrough("stash", args, kwargs)
}

func (r *GitReceiver) tag(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return r.passThrough("tag", args, kwargs)
}

func (r *GitReceiver) add(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return r.passThrough("add", args, kwargs)
}

func (r *GitReceiver) commit(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return r.passThrough("commit", args, kwargs)
}

func (r *GitReceiver) gitStatus(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return r.passThrough("status", args, kwargs)
}

func (r *GitReceiver) diff(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return r.passThrough("diff", args, kwargs)
}

func (r *GitReceiver) gitLog(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return r.passThrough("log", args, kwargs)
}

func (r *GitReceiver) remote(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return r.passThrough("remote", args, kwargs)
}

// =============================================================================
// Config Operations
// =============================================================================

func (r *GitReceiver) configGet(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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

func (r *GitReceiver) configSet(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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

	_, _ = fmt.Fprintf(r.output, "  [git] config %s = %s\n", key, value)
	return r.runGit(cmdArgs)
}

// =============================================================================
// Query Operations
// =============================================================================

func (r *GitReceiver) installed(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	_, err := exec.LookPath("git")
	return starlark.Bool(err == nil), nil
}

func (r *GitReceiver) gitVersion(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
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

func (r *GitReceiver) repoRoot(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return starlark.String(""), nil
	}
	return starlark.String(strings.TrimSpace(string(output))), nil
}

func (r *GitReceiver) currentBranch(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return starlark.String(""), nil
	}
	return starlark.String(strings.TrimSpace(string(output))), nil
}

func (r *GitReceiver) remoteURL(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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

func (r *GitReceiver) isClean(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return starlark.False, nil
	}
	return starlark.Bool(len(strings.TrimSpace(string(output))) == 0), nil
}

func (r *GitReceiver) latestTag(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	cmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
	output, err := cmd.Output()
	if err != nil {
		return starlark.String(""), nil
	}
	return starlark.String(strings.TrimSpace(string(output))), nil
}

func (r *GitReceiver) commitHash(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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

func (r *GitReceiver) passThrough(subcommand string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	cmdArgs := []string{subcommand}
	cmdArgs = append(cmdArgs, kwargsToFlags(kwargs)...)
	for _, arg := range args {
		cmdArgs = append(cmdArgs, argToString(arg))
	}

	_, _ = fmt.Fprintf(r.output, "  [git] %s\n", strings.Join(cmdArgs, " "))
	return r.runGit(cmdArgs)
}

func (r *GitReceiver) runGit(args []string) (starlark.Value, error) {
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
