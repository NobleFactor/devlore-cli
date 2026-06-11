// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// Signature is the provenance signature carried by a signable artifact — a [Graph] or a trace.
//
// Placeholder until pkg/signing lands (data-layer signing over the artifact's canonical bytes). `Value` is the raw
// signature; `Algorithm` and `PublicKey` carry the verification material so the envelope is self-describing — Ed25519
// produces a 64-byte `Value`, ECDSA P-256 an ASN.1-DER `Value`. When pkg/signing is designed, evaluate a standard
// envelope (DSSE / JWS / COSE) rather than extending this struct by hand.
type Signature struct {

	// Algorithm identifies the signature scheme: "ed25519" or "ecdsa-p256".
	Algorithm string `json:"algorithm" yaml:"algorithm"`

	// Value is the raw signature over the artifact's canonical bytes.
	Value []byte `json:"value" yaml:"value"`

	// PublicKey is the verifying key; empty when the verifier supplies it out of band.
	PublicKey []byte `json:"public_key,omitempty" yaml:"public_key,omitempty"`
}
