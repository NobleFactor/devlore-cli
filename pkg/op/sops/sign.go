// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package sops

import "fmt"

// Signature represents a cryptographic signature produced by a SOPS-configured backend.
type Signature struct {
	Method string `json:"method" yaml:"method"` // gpg, aws_kms, gcp_kms, azure_kv
	Value  string `json:"value" yaml:"value"`   // base64-encoded signature data
	KeyID  string `json:"key_id" yaml:"key_id"` // fingerprint, ARN, key URL
}

// signer is the internal interface for signing backends.
type signer interface {
	name() string
	available() bool
	sign(data []byte) (*Signature, error)
}

// backend represents a signing backend type.
type backend string

// Signing backend constants.
const (
	backendPGP     backend = "pgp"
	backendAWSKMS  backend = "aws_kms"
	backendGCPKMS  backend = "gcp_kms"
	backendAzureKV backend = "azure_kv"
)

// parsedBackend contains the backend type and its configuration value.
type parsedBackend struct {
	backendType backend
	value       string
}

// region EXPORTED METHODS

// region Behaviors

// Fallible actions

// Sign signs data using the first available backend from .sops.yaml. Returns nil signature if no signing backends are
// configured (age-only configs have no signing capability).
//
// Parameters:
//   - data: content to sign
//
// Returns:
//   - *Signature: the cryptographic signature, or nil if no backends are available
//   - error: signing error (nil when no backends are available)
func (c *Client) Sign(data []byte) (*Signature, error) {

	signers := c.buildSigners()
	for _, s := range signers {
		if !s.available() {
			continue
		}
		sig, err := s.sign(data)
		if err != nil {
			// This signer failed, try the next one
			continue
		}
		return sig, nil
	}
	// No signers available or all failed — signing is optional
	return nil, nil
}

// Verify checks a signature against data using the backend identified by sig.Method.
//
// Parameters:
//   - data: original content that was signed
//   - sig: signature to verify
//
// Returns:
//   - error: verification error
func (c *Client) Verify(data []byte, sig *Signature) error {

	switch sig.Method {
	case "gpg":
		return verifyGPG(data, sig)
	case "aws_kms":
		return verifyAWSKMS(data, sig)
	case "gcp_kms":
		return verifyGCPKMS(data, sig)
	case "azure_kv":
		return verifyAzureKV(data, sig)
	default:
		return fmt.Errorf("sops: unknown signature method %q", sig.Method)
	}
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// buildSigners creates signer instances from the first creation rule's backends.
//
// Returns:
//   - []signer: ordered signer instances
func (c *Client) buildSigners() []signer {

	if len(c.config.CreationRules) == 0 {
		return nil
	}

	rule := c.config.CreationRules[0]
	backends := orderedBackends(&rule)

	signers := make([]signer, 0, len(backends))
	for _, b := range backends {
		switch b.backendType {
		case backendPGP:
			signers = append(signers, newGPGSigner(b.value))
		case backendAWSKMS:
			signers = append(signers, newAWSKMSSigner(b.value))
		case backendGCPKMS:
			signers = append(signers, newGCPKMSSigner(b.value))
		case backendAzureKV:
			signers = append(signers, newAzureKVSigner(b.value))
		}
	}

	return signers
}

// endregion

// endregion

// orderedBackends returns the backends from a creation rule in priority order (Azure → GCP → AWS → GPG).
//
// Parameters:
//   - rule: creation rule to extract backends from
//
// Returns:
//   - []parsedBackend: backends in priority order
func orderedBackends(rule *creationRule) []parsedBackend {

	var backends []parsedBackend

	if rule.AzureKV != "" {
		backends = append(backends, parsedBackend{backendType: backendAzureKV, value: rule.AzureKV})
	}
	if rule.GCPKMS != "" {
		backends = append(backends, parsedBackend{backendType: backendGCPKMS, value: rule.GCPKMS})
	}
	if rule.AWSKMS != "" {
		backends = append(backends, parsedBackend{backendType: backendAWSKMS, value: rule.AWSKMS})
	}
	if rule.PGP != "" {
		backends = append(backends, parsedBackend{backendType: backendPGP, value: rule.PGP})
	}

	return backends
}
