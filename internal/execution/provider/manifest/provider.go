// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package manifest

import (
	"fmt"
	"os"

	devmanifest "github.com/NobleFactor/devlore-cli/internal/manifest"
)

// Provider provides manifest resolution operations. When the graph builder
// encounters a packages-manifest.yaml, it creates a manifest-resolve node.
// The planning step resolves the manifest into a lore package lifecycle pipeline.
type Provider struct{}

// Resolve reads a packages-manifest file and returns the parsed package names.
func (p *Provider) Resolve(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	m, err := devmanifest.Parse(data, path)
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	return m.PackageNames(), nil
}
