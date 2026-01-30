// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package writ

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"

	"filippo.io/age"

	"github.com/NobleFactor/devlore-cli/internal/execution"
)

// VerifyResult indicates the outcome of signature verification.
type VerifyResult int

const (
	VerifyOK VerifyResult = iota
	VerifyUnsigned
	VerifyInvalid
	VerifyMissing
)

// String returns a human-readable description of the verify result.
func (r VerifyResult) String() string {
	switch r {
	case VerifyOK:
		return "valid"
	case VerifyUnsigned:
		return "unsigned"
	case VerifyInvalid:
		return "invalid"
	case VerifyMissing:
		return "missing"
	default:
		return "unknown"
	}
}

// VerifyGraphSignature verifies the graph signature using the provided identities.
// Returns the verification result and any error encountered.
func VerifyGraphSignature(g *execution.Graph, identities []age.Identity) (VerifyResult, error) {
	if g.Signature == nil {
		return VerifyUnsigned, nil
	}

	if g.Signature.Method != "age" {
		return VerifyInvalid, fmt.Errorf("unsupported signature method: %s", g.Signature.Method)
	}

	// Decode the encrypted signature
	encrypted, err := base64.StdEncoding.DecodeString(g.Signature.Value)
	if err != nil {
		return VerifyInvalid, fmt.Errorf("decode signature: %w", err)
	}

	// Decrypt the hash using identities
	reader, err := age.Decrypt(bytes.NewReader(encrypted), identities...)
	if err != nil {
		return VerifyInvalid, fmt.Errorf("decrypt signature: %w", err)
	}

	decryptedHash, err := io.ReadAll(reader)
	if err != nil {
		return VerifyInvalid, fmt.Errorf("read decrypted hash: %w", err)
	}

	// Get canonical content and compute expected hash
	canonical, err := g.CanonicalContent()
	if err != nil {
		return VerifyInvalid, fmt.Errorf("canonical content: %w", err)
	}
	expectedHash := sha256.Sum256(canonical)

	// Compare hashes
	if !bytes.Equal(decryptedHash, expectedHash[:]) {
		return VerifyInvalid, fmt.Errorf("hash mismatch")
	}

	return VerifyOK, nil
}
