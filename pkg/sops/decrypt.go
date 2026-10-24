// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package sops

import (
	"fmt"
	"path/filepath"

	"github.com/getsops/sops/v3/cmd/sops/formats"
	"github.com/getsops/sops/v3/decrypt"
)

// region EXPORTED METHODS

// region Behaviors

// Fallible actions

// Decrypt decrypts a SOPS document in sops's binary mode, the one mode writ encrypts in, and returns the bytes that
// were encrypted, whatever the file is called (#977). Data that is not a SOPS document passes through unchanged.
//
// Parameters:
//   - data: encrypted (or plaintext) content
//
// Returns:
//   - []byte: the bytes that were encrypted (or the unchanged plaintext)
//   - error: decryption error
func (c *Client) Decrypt(data []byte) ([]byte, error) {

	if !IsEncrypted(data) {
		return data, nil
	}

	plaintext, err := decrypt.DataWithFormat(data, formats.Binary)
	if err != nil {
		return nil, fmt.Errorf("sops decrypt: %w", err)
	}
	return plaintext, nil
}

// Actions

// Decryptor returns a decryption function matching the signature expected by the execution engine:
// func(source string, data []byte) ([]byte, error). The source names the file in an error, and nothing else.
//
// Returns:
//   - func(string, []byte) ([]byte, error): decryption function
func (c *Client) Decryptor() func(source string, data []byte) ([]byte, error) {

	return func(source string, data []byte) ([]byte, error) {
		plaintext, err := c.Decrypt(data)
		if err != nil {
			return nil, fmt.Errorf("decrypt %s: %w", filepath.Base(source), err)
		}
		return plaintext, nil
	}
}

// endregion

// endregion
