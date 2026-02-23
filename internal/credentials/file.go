// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package credentials

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
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
func fileGet(key string) (string, error) {
	path, err := credentialsPath()
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	creds := make(map[string]string)
	if err := yaml.Unmarshal(data, &creds); err != nil {
		return "", err
	}

	// Normalize key: "ai/anthropic" stored as "ai/anthropic"
	return creds[key], nil
}

// fileSet stores a credential in the credentials file.
func fileSet(key, secret string) error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}

	// Load existing credentials
	creds := make(map[string]string)
	data, err := os.ReadFile(path)
	if err == nil {
		if err := yaml.Unmarshal(data, &creds); err != nil {
			return fmt.Errorf("parsing credentials file: %w", err)
		}
	}

	// Update
	creds[key] = secret

	// Write with header comment
	var sb strings.Builder
	sb.WriteString("# DevLore credentials - stored with 0600 permissions\n")
	sb.WriteString("# Prefer environment variables or credential helpers for better security\n")

	data, err = yaml.Marshal(creds)
	if err != nil {
		return err
	}
	sb.Write(data)

	return os.WriteFile(path, []byte(sb.String()), 0o600)
}

// fileDelete removes a credential from the credentials file.
func fileDelete(key string) error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	creds := make(map[string]string)
	if err := yaml.Unmarshal(data, &creds); err != nil {
		return err
	}

	delete(creds, key)

	if len(creds) == 0 {
		return os.Remove(path)
	}

	data, err = yaml.Marshal(creds)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o600)
}
