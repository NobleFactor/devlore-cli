// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package signing

import (
	"errors"
	"fmt"
	"os"
)

// Common errors.
var (
	ErrNoKeyAvailable = errors.New("no signing key available")
	ErrWrongMethod    = errors.New("wrong signature method")
)

// SignError represents a signing failure.
type SignError struct {
	Backend string
	Err     error
	Details string
}

func (e *SignError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("%s sign: %v: %s", e.Backend, e.Err, e.Details)
	}
	return fmt.Sprintf("%s sign: %v", e.Backend, e.Err)
}

func (e *SignError) Unwrap() error {
	return e.Err
}

// VerifyError represents a verification failure.
type VerifyError struct {
	Backend string
	Err     error
	Details string
}

func (e *VerifyError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("%s verify: %v: %s", e.Backend, e.Err, e.Details)
	}
	return fmt.Sprintf("%s verify: %v", e.Backend, e.Err)
}

func (e *VerifyError) Unwrap() error {
	return e.Err
}

// createTempFile creates a temporary file with the given content.
func createTempFile(pattern string, data []byte) (string, error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(data); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}

	return f.Name(), nil
}

// removeTempFile removes a temporary file, ignoring errors.
func removeTempFile(path string) {
	_ = os.Remove(path)
}
