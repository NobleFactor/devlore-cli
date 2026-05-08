// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"encoding/hex"
	"fmt"
	"regexp"
)

// Digest is the honest content hash of a [Resource]. It is one of two change-detection signals every Resource
// exposes; the other is the cheap [Resource.Etag]. The catalog consults Digest only when Etag mismatches —
// touch-style drift (mtime updates without content change) is caught at the Etag tier and never reaches Digest.
//
// Algorithm names use the OCI convention: a lowercase identifier such as "sha256". Bytes is the raw digest
// payload; render the canonical "<algo>:<hex>" form via [Digest.String].
type Digest struct {
	Algorithm string
	Bytes     []byte
}

// digestPattern is the strict CAS digest grammar: a lowercase algorithm token, a colon, and lowercase hex
// content. Uppercase hex, missing payload, and embedded whitespace all fail to match. Algorithm allowlist
// (length, supported names) is enforced after the regex match by [ParseDigest].
var digestPattern = regexp.MustCompile(`^([a-z][a-z0-9]*):([0-9a-f]+)$`)

// ParseDigest parses the canonical "<algo>:<hex>" digest form into a [Digest].
//
// Strict per the locked decision in 13.0(k) F5: algorithm must be a lowercase identifier in the supported
// allowlist (currently sha256 only); hex must be lowercase; sha256 payloads must be exactly 32 bytes.
// Uppercase hex, malformed shape, unknown algorithm, and wrong-length payloads all fail with an explicit
// error.
//
// Parameters:
//   - s: the canonical digest string.
//
// Returns:
//   - Digest: the parsed digest with Algorithm and Bytes populated.
//   - error: non-nil on any syntactic or semantic defect.
func ParseDigest(s string) (Digest, error) {

	m := digestPattern.FindStringSubmatch(s)
	if m == nil {
		return Digest{}, fmt.Errorf("op.ParseDigest: malformed digest %q (want \"<algo>:<hex>\")", s)
	}

	algo, encoded := m[1], m[2]

	bytes, err := hex.DecodeString(encoded)
	if err != nil {
		return Digest{}, fmt.Errorf("op.ParseDigest: hex decode %q: %w", encoded, err)
	}

	switch algo {
	case "sha256":
		if len(bytes) != 32 {
			return Digest{}, fmt.Errorf("op.ParseDigest: sha256 requires 32 bytes, got %d", len(bytes))
		}
	default:
		return Digest{}, fmt.Errorf("op.ParseDigest: unsupported algorithm %q", algo)
	}

	return Digest{Algorithm: algo, Bytes: bytes}, nil
}

// String returns the canonical "<algo>:<hex>" form. Hex is lowercase; round-trips through [ParseDigest].
func (d Digest) String() string {
	return d.Algorithm + ":" + hex.EncodeToString(d.Bytes)
}

// Equal reports whether d and other represent the same content under the same algorithm.
//
// Two Digests with different algorithms are never equal even when the bytes coincidentally match — they encode
// hashes from different functions and represent unrelated identities. Reflexivity, symmetry, and consistency
// follow trivially from byte-for-byte comparison.
//
// Parameters:
//   - other: the digest to compare against.
//
// Returns:
//   - bool: true iff Algorithm and Bytes match exactly.
func (d Digest) Equal(other Digest) bool {

	if d.Algorithm != other.Algorithm {
		return false
	}

	if len(d.Bytes) != len(other.Bytes) {
		return false
	}

	for i := range d.Bytes {
		if d.Bytes[i] != other.Bytes[i] {
			return false
		}
	}

	return true
}