// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package credentials

import (
	"errors"
	"os"

	"github.com/NobleFactor/devlore-cli/internal/document"
	"github.com/NobleFactor/devlore-cli/pkg/xdg"
)

// credentialsPath returns the path to the credentials file.
func credentialsPath() string {
	return xdg.ConfigPath("devlore", "credentials.yaml")
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

	path := credentialsPath()

	creds, err := document.ReadFile[map[string]string](path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}

	return (*creds)[key], nil
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

	path := credentialsPath()

	// Load existing credentials
	creds, readErr := document.ReadFile[map[string]string](path)
	if readErr != nil {
		if !errors.Is(readErr, os.ErrNotExist) {
			return readErr
		}
		empty := make(map[string]string)
		creds = &empty
	}

	// Update
	(*creds)[key] = secret

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

	path := credentialsPath()

	creds, err := document.ReadFile[map[string]string](path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	delete(*creds, key)

	if len(*creds) == 0 {
		return os.Remove(path)
	}

	return document.Write(path, creds)
}
