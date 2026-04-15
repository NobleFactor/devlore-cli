// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package encryption provides encryption and decryption actions for the operation graph.
package encryption

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
)

// Provider provides encryption and decryption actions.
//
// +devlore:access=planned
type Provider struct {
	op.ProviderBase
}

func NewProvider(ctx *op.ExecutionContext) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(ctx)}
}

// DecryptSopsFile reads an encrypted SOPS file and writes the decrypted content to destinationPath.
//
// Identity for the destination is constructed by [Provider.DecryptSopsFilePlanned].
//
// Parameters:
//   - source: [file.Resource] identifying the encrypted SOPS file.
//   - destinationPath: the path where the decrypted content will be written. Coerced to a [file.Resource]
//     via [Provider.DecryptSopsFilePlanned].
//
// Returns:
//   - *file.Resource: the destination resource with populated metadata.
//   - Tombstone: compensation state for removing the decrypted file.
//   - error: any error from reading, decrypting, or writing.
func (p *Provider) DecryptSopsFile(source *file.Resource, destinationPath string) (*file.Resource, Tombstone, error) {

	result, err := p.DecryptSopsFilePlanned(source, destinationPath)
	if err != nil {
		return nil, Tombstone{}, fmt.Errorf("failed to plan destination: %w", err)
	}

	root := p.ExecutionContext().Root

	// 1. Read the source file into memory
	data, err := root.ReadFile(root.NewPath(source.SourcePath.Abs()))
	if err != nil {
		return nil, Tombstone{}, fmt.Errorf("failed to read source: %w", err)
	}

	// 2. Decrypt via SopsClient
	sopsClient := p.ExecutionContext().Sops
	if sopsClient == nil {
		return nil, Tombstone{}, fmt.Errorf("sops client not configured")
	}

	cleartext, err := sopsClient.Decrypt(data, source.SourcePath.Abs())
	if err != nil {
		return nil, Tombstone{}, fmt.Errorf("sops decryption failed: %w", err)
	}

	// 3. Write cleartext to the destination path
	if err := root.WriteFile(root.NewPath(result.SourcePath.Abs()), cleartext, 0o600); err != nil {
		return nil, Tombstone{}, fmt.Errorf("failed to write destination: %w", err)
	}

	if err := result.Resolve(); err != nil {
		return nil, Tombstone{}, fmt.Errorf("failed to resolve destination: %w", err)
	}

	return result, Tombstone{DestinationPath: result.SourcePath.Abs()}, nil
}

// DecryptSopsFilePlanned is the Planned companion for [Provider.DecryptSopsFile]. Pure: no I/O.
//
// Parameters:
//   - source: ignored; present to match [Provider.DecryptSopsFile]'s signature exactly.
//   - destinationPath: the destination path whose identity should be constructed.
//
// Returns:
//   - *file.Resource: the destination resource with URI set and metadata empty.
//   - error: any error from resource construction.
func (p *Provider) DecryptSopsFilePlanned(_ *file.Resource, destinationPath string) (*file.Resource, error) {
	return file.NewResource(p.ExecutionContext(), destinationPath)
}

// CompensateDecryptSopsFile removes the decrypted file created by DecryptSopsFile.
func (p *Provider) CompensateDecryptSopsFile(state Tombstone) error {

	if state.DestinationPath == "" {
		return nil
	}

	root := p.ExecutionContext().Root
	return root.Remove(root.NewPath(state.DestinationPath))
}
