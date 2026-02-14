// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import "fmt"

// EncryptionService provides encryption and decryption operations.
// The actual crypto backend (SOPS, age, etc.) is injected via function
// parameters, keeping this service independent of specific libraries.
type EncryptionService struct{}

// Decrypt decrypts content using the provided decryptor function.
// The source path enables format detection (e.g., .sops.yaml vs .age).
// Returns the decrypted bytes.
func (e *EncryptionService) Decrypt(decryptor func(string, []byte) ([]byte, error), source string, content []byte) ([]byte, error) {
	if decryptor == nil {
		return nil, fmt.Errorf("no decryptor configured")
	}
	return decryptor(source, content)
}
