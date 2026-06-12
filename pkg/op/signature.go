// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// Signature is the provenance signature carried by a signable artifact: a [Graph] or a [Trace].
type Signature struct {

	// Algorithm identifies the signature scheme: "ed25519" or "ecdsa-p256".
	Algorithm string `json:"algorithm" yaml:"algorithm"`

	// PublicKey is the verifying key; empty when the verifier supplies it out of band.
	PublicKey []byte `json:"public_key,omitempty" yaml:"public_key,omitempty"`

	// Value is the raw signature over the artifact's canonical bytes.
	Value []byte `json:"value" yaml:"value"`
}
