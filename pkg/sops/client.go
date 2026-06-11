// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package sops provides SOPS decryption and encryption detection over the getsops library. Decryption is config-free
// — the encrypted file carries its own recipients and is unlocked with ambient identities — so the client holds no
// configuration. (Encryption discovery and signing live elsewhere: encryption per-file via getsops, signing in
// pkg/signing.)
package sops

// Client provides SOPS operations over getsops. It carries no configuration: decryption needs none.
type Client struct{}
