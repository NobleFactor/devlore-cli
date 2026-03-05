// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package encryption provides encryption and decryption actions for the operation graph.
package encryption

import (
	"bytes"
	"fmt"
	"os"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	"github.com/getsops/sops/v3/decrypt"
	"gopkg.in/yaml.v3"
)

// Provider provides encryption and decryption actions.
//
// +devlore:access=both
type Provider struct {
	op.ProviderBase
}

// DecryptSopsFile takes a file.Resource, reads it into memory, and decrypts it via SOPS.
//
// Parameters:
//
//	sourceFile: The file.Resource to decrypt.
//	destinationFilename: The filename to write the decrypted content to.
//
// Returns:
//
//   - result: a file.Resource containing the decrypted content.
//   - undo: a map of compensation receipts.
//   - err: an error, if any.
func (p *Provider) DecryptSopsFile(sourceFile file.Resource, destinationFilename string) (result file.Resource, undo map[string]any, err error) {

	// 1. Read the source file into memory

	buffer := bytes.NewBuffer(make([]byte, 0, sourceFile.Size))

	if _, err := sourceFile.WriteTo(buffer); err != nil {
		return file.Resource{}, nil, fmt.Errorf("failed to read source: %w", err)
	}

	// 2. Decrypt the file

	cleartext, err := decrypt.Data(buffer.Bytes(), "yaml")

	if err != nil {
		return file.Resource{}, nil, fmt.Errorf("sops decryption failed: %w", err)
	}

	// 3. Write cleartext to the destination path

	if err := os.WriteFile(destinationFilename, cleartext, 0600); err != nil {
		return file.Resource{}, nil, fmt.Errorf("failed to write destination: %w", err)
	}

	// 4. Wrap the new file in a Resource

	result, err = file.NewResource(destinationFilename)

	if err != nil {
		return file.Resource{}, nil, fmt.Errorf("failed to initialize destination: %w", err)
	}

	// 5. Parse Metadata (Optional/Context Dependent)

	var data map[string]any

	if err := yaml.Unmarshal(cleartext, &data); err != nil {
		return result, nil, fmt.Errorf("failed to parse decrypted metadata: %w", err)
	}

	return result, data, nil
}

func (p *Provider) CompensateDecryptSopsFile(undo map[string]any) error {
	panic("not implemented: encryption.CompensateDecryptSopsFile")
}
