// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"
	"os/exec"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// GitBindings provides the git.* API to Starlark scripts.
//
// Uses kwargs pass-through: any keyword argument is converted to a CLI flag.
// This means all git flags work automatically without explicit binding code.
//
// Example:
//
//	git.clone("https://github.com/user/repo", depth=1, branch="main")
//	# Executes: git clone --depth 1 --branch main https://github.com/user/repo
type GitBindings struct {
	bindings *Bindings
}

// NewGitBindings creates git bindings attached to the parent bindings.
func NewGitBindings(b *Bindings) *GitBindings {
	return &GitBindings{bindings: b}
}

// Struct returns the git.* namespace for Starlark.
func (g *GitBindings) Struct() *starlarkstruct.Struct {
	return starlarkstruct.FromStringDict(starlark.String("git"), starlark.StringDict{
		// Core operations (kwargs pass-through)
		"clone":    starlark.NewBuiltin("git.clone", g.clone),
		"pull":     starlark.NewBuiltin("git.pull", g.pull),
		"push":     starlark.NewBuiltin("git.push", g.push),
		"fetch":    starlark.NewBuiltin("git.fetch", g.fetch),
		"checkout": starlark.NewBuiltin("git.checkout", g.checkout),
		"branch":   starlark.NewBuiltin("git.branch", g.branch),
		"merge":    starlark.NewBuiltin("git.merge", g.merge),
		"rebase":   starlark.NewBuiltin("git.rebase", g.rebaseCmd),
		"reset":    starlark.NewBuiltin("git.reset", g.reset),
		"stash":    starlark.NewBuiltin("git.stash", g.stash),
		"tag":      starlark.NewBuiltin("git.tag", g.tag),
		"add":      starlark.NewBuiltin("git.add", g.add),
		"commit":   starlark.NewBuiltin("git.commit", g.commit),
		"status":   starlark.NewBuiltin("git.status", g.status),
		"diff":     starlark.NewBuiltin("git.diff", g.diff),
		"log":      starlark.NewBuiltin("git.log", g.log),
		"remote":   starlark.NewBuiltin("git.remote", g.remote),

		// Config operations
		"config_get": starlark.NewBuiltin("git.config_get", g.configGet),
		"config_set": starlark.NewBuiltin("git.config_set", g.configSet),

		// Query operations (special return types)
		"installed":      starlark.NewBuiltin("git.installed", g.installed),
		"version":        starlark.NewBuiltin("git.version", g.version),
		"repo_root":      starlark.NewBuiltin("git.repo_root", g.repoRoot),
		"current_branch": starlark.NewBuiltin("git.current_branch", g.currentBranch),
		"remote_url":     starlark.NewBuiltin("git.remote_url", g.remoteURL),
		"is_clean":       starlark.NewBuiltin("git.is_clean", g.isClean),
		"latest_tag":     starlark.NewBuiltin("git.latest_tag", g.latestTag),
		"commit_hash":    starlark.NewBuiltin("git.commit_hash", g.commitHash),
	})
}

// =============================================================================
// Core Operations (kwargs pass-through)
// =============================================================================

// clone clones a repository.
// git.clone("https://github.com/user/repo", "/tmp/repo", depth=1, branch="main")
func (g *GitBindings) clone(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("clone", args, kwargs)
}

// pull pulls from remote.
// git.pull(rebase=True, ff_only=True)
func (g *GitBindings) pull(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("pull", args, kwargs)
}

// push pushes to remote.
// git.push("origin", "main", force=True, tags=True)
func (g *GitBindings) push(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("push", args, kwargs)
}

// fetch fetches from remote.
// git.fetch("origin", all=True, prune=True)
func (g *GitBindings) fetch(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("fetch", args, kwargs)
}

// checkout checks out a branch, tag, or commit.
// git.checkout("main") or git.checkout("feature", b=True)
func (g *GitBindings) checkout(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("checkout", args, kwargs)
}

// branch manages branches.
// git.branch("new-feature") or git.branch(delete="old-branch")
func (g *GitBindings) branch(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("branch", args, kwargs)
}

// merge merges branches.
// git.merge("feature-branch", no_ff=True)
func (g *GitBindings) merge(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("merge", args, kwargs)
}

// rebaseCmd rebases commits.
// git.rebase("main", interactive=True)
func (g *GitBindings) rebaseCmd(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("rebase", args, kwargs)
}

// reset resets HEAD.
// git.reset("HEAD~1", hard=True)
func (g *GitBindings) reset(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("reset", args, kwargs)
}

// stash stashes changes.
// git.stash() or git.stash("pop") or git.stash("push", message="WIP")
func (g *GitBindings) stash(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("stash", args, kwargs)
}

// tag manages tags.
// git.tag("v1.0.0", annotate=True, message="Release 1.0")
func (g *GitBindings) tag(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("tag", args, kwargs)
}

// add stages files.
// git.add(".") or git.add("file.txt", "other.txt", all=True)
func (g *GitBindings) add(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("add", args, kwargs)
}

// commit commits staged changes.
// git.commit(message="Fix bug", all=True)
func (g *GitBindings) commit(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("commit", args, kwargs)
}

// status shows working tree status.
// git.status(short=True, branch=True)
func (g *GitBindings) status(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("status", args, kwargs)
}

// diff shows changes.
// git.diff("HEAD~1", cached=True, stat=True)
func (g *GitBindings) diff(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("diff", args, kwargs)
}

// log shows commit history.
// git.log(oneline=True, n=10, graph=True)
func (g *GitBindings) log(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("log", args, kwargs)
}

// remote manages remotes.
// git.remote("add", "upstream", "https://...") or git.remote(verbose=True)
func (g *GitBindings) remote(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return g.passThrough("remote", args, kwargs)
}

// =============================================================================
// Config Operations
// =============================================================================

// configGet gets a git config value.
// git.config_get("user.email") -> "user@example.com"
func (g *GitBindings) configGet(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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

// configSet sets a git config value.
// git.config_set("user.email", "user@example.com", global=True)
func (g *GitBindings) configSet(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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

	fmt.Fprintf(g.bindings.output, "  [git] config %s = %s\n", key, value)

	return g.runGit(cmdArgs)
}

// =============================================================================
// Query Operations (special return types)
// =============================================================================

// installed checks if git is available.
// git.installed() -> True/False
func (g *GitBindings) installed(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	_, err := exec.LookPath("git")
	return starlark.Bool(err == nil), nil
}

// version returns the git version.
// git.version() -> "2.43.0"
func (g *GitBindings) version(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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

// repoRoot returns the repository root directory.
// git.repo_root() -> "/path/to/repo" or ""
func (g *GitBindings) repoRoot(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return starlark.String(""), nil
	}
	return starlark.String(strings.TrimSpace(string(output))), nil
}

// currentBranch returns the current branch name.
// git.current_branch() -> "main"
func (g *GitBindings) currentBranch(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return starlark.String(""), nil
	}
	return starlark.String(strings.TrimSpace(string(output))), nil
}

// remoteURL returns the URL of a remote.
// git.remote_url() or git.remote_url("upstream")
func (g *GitBindings) remoteURL(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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

// isClean returns true if the working directory is clean.
// git.is_clean() -> True/False
func (g *GitBindings) isClean(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return starlark.False, nil
	}
	return starlark.Bool(len(strings.TrimSpace(string(output))) == 0), nil
}

// latestTag returns the most recent tag.
// git.latest_tag() -> "v1.2.3" or ""
func (g *GitBindings) latestTag(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	cmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
	output, err := cmd.Output()
	if err != nil {
		return starlark.String(""), nil
	}
	return starlark.String(strings.TrimSpace(string(output))), nil
}

// commitHash returns the current commit hash.
// git.commit_hash() or git.commit_hash(short=True)
func (g *GitBindings) commitHash(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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

// passThrough converts Starlark args/kwargs to git CLI arguments.
func (g *GitBindings) passThrough(subcommand string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	cmdArgs := []string{subcommand}

	// Convert kwargs to flags
	cmdArgs = append(cmdArgs, g.kwargsToFlags(kwargs)...)

	// Append positional args
	for _, arg := range args {
		cmdArgs = append(cmdArgs, argToString(arg))
	}

	fmt.Fprintf(g.bindings.output, "  [git] %s\n", strings.Join(cmdArgs, " "))

	return g.runGit(cmdArgs)
}

// kwargsToFlags converts Starlark kwargs to CLI flags.
func (g *GitBindings) kwargsToFlags(kwargs []starlark.Tuple) []string {
	var flags []string

	for _, kv := range kwargs {
		key := strings.ReplaceAll(string(kv[0].(starlark.String)), "_", "-")
		val := kv[1]

		switch v := val.(type) {
		case starlark.Bool:
			if bool(v) {
				// Single-char flags use single dash
				if len(key) == 1 {
					flags = append(flags, "-"+key)
				} else {
					flags = append(flags, "--"+key)
				}
			}
		case starlark.String:
			if s := string(v); s != "" {
				if len(key) == 1 {
					flags = append(flags, "-"+key, s)
				} else {
					flags = append(flags, "--"+key, s)
				}
			}
		case starlark.Int:
			i, _ := v.Int64()
			if len(key) == 1 {
				flags = append(flags, "-"+key, fmt.Sprintf("%d", i))
			} else {
				flags = append(flags, "--"+key, fmt.Sprintf("%d", i))
			}
		case *starlark.List:
			for i := 0; i < v.Len(); i++ {
				if len(key) == 1 {
					flags = append(flags, "-"+key, argToString(v.Index(i)))
				} else {
					flags = append(flags, "--"+key, argToString(v.Index(i)))
				}
			}
		default:
			flags = append(flags, "--"+key, val.String())
		}
	}

	return flags
}

// runGit executes git with the given arguments.
func (g *GitBindings) runGit(args []string) (starlark.Value, error) {
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
