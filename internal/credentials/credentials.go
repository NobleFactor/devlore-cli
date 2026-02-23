// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package credentials provides secure credential storage using native OS
// keychains (macOS Keychain, Linux libsecret, Windows Credential Manager)
// with file-based fallback.
//
// Keychain entries use:
//   - Service: com.noblefactor.DevLore
//   - Account: the key (e.g., "ai/anthropic", "ai/openai")
//   - Password: the secret value
//
// Lookup priority: Native keystore > Credentials file
// Environment variables are handled by the caller, not here.
package credentials

// Get retrieves a credential with priority: keychain > file.
// Environment variables should be checked by the caller before calling this.
func Get(key string) (string, error) {
	// 1. Native keychain (macOS Keychain, Linux libsecret, Windows PasswordVault)
	if helper := detectHelper(); helper != "" {
		if secret, err := helperGet(helper, key); err == nil && secret != "" {
			return secret, nil
		}
	}

	// 2. Credentials file fallback
	return fileGet(key)
}

// Set stores a credential in keychain (preferred) or file fallback.
func Set(key, secret string) error {
	// Try native keychain first
	if helper := detectHelper(); helper != "" {
		if err := helperStore(helper, key, secret); err == nil {
			return nil
		}
		// Keychain failed, fall back to file
	}

	return fileSet(key, secret)
}

// Delete removes a credential from keychain and/or file.
func Delete(key string) error {
	// Try both - don't fail if one doesn't have it
	if helper := detectHelper(); helper != "" {
		_ = helperErase(helper, key) //nolint:errcheck // best-effort erase, not critical
	}
	return fileDelete(key)
}
