// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package sops

import "fmt"

// Encrypter encrypts cleartext for the SOPS recipients resolved from the `.sops.yaml` governing a target path.
//
// It owns the per-session config-discovery cache (the upward walk, memoized). One Encrypter is held per encryption
// provider, which is itself per-RuntimeEnvironment.
//
// STUB: discovery, getsops recipient resolution, and `sops.Tree` encryption are not yet implemented — see
// docs/plans/extract-starlark-from-op/phase-8/sops-config-discovery.md. The visited cache fields land with that work.
type Encrypter struct{}

// NewEncrypter returns an Encrypter ready to encrypt; discovery state is built lazily on first use.
//
// Returns:
//   - `*Encrypter`: a ready encrypter.
func NewEncrypter() *Encrypter {
	return &Encrypter{}
}

// Encrypt encrypts data for the SOPS recipients governing sourcePath and returns the encrypted SOPS document.
//
// Discovery walks up from sourcePath's directory to rootDir, then the XDG fallback, to locate the `.sops.yaml`;
// getsops resolves the matching creation rule's recipients; the document is emitted in the format inferred from
// sourcePath.
//
// Parameters:
//   - `data`: the cleartext to encrypt.
//   - `sourcePath`: the target file's path; selects the creation rule and the document format.
//   - `rootDir`: the upper boundary for the `.sops.yaml` walk (the RuntimeEnvironment Root directory).
//
// Returns:
//   - `[]byte`: the encrypted SOPS document.
//   - `error`: discovery, resolution, or encryption failure.
func (e *Encrypter) Encrypt(data []byte, sourcePath, rootDir string) ([]byte, error) {
	return nil, fmt.Errorf("sops.Encrypter.Encrypt: not yet implemented")
}
