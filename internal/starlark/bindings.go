// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package starlark provides the Starlark runtime and host bindings for lore.
package starlark

import (
	"fmt"
	"io"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/NobleFactor/devlore-cli/internal/execution/provider/archive"
	"github.com/NobleFactor/devlore-cli/internal/execution/provider/file"
	"github.com/NobleFactor/devlore-cli/internal/execution/provider/git"
	"github.com/NobleFactor/devlore-cli/internal/execution/provider/net"
	"github.com/NobleFactor/devlore-cli/internal/execution/provider/shell"
	"github.com/NobleFactor/devlore-cli/internal/host"
)

// Bindings provides lore's host API to Starlark scripts.
type Bindings struct {
	host   host.Host
	output io.Writer

	features []string
	settings map[string]string
}

// NewBindings creates a new Bindings instance.
func NewBindings(features []string, settings map[string]string, output io.Writer) *Bindings {
	return &Bindings{
		host:     host.NewHost(),
		features: features,
		settings: settings,
		output:   output,
	}
}

// Globals returns the predeclared globals for Starlark scripts.
func (b *Bindings) Globals() starlark.StringDict {
	logRecv := NewLogReceiver(b.output)

	return starlark.StringDict{
		"platform": b.platformStruct(),
		"archive":  NewArchiveReceiver(&archive.Provider{}, b.output),
		"docker":   NewDockerReceiver(b.output),
		"env":      NewEnvReceiver(),
		"file":     NewFileReceiver(&file.Provider{}, b.output),
		"git":      NewGitReceiver(&git.Provider{}, b.output),
		"net":      NewNetReceiver(&net.Provider{}, b.output),
		"npm":      NewNpmReceiver(b.output),
		"package":  NewPackageReceiver(b.host.PackageManager(), b.features, b.settings, b.output),
		"service":  NewServiceReceiver(b.host.ServiceManager(), b.output),
		"shell":    NewShellReceiver(&shell.Provider{}, b.output),
		"log":      logRecv,
		"note":     starlark.NewBuiltin("note", logRecv.note),
		"warn":     starlark.NewBuiltin("warn", logRecv.warn),
		"error":    starlark.NewBuiltin("error", logRecv.errorFunc),
		"success":  starlark.NewBuiltin("success", logRecv.success),
		"fail":     starlark.NewBuiltin("fail", logRecv.fail),
	}
}

// platformStruct returns read-only platform info as a Starlark struct.
func (b *Bindings) platformStruct() *starlarkstruct.Struct {
	p := b.host.Platform()
	return starlarkstruct.FromStringDict(starlark.String("platform"), starlark.StringDict{
		"os":       starlark.String(p.OS),
		"arch":     starlark.String(p.Arch),
		"distro":   starlark.String(p.Distro),
		"version":  starlark.String(p.Version),
		"hostname": starlark.String(p.Hostname),
	})
}

// =============================================================================
// Helpers
// =============================================================================

// argToString converts a Starlark value to a string for CLI args.
func argToString(val starlark.Value) string {
	switch v := val.(type) {
	case starlark.String:
		return string(v)
	case starlark.Int:
		i, _ := v.Int64()
		return fmt.Sprintf("%d", i)
	case starlark.Bool:
		return fmt.Sprintf("%t", bool(v))
	default:
		return v.String()
	}
}

// kwargsToFlags converts Starlark kwargs to CLI flags (generic version).
// Single-char keys use single dash, multi-char use double dash.
func kwargsToFlags(kwargs []starlark.Tuple) []string {
	var flags []string

	for _, kv := range kwargs {
		key := strings.ReplaceAll(string(kv[0].(starlark.String)), "_", "-")
		val := kv[1]

		switch v := val.(type) {
		case starlark.Bool:
			if bool(v) {
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
