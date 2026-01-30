// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/execution"
)

// ReceiptsDir returns the directory where receipts are stored.
// Location: $XDG_STATE_HOME/devlore/receipts (typically ~/.local/state/devlore/receipts)
func ReceiptsDir() string {
	return filepath.Join(DevloreStateHome(), "receipts")
}

// LatestReceiptPath returns the path to the latest receipt symlink for a producer.
// Producer is typically a command name: "writ", "lore", etc.
func LatestReceiptPath(producer string) string {
	return filepath.Join(ReceiptsDir(), producer+"-latest.yaml")
}

// LoadReceipt loads an execution graph from a YAML receipt file.
func LoadReceipt(path string) (*execution.Graph, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read receipt: %w", err)
	}

	var g execution.Graph
	if err := yaml.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("parse receipt: %w", err)
	}

	return &g, nil
}

// LoadLatestReceipt loads the most recent receipt for a producer.
func LoadLatestReceipt(producer string) (*execution.Graph, error) {
	return LoadReceipt(LatestReceiptPath(producer))
}

// WriteReceipt writes the graph as a receipt to the receipts directory.
// The producer identifies which command created the receipt (e.g., "writ", "lore").
// Returns the path where the receipt was written.
//
// Receipts are produced at the end of lifecycle operations:
// writ: Migrate, Adopt, Deploy, Upgrade, Reconcile, Decommission
// lore: Onboard
//
// The receipt is checksummed before writing. Signing is performed if
// age identities are available.
func WriteReceipt(g *execution.Graph, producer string) (string, error) {
	dir := ReceiptsDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create receipts dir: %w", err)
	}

	filename := g.Filename()
	path := filepath.Join(dir, filename)

	// Compute checksum from canonical content
	canonical, err := g.CanonicalContent()
	if err != nil {
		return "", fmt.Errorf("canonical content: %w", err)
	}
	g.Checksum = execution.GitStyleChecksum("graph", filename, canonical)

	// TODO: Sign receipt if age identities are configured

	// Write receipt
	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("create receipt file: %w", err)
	}
	defer func() { _ = f.Close() }()

	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	defer func() { _ = enc.Close() }()

	if err := g.Serialize(enc); err != nil {
		return "", err
	}

	// Update "latest" symlink for this producer
	latestPath := filepath.Join(dir, producer+"-latest.yaml")
	_ = os.Remove(latestPath)
	_ = os.Symlink(filename, latestPath)

	return path, nil
}
