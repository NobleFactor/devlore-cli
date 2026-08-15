// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package xdg resolves the XDG base directories, rooted at the user's home directory on every platform.
//
// This is the single place to ask where user-scoped files belong, and the single way to resolve the home
// directory. No other package resolves home for itself: naming `HOME` — or `USERPROFILE` — anywhere else is
// what made the same logical location resolve to two different places on Windows.
//
// # Layout
//
// The standard XDG directory names, rooted at home, on every platform: `~/.config`, `~/.local/share`,
// `~/.local/state`, `~/.cache`, plus `~/.local/bin` for the de-facto XDG_BIN_HOME extension. The `XDG_*`
// variables override, as the specification requires. Windows gets no special case — it receives
// `%USERPROFILE%\.config` and friends rather than `%LOCALAPPDATA%`.
//
// That is a deliberate divergence from the cross-platform directory libraries (Go's `adrg/xdg`, Python's
// `platformdirs`, Rust's `dirs`), which map the XDG roles onto Windows Known Folders. It follows git,
// Microsoft's own OpenSSH port, Docker, kubectl, cargo, Starship and WezTerm instead, so that people working
// across platforms meet one layout rather than three.
//
// # Resolving home
//
// A ladder, because no single source always answers:
//
//  1. The role's `XDG_*` variable, when it names an **absolute** path. A relative value is invalid and
//     ignored, per the specification, so resolution continues as though it were unset.
//  2. [os.UserHomeDir] — the environment's answer, under whichever variable name the platform uses. It is a
//     switch on GOOS, so it reads exactly one name per platform and never the others.
//  3. [user.Current] — the operating system's answer: the process token's profile directory on Windows, the
//     passwd entry on Unix. Reached when the environment is silent, which happens to services and scheduled
//     tasks on Windows.
//  4. Nothing left. That is an environment in which no anchor can be honored, so it asserts rather than
//     returning a path no caller could use.
//
// A fully configured environment never reaches rung 2: set every `XDG_*` variable absolutely and home is
// never consulted at all.
//
// # No runtime directory
//
// `XDG_RUNTIME_DIR` is deliberately absent. The specification gives it no default and tells applications
// without it to "fall back to a replacement directory with similar capabilities" — user-owned, 0700, lifetime
// bound to the session. That is precisely the session scratch directory (`op.RuntimeEnvironment.Scratch`),
// which exists on every platform and is removed when the session closes. An accessor here would report
// "unset" on macOS and Windows while the correct answer sat one call away under a better name.
package xdg

import (
	"os"
	"os/user"
	"path/filepath"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// Environment variable names read by this package.
//
// They appear here and nowhere else: a home-directory variable is never named outside this file, because
// [os.UserHomeDir] already knows which one each platform uses.
const (
	envBinHome    = "XDG_BIN_HOME"
	envCacheHome  = "XDG_CACHE_HOME"
	envConfigDirs = "XDG_CONFIG_DIRS"
	envConfigHome = "XDG_CONFIG_HOME"
	envDataDirs   = "XDG_DATA_DIRS"
	envDataHome   = "XDG_DATA_HOME"
	envStateHome  = "XDG_STATE_HOME"
)

// region EXPORTED FUNCTIONS

// BinHome returns the base directory for user-specific executables.
//
// `XDG_BIN_HOME` is a de-facto extension rather than part of the base specification, but `~/.local/bin` is
// where self-installation puts binaries and where shells expect to find them.
//
// Returns:
//   - `string`: the value of `XDG_BIN_HOME` when absolute, else `~/.local/bin`.
func BinHome() string { return base(envBinHome, ".local", "bin") }

// BinPath joins `elem` onto [BinHome].
//
// Parameters:
//   - `elem`: path elements below the base directory; the application name is simply the first of them.
//
// Returns:
//   - `string`: the joined path.
func BinPath(elem ...string) string { return join(BinHome(), elem) }

// CacheHome returns the base directory for user-specific non-essential data.
//
// Returns:
//   - `string`: the value of `XDG_CACHE_HOME` when absolute, else `~/.cache`.
func CacheHome() string { return base(envCacheHome, ".cache") }

// CachePath joins `elem` onto [CacheHome].
//
// Parameters:
//   - `elem`: path elements below the base directory; the application name is simply the first of them.
//
// Returns:
//   - `string`: the joined path.
func CachePath(elem ...string) string { return join(CacheHome(), elem) }

// ConfigDirs returns the preference-ordered directories to search for configuration, after [ConfigHome].
//
// Returns:
//   - `[]string`: the absolute entries of `XDG_CONFIG_DIRS`, else the specification's default of `/etc/xdg`.
func ConfigDirs() []string { return dirs(envConfigDirs, "/etc/xdg") }

// ConfigHome returns the base directory for user-specific configuration.
//
// Returns:
//   - `string`: the value of `XDG_CONFIG_HOME` when absolute, else `~/.config`.
func ConfigHome() string { return base(envConfigHome, ".config") }

// ConfigPath joins `elem` onto [ConfigHome].
//
// Parameters:
//   - `elem`: path elements below the base directory; the application name is simply the first of them.
//
// Returns:
//   - `string`: the joined path.
func ConfigPath(elem ...string) string { return join(ConfigHome(), elem) }

// DataDirs returns the preference-ordered directories to search for data files, after [DataHome].
//
// Returns:
//   - `[]string`: the absolute entries of `XDG_DATA_DIRS`, else the specification's defaults.
func DataDirs() []string { return dirs(envDataDirs, "/usr/local/share", "/usr/share") }

// DataHome returns the base directory for user-specific data files.
//
// Returns:
//   - `string`: the value of `XDG_DATA_HOME` when absolute, else `~/.local/share`.
func DataHome() string { return base(envDataHome, ".local", "share") }

// DataPath joins `elem` onto [DataHome].
//
// Parameters:
//   - `elem`: path elements below the base directory; the application name is simply the first of them.
//
// Returns:
//   - `string`: the joined path.
func DataPath(elem ...string) string { return join(DataHome(), elem) }

// StateHome returns the base directory for state that should persist between restarts but is not portable.
//
// Logs, history, and recently-used lists live here rather than in [DataHome].
//
// Returns:
//   - `string`: the value of `XDG_STATE_HOME` when absolute, else `~/.local/state`.
func StateHome() string { return base(envStateHome, ".local", "state") }

// StatePath joins `elem` onto [StateHome].
//
// Parameters:
//   - `elem`: path elements below the base directory; the application name is simply the first of them.
//
// Returns:
//   - `string`: the joined path.
func StatePath(elem ...string) string { return join(StateHome(), elem) }

// UserHomeDir returns the user's home directory, asking the operating system when the environment is silent.
//
// Rungs 2 through 4 of the ladder described in the package documentation. Callers wanting a standard location
// should use the base accessors instead; this exists for the cases that genuinely mean *the home directory* —
// a deploy target, or expanding a leading `~` in user input.
//
// Panics when neither the environment nor the user account yields a home directory, which requires the
// platform's home variable to be unset, the user account to be unreadable, and — on Unix built without cgo —
// no matching `/etc/passwd` entry. No anchor of any kind can be honored in that environment.
//
// Returns:
//   - `string`: the home directory, never empty and never relative.
func UserHomeDir() string {

	if home, err := os.UserHomeDir(); err == nil && filepath.IsAbs(home) {
		return home
	}

	if account, err := user.Current(); err == nil && filepath.IsAbs(account.HomeDir) {
		return account.HomeDir
	}

	assert.Failf("xdg.UserHomeDir: no home directory from the environment or the user account")
	return ""
}

// endregion

// region HELPER FUNCTIONS

// base returns the XDG base directory for one role.
//
// The variable wins when it names an absolute path. An empty or relative value is invalid and ignored per the
// specification — [filepath.IsAbs] is false for both, so one test covers unset and invalid alike, and a
// relative anchor becomes structurally impossible rather than merely guarded against.
//
// diagnose-ignored-error: a relative XDG_* value is discarded here, and the user is told nothing until the
// diagnostics stream exists; see docs/architecture/2.8-eventing-infrastructure.md. The specification itself
// tells implementations to warn, and this package must not take a dependency on a narrator to say so.
//
// Parameters:
//   - `variable`: the `XDG_*` environment variable name.
//   - `elem`: the default's path elements below the home directory.
//
// Returns:
//   - `string`: the resolved base directory, always absolute.
func base(variable string, elem ...string) string {

	if dir := os.Getenv(variable); filepath.IsAbs(dir) {
		return filepath.Clean(dir)
	}

	return join(UserHomeDir(), elem)
}

// dirs returns the absolute entries of a search-path variable, or the supplied defaults.
//
// Relative entries are dropped rather than resolved, for the reason [base] gives. An entirely relative or
// empty variable therefore yields the defaults, not an empty list.
//
// Parameters:
//   - `variable`: the `XDG_*_DIRS` environment variable name.
//   - `fallback`: the specification's defaults, used when the variable contributes no absolute entry.
//
// Returns:
//   - `[]string`: the search path in preference order.
func dirs(variable string, fallback ...string) []string {

	var found []string

	for _, dir := range filepath.SplitList(os.Getenv(variable)) {
		if filepath.IsAbs(dir) {
			found = append(found, filepath.Clean(dir))
		}
	}

	if len(found) == 0 {
		return fallback
	}

	return found
}

// join appends path elements to a base directory.
//
// Parameters:
//   - `dir`: the base directory.
//   - `elem`: the elements to append; none yields the base directory itself.
//
// Returns:
//   - `string`: the joined path.
func join(dir string, elem []string) string {

	if len(elem) == 0 {
		return dir
	}

	return filepath.Join(append([]string{dir}, elem...)...)
}

// endregion
