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

// BashCompletionPath returns the user's bash completion directory, where the devlore tools install their scripts.
//
// Returns:
//   - `string`: `bash-completion/completions` beneath [xdg.DataHome].
func BashCompletionPath() string { return xdg.DataPath("bash-completion", "completions") }

// CacheHome returns the cache directory shared by the devlore tools.
//
// Returns:
//   - `string`: the `devlore` directory beneath [xdg.CacheHome].
func CacheHome() string { return xdg.CachePath("devlore") }

// ConfigHome returns the configuration directory shared by the devlore tools.
//
// Returns:
//   - `string`: the `devlore` directory beneath [xdg.ConfigHome].
func ConfigHome() string { return xdg.ConfigPath("devlore") }

// DataHome returns the data directory shared by the devlore tools.
//
// Returns:
//   - `string`: the `devlore` directory beneath [xdg.DataHome].
func DataHome() string { return xdg.DataPath("devlore") }

// FishCompletionPath returns the user's fish completion directory, where the devlore tools install their scripts.
//
// Returns:
//   - `string`: `fish/completions` beneath [xdg.ConfigHome].
func FishCompletionPath() string { return xdg.ConfigPath("fish", "completions") }

// ManPath returns the user's section 1 manual directory, where the devlore tools install their pages.
//
// Returns:
//   - `string`: `man/man1` beneath [xdg.DataHome].
func ManPath() string { return xdg.DataPath("man", "man1") }

// StateHome returns the state directory shared by the devlore tools.
//
// Returns:
//   - `string`: the `devlore` directory beneath [xdg.StateHome].
func StateHome() string { return xdg.StatePath("devlore") }

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

// ZshCompletionPath returns the user's zsh function directory, where the devlore tools install their completions.
//
// Returns:
//   - `string`: `zsh/site-functions` beneath [xdg.DataHome].
func ZshCompletionPath() string { return xdg.DataPath("zsh", "site-functions") }

// endregion
