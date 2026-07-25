// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// Signature is the publisher signature carried by a signable artifact: a [Graph] or a [Trace].
//
// The three-field record is settled in the signing design (docs/plans/.../signing-options.md): the algorithm
// names the whole ciphersuite, the value is a RAW signature (no envelope) over the artifact's
// namespace-prefixed canonical bytes, and the public key is the publisher's key in OpenSSH wire format — what
// the verifier's `allowed_signers` trust list keys on. Deliberately absent: a hash field (intrinsic to the
// ciphersuite), an envelope, the trust list, and any identity string (identity comes from the verifier's
// mapping, never from the document).
type Signature struct {

	// Algorithm is the full ciphersuite as an OpenSSH key-type name: "ssh-ed25519" (the default);
	// "ecdsa-sha2-nistp256"/"-384" and "rsa-sha2-256"/"-512" are acceptable suites for later backends.
	Algorithm string `json:"algorithm" yaml:"algorithm"`

	// PublicKey is the publisher's verifying key in OpenSSH wire format.
	PublicKey []byte `json:"public_key,omitempty" yaml:"public_key,omitempty"`

	// Value is the raw signature over `namespace ‖ CanonicalContent` (domain-separated per artifact kind:
	// devlore.graph.v1 / devlore.trace.v1 — the pkg/signing namespace constants).
	Value []byte `json:"value" yaml:"value"`
}
