// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package encryption provides encryption and decryption actions for the operation graph.
package encryption

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	"github.com/NobleFactor/devlore-cli/pkg/sops"
)

// Provider provides encryption and decryption actions.
//
// +devlore:access=planned
type Provider struct {
	op.ProviderBase
	sops sops.Client
}

func NewProvider(runtimeEnvironment *op.RuntimeEnvironment) *Provider {

	return &Provider{
		ProviderBase: op.NewProviderBase(runtimeEnvironment),
	}
}

// DecryptSopsFile reads an encrypted SOPS file and writes the decrypted content to destinationPath.
//
// Identity for the destination is constructed by [file.NewResource].
//
// Parameters:
//   - `source`: [file.Resource] identifying the encrypted SOPS file.
//   - `destinationPath`: the path where the decrypted content will be written.
//
// Returns:
//   - `*file.Resource`: the destination resource with populated metadata.
//   - `Receipt`: compensation state for removing the decrypted file.
//   - `error`: any error from reading, decrypting, or writing.
func (p *Provider) DecryptSopsFile(source *file.Resource, destinationPath string) (*file.Resource, *Receipt, error) {

	result, err := file.DiscoverResource(p.RuntimeEnvironment(), destinationPath)

	if err != nil {
		return nil, nil, err
	}

	root := p.RuntimeEnvironment().Root

	// 1. Read the source file into memory
	data, err := root.ReadFile(root.NewPath(source.SourcePath.Abs()))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read source: %w", err)
	}

	// 2. Decrypt via SopsClient

	cleartext, err := p.sops.Decrypt(data, source.SourcePath.Abs())
	if err != nil {
		return nil, nil, fmt.Errorf("sops decryption failed: %w", err)
	}

	// 3. Write cleartext to the destination path
	if err := root.WriteFile(root.NewPath(result.SourcePath.Abs()), cleartext, 0o600); err != nil {
		return nil, nil, fmt.Errorf("failed to write destination: %w", err)
	}

	if err := result.Resolve(); err != nil {
		return nil, nil, fmt.Errorf("failed to resolve destination: %w", err)
	}

	return result, &Receipt{ReceiptBase: op.NewReceiptBase(result)}, nil
}

// CompensateDecryptSopsFile removes the decrypted file created by DecryptSopsFile.
func (p *Provider) CompensateDecryptSopsFile(receipt *Receipt) error {

	if receipt == nil || receipt.Resource() == nil {
		return nil
	}

	resource, ok := receipt.Resource().(*file.Resource)
	if !ok {
		return fmt.Errorf("compensate decrypt sops file: unexpected resource type %T", receipt.Resource())
	}

	root := p.RuntimeEnvironment().Root
	return root.Remove(root.NewPath(resource.SourcePath.Abs()))
}
