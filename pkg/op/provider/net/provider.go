// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package net provides network actions for the operation graph.
package net //nolint:revive // package name is domain-specific

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Provider provides network actions.
//
// +devlore:access=both
type Provider struct {
	op.ProviderBase
}

// Download fetches the content at the given URL and returns the response body.
//
// Parameters:
//   - url: HTTP(S) URL to fetch
func (p *Provider) Download(url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // URL comes from plan configuration
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("download %s: read body: %w", url, err)
	}
	return data, nil
}
