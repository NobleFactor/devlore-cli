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

	// sops is the SOPS client used to decrypt encrypted documents.
	sops sops.Client

	// encrypter is the SOPS encrypter used to encrypt cleartext documents.
	encrypter *sops.Encrypter
}

// NewProvider creates an encryption provider bound to the given runtime environment.
func NewProvider(runtimeEnvironment *op.RuntimeEnvironment) *Provider {

	return &Provider{
		ProviderBase: op.NewProviderBase(runtimeEnvironment),
		encrypter:    sops.NewEncrypter(),
	}
}

// region EXPORTED METHODS

// region Behaviors

// Compensable actions

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
//   - `*Receipt`: compensation state for removing the decrypted file.
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
//
// Parameters:
//   - `receipt`: the [Receipt] from [Provider.DecryptSopsFile]; nil or nil-resource receipts return nil.
//
// Returns:
//   - `error`: non-nil when the decrypted file cannot be removed or the receipt's resource is not a [file.Resource].
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

// EncryptFile reads source's cleartext and writes the SOPS-encrypted content to destinationPath.
//
// Recipients and document format come from the `.sops.yaml` governing source's path — discovered by the
// [sops.Encrypter] walking up from source to the [RuntimeEnvironment] Root, then the XDG fallback. Identity for the
// destination is constructed by [file.DiscoverResource].
//
// Parameters:
//   - `source`: [file.Resource] identifying the cleartext file to encrypt.
//   - `destinationPath`: the path where the encrypted content will be written.
//
// Returns:
//   - `*file.Resource`: the destination resource with populated metadata.
//   - `*Receipt`: compensation state for removing the encrypted file.
//   - `error`: any error from reading, encrypting, or writing.
func (p *Provider) EncryptFile(source *file.Resource, destinationPath string) (*file.Resource, *Receipt, error) {

	result, err := file.DiscoverResource(p.RuntimeEnvironment(), destinationPath)

	if err != nil {
		return nil, nil, err
	}

	root := p.RuntimeEnvironment().Root

	// 1. Read the source cleartext into memory
	data, err := root.ReadFile(root.NewPath(source.SourcePath.Abs()))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read source: %w", err)
	}

	// 2. Encrypt for the recipients resolved from the .sops.yaml governing the source path
	ciphertext, err := p.encrypter.Encrypt(data, source.SourcePath.Abs(), root.Name())
	if err != nil {
		return nil, nil, fmt.Errorf("sops encryption failed: %w", err)
	}

	// 3. Write the ciphertext to the destination path
	if err := root.WriteFile(root.NewPath(result.SourcePath.Abs()), ciphertext, 0o600); err != nil {
		return nil, nil, fmt.Errorf("failed to write destination: %w", err)
	}

	if err := result.Resolve(); err != nil {
		return nil, nil, fmt.Errorf("failed to resolve destination: %w", err)
	}

	return result, &Receipt{ReceiptBase: op.NewReceiptBase(result)}, nil
}

// CompensateEncryptFile removes the encrypted file created by EncryptFile.
//
// Parameters:
//   - `receipt`: the [Receipt] from [Provider.EncryptFile]; nil or nil-resource receipts return nil.
//
// Returns:
//   - `error`: non-nil when the encrypted file cannot be removed or the receipt's resource is not a [file.Resource].
func (p *Provider) CompensateEncryptFile(receipt *Receipt) error {

	if receipt == nil || receipt.Resource() == nil {
		return nil
	}

	resource, ok := receipt.Resource().(*file.Resource)
	if !ok {
		return fmt.Errorf("compensate encrypt file: unexpected resource type %T", receipt.Resource())
	}

	root := p.RuntimeEnvironment().Root
	return root.Remove(root.NewPath(resource.SourcePath.Abs()))
}

// endregion

// endregion
