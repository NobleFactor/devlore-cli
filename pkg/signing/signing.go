// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package signing signs and verifies the two signable op artifacts — graphs and execution traces — per the
// settled signing design (docs/plans/extract-starlark-from-op/phase-8/signing-options.md and graph-signing.md;
// phase-8 step 46).
//
// The model is publisher identity with verifier-side trust: a raw ssh-ed25519 signature over the artifact's
// namespace-prefixed canonical bytes ([NamespaceGraph] / [NamespaceTrace] give domain separation), the
// publisher's key riding the document in OpenSSH wire format, and trust resolved against an OpenSSH
// `allowed_signers` file the verifier owns. No envelope, no hash options — the algorithm names the whole
// ciphersuite.
//
// This package implements the DEFAULT custody tier: the developer's SSH key (`~/.ssh/id_ed25519`) with a
// generated local Ed25519 keyfile as the fallback. The ssh-agent, cloud-KMS, and keyless tiers are chartered
// opt-ins (the support matrix in signing-options.md) and are deliberately absent so their dependency weight
// stays out of the default build.
//
// What a consumer does with a verification outcome is governed by [Policy] — the settled four-tier ladder —
// through one enforcement point, [Judge]. Like its prior art (PowerShell's ExecutionPolicy), the policy is a
// safety feature, not a security boundary.
package signing

import (
	"crypto/ed25519"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

const (
	// NamespaceGraph is the domain-separation prefix signed ahead of a graph's canonical bytes.
	NamespaceGraph = "devlore.graph.v1"

	// NamespaceTrace is the domain-separation prefix signed ahead of a trace's canonical bytes.
	NamespaceTrace = "devlore.trace.v1"

	// AlgorithmEd25519 is the default ciphersuite — Ed25519 under its OpenSSH key-type name.
	AlgorithmEd25519 = "ssh-ed25519"
)

// Signer signs canonical artifact bytes under a namespace, producing the document's [op.Signature].
type Signer interface {

	// Sign produces the signature over `namespace ‖ canonical`.
	//
	// Parameters:
	//   - `namespace`: the artifact-kind domain separator ([NamespaceGraph] or [NamespaceTrace]).
	//   - `canonical`: the artifact's canonical bytes.
	//
	// Returns:
	//   - `*op.Signature`: the algorithm, the publisher key (OpenSSH wire format), and the raw signature.
	//   - `error`: non-nil when signing fails.
	Sign(namespace string, canonical []byte) (*op.Signature, error)
}

// keyfileSigner is the default-tier [Signer]: a loaded Ed25519 private key.
type keyfileSigner struct {

	// private is the loaded signing key.
	private ed25519.PrivateKey

	// publicWire is the corresponding public key in OpenSSH wire format.
	publicWire []byte
}

// Sign implements [Signer]: a raw ed25519 signature over the namespace-prefixed canonical bytes.
//
// Parameters:
//   - `namespace`: the artifact-kind domain separator.
//   - `canonical`: the artifact's canonical bytes.
//
// Returns:
//   - `*op.Signature`: the ssh-ed25519 record.
//   - `error`: always nil; present to satisfy [Signer].
func (k *keyfileSigner) Sign(namespace string, canonical []byte) (*op.Signature, error) {

	message := append([]byte(namespace), canonical...)

	return &op.Signature{
		Algorithm: AlgorithmEd25519,
		PublicKey: k.publicWire,
		Value:     ed25519.Sign(k.private, message),
	}, nil
}

// DefaultSigner resolves the default-tier signer: the developer's SSH key, else the generated local key.
//
// Resolution order (signing-options.md): `~/.ssh/id_ed25519` when present and parseable without a passphrase;
// otherwise the generated local Ed25519 keyfile under the user config directory
// (`<config>/devlore/signing/ed25519`, created on first use with its `.pub` in authorized_keys format for
// `allowed_signers` seeding). The generated path honors `XDG_CONFIG_HOME` on every platform (the devlore
// XDG convention).
//
// Returns:
//   - `Signer`: the resolved signer.
//   - `error`: non-nil when no key can be loaded or generated.
func DefaultSigner(configRoot fsroot.Root) (Signer, error) {

	// Confinement: the user's own SSH directory is not ours to confine — we read a key they placed there,
	// under their own permissions, and a root anchored at our config tree cannot address it.
	if home, err := os.UserHomeDir(); err == nil {
		if signer, err := loadSSHKeyfile(filepath.Join(home, ".ssh", "id_ed25519")); err == nil {
			return signer, nil
		}
	}

	return localSigner(configRoot)
}

// region HELPER FUNCTIONS

// loadSSHKeyfile loads an OpenSSH Ed25519 private-key file as a signer.
//
// Parameters:
//   - `path`: the private-key file.
//
// Returns:
//   - `Signer`: the keyfile signer.
//   - `error`: non-nil when the file is absent, passphrase-protected, or not an Ed25519 key.
func loadSSHKeyfile(path string) (Signer, error) {

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	parsed, err := ssh.ParseRawPrivateKey(data)
	if err != nil {
		// Passphrase-protected keys land here; the caller falls back to the generated local key.
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	private, ok := parsed.(*ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%s is not an ed25519 key (%T)", path, parsed)
	}

	return newKeyfileSigner(*private)
}

// localSigner loads the generated local key, creating it on first use.
//
// Returns:
//   - `Signer`: the local-keyfile signer.
//   - `error`: non-nil when the key can neither be loaded nor generated.
func localSigner(configRoot fsroot.Root) (Signer, error) {

	keyPath := configRoot.NewPath(filepath.Join("signing", "ed25519"))

	if data, err := configRoot.ReadFile(keyPath); err == nil {
		parsed, parseErr := ssh.ParseRawPrivateKey(data)
		if parseErr != nil {
			return nil, fmt.Errorf("parse %s: %w", keyPath.Abs(), parseErr)
		}
		private, ok := parsed.(*ed25519.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("%s is not an ed25519 key (%T)", keyPath.Abs(), parsed)
		}
		return newKeyfileSigner(*private)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	return generateLocalKey(configRoot, keyPath)
}

// generateLocalKey mints the local Ed25519 keypair at `keyPath` (private, OpenSSH PEM, 0600) with its `.pub`
// (authorized_keys format — one copy-paste from an `allowed_signers` line).
//
// Every write goes through the caller's root, so the 0700 directory and the 0600 key are enforced on Windows
// by a protected DACL rather than being silently ignored (#405). The `.pub` half stays 0644 deliberately: it is
// public trust material, and ruling 4's boundary leaves anything granting `other` on its parent's inherited
// DACL. Only the private half is protected, which is the point.
//
// Parameters:
//   - `configRoot`: the root the key tree is created under, supplied by the caller that owns it.
//   - `keyPath`: the private-key destination within that root.
//
// Returns:
//   - `Signer`: the freshly generated signer.
//   - `error`: non-nil when generation or persistence fails.
func generateLocalKey(configRoot fsroot.Root, keyPath fsroot.Path) (Signer, error) {

	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, fmt.Errorf("generate ed25519 key: %w", err)
	}

	if err := configRoot.MkdirAll(configRoot.NewPath(filepath.Dir(keyPath.Rel())), 0o700); err != nil {
		return nil, err
	}

	pemBlock, err := ssh.MarshalPrivateKey(private, "devlore signing key")
	if err != nil {
		return nil, fmt.Errorf("marshal private key: %w", err)
	}
	if err := configRoot.WriteFile(keyPath, pem.EncodeToMemory(pemBlock), 0o600); err != nil {
		return nil, err
	}

	sshPublic, err := ssh.NewPublicKey(public)
	if err != nil {
		return nil, fmt.Errorf("wrap public key: %w", err)
	}
	//nolint:gosec // G306: the .pub half is public trust material, deliberately world-readable (0o644).
	if err := configRoot.WriteFile(configRoot.NewPath(keyPath.Rel()+".pub"),
		ssh.MarshalAuthorizedKey(sshPublic), 0o644); err != nil {
		return nil, err
	}

	return newKeyfileSigner(private)
}

// configHome resolves the user config directory per the devlore XDG convention (XDG_CONFIG_HOME, else
// ~/.config on every platform — matching internal/cli's xdg helpers; [os.UserConfigDir] diverges on darwin).
//
// Returns:
//   - `string`: the config home.
//   - `error`: non-nil when the home directory cannot be resolved.
func configHome() (string, error) {

	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return xdg, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config"), nil
}

// newKeyfileSigner wraps a loaded private key with its wire-format public half.
//
// Parameters:
//   - `private`: the Ed25519 private key.
//
// Returns:
//   - `Signer`: the keyfile signer.
//   - `error`: non-nil when the public half cannot be wrapped.
func newKeyfileSigner(private ed25519.PrivateKey) (Signer, error) {

	sshPublic, err := ssh.NewPublicKey(assert.Type[ed25519.PublicKey]("ed25519 public key", private.Public()))
	if err != nil {
		return nil, fmt.Errorf("wrap public key: %w", err)
	}

	return &keyfileSigner{private: private, publicWire: sshPublic.Marshal()}, nil
}

// endregion
