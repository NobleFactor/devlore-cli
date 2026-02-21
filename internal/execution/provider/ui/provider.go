// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package ui

import (
	"fmt"
	"io"
)

// Provider provides user-facing terminal messaging.
type Provider struct{}

// Note informs the user of progress.
func (p *Provider) Note(msg string, output io.Writer) error {
	_, err := fmt.Fprintf(output, "  [note] %s\n", msg)
	return err
}

// Warn alerts the user to a potential issue.
func (p *Provider) Warn(msg string, output io.Writer) error {
	_, err := fmt.Fprintf(output, "  [warn] %s\n", msg)
	return err
}

// Error reports a problem to the user.
func (p *Provider) Error(msg string, output io.Writer) error {
	_, _ = fmt.Fprintf(output, "  [ERROR] %s\n", msg)
	return fmt.Errorf("error: %s", msg)
}

// Success confirms completion to the user.
func (p *Provider) Success(msg string, output io.Writer) error {
	_, err := fmt.Fprintf(output, "  [SUCCESS] %s\n", msg)
	return err
}

// Fail aborts execution with a message to the user.
func (p *Provider) Fail(msg string, output io.Writer) error {
	_, _ = fmt.Fprintf(output, "  [FAIL] %s\n", msg)
	return fmt.Errorf("fail: %s", msg)
}
