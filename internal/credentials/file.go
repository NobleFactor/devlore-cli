// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package credentials

import (
	"os"
	"path/filepath"

	"github.com/NobleFactor/devlore-cli/internal/document"
)

// credentialsPath returns the path to the credentials file.
func credentialsPath() (string, error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "devlore", "credentials.yaml"), nil
}

// fileGet retrieves a credential from the credentials file.
//
// Parameters:
//   - key: credential key (e.g., "ai/anthropic")
//
// Returns:
//   - string: credential value, or empty string if not found
//   - error: read or parse error
func fileGet(key string) (string, error) {

	path, err := credentialsPath()
	if err != nil {
		return "", err
	}

	creds := make(map[string]string)
	if _, err := document.ReadIfExists(path, &creds); err != nil {
		return "", err
	}

	return creds[key], nil
}

// fileSet stores a credential in the credentials file.
//
// Parameters:
//   - key: credential key (e.g., "ai/anthropic")
//   - secret: credential value to store
//
// Returns:
//   - error: read, merge, or write error
func fileSet(key, secret string) error {

	path, err := credentialsPath()
	if err != nil {
		return err
	}

	// Load existing credentials
	creds := make(map[string]string)
	if _, err := document.ReadIfExists(path, &creds); err != nil {
		return err
	}

	// Update
	creds[key] = secret

	// Write with header comment
	header := "# DevLore credentials - stored with 0600 permissions\n" +
		"# Prefer environment variables or credential helpers for better security\n"

	return document.Write(path, creds, document.WithHeader(header))
}

// fileDelete removes a credential from the credentials file. No-op if the file does not exist.
//
// Parameters:
//   - key: credential key to remove
//
// Returns:
//   - error: read, merge, or write error
func fileDelete(key string) error {

	path, err := credentialsPath()
	if err != nil {
		return err
	}

	creds := make(map[string]string)
	found, err := document.ReadIfExists(path, &creds)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}

	delete(creds, key)

	if len(creds) == 0 {
		return os.Remove(path)
	}

	return document.Write(path, creds)
}
