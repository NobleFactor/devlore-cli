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

func NewProvider(ctx op.Context) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(ctx)}
}

// DecryptSopsFile takes a file.Resource, reads it into memory, and decrypts it via SOPS.
//
// Parameters:
//   - source: file resource identifying the encrypted SOPS file
//   - destination: file resource identifying where to write the decrypted content
func (p *Provider) DecryptSopsFile(source file.Resource, destination file.Resource) (file.Resource, Tombstone, error) {

	root := p.Context().Root

	// 1. Read the source file into memory
	data, err := root.ReadFile(root.NewPath(source.SourcePath.Abs()))
	if err != nil {
		return file.Resource{}, Tombstone{}, fmt.Errorf("failed to read source: %w", err)
	}

	// 2. Decrypt via SopsClient
	sopsClient := p.Context().SopsClient
	if sopsClient == nil {
		return file.Resource{}, Tombstone{}, fmt.Errorf("sops client not configured")
	}

	cleartext, err := sopsClient.Decrypt(data, source.SourcePath.Abs())
	if err != nil {
		return file.Resource{}, Tombstone{}, fmt.Errorf("sops decryption failed: %w", err)
	}

	// 3. Write cleartext to the destination path
	if err := root.WriteFile(root.NewPath(destination.SourcePath.Abs()), cleartext, 0o600); err != nil {
		return file.Resource{}, Tombstone{}, fmt.Errorf("failed to write destination: %w", err)
	}

	// 4. Wrap the new file in a Resource
	result := file.NewResource(destination.SourcePath.Abs())
	if err := result.Resolve(root); err != nil {
		return file.Resource{}, Tombstone{}, fmt.Errorf("failed to resolve destination: %w", err)
	}

	return result, Tombstone{DestinationPath: destination.SourcePath.Abs()}, nil
}

// CompensateDecryptSopsFile removes the decrypted file created by DecryptSopsFile.
func (p *Provider) CompensateDecryptSopsFile(state Tombstone) error {

	if state.DestinationPath == "" {
		return nil
	}

	root := p.Context().Root
	return root.Remove(root.NewPath(state.DestinationPath))
}
