// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package exec

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"
	"filippo.io/age/agessh"
)

// LoadIdentities loads age identities from standard locations.
// Searches in order:
//  1. AGE_IDENTITY environment variable (comma-separated paths)
//  2. ~/.config/age/keys.txt
//  3. ~/.ssh/id_ed25519, ~/.ssh/id_rsa (SSH keys)
func LoadIdentities() ([]age.Identity, error) {
	var identities []age.Identity

	// 1. AGE_IDENTITY environment variable
	if env := os.Getenv("AGE_IDENTITY"); env != "" {
		paths := strings.Split(env, ",")
		for _, path := range paths {
			path = strings.TrimSpace(path)
			if path == "" {
				continue
			}
			ids, err := loadIdentityFile(expandPath(path))
			if err != nil {
				return nil, fmt.Errorf("load %s: %w", path, err)
			}
			identities = append(identities, ids...)
		}
	}

	// 2. ~/.config/age/keys.txt
	ageKeysPath := expandPath("~/.config/age/keys.txt")
	if ids, err := loadIdentityFile(ageKeysPath); err == nil {
		identities = append(identities, ids...)
	}

	// 3. SSH keys
	sshKeys := []string{
		"~/.ssh/id_ed25519",
		"~/.ssh/id_rsa",
	}
	for _, keyPath := range sshKeys {
		path := expandPath(keyPath)
		if ids, err := loadSSHIdentity(path); err == nil {
			identities = append(identities, ids...)
		}
	}

	if len(identities) == 0 {
		return nil, fmt.Errorf("no age identities found (set AGE_IDENTITY or create ~/.config/age/keys.txt)")
	}

	return identities, nil
}

// LoadIdentitiesFromPaths loads identities from specific paths.
func LoadIdentitiesFromPaths(paths []string) ([]age.Identity, error) {
	var identities []age.Identity

	for _, path := range paths {
		path = expandPath(path)

		// Try as age identity first
		if ids, err := loadIdentityFile(path); err == nil {
			identities = append(identities, ids...)
			continue
		}

		// Try as SSH key
		if ids, err := loadSSHIdentity(path); err == nil {
			identities = append(identities, ids...)
			continue
		}

		return nil, fmt.Errorf("failed to load identity from %s", path)
	}

	return identities, nil
}

// loadIdentityFile loads age identities from a file.
func loadIdentityFile(path string) ([]age.Identity, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return age.ParseIdentities(file)
}

// loadSSHIdentity loads an SSH private key as an age identity.
func loadSSHIdentity(path string) ([]age.Identity, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Check if this looks like an SSH private key
	if !strings.Contains(string(content), "PRIVATE KEY") {
		return nil, fmt.Errorf("not an SSH private key")
	}

	// Try to parse as SSH identity
	// Note: agessh.ParseIdentity handles passphrase-protected keys by returning an error
	identity, err := agessh.ParseIdentity(content)
	if err != nil {
		// Check if it's a passphrase-protected key
		if strings.Contains(err.Error(), "encrypted") {
			return nil, fmt.Errorf("passphrase-protected SSH keys not supported (use age identity instead)")
		}
		return nil, err
	}

	return []age.Identity{identity}, nil
}

// expandPath expands ~ to the user's home directory.
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// GenerateIdentity creates a new age identity and returns the key.
func GenerateIdentity() (*age.X25519Identity, error) {
	return age.GenerateX25519Identity()
}

// IdentityToRecipient returns the public key (recipient) for an identity.
func IdentityToRecipient(identity *age.X25519Identity) string {
	return identity.Recipient().String()
}

// ParseRecipients parses age recipient strings (public keys).
func ParseRecipients(recipients []string) ([]age.Recipient, error) {
	var result []age.Recipient

	for _, r := range recipients {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}

		// Check if it's a file path
		if strings.HasPrefix(r, "/") || strings.HasPrefix(r, "~") {
			recs, err := loadRecipientsFile(expandPath(r))
			if err != nil {
				return nil, fmt.Errorf("load recipients from %s: %w", r, err)
			}
			result = append(result, recs...)
			continue
		}

		// Parse as recipient string
		rec, err := age.ParseX25519Recipient(r)
		if err != nil {
			// Try as SSH public key
			sshRec, sshErr := agessh.ParseRecipient(r)
			if sshErr != nil {
				return nil, fmt.Errorf("parse recipient %q: %w", r, err)
			}
			result = append(result, sshRec)
			continue
		}
		result = append(result, rec)
	}

	return result, nil
}

// loadRecipientsFile loads recipients from a file (one per line).
func loadRecipientsFile(path string) ([]age.Recipient, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var recipients []age.Recipient
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		rec, err := age.ParseX25519Recipient(line)
		if err != nil {
			// Try as SSH public key
			sshRec, sshErr := agessh.ParseRecipient(line)
			if sshErr != nil {
				return nil, fmt.Errorf("parse recipient %q: %w", line, err)
			}
			recipients = append(recipients, sshRec)
			continue
		}
		recipients = append(recipients, rec)
	}

	return recipients, scanner.Err()
}
