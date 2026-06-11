// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package sops

import (
	"bytes"
	"context"
	"encoding/base64"
	"os/exec"
	"strings"
)

// Interface guard.
var _ signer = (*gpgSigner)(nil)

// gpgSigner signs using GPG (GNU Privacy Guard).
type gpgSigner struct {
	fingerprints []string
}

// newGPGSigner creates a GPG signer with the given fingerprints. The fingerprints string can contain multiple
// fingerprints separated by commas or newlines.
//
// Parameters:
//   - fingerprints: comma/newline-separated GPG fingerprints
//
// Returns:
//   - *gpgSigner: the configured signer
func newGPGSigner(fingerprints string) *gpgSigner {

	var fps []string
	for _, line := range strings.Split(fingerprints, "\n") {
		for _, fp := range strings.Split(line, ",") {
			fp = strings.TrimSpace(fp)
			if fp != "" {
				fps = append(fps, fp)
			}
		}
	}
	return &gpgSigner{fingerprints: fps}
}

// region EXPORTED METHODS
// (none — gpgSigner is unexported)
// endregion

// region UNEXPORTED METHODS

// region Behaviors

func (g *gpgSigner) name() string { return "gpg" }

// available returns true if gpg is installed and we have a secret key matching one of the configured fingerprints.
func (g *gpgSigner) available() bool {

	if _, err := exec.LookPath("gpg"); err != nil {
		return false
	}

	for _, fp := range g.fingerprints {
		if g.hasSecretKey(fp) {
			return true
		}
	}
	return false
}

// sign signs the data using GPG.
func (g *gpgSigner) sign(data []byte) (*Signature, error) {

	keyID := g.findAvailableKey()
	if keyID == "" {
		return nil, ErrNoKeyAvailable
	}

	keyID = strings.ReplaceAll(keyID, " ", "")

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

// hasSecretKey checks if we have a secret key for the given fingerprint.
func (g *gpgSigner) hasSecretKey(fingerprint string) bool {

	fp := strings.ReplaceAll(fingerprint, " ", "")
	cmd := exec.CommandContext(context.TODO(), "gpg", "--list-secret-keys", "--with-colons", fp) //nolint:gosec // G204: gpg invocation with validated arguments
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), "sec:")
}

// findAvailableKey returns the first fingerprint for which we have a secret key.
func (g *gpgSigner) findAvailableKey() string {

	for _, fp := range g.fingerprints {
		if g.hasSecretKey(fp) {
			return fp
		}
	}
	return ""
}

// endregion

// endregion

// verifyGPG verifies a GPG signature.
//
// Parameters:
//   - data: original content
//   - sig: signature to verify
//
// Returns:
//   - error: verification error
func verifyGPG(data []byte, sig *Signature) error {

	if sig.Method != "gpg" {
		return &VerifyError{Backend: "gpg", Err: ErrWrongMethod}
	}

	sigBytes, err := base64.StdEncoding.DecodeString(sig.Value)
	if err != nil {
		return &VerifyError{Backend: "gpg", Err: err}
	}

	return verifyGPGWithTempFiles(data, sigBytes)
}

// verifyGPGWithTempFiles performs GPG verification using temp files for data and signature.
//
// Parameters:
//   - data: original content
//   - sigBytes: raw signature bytes
//
// Returns:
//   - error: verification error
func verifyGPGWithTempFiles(data, sigBytes []byte) error {

	dataFile, err := createTempFile("gpg-verify-data-*", data)
	if err != nil {
		return &VerifyError{Backend: "gpg", Err: err}
	}
	defer removeTempFile(dataFile)

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
