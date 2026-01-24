// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package secrets

import (
	"bytes"
	"encoding/json"
	"strings"

	"gopkg.in/yaml.v3"
)

// IsEncrypted checks if data is a SOPS-encrypted envelope.
// Returns false if the data is plaintext (e.g., smudge filter decrypted it).
func IsEncrypted(data []byte) bool {
	// Check for SOPS metadata in structured files (JSON/YAML)
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

// hasSopsMetadata checks for SOPS metadata in JSON or YAML structured files.
// SOPS adds a "sops" key with encryption metadata.
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

// IsSecretFile checks if a filename indicates an encrypted file.
// This checks extensions, not file content.
func IsSecretFile(filename string) bool {
	lower := strings.ToLower(filename)
	return strings.HasSuffix(lower, ".sops") ||
		strings.HasSuffix(lower, ".age") ||
		strings.HasSuffix(lower, ".sops.yaml") ||
		strings.HasSuffix(lower, ".sops.json")
}
