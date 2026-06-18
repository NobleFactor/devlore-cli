// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package appnet provides network actions for the operation graph.
package appnet

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Provider provides network actions.
//
// +devlore:access=planned
type Provider struct {
	op.ProviderBase
}

// NewProvider creates a network-actions provider bound to the given context.
func NewProvider(runtimeEnvironment *op.RuntimeEnvironment) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(runtimeEnvironment)}
}

// region EXPORTED METHODS

// region Behaviors

// Download fetches the content at the given URL and returns the response body.
//
// Parameters:
//   - `url`: network resource identifying the URL to fetch.
//
// Returns:
//   - `[]byte`: the response body read from the URL.
//   - `error`: non-nil if the request fails, the response status is not 200 OK, or the body cannot be read.
func (p *Provider) Download(url *Resource) (_ []byte, err error) {

	rawURL := url.SourceURL.String()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", rawURL, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", rawURL, err)
	}
	defer iox.Close(&err, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: HTTP %d", rawURL, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("download %s: read body: %w", rawURL, err)
	}

	return data, nil
}

// endregion

// endregion
