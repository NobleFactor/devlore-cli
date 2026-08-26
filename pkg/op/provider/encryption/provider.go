// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package encryption provides encryption and decryption actions for the operation graph.
package encryption

import (
	"fmt"
	"os"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	"github.com/NobleFactor/devlore-cli/pkg/sops"
)

// Provider provides encryption and decryption actions.
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
// Identity for the destination is constructed by [file.DiscoverRegular].
//
// `mode` is floored: the decrypted product is plaintext whose sensitivity was already declared by the act of
// encrypting it, so a mode carrying group or other bits is refused rather than honored. 0o600 and 0o400 are the
// useful values; the default is 0o600.
//
// Parameters:
//   - `activationRecord`: the dispatch activation (the required floor for compensable actions — step 27).
//   - `source`: [file.Regular] identifying the encrypted SOPS file.
//   - `destinationPath`: the path where the decrypted content will be written.
//   - `mode`: the [os.FileMode] applied to the decrypted file; refused if it grants group or other access.
//
// Returns:
//   - `*file.Regular`: the destination resource with populated metadata.
//   - `*Receipt`: compensation state for removing the decrypted file.
//   - `error`: any error from the mode floor, reading, decrypting, or writing.
//
// +devlore:defaults mode=0o600
//
// +devlore:claim=sandboxed
func (p *Provider) DecryptSopsFile(activationRecord *op.ActivationRecord, source *file.Regular, destinationPath string, mode os.FileMode) (*file.Regular, *Receipt, error) {

	if err := enforceSecretFloor(mode); err != nil {
		return nil, nil, err
	}

	result, err := file.DiscoverRegular(p.RuntimeEnvironment(), destinationPath)

	if err != nil {
		return nil, nil, err
	}

	root := p.RuntimeEnvironment().Root()

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
	if err := root.WriteFile(root.NewPath(result.SourcePath.Abs()), cleartext, mode); err != nil {
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
//   - `activationRecord`: the dispatch activation (the required floor for compensating actions — step 27).
//   - `receipt`: the [Receipt] from [Provider.DecryptSopsFile]; nil or nil-resource receipts return nil.
//
// Returns:
//   - `error`: non-nil when the decrypted file cannot be removed or the receipt's resource is not a [file.Regular].
func (p *Provider) CompensateDecryptSopsFile(activationRecord *op.ActivationRecord, receipt *Receipt) error {

	if receipt == nil || receipt.Resource() == nil {
		return nil
	}

	resource, ok := receipt.Resource().(*file.Regular)
	if !ok {
		return fmt.Errorf("compensate decrypt sops file: unexpected resource type %T", receipt.Resource())
	}

	root := p.RuntimeEnvironment().Root()
	return root.Remove(root.NewPath(resource.SourcePath.Abs()))
}

// EncryptFile reads source's cleartext and writes the SOPS-encrypted content to destinationPath.
//
// Recipients and document format come from the `.sops.yaml` governing source's path — discovered by the
// [sops.Encrypter] walking up from source to the [RuntimeEnvironment] Root, then the XDG fallback. Identity for the
// destination is constructed by [file.DiscoverRegular].
//
// `mode` is NOT floored here: the product is ciphertext, which is safe at rest by construction and is typically
// committed to a repository that will store it 0o644 regardless. The default stays 0o600 so behavior is unchanged
// unless a caller asks otherwise.
//
// Parameters:
//   - `activationRecord`: the dispatch activation (the required floor for compensable actions — step 27).
//   - `source`: [file.Regular] identifying the cleartext file to encrypt.
//   - `destinationPath`: the path where the encrypted content will be written.
//   - `mode`: the [os.FileMode] applied to the encrypted file.
//
// Returns:
//   - `*file.Regular`: the destination resource with populated metadata.
//   - `*Receipt`: compensation state for removing the encrypted file.
//   - `error`: any error from reading, encrypting, or writing.
//
// +devlore:defaults mode=0o600
//
// +devlore:claim=sandboxed
func (p *Provider) EncryptFile(activationRecord *op.ActivationRecord, source *file.Regular, destinationPath string, mode os.FileMode) (*file.Regular, *Receipt, error) {

	result, err := file.DiscoverRegular(p.RuntimeEnvironment(), destinationPath)

	if err != nil {
		return nil, nil, err
	}

	root := p.RuntimeEnvironment().Root()

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
	if err := root.WriteFile(root.NewPath(result.SourcePath.Abs()), ciphertext, mode); err != nil {
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
//   - `activationRecord`: the dispatch activation (the required floor for compensating actions — step 27).
//   - `receipt`: the [Receipt] from [Provider.EncryptFile]; nil or nil-resource receipts return nil.
//
// Returns:
//   - `error`: non-nil when the encrypted file cannot be removed or the receipt's resource is not a [file.Regular].
func (p *Provider) CompensateEncryptFile(activationRecord *op.ActivationRecord, receipt *Receipt) error {

	if receipt == nil || receipt.Resource() == nil {
		return nil
	}

	resource, ok := receipt.Resource().(*file.Regular)
	if !ok {
		return fmt.Errorf("compensate encrypt file: unexpected resource type %T", receipt.Resource())
	}

	root := p.RuntimeEnvironment().Root()
	return root.Remove(root.NewPath(resource.SourcePath.Abs()))
}

// endregion

// endregion

// ---------------------------------------------------------------------------------------------------- helpers

// enforceSecretFloor rejects a mode that would leave decrypted plaintext readable beyond its owner.
//
// Encrypting a file is itself the declaration that its contents are sensitive, so the deployed mode is derived from
// that declaration rather than re-stated per call site. Taking `mode` as a parameter would reopen that decision to a
// caller who can get it wrong, and a world-readable secret fails silently — the file is written, the run succeeds, and
// nothing reports it. The floor keeps the useful variation (0o600 versus a read-only 0o400) and refuses the rest.
//
// Parameters:
//   - `mode`: the caller-supplied mode for a decrypted product.
//
// Returns:
//   - `error`: non-nil when `mode` grants any group or other access.
func enforceSecretFloor(mode os.FileMode) error {

	if beyondOwner := mode.Perm() & 0o077; beyondOwner != 0 {
		return fmt.Errorf(
			"encryption: mode %04o would leave decrypted plaintext readable beyond its owner (offending bits %04o); "+
				"a decrypted secret must not grant group or other access", mode.Perm(), beyondOwner)
	}

	return nil
}
