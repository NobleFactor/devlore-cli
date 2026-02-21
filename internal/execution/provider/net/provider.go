// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package net

import (
	"fmt"
	"io"
	"net/http"
)

// Provider provides network actions.
//
//devlore:plannable
type Provider struct{}

// Download fetches the content at the given URL and returns the response body.
func (p *Provider) Download(url string) ([]byte, error) {
	resp, err := http.Get(url) //nolint:gosec // URL comes from plan configuration
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
