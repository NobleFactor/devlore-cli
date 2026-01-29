// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package secrets handles encryption/decryption operations via SOPS.
// It implements the design from ADR-050: Writ Encrypted Files via SOPS.
package secrets

import (
	"fmt"
	"os"
	"path/filepath"
)

// Manager handles encryption/decryption operations via SOPS.
type Manager struct {
	sourceRoot string
	configPath string // path to .sops.yaml if found
}

// NewManager creates a Manager that searches for .sops.yaml
// starting from sourceRoot, walking up the directory tree.
// Returns nil (not an error) if no .sops.yaml is found.
func NewManager(sourceRoot string) (*Manager, error) {
	configPath := findSopsConfig(sourceRoot)
	return &Manager{
		sourceRoot: sourceRoot,
		configPath: configPath,
	}, nil
}

// HasConfig returns true if a .sops.yaml was found.
func (m *Manager) HasConfig() bool {
	return m.configPath != ""
}

// ConfigPath returns the path to the .sops.yaml file, or empty string if none.
func (m *Manager) ConfigPath() string {
	return m.configPath
}

// findSopsConfig walks up from dir looking for .sops.yaml.
func findSopsConfig(dir string) string {
	dir, _ = filepath.Abs(dir)
	for {
		candidate := filepath.Join(dir, ".sops.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// Decryptor returns a decryption function suitable for the engine.
// The returned function takes a source path and encrypted data,
// returning plaintext. It handles both .sops and .age files via SOPS.
func (m *Manager) Decryptor() func(source string, data []byte) ([]byte, error) {
	return func(source string, data []byte) ([]byte, error) {
		// Check if already decrypted (smudge filter active)
		if !IsEncrypted(data) {
			return data, nil
		}

		// Decrypt via SOPS
		plaintext, err := DecryptData(data, source)
		if err != nil {
			return nil, fmt.Errorf("decrypt %s: %w", filepath.Base(source), err)
		}
		return plaintext, nil
	}
}
