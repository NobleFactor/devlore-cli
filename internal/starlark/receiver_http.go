// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/host"
)

// HTTPReceiver provides the http.* Starlark namespace.
//
// Backing implementation: net/http (Get) for requests,
// host.Host (ExpandPath) for path expansion.
type HTTPReceiver struct {
	Receiver
	host   host.Host
	output io.Writer
}

// NewHTTPReceiver creates a new http receiver.
func NewHTTPReceiver(h host.Host, output io.Writer) *HTTPReceiver {
	return &HTTPReceiver{
		Receiver: NewReceiver("http"),
		host:     h,
		output:   output,
	}
}

func (r *HTTPReceiver) Attr(name string) (starlark.Value, error) {
	switch name {
	case "download":
		return MakeAttr("http.download", r.download), nil
	case "get":
		return MakeAttr("http.get", r.get), nil
	default:
		return nil, NoSuchAttrError("http", name)
	}
}

func (r *HTTPReceiver) AttrNames() []string {
	return []string{"download", "get"}
}

func (r *HTTPReceiver) download(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var url, dest string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "url", &url, "dest", &dest); err != nil {
		return nil, err
	}
	dest = r.host.ExpandPath(dest)

	_, _ = fmt.Fprintf(r.output, "  [http] Downloading %s -> %s\n", url, dest)

	resp, err := http.Get(url)
	if err != nil {
		return resultToStarlark(host.Result{OK: false, Stderr: err.Error()}), nil
	}
	defer func() { _ = resp.Body.Close() }()

	out, err := os.Create(dest)
	if err != nil {
		return resultToStarlark(host.Result{OK: false, Stderr: err.Error()}), nil
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return resultToStarlark(host.Result{OK: false, Stderr: err.Error()}), nil
	}

	return resultToStarlark(host.Result{OK: true}), nil
}

func (r *HTTPReceiver) get(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var url string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "url", &url); err != nil {
		return nil, err
	}

	resp, err := http.Get(url)
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
