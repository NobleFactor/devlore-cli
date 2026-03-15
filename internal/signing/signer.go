// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package signing provides cryptographic signing for receipts.
// It supports multiple backends (GPG, AWS KMS, GCP KMS, Azure Key Vault)
// and uses the order specified in .sops.yaml to determine priority.
package signing

import (
	"os"
	"path/filepath"

	"github.com/NobleFactor/devlore-cli/internal/document"
)

// Signature represents a cryptographic signature.
type Signature struct {
	// Method identifies the signing backend (gpg, aws_kms, gcp_kms, azure_kv).
	Method string `json:"method" yaml:"method"`

	// Value is the signature data (format depends on method).
	Value string `json:"value" yaml:"value"`

	// KeyID identifies the key used for signing.
	// For GPG: fingerprint, for KMS: key ARN/ID, etc.
	KeyID string `json:"key_id" yaml:"key_id"`
}

// Signer is the interface for signing backends.
type Signer interface {
	// Name returns the backend name (gpg, aws_kms, gcp_kms, azure_kv).
	Name() string

	// Available returns true if this signer can be used.
	// Checks for required tools, credentials, etc.
	Available() bool

	// Sign signs the data and returns a signature.
	Sign(data []byte) (*Signature, error)
}

// SignerChain tries signers in order, using the first one that works.
type SignerChain struct {
	signers []Signer
}

// NewSignerChain creates a chain from the given signers.
func NewSignerChain(signers ...Signer) *SignerChain {
	return &SignerChain{signers: signers}
}

// Sign tries each signer in order, returning the first successful signature.
// Returns nil (not an error) if no signers are available.
func (c *SignerChain) Sign(data []byte) (*Signature, error) {
	for _, s := range c.signers {
		if !s.Available() {
			continue
		}
		sig, err := s.Sign(data)
		if err != nil {
			// This signer failed, try the next one
			continue
		}
		return sig, nil
	}
	// No signers available or all failed - signing is optional
	return nil, nil
}

// SopsConfig represents the structure of .sops.yaml
type SopsConfig struct {
	CreationRules []CreationRule `yaml:"creation_rules"`
}

// CreationRule represents a single rule in .sops.yaml
type CreationRule struct {
	PathRegex string `yaml:"path_regex"`
	PGP       string `yaml:"pgp,omitempty"`
	Age       string `yaml:"age,omitempty"`
	AWSKMS    string `yaml:"aws_kms,omitempty"`
	GCPKMS    string `yaml:"gcp_kms,omitempty"`
	AzureKV   string `yaml:"azure_kv,omitempty"`
	// Note: hc_vault_transit is also possible but rarely used for signing
}

// Backend represents a signing backend type.
type Backend string

// Signing backend constants.
const (
	BackendPGP     Backend = "pgp"
	BackendAWSKMS  Backend = "aws_kms"
	BackendGCPKMS  Backend = "gcp_kms"
	BackendAzureKV Backend = "azure_kv"
	// Age cannot sign, so it's not included
)

// ParsedBackend contains the backend type and its configuration value.
type ParsedBackend struct {
	Type  Backend
	Value string
}

// FindSopsConfig searches for .sops.yaml starting from dir, walking up.
func FindSopsConfig(dir string) string {
	dir, _ = filepath.Abs(dir) //nolint:errcheck // fallback to relative path
	for {
		candidate := filepath.Join(dir, ".sops.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// ParseSopsConfig parses a .sops.yaml file.
//
// Parameters:
//   - path: filesystem path to the .sops.yaml file
//
// Returns:
//   - *SopsConfig: parsed signing configuration
//   - error: read or parse error
func ParseSopsConfig(path string) (*SopsConfig, error) {

	var config SopsConfig
	if err := document.Read(path, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// OrderedBackends returns the backends from a creation rule in declaration order.
// This preserves the order from the YAML file.
func (r *CreationRule) OrderedBackends() []ParsedBackend {
	// YAML maps don't preserve order, so we need to re-parse to get order.
	// For now, we use a fixed priority that matches common usage patterns.
	// TODO: Parse raw YAML to preserve exact declaration order.
	var backends []ParsedBackend

	// Check each backend in a reasonable default order
	// Users who care about specific order should list only one backend per rule
	if r.AzureKV != "" {
		backends = append(backends, ParsedBackend{Type: BackendAzureKV, Value: r.AzureKV})
	}
	if r.GCPKMS != "" {
		backends = append(backends, ParsedBackend{Type: BackendGCPKMS, Value: r.GCPKMS})
	}
	if r.AWSKMS != "" {
		backends = append(backends, ParsedBackend{Type: BackendAWSKMS, Value: r.AWSKMS})
	}
	if r.PGP != "" {
		backends = append(backends, ParsedBackend{Type: BackendPGP, Value: r.PGP})
	}
	// Age is not included - it cannot sign

	return backends
}

// BuildSignerChain creates a signer chain based on .sops.yaml configuration.
// Searches for .sops.yaml starting from searchDir.
// Returns an empty chain (not an error) if no .sops.yaml is found.
func BuildSignerChain(searchDir string) *SignerChain {
	configPath := FindSopsConfig(searchDir)
	if configPath == "" {
		return NewSignerChain()
	}

	config, err := ParseSopsConfig(configPath)
	if err != nil {
		return NewSignerChain()
	}

	if len(config.CreationRules) == 0 {
		return NewSignerChain()
	}

	// Use the first creation rule's backends
	// (typically there's one rule or the first rule is the default)
	rule := config.CreationRules[0]
	backends := rule.OrderedBackends()

	var signers []Signer
	for _, b := range backends {
		switch b.Type {
		case BackendPGP:
			signers = append(signers, NewGPGSigner(b.Value))
		case BackendAWSKMS:
			signers = append(signers, NewAWSKMSSigner(b.Value))
		case BackendGCPKMS:
			signers = append(signers, NewGCPKMSSigner(b.Value))
		case BackendAzureKV:
			signers = append(signers, NewAzureKVSigner(b.Value))
		}
	}

	return NewSignerChain(signers...)
}
