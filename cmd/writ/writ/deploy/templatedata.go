// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package deploy

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"
)

// templateData assembles the render-chain data map: the builtin platform/user/XDG values overlaid with the
// user-configured variables.
//
// Parameters:
//   - `cfg`: the deploy configuration supplying segments and user variables.
//
// Returns:
//   - `map[string]any`: the merged template data.
func templateData(cfg *Config) map[string]any {

	data := builtinTemplateData(segmentMap(cfg.Segments))
	for k, v := range cfg.Vars {
		data[k] = v
	}
	return data
}

// segmentMap projects the non-empty segment values into a name → value map for template data.
//
// Parameters:
//   - `segments`: the segments to project.
//
// Returns:
//   - `map[string]string`: the non-empty segment values by name.
func segmentMap(segments segment.Segments) map[string]string {

	values := make(map[string]string)
	for _, seg := range segments {
		if seg.Value != "" {
			values[seg.Name] = seg.Value
		}
	}
	return values
}

// builtinTemplateData returns the default template data available to every rendered template.
//
// The data map rides the planned graph document as an immediate, so it holds plain serializable values only —
// the old in-process engine's `Env` lookup function cannot exist here; templates read platform/user/XDG values
// from the named keys or from explicitly configured vars.
//
// Parameters:
//   - `segMap`: the segment name → value map exposed as `.Segments`.
//
// Returns:
//   - `map[string]any`: platform, user, segment, and XDG values.
func builtinTemplateData(segMap map[string]string) map[string]any {

	data := make(map[string]any)

	data["OS"] = runtime.GOOS
	data["ARCH"] = runtime.GOARCH
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	data["Hostname"] = hostname

	data["Home"] = os.Getenv("HOME")
	if u, err := user.Current(); err == nil {
		data["Username"] = u.Username
	} else {
		data["Username"] = os.Getenv("USER")
	}

	data["Segments"] = segMap

	home := os.Getenv("HOME")
	data["ConfigHome"] = xdgPath("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	data["DataHome"] = xdgPath("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	data["StateHome"] = xdgPath("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	data["CacheHome"] = xdgPath("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	return data
}

// xdgPath returns the XDG directory from the environment variable, or the default path.
//
// Parameters:
//   - `envVar`: the XDG environment variable name.
//   - `defaultPath`: the fallback when the variable is unset.
//
// Returns:
//   - `string`: the resolved directory.
func xdgPath(envVar, defaultPath string) string {

	if v := os.Getenv(envVar); v != "" {
		return v
	}
	return defaultPath
}
