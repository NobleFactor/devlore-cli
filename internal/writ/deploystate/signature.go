// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package deploystate

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"

	"filippo.io/age"
	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/execution"
)

// Sign signs the state using the provided age identity.
func (s *State) Sign(identity *age.X25519Identity) error {
	// Get canonical content (state without signature)
	content, err := s.canonicalContent()
	if err != nil {
		return fmt.Errorf("serialize state: %w", err)
	}

	// Compute SHA256 hash
	hash := sha256.Sum256(content)

	// Encrypt the hash with age
	recipient := identity.Recipient()
	var buf bytes.Buffer
	writer, err := age.Encrypt(&buf, recipient)
	if err != nil {
		return fmt.Errorf("create age writer: %w", err)
	}
	if _, err := writer.Write(hash[:]); err != nil {
		return fmt.Errorf("write hash: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close age writer: %w", err)
	}

	// Store signature
	s.Signature = &execution.Signature{
		Method:    "age",
		Value:     base64.StdEncoding.EncodeToString(buf.Bytes()),
		Recipient: recipient.String(),
	}

	return nil
}

// Verify verifies the state signature using the provided identities.
func (s *State) Verify(identities []age.Identity) error {
	if s.Signature == nil {
		// Unsigned state - allow for migration
		return nil
	}

	if s.Signature.Method != "age" {
		return fmt.Errorf("unsupported signature method: %s", s.Signature.Method)
	}

	// Decode the encrypted signature
	encrypted, err := base64.StdEncoding.DecodeString(s.Signature.Value)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}

	// Decrypt the hash using identities
	reader, err := age.Decrypt(bytes.NewReader(encrypted), identities...)
	if err != nil {
		return fmt.Errorf("state signature invalid, redeploy to regenerate: %w", err)
	}

	decryptedHash, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("read decrypted hash: %w", err)
	}

	// Get canonical content and compute expected hash
	content, err := s.canonicalContent()
	if err != nil {
		return fmt.Errorf("serialize state: %w", err)
	}
	expectedHash := sha256.Sum256(content)

	// Compare hashes
	if !bytes.Equal(decryptedHash, expectedHash[:]) {
		return fmt.Errorf("state signature invalid, redeploy to regenerate")
	}

	return nil
}

// IsSigned returns true if the state has a signature.
func (s *State) IsSigned() bool {
	return s.Signature != nil
}

// canonicalContent returns the state serialized without the signature field.
func (s *State) canonicalContent() ([]byte, error) {
	// Create a copy without signature for serialization
	type StateNoSig struct {
		Version     string                `yaml:"version"`
		LastUpdated string                `yaml:"last_updated"`
		SourceRoot  string                `yaml:"source_root"`
		TargetRoot  string                `yaml:"target_root"`
		Files       map[string]*FileEntry `yaml:"files"`
	}

	nosig := StateNoSig{
		Version:     s.Version,
		LastUpdated: s.LastUpdated.Format("2006-01-02T15:04:05.999999999Z07:00"),
		SourceRoot:  s.SourceRoot,
		TargetRoot:  s.TargetRoot,
		Files:       s.Files,
	}

	return yaml.Marshal(nosig)
}
