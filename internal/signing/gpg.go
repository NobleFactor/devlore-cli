// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package signing

import (
	"bytes"
	"context"
	"encoding/base64"
	"os/exec"
	"strings"
)

// GPGSigner signs using GPG (GNU Privacy Guard).
type GPGSigner struct {
	fingerprints []string
}

// NewGPGSigner creates a GPG signer with the given fingerprints.
// The fingerprints string can contain multiple fingerprints separated by commas or newlines.
func NewGPGSigner(fingerprints string) *GPGSigner {
	var fps []string
	for _, line := range strings.Split(fingerprints, "\n") {
		for _, fp := range strings.Split(line, ",") {
			fp = strings.TrimSpace(fp)
			if fp != "" {
				fps = append(fps, fp)
			}
		}
	}
	return &GPGSigner{fingerprints: fps}
}

// Name returns "gpg".
func (g *GPGSigner) Name() string {
	return "gpg"
}

// Available returns true if gpg is installed and we have a secret key
// matching one of the configured fingerprints.
func (g *GPGSigner) Available() bool {
	// Check if gpg is available
	if _, err := exec.LookPath("gpg"); err != nil {
		return false
	}

	// Check if we have any secret key matching the fingerprints
	for _, fp := range g.fingerprints {
		if g.hasSecretKey(fp) {
			return true
		}
	}

	return false
}

// hasSecretKey checks if we have a secret key for the given fingerprint.
func (g *GPGSigner) hasSecretKey(fingerprint string) bool {
	// Normalize fingerprint (remove spaces)
	fp := strings.ReplaceAll(fingerprint, " ", "")

	cmd := exec.CommandContext(context.TODO(), "gpg", "--list-secret-keys", "--with-colons", fp) //nolint:gosec // G204: gpg invocation with validated arguments
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	// If we get output with "sec" lines, we have the secret key
	return strings.Contains(string(output), "sec:")
}

// findAvailableKey returns the first fingerprint for which we have a secret key.
func (g *GPGSigner) findAvailableKey() string {
	for _, fp := range g.fingerprints {
		if g.hasSecretKey(fp) {
			return fp
		}
	}
	return ""
}

// Sign signs the data using GPG.
func (g *GPGSigner) Sign(data []byte) (*Signature, error) {
	keyID := g.findAvailableKey()
	if keyID == "" {
		return nil, ErrNoKeyAvailable
	}

	// Normalize fingerprint
	keyID = strings.ReplaceAll(keyID, " ", "")

	// Create detached armored signature
	cmd := exec.CommandContext(context.TODO(), "gpg", //nolint:gosec // G204: gpg invocation with validated arguments
		"--detach-sign",
		"--armor",
		"--local-user", keyID,
		"--batch",
		"--yes",
	)

	cmd.Stdin = bytes.NewReader(data)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, &SignError{Backend: "gpg", Err: err, Details: stderr.String()}
	}

	return &Signature{
		Method: "gpg",
		Value:  base64.StdEncoding.EncodeToString(stdout.Bytes()),
		KeyID:  keyID,
	}, nil
}

// VerifyGPG verifies a GPG signature.
func VerifyGPG(data []byte, sig *Signature) error {
	if sig.Method != "gpg" {
		return &VerifyError{Backend: "gpg", Err: ErrWrongMethod}
	}

	sigBytes, err := base64.StdEncoding.DecodeString(sig.Value)
	if err != nil {
		return &VerifyError{Backend: "gpg", Err: err}
	}

	return verifyGPGWithTempFiles(data, sigBytes)
}

func verifyGPGWithTempFiles(data, sigBytes []byte) error {
	// Write data to temp file
	dataFile, err := createTempFile("gpg-verify-data-*", data)
	if err != nil {
		return &VerifyError{Backend: "gpg", Err: err}
	}
	defer removeTempFile(dataFile)

	// Write signature to temp file
	sigFile, err := createTempFile("gpg-verify-sig-*.asc", sigBytes)
	if err != nil {
		return &VerifyError{Backend: "gpg", Err: err}
	}
	defer removeTempFile(sigFile)

	cmd := exec.CommandContext(context.Background(), "gpg", "--verify", "--batch", sigFile, dataFile) //nolint:gosec // G204: gpg invocation with validated arguments
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return &VerifyError{Backend: "gpg", Err: err, Details: stderr.String()}
	}

	return nil
}
