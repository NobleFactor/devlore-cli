// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package devlore names the locations the devlore tools share, beneath the XDG base directories.
//
// One application tree — `devlore` — holds what writ and lore both read: configuration, cached artifacts, data, and
// run state. [github.com/NobleFactor/devlore-cli/pkg/xdg] resolves the bases; this package names what sits beneath
// them, and resolves nothing for itself.
//
// Manual pages and shell completions are the exception, and sit directly beneath the base directories rather than in
// the application tree. Those locations are not ours to choose: `man` searches `<data>/man`, bash-completion scans
// `<data>/bash-completion/completions`, and fish scans `<config>/fish/completions`. A file installed anywhere else is
// a file the reader never finds.
package devlore

import (
	"github.com/NobleFactor/devlore-cli/pkg/xdg"
)

// region EXPORTED FUNCTIONS

// BashCompletionPath joins `elem` onto the user's bash completion directory.
//
// Parameters:
//   - `elem`: path elements below the directory; none yields the directory itself.
//
// Returns:
//   - `string`: `bash-completion/completions` beneath [xdg.DataHome], plus `elem`.
func BashCompletionPath(elem ...string) string {
	return xdg.DataPath(append([]string{"bash-completion", "completions"}, elem...)...)
}

// CacheHome returns the cache directory shared by the devlore tools.
//
// Returns:
//   - `string`: the `devlore` directory beneath [xdg.CacheHome].
func CacheHome() string { return xdg.CachePath("devlore") }

// CachePath joins `elem` onto [CacheHome].
//
// Parameters:
//   - `elem`: path elements below the shared cache directory.
//
// Returns:
//   - `string`: the joined path.
func CachePath(elem ...string) string { return xdg.CachePath(append([]string{"devlore"}, elem...)...) }

// ConfigHome returns the configuration directory shared by the devlore tools.
//
// Returns:
//   - `string`: the `devlore` directory beneath [xdg.ConfigHome].
func ConfigHome() string { return xdg.ConfigPath("devlore") }

// ConfigPath joins `elem` onto [ConfigHome].
//
// Parameters:
//   - `elem`: path elements below the shared configuration directory.
//
// Returns:
//   - `string`: the joined path.
func ConfigPath(elem ...string) string {
	return xdg.ConfigPath(append([]string{"devlore"}, elem...)...)
}

// DataHome returns the data directory shared by the devlore tools.
//
// Returns:
//   - `string`: the `devlore` directory beneath [xdg.DataHome].
func DataHome() string { return xdg.DataPath("devlore") }

// DataPath joins `elem` onto [DataHome].
//
// Parameters:
//   - `elem`: path elements below the shared data directory.
//
// Returns:
//   - `string`: the joined path.
func DataPath(elem ...string) string { return xdg.DataPath(append([]string{"devlore"}, elem...)...) }

// FishCompletionPath joins `elem` onto the user's fish completion directory.
//
// Parameters:
//   - `elem`: path elements below the directory; none yields the directory itself.
//
// Returns:
//   - `string`: `fish/completions` beneath [xdg.ConfigHome], plus `elem`.
func FishCompletionPath(elem ...string) string {
	return xdg.ConfigPath(append([]string{"fish", "completions"}, elem...)...)
}

// ManPath joins `elem` onto the user's section 1 manual directory.
//
// Parameters:
//   - `elem`: path elements below the directory; none yields the directory itself.
//
// Returns:
//   - `string`: `man/man1` beneath [xdg.DataHome], plus `elem`.
func ManPath(elem ...string) string { return xdg.DataPath(append([]string{"man", "man1"}, elem...)...) }

// StateHome returns the state directory shared by the devlore tools.
//
// Returns:
//   - `string`: the `devlore` directory beneath [xdg.StateHome].
func StateHome() string { return xdg.StatePath("devlore") }

// StatePath joins `elem` onto [StateHome].
//
// Parameters:
//   - `elem`: path elements below the shared state directory.
//
// Returns:
//   - `string`: the joined path.
func StatePath(elem ...string) string { return xdg.StatePath(append([]string{"devlore"}, elem...)...) }

// WritLayersDir returns the layer registry — one symlink per layer, each naming that layer's working tree.
//
// Returns:
//   - `string`: `devlore/writ/layers` beneath [xdg.DataHome].
func WritLayersDir() string { return xdg.DataPath("devlore", "writ", "layers") }

// WritReposDir returns the writ-owned repository home, the default clone destination for `writ repo add`.
//
// Returns:
//   - `string`: `devlore/writ/repos` beneath [xdg.DataHome].
func WritReposDir() string { return xdg.DataPath("devlore", "writ", "repos") }

// ZshCompletionPath joins `elem` onto the user's zsh function directory.
//
// Parameters:
//   - `elem`: path elements below the directory; none yields the directory itself.
//
// Returns:
//   - `string`: `zsh/site-functions` beneath [xdg.DataHome], plus `elem`.
func ZshCompletionPath(elem ...string) string {
	return xdg.DataPath(append([]string{"zsh", "site-functions"}, elem...)...)
}

// endregion
