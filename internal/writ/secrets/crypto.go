// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package secrets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/getsops/sops/v3/cmd/sops/formats"
	"github.com/getsops/sops/v3/decrypt"
)

// DecryptData decrypts SOPS-encrypted data using the source path to determine format.
// SOPS handles key resolution via .sops.yaml + environment variables:
//   - SOPS_AGE_KEY: age key contents
//   - SOPS_AGE_KEY_FILE: path to age key file
//   - ~/.config/sops/age/keys.txt: default age key location
func DecryptData(data []byte, sourcePath string) ([]byte, error) {
	format := detectFormat(sourcePath, data)
	plaintext, err := decrypt.DataWithFormat(data, formats.FormatFromString(format))
	if err != nil {
		return nil, fmt.Errorf("sops decrypt: %w", err)
	}
	return plaintext, nil
}

// DecryptFile decrypts a SOPS-encrypted file and writes to target with
// the specified permissions. If the file is already plaintext
// (smudge filter active), it copies as-is.
func DecryptFile(src, dst string, mode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}

	// Check if already decrypted
	if !IsEncrypted(data) {
		// Already decrypted (smudge filter active) — copy as-is
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return fmt.Errorf("create parent dir: %w", err)
		}
		return os.WriteFile(dst, data, mode)
	}

	// Decrypt via SOPS
	plaintext, err := DecryptData(data, src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	return os.WriteFile(dst, plaintext, mode)
}

// detectFormat determines the SOPS format from filename and content.
func detectFormat(path string, data []byte) string {
	ext := strings.ToLower(filepath.Ext(path))

	// Strip .sops or .age extension to get actual format
	base := strings.TrimSuffix(strings.TrimSuffix(path, ".sops"), ".age")
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
