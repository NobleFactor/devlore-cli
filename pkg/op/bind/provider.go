// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bind

import (
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/starlark"
)

var (
	_ starlark.Value    = (*Provider)(nil) // Interface Guard: ensures *Provider implements starlark.Value.
	_ starlark.HasAttrs = (*Provider)(nil) // Interface Guard: ensures *Provider implements starlark.HasAttrs.
)

// Provider wraps a Go provider instance for immediate-mode starlark use.
//
// It embeds [executingReceiver] for dispatch, attribute resolution, and the starlark value protocol.
type Provider struct {
	executingReceiver
}

// NewProvider creates a [Provider] wrapping the given instance.
//
// Parameters:
//   - rt: the provider receiver type descriptor.
//   - instance: the Go provider.
//
// Returns:
//   - *Provider: the starlark-ready wrapper.
func NewProvider(rt op.ProviderReceiverType, instance op.Provider) *Provider {

	return &Provider{
		executingReceiver: newExecutingReceiver(rt, instance),
	}
}
