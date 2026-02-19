// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"io"
	"net/http"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/execution/provider/net"
)

// NetReceiver provides the net.* Starlark namespace.
// Forward operations (download) delegate to net.Provider.
// Query operations (get) perform read-only HTTP requests directly.
type NetReceiver struct {
	Receiver
	provider *net.Provider
	output   io.Writer
}

// NewNetReceiver creates a new net receiver.
func NewNetReceiver(provider *net.Provider, output io.Writer) *NetReceiver {
	return &NetReceiver{
		Receiver: NewReceiver("net"),
		provider: provider,
		output:   output,
	}
}

func (r *NetReceiver) queryAttr(name string) (starlark.Value, error) {
	switch name {
	case "get":
		return MakeAttr("net.get", r.get), nil
	default:
		return nil, NoSuchAttrError("net", name)
	}
}

func (r *NetReceiver) get(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var url string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "url", &url); err != nil {
		return nil, err
	}

	resp, err := http.Get(url) //nolint:gosec // URL comes from script input
	if err != nil {
		return starlark.String(""), nil
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return starlark.String(""), nil
	}
	return starlark.String(body), nil
}
