// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package sops

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/getsops/sops/v3/cmd/sops/formats"
	"github.com/getsops/sops/v3/decrypt"
)

// region EXPORTED METHODS

// region Behaviors

// Fallible actions

// Decrypt decrypts SOPS-encrypted data. Format is inferred from sourcePath extension. Plaintext data passes through
// unchanged.
//
// Parameters:
//   - data: encrypted (or plaintext) content
//   - sourcePath: original file path used to determine SOPS format
//
// Returns:
//   - []byte: decrypted content (or unchanged plaintext)
//   - error: decryption error
func (c *Client) Decrypt(data []byte, sourcePath string) ([]byte, error) {

	if !IsEncrypted(data) {
		return data, nil
	}

	format := detectFormat(sourcePath, data)
	plaintext, err := decrypt.DataWithFormat(data, formats.FormatFromString(format))
	if err != nil {
		return nil, fmt.Errorf("sops decrypt: %w", err)
	}
	return plaintext, nil
}

// Actions

// Decryptor returns a decryption function matching the signature expected by the execution engine:
// func(source string, data []byte) ([]byte, error).
//
// Returns:
//   - func(string, []byte) ([]byte, error): decryption function
func (c *Client) Decryptor() func(source string, data []byte) ([]byte, error) {

	return func(source string, data []byte) ([]byte, error) {
		plaintext, err := c.Decrypt(data, source)
		if err != nil {
			return nil, fmt.Errorf("decrypt %s: %w", filepath.Base(source), err)
		}
		return plaintext, nil
	}
}

// endregion

// endregion

// detectFormat determines the SOPS format from filename and content.
//
// Parameters:
//   - path: source file path
//   - data: file content for content-based detection fallback
//
// Returns:
//   - string: SOPS format name (yaml, json, dotenv, ini, binary)
func detectFormat(path string, data []byte) string {

	ext := strings.ToLower(filepath.Ext(path))

	// Strip .sops extension to get actual format
	base := strings.TrimSuffix(path, ".sops")
	innerExt := strings.ToLower(filepath.Ext(base))

	// Check inner extension for structured formats
	switch innerExt {
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".env":
		return "dotenv"
	case ".ini":
		return "ini"
	}

	// Check outer extension
	switch ext {
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".env":
		return "dotenv"
	case ".ini":
		return "ini"
	}

	// Fallback: detect from content
	if len(data) > 0 {
		trimmed := strings.TrimSpace(string(data))
		if strings.HasPrefix(trimmed, "{") {
			return "json"
		}
		// Check for age armor (binary format)
		if strings.HasPrefix(trimmed, "-----BEGIN AGE") {
			return "binary"
		}
	}

	// Default to binary (opaque encrypted blob)
	return "binary"
}
