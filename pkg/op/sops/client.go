// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package sops provides unified SOPS operations through a single Client type. It consolidates config discovery,
// decryption, signing, verification, and encryption detection.
package sops

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/NobleFactor/devlore-cli/internal/document"
)

// Client provides SOPS operations. Config discovery happens at construction time.
type Client struct {
	config *sopsConfig
}

// NewClient creates a Client by searching for .sops.yaml upward from searchDir. Returns an error if no .sops.yaml is
// found.
//
// Parameters:
//   - searchDir: directory to start searching upward from
//
// Returns:
//   - *Client: the configured client
//   - error: if no .sops.yaml is found or config is unparseable
func NewClient(searchDir string) (*Client, error) {

	configPath := findConfig(searchDir)
	if configPath == "" {
		return nil, fmt.Errorf("sops: no .sops.yaml found at or above %s", searchDir)
	}

	cfg, err := parseConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("sops: parse %s: %w", configPath, err)
	}

	return &Client{
		config: cfg,
	}, nil
}

// sopsConfig models the .sops.yaml file structure.
type sopsConfig struct {
	CreationRules []creationRule `yaml:"creation_rules"`
}

// creationRule represents a single rule in .sops.yaml.
type creationRule struct {
	PathRegex string `yaml:"path_regex"`
	PGP       string `yaml:"pgp,omitempty"`
	Age       string `yaml:"age,omitempty"`
	AWSKMS    string `yaml:"aws_kms,omitempty"`
	GCPKMS    string `yaml:"gcp_kms,omitempty"`
	AzureKV   string `yaml:"azure_kv,omitempty"`
}

// findConfig searches for .sops.yaml starting from dir, walking up the directory tree.
//
// Parameters:
//   - dir: starting directory for the upward walk
//
// Returns:
//   - string: absolute path to .sops.yaml, or empty string if not found
func findConfig(dir string) string {

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	dir = absDir

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

// parseConfig reads and parses a .sops.yaml file.
//
// Parameters:
//   - path: filesystem path to the .sops.yaml file
//
// Returns:
//   - *sopsConfig: parsed configuration
//   - error: read or parse error
func parseConfig(path string) (*sopsConfig, error) {

	return document.ReadFile[sopsConfig](path)
}
