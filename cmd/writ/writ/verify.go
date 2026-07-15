// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package writ

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"

	"filippo.io/age"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// VerifyResult indicates the outcome of signature verification.
type VerifyResult int

// VerifyOK indicates the signature matched, VerifyUnsigned means no signature was
// present, VerifyInvalid means the signature failed verification, and VerifyMissing
// means the expected signature data was absent.
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
//
// The signature is read through the sealed accessor ([op.Graph.Signature]); its [op.Signature.Value] carries the
// age-encrypted SHA-256 over the graph's canonical content as raw bytes, and [op.Signature.Algorithm] names the
// scheme.
//
// Parameters:
//   - `g`: the graph whose signature to verify.
//   - `identities`: the age identities to decrypt the signature with.
//
// Returns:
//   - `VerifyResult`: valid / unsigned / invalid / missing.
//   - `error`: the reason when the result is not [VerifyOK] / [VerifyUnsigned].
func VerifyGraphSignature(g *op.Graph, identities []age.Identity) (VerifyResult, error) {

	signature := g.Signature()
	if signature == nil {
		return VerifyUnsigned, nil
	}

	if signature.Algorithm != "age" {
		return VerifyInvalid, fmt.Errorf("unsupported signature algorithm: %s", signature.Algorithm)
	}

	// Decrypt the hash using identities
	reader, err := age.Decrypt(bytes.NewReader(signature.Value), identities...)
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
