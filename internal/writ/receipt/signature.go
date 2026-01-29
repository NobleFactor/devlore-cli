// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package receipt

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"filippo.io/age"
	"gopkg.in/yaml.v3"
)

// Signature contains the cryptographic signature of a receipt.
type Signature struct {
	// Method is the signing method used (always "age" for now).
	Method string `json:"method" yaml:"method"`

	// Value is the age-encrypted hash of the receipt content (base64-encoded).
	Value string `json:"value" yaml:"value"`

	// Recipient is the age public key that can verify the signature.
	Recipient string `json:"recipient" yaml:"recipient"`
}

// Sign signs the receipt using the provided age identity.
// The signature is computed by:
// 1. Serializing the receipt nodes+edges to canonical YAML
// 2. Computing SHA256 of the serialized content
// 3. Encrypting the hash with age to the identity's public key
// 4. Storing the encrypted hash as the signature
func (r *Receipt) Sign(identity *age.X25519Identity) error {
	// Get canonical content (nodes + edges, without signature)
	content, err := r.canonicalContent()
	if err != nil {
		return fmt.Errorf("serialize receipt: %w", err)
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
	r.Signature = &Signature{
		Method:    "age",
		Value:     base64.StdEncoding.EncodeToString(buf.Bytes()),
		Recipient: recipient.String(),
	}

	return nil
}

// Verify verifies the receipt signature using the provided identities.
// Returns nil if the signature is valid, an error otherwise.
func (r *Receipt) Verify(identities []age.Identity) error {
	if r.Signature == nil {
		// Unsigned receipt - check if it was loaded from legacy
		if r.Version == "2" || r.Version == "1" {
			return nil // Legacy receipts allowed during migration
		}
		return fmt.Errorf("receipt missing signature")
	}

	if r.Signature.Method != "age" {
		return fmt.Errorf("unsupported signature method: %s", r.Signature.Method)
	}

	// Decode the encrypted signature
	encrypted, err := base64.StdEncoding.DecodeString(r.Signature.Value)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}

	// Decrypt the hash using identities
	reader, err := age.Decrypt(bytes.NewReader(encrypted), identities...)
	if err != nil {
		return fmt.Errorf("receipt signature invalid, redeploy to regenerate: %w", err)
	}

	decryptedHash, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("read decrypted hash: %w", err)
	}

	// Get canonical content and compute expected hash
	content, err := r.canonicalContent()
	if err != nil {
		return fmt.Errorf("serialize receipt: %w", err)
	}
	expectedHash := sha256.Sum256(content)

	// Compare hashes
	if !bytes.Equal(decryptedHash, expectedHash[:]) {
		return fmt.Errorf("receipt signature invalid, redeploy to regenerate")
	}

	return nil
}

// IsSigned returns true if the receipt has a signature.
func (r *Receipt) IsSigned() bool {
	return r.Signature != nil
}

// IsLegacy returns true if this is an unsigned legacy receipt (v1 or v2).
func (r *Receipt) IsLegacy() bool {
	return r.Signature == nil && (r.Version == "1" || r.Version == "2")
}

// canonicalContent returns the receipt serialized without the signature field.
// For v4: serializes version, format, timestamp, tool, platform, context, roots, nodes, edges.
// This ensures consistent content for signing and verification.
func (r *Receipt) canonicalContent() ([]byte, error) {
	type CanonicalReceipt struct {
		Version   string      `yaml:"version"`
		Format    string      `yaml:"format"`
		Timestamp string      `yaml:"timestamp"`
		Tool      string      `yaml:"tool"`
		Platform  Platform    `yaml:"platform"`
		Context   WritContext `yaml:"context"`
		Roots     []string    `yaml:"roots"`
		Nodes     []Node      `yaml:"nodes"`
		Edges     []Edge      `yaml:"edges,omitempty"`
	}

	canonical := CanonicalReceipt{
		Version:   r.Version,
		Format:    r.Format,
		Timestamp: r.Timestamp.Format("2006-01-02T15:04:05.999999999Z07:00"),
		Tool:      r.Tool,
		Platform:  r.Platform,
		Context:   r.Context,
		Roots:     r.Roots,
		Nodes:     r.Nodes,
		Edges:     r.Edges,
	}

	return yaml.Marshal(canonical)
}

// VerifyResult represents the result of signature verification.
type VerifyResult int

const (
	// VerifyOK means the signature is valid.
	VerifyOK VerifyResult = iota
	// VerifyLegacy means the receipt is unsigned but allowed (v1/v2).
	VerifyLegacy
	// VerifyInvalid means the signature is invalid.
	VerifyInvalid
	// VerifyMissing means the receipt is missing (no signature, not legacy).
	VerifyMissing
)

// String returns a human-readable description of the verify result.
func (v VerifyResult) String() string {
	switch v {
	case VerifyOK:
		return "signature valid"
	case VerifyLegacy:
		return "unsigned (legacy)"
	case VerifyInvalid:
		return "signature invalid"
	case VerifyMissing:
		return "signature missing"
	default:
		return "unknown"
	}
}

// VerifyWithResult verifies the signature and returns a detailed result.
func (r *Receipt) VerifyWithResult(identities []age.Identity) (VerifyResult, error) {
	if r.Signature == nil {
		if r.Version == "2" || r.Version == "1" {
			return VerifyLegacy, nil
		}
		return VerifyMissing, fmt.Errorf("receipt missing signature")
	}

	if err := r.Verify(identities); err != nil {
		if strings.Contains(err.Error(), "invalid") {
			return VerifyInvalid, err
		}
		return VerifyInvalid, err
	}

	return VerifyOK, nil
}
