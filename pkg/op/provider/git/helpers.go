// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// buildCloneArgs composes the argv passed to `git clone` from Provider.Clone's parameters.
//
// Known options are emitted first in alphabetical order (by flag name); unknown kwargs are then emitted in
// alphabetical order by key, each transformed via the kwarg→flag rule (`_` → `-`, `--` prefix). Positional
// arguments (repository, directory) are appended last. The leading "clone" subcommand is included so the slice
// is a complete argv for `exec.Command("git", args...)`.
//
// Value handling for the kwargs map: bool emits the flag when true and nothing when false; empty string skips;
// int/int64/float64 format as `--flag=<value>`; nil skips; any other type is formatted via fmt.Sprint.
//
// Parameters:
//   - repository:        remote git URL (HTTPS, SSH, git protocol, or local path) to clone from.
//   - directory:         local filesystem path where the repository will be cloned; must be non-empty.
//   - bare:              when true, append `--bare`.
//   - branch:            when non-empty, append `--branch <branch>`.
//   - depth:             when > 0, append `--depth <depth>`.
//   - filter:            when non-empty, append `--filter=<filter>`.
//   - noCheckout:        when true, append `--no-checkout`.
//   - noTags:            when true, append `--no-tags`.
//   - origin:            when non-empty, append `--origin <origin>`.
//   - recurseSubmodules: when true, append `--recurse-submodules`.
//   - singleBranch:      when true, append `--single-branch`.
//   - kwargs:            catch-all for options not in the known set; each entry becomes an additional flag.
//
// Returns:
//   - []string: the complete argv, starting with "clone".
func buildCloneArgs(
	repository string,
	directory string,
	bare bool,
	branch string,
	depth int,
	filter string,
	noCheckout bool,
	noTags bool,
	origin string,
	recurseSubmodules bool,
	singleBranch bool,
	kwargs map[string]any,
) []string {

	args := []string{"clone"}

	if bare {
		args = append(args, "--bare")
	}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	if depth > 0 {
		args = append(args, "--depth", strconv.Itoa(depth))
	}
	if filter != "" {
		args = append(args, "--filter="+filter)
	}
	if noCheckout {
		args = append(args, "--no-checkout")
	}
	if noTags {
		args = append(args, "--no-tags")
	}
	if origin != "" {
		args = append(args, "--origin", origin)
	}
	if recurseSubmodules {
		args = append(args, "--recurse-submodules")
	}
	if singleBranch {
		args = append(args, "--single-branch")
	}

	for _, name := range slices.Sorted(maps.Keys(kwargs)) {

		flag := "--" + strings.ReplaceAll(name, "_", "-")

		switch v := kwargs[name].(type) {
		case bool:
			if v {
				args = append(args, flag)
			}
		case string:
			if v != "" {
				args = append(args, flag+"="+v)
			}
		case int:
			args = append(args, flag+"="+strconv.Itoa(v))
		case int64:
			args = append(args, flag+"="+strconv.FormatInt(v, 10))
		case float64:
			args = append(args, flag+"="+strconv.FormatFloat(v, 'g', -1, 64))
		case nil:
			// nil values are skipped — caller passed the kwarg explicitly as None.
		default:
			args = append(args, flag+"="+fmt.Sprint(v))
		}
	}

	return append(args, repository, directory)
}

// cleanControlChars replaces sub-0x20 bytes and whitespace runs with a single ASCII space, stripping leading
// and trailing whitespace. Mirrors the final cleanup loop of git's git_url_basename (see [guessDirName]).
//
// Parameters:
//   - s: the input string to clean.
//
// Returns:
//   - string: the cleaned string; may be empty if s contained only whitespace/control characters.
func cleanControlChars(s string) string {

	var b strings.Builder
	b.Grow(len(s))

	prevSpace := true // initial true strips leading whitespace.

	for i := 0; i < len(s); i++ {

		c := s[i]
		if c < 0x20 {
			c = ' '
		}

		if c == ' ' {
			if prevSpace {
				continue
			}
			prevSpace = true
		} else {
			prevSpace = false
		}

		b.WriteByte(c)
	}

	out := b.String()
	if len(out) > 0 && out[len(out)-1] == ' ' {
		out = out[:len(out)-1]
	}

	return out
}

// guessDirName returns the directory name git would use for `git clone <repository>` when no directory is
// specified. Mirrors the algorithm in git's builtin/clone.c (git_url_basename) for the non-bare, non-bundle
// case — the only case this package exercises.
//
// Algorithm (in order):
//  1. Skip the URL scheme ("://" separator).
//  2. Skip authentication (greedy — up to the last '@' before the first directory separator).
//  3. Strip trailing whitespace, directory separators, and a trailing "/.git".
//  4. If the remaining substring has no '/' but does contain ':', strip a trailing port number (digits
//     after ':', then drop the ':' itself).
//  5. Walk back to the start of the last path component, stopping at '/' or ':' (colons are treated as
//     path separators for SCP-style URLs like `git@host:path/repo`).
//  6. Strip a trailing ".git" suffix.
//  7. Replace sub-0x20 and whitespace runs with a single ASCII space; trim leading/trailing whitespace
//     via [cleanControlChars].
//
// Parameters:
//   - repository: the git repository URL or local path.
//
// Returns:
//   - string: the guessed directory name.
//   - error:  non-nil if no directory name can be derived (e.g. the input is empty, is just "/",
//     or becomes empty after cleaning).
func guessDirName(repository string) (string, error) {

	end := len(repository)
	start := 0

	// (1) Skip scheme.
	if i := strings.Index(repository, "://"); i >= 0 {
		start = i + 3
	}

	// (2) Skip authentication data — greedy: last '@' before the first dir separator.
	for i := start; i < end && !isDirSep(repository[i]); i++ {
		if repository[i] == '@' {
			start = i + 1
		}
	}

	// (3) Strip trailing whitespace and directory separators, then a trailing "/.git", then trailing
	// separators again.
	for start < end && (isDirSep(repository[end-1]) || isASCIISpace(repository[end-1])) {
		end--
	}
	if end-start > 5 && isDirSep(repository[end-5]) && repository[end-4:end] == ".git" {
		end -= 5
		for start < end && isDirSep(repository[end-1]) {
			end--
		}
	}

	if end < start {
		return "", fmt.Errorf("git: no directory name could be guessed from %q", repository)
	}

	// (4) Strip trailing port number if hostname-only URL.
	if !containsByte(repository, start, end, '/') && containsByte(repository, start, end, ':') {
		ptr := end
		for start < ptr && isASCIIDigit(repository[ptr-1]) && repository[ptr-1] != ':' {
			ptr--
		}
		if start < ptr && repository[ptr-1] == ':' {
			end = ptr - 1
		}
	}

	// (5) Find last path component — walk back while not at '/' or ':'.
	ptr := end
	for start < ptr && !isDirSep(repository[ptr-1]) && repository[ptr-1] != ':' {
		ptr--
	}
	start = ptr

	// (6) Strip a trailing ".git" suffix.
	name := strings.TrimSuffix(repository[start:end], ".git")

	if name == "" || name == "/" {
		return "", fmt.Errorf("git: no directory name could be guessed from %q", repository)
	}

	// (7) Collapse control characters and whitespace runs into single spaces.
	name = cleanControlChars(name)

	if name == "" {
		return "", fmt.Errorf("git: no directory name could be guessed from %q", repository)
	}

	return name, nil
}

// containsByte reports whether s[start:end] contains c.
//
// Parameters:
//   - s:     the source string.
//   - start: inclusive start index.
//   - end:   exclusive end index.
//   - c:     the byte to search for.
//
// Returns:
//   - bool: true if c appears at any position in s[start:end].
func containsByte(s string, start, end int, c byte) bool {
	return strings.IndexByte(s[start:end], c) != -1
}

// isASCIIDigit reports whether c is an ASCII decimal digit.
//
// Parameters:
//   - c: the byte to classify.
//
// Returns:
//   - bool: true when c is in '0'..'9'.
func isASCIIDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// isASCIISpace reports whether c is an ASCII whitespace character as defined by C's isspace.
//
// Parameters:
//   - c: the byte to classify.
//
// Returns:
//   - bool: true when c is space, tab, newline, vertical tab, form feed, or carriage return.
func isASCIISpace(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\v', '\f', '\r':
		return true
	}
	return false
}

// isDirSep reports whether c is a Unix directory separator.
//
// Parameters:
//   - c: the byte to classify.
//
// Returns:
//   - bool: true when c is '/'.
func isDirSep(c byte) bool {
	return c == '/'
}

// isDirtyRepo reports whether the working tree at path has uncommitted changes.
//
// Shells out to `git -C <path> status --porcelain`; any non-empty output means dirty. Errors (git not
// installed, path not a repo) are swallowed and reported as clean — callers are expected to have already
// verified via [isGitRepo] that the path is a repository.
//
// Parameters:
//   - path: absolute path to a git working tree.
//
// Returns:
//   - bool: true when `git status --porcelain` emits any non-empty output.
func isDirtyRepo(path string) bool {

	cmd := exec.Command("git", "-C", path, "status", "--porcelain")

	out, err := cmd.Output()
	if err != nil {
		return false
	}

	return strings.TrimSpace(string(out)) != ""
}

// isGitRepo reports whether path is a git repository root, and whether it is bare.
//
// A path is a working-tree repository when `<path>/.git` is a directory. A path is a bare repository when
// `<path>/HEAD` and `<path>/config` both exist at the top level (the two files git itself uses to identify
// a bare repository). Paths that are inside a working tree but not themselves the fsroot are rejected.
//
// Parameters:
//   - path: absolute path to probe.
//
// Returns:
//   - repo: true when path is a git repository root.
//   - bare: true when the repository is bare; meaningful only when repo is true.
func isGitRepo(path string) (repo, bare bool) {

	if info, err := os.Stat(filepath.Join(path, ".git")); err == nil && info.IsDir() {
		return true, false
	}

	if _, err := os.Stat(filepath.Join(path, "HEAD")); err != nil {
		return false, false
	}
	if _, err := os.Stat(filepath.Join(path, "config")); err != nil {
		return false, false
	}

	return true, true
}

// parseRemotesOutput parses the output of `git config --get-regexp ^remote\.[^.]+\.(url|pushurl)$` into a
// map keyed by remote name.
//
// Each input line is "remote.<name>.<attr> <value>". Unknown attrs, malformed lines, and empty names are
// silently skipped.
//
// Parameters:
//   - output: raw stdout from the `git config --get-regexp` invocation.
//
// Returns:
//   - map[string]Remote: remotes keyed by name; nil when the output contains no parseable entries.
func parseRemotesOutput(output string) map[string]Remote {

	remotes := map[string]Remote{}

	for _, line := range strings.Split(strings.TrimRight(output, "\n"), "\n") {

		if line == "" {
			continue
		}

		space := strings.IndexByte(line, ' ')
		if space < 0 {
			continue
		}

		key, value := line[:space], line[space+1:]

		if !strings.HasPrefix(key, "remote.") {
			continue
		}

		rest := strings.TrimPrefix(key, "remote.")
		dot := strings.LastIndexByte(rest, '.')
		if dot < 0 {
			continue
		}

		name, attr := rest[:dot], rest[dot+1:]
		if name == "" {
			continue
		}

		remote := remotes[name]

		switch attr {
		case "url":
			remote.FetchURL = value
		case "pushurl":
			remote.PushURL = value
		default:
			continue
		}

		remotes[name] = remote
	}

	if len(remotes) == 0 {
		return nil
	}

	return remotes
}

// readBranchName returns the short branch name HEAD points at, or empty string when HEAD is detached.
//
// Shells out to `git -C <path> symbolic-ref --short HEAD`. A non-zero exit (detached HEAD, no HEAD, or not
// a repository) is treated as "no branch" and returns the empty string.
//
// Parameters:
//   - path: absolute path to a git repository root.
//
// Returns:
//   - string: the short branch name, or "" when HEAD is detached or unreadable.
func readBranchName(path string) string {

	cmd := exec.Command("git", "-C", path, "symbolic-ref", "--short", "HEAD")

	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(out))
}

// readHEADSha returns the commit SHA HEAD resolves to, or empty string when HEAD cannot be resolved.
//
// Shells out to `git -C <path> rev-parse HEAD`. Handles both attached (via symbolic ref) and detached HEAD
// uniformly — the SHA is returned in either case. A non-zero exit is treated as "no resolvable HEAD" and
// returns the empty string.
//
// Parameters:
//   - path: absolute path to a git repository root.
//
// Returns:
//   - string: the 40-char hex SHA HEAD resolves to, or "" on failure.
func readHEADSha(path string) string {

	cmd := exec.Command("git", "-C", path, "rev-parse", "HEAD")

	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(out))
}

// readRemotes returns the Remotes map for the repository at path.
//
// Shells out to `git -C <path> config --get-regexp ^remote\.[^.]+\.(url|pushurl)$` and parses the output via
// [parseRemotesOutput]. A non-zero exit (no remotes configured, git unavailable, not a repository) returns
// nil.
//
// Parameters:
//   - path: absolute path to a git repository root.
//
// Returns:
//   - map[string]Remote: remotes keyed by name; nil when the repository has no remotes configured or the
//     git invocation fails.
func readRemotes(path string) map[string]Remote {

	cmd := exec.Command("git", "-C", path, "config", "--get-regexp", `^remote\.[^.]+\.(url|pushurl)$`)

	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	return parseRemotesOutput(string(out))
}
