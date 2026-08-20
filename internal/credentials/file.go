// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package credentials

import (
	"errors"
	"os"

	"github.com/NobleFactor/devlore-cli/pkg/document"
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/xdg"
)

// credentialsFileName is the credentials file's name within the devlore config tree — the tree the roots
// received by [Set] and [Delete] are anchored at.
const credentialsFileName = "credentials.yaml"

// credentialsPath returns the path to the credentials file, for the read side, which takes no root.
func credentialsPath() string {
	return xdg.ConfigPath("devlore", credentialsFileName)
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

// fileSet stores a credential in the credentials file, writing through the received config-tree root.
//
// Parameters:
//   - configRoot: the devlore config tree, opened by the caller
//   - key: credential key (e.g., "ai/anthropic")
//   - secret: credential value to store
//
// Returns:
//   - error: read, merge, or write error
func fileSet(configRoot fsroot.Dir, key, secret string) error {

	// Load existing credentials
	creds, readErr := document.ReadFile[map[string]string](credentialsPath())
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

	return document.WriteFile(configRoot, configRoot.NewPath(credentialsFileName), creds, document.WithHeader(header))
}

// fileDelete removes a credential from the credentials file, mutating through the received config-tree root.
// No-op if the file does not exist.
//
// Parameters:
//   - configRoot: the devlore config tree, opened by the caller
//   - key: credential key to remove
//
// Returns:
//   - error: read, merge, removal, or write error
func fileDelete(configRoot fsroot.Dir, key string) error {

	creds, err := document.ReadFile[map[string]string](credentialsPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	delete(*creds, key)

	if len(*creds) == 0 {
		return configRoot.Remove(configRoot.NewPath(credentialsFileName))
	}

	return document.WriteFile(configRoot, configRoot.NewPath(credentialsFileName), creds)
}
