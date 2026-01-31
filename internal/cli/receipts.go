// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/signing"
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
// The receipt is checksummed before writing. Signing is performed using
// the first available backend from .sops.yaml (GPG, AWS KMS, GCP KMS, or Azure Key Vault).
// The .sops.yaml is expected at ${XDG_STATE_HOME}/devlore/.sops.yaml.
func WriteReceipt(g *execution.Graph, producer string) (string, error) {
	// Search for .sops.yaml from the devlore state directory
	// Expected location: ${XDG_STATE_HOME}/devlore/.sops.yaml
	return WriteReceiptWithSigningDir(g, producer, DevloreStateHome())
}

// WriteReceiptWithSigningDir writes the graph as a receipt, searching for
// .sops.yaml starting from signingDir to configure signing backends.
func WriteReceiptWithSigningDir(g *execution.Graph, producer, signingDir string) (string, error) {
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

	// Sign receipt using backends from .sops.yaml
	signGraph(g, canonical, signingDir)

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

// signGraph signs the graph using the first available signing backend.
// Searches for .sops.yaml starting from searchDir.
// If no backends are available, signing is skipped (g.Signature remains nil).
func signGraph(g *execution.Graph, canonical []byte, searchDir string) {
	chain := signing.BuildSignerChain(searchDir)

	sig, err := chain.Sign(canonical)
	if err != nil || sig == nil {
		// No signing backend available - that's OK
		return
	}

	// Convert signing.Signature to execution.Signature
	g.Signature = &execution.Signature{
		Method: sig.Method,
		Value:  sig.Value,
		KeyID:  sig.KeyID,
	}
}
