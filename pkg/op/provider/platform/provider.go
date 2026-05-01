// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package platform provides access to platform information by graph actions and executing receivers.
package platform

import (
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Provider exposes host platform metadata to Starlark scripts and graph actions.
//
// All accessors delegate to the [op.Platform] on the provider's [op.RuntimeEnvironment]. When the context
// has no platform (nil), accessors return zero values.
//
// +devlore:access=both
type Provider struct {
	op.ProviderBase
}

// NewProvider returns a new platform [Provider] with the given [op.RuntimeEnvironment].
//
// Parameters:
//   - ctx: the execution context (must carry a non-nil Platform for accessors to return data).
//
// Returns:
//   - *Provider: the configured provider.
func NewProvider(ctx *op.RuntimeEnvironment) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(ctx)}
}

// region EXPORTED METHODS

// Arch returns the CPU architecture (e.g., "amd64", "arm64").
//
// Returns:
//   - string: the architecture identifier, or "" if platform is nil.
func (p *Provider) Arch() string {
	if plat := p.RuntimeEnvironment().Platform; plat != nil {
		return plat.Arch
	}
	return ""
}

// Distro returns the OS distribution (e.g., "Ubuntu", "Fedora").
//
// Returns:
//   - string: the distribution name, or "" if unavailable or platform is nil.
func (p *Provider) Distro() string {
	if plat := p.RuntimeEnvironment().Platform; plat != nil {
		return plat.Distro
	}
	return ""
}

// Hostname returns the machine hostname.
//
// Returns:
//   - string: the hostname, or "" if unavailable or platform is nil.
func (p *Provider) Hostname() string {
	if plat := p.RuntimeEnvironment().Platform; plat != nil {
		return plat.Hostname
	}
	return ""
}

// OS returns the operating system name (e.g., "darwin", "linux", "windows").
//
// Returns:
//   - string: the OS identifier, or "" if platform is nil.
func (p *Provider) OS() string {
	if plat := p.RuntimeEnvironment().Platform; plat != nil {
		return plat.OS
	}
	return ""
}

// Version returns the OS version string.
//
// Returns:
//   - string: the version, or "" if unavailable or platform is nil.
func (p *Provider) Version() string {
	if plat := p.RuntimeEnvironment().Platform; plat != nil {
		return plat.Version
	}
	return ""
}

// endregion
