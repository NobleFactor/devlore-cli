// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package sops

import (
	"bytes"
	"encoding/json"
	"strings"

	"gopkg.in/yaml.v3"
)

// IsEncrypted reports whether data contains SOPS metadata or age armor.
//
// Parameters:
//   - data: content to inspect
//
// Returns:
//   - bool: true if the data appears to be SOPS-encrypted
func IsEncrypted(data []byte) bool {

	if hasSopsMetadata(data) {
		return true
	}

	// Check for age armor header (SOPS uses age in armored format)
	if bytes.HasPrefix(data, []byte("-----BEGIN AGE ENCRYPTED FILE-----")) {
		return true
	}

	// Check for age binary format (starts with "age-encryption.org")
	if bytes.HasPrefix(data, []byte("age-encryption.org")) {
		return true
	}

	return false
}

// IsSecretFile reports whether a filename indicates a SOPS-encrypted file.
//
// Parameters:
//   - filename: filename to check (path or basename)
//
// Returns:
//   - bool: true if the filename ends with .sops, .sops.yaml, or .sops.json
func IsSecretFile(filename string) bool {

	lower := strings.ToLower(filename)
	return strings.HasSuffix(lower, ".sops") ||
		strings.HasSuffix(lower, ".sops.yaml") ||
		strings.HasSuffix(lower, ".sops.json")
}

// hasSopsMetadata checks for SOPS metadata in JSON or YAML structured files.
// SOPS adds a "sops" key with encryption metadata.
//
// Parameters:
//   - data: content to inspect
//
// Returns:
//   - bool: true if the data contains a "sops" key
func hasSopsMetadata(data []byte) bool {

	// Try JSON first
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		var obj map[string]any
		if err := json.Unmarshal(data, &obj); err == nil {
			if _, ok := obj["sops"]; ok {
				return true
			}
		}
	}

	// Try YAML
	if !bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		var obj map[string]any
		if err := yaml.Unmarshal(data, &obj); err == nil {
			if _, ok := obj["sops"]; ok {
				return true
			}
		}
	}

	return false
}
