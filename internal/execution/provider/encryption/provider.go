// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package encryption

import "fmt"

// Provider provides encryption and decryption actions.
// The actual crypto backend (SOPS, age, etc.) is injected via function
// parameters, keeping this provider independent of specific libraries.
//
//devlore:plannable
type Provider struct{}

// Decrypt decrypts content using the provided decryptor function.
// The source path enables format detection (e.g., .sops.yaml vs .sops.json).
// Returns the decrypted bytes.
//
// Parameters:
//   - source: Path to the encrypted file (enables format detection)
func (p *Provider) Decrypt(decryptor func(string, []byte) ([]byte, error), source string, content []byte) ([]byte, error) {
	if decryptor == nil {
		return nil, fmt.Errorf("no decryptor configured")
	}
	return decryptor(source, content)
}
