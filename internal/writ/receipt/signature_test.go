// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package receipt

import (
	"testing"
	"time"

	"filippo.io/age"
)

func TestSignAndVerify(t *testing.T) {
	// Generate a test identity
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}

	// Create a test v4 receipt
	rcpt := &Receipt{
		Version:   CurrentVersion,
		Format:    "graph",
		Timestamp: time.Now(),
		Tool:      "writ",
		Platform:  Platform{OS: "darwin", Arch: "arm64"},
		Context: WritContext{
			SourceRoot: "/home/user/environment",
			TargetRoot: "/home/user",
			Projects:   []string{"all", "noblefactor"},
			Segments:   map[string]string{"OS": "Darwin", "ARCH": "arm64"},
		},
		Roots: []string{"all", "noblefactor"},
		Nodes: []Node{
			{
				ID:        ".bashrc",
				Operation: "link",
				Status:    "completed",
				Source:    "/home/user/environment/all/.bashrc",
				Target:    "/home/user/.bashrc",
				Project:   "all",
			},
			{
				ID:             ".gitconfig",
				Operation:      "expand",
				Status:         "completed",
				Source:         "/home/user/environment/noblefactor/.gitconfig.template",
				Target:         "/home/user/.gitconfig",
				Project:        "noblefactor",
				SourceChecksum: "sha256:abc123",
				TargetChecksum: "sha256:def456",
			},
		},
	}
	rcpt.ComputeSummary()

	// Sign the receipt
	if err := rcpt.Sign(identity); err != nil {
		t.Fatalf("sign receipt: %v", err)
	}

	// Verify receipt has signature
	if rcpt.Signature == nil {
		t.Fatal("expected signature to be set")
	}
	if rcpt.Signature.Method != "age" {
		t.Errorf("expected method 'age', got %q", rcpt.Signature.Method)
	}
	if rcpt.Signature.Recipient != identity.Recipient().String() {
		t.Errorf("expected recipient %q, got %q", identity.Recipient().String(), rcpt.Signature.Recipient)
	}

	// Verify the signature
	if err := rcpt.Verify([]age.Identity{identity}); err != nil {
		t.Fatalf("verify signature: %v", err)
	}

	// Test VerifyWithResult
	result, err := rcpt.VerifyWithResult([]age.Identity{identity})
	if err != nil {
		t.Fatalf("verify with result: %v", err)
	}
	if result != VerifyOK {
		t.Errorf("expected VerifyOK, got %v", result)
	}
}

func TestVerifyTamperedReceipt(t *testing.T) {
	// Generate a test identity
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}

	// Create and sign a test receipt
	rcpt := &Receipt{
		Version:   CurrentVersion,
		Format:    "graph",
		Timestamp: time.Now(),
		Tool:      "writ",
		Platform:  Platform{OS: "darwin", Arch: "arm64"},
		Context: WritContext{
			SourceRoot: "/home/user/environment",
			TargetRoot: "/home/user",
			Projects:   []string{"all"},
			Segments:   map[string]string{},
		},
		Roots: []string{"all"},
		Nodes: []Node{
			{
				ID:        ".bashrc",
				Operation: "link",
				Status:    "completed",
				Source:    "/home/user/environment/all/.bashrc",
				Target:    "/home/user/.bashrc",
				Project:   "all",
			},
		},
	}
	rcpt.ComputeSummary()

	if err := rcpt.Sign(identity); err != nil {
		t.Fatalf("sign receipt: %v", err)
	}

	// Tamper with the receipt
	rcpt.Roots = append(rcpt.Roots, "tampered")

	// Verification should fail
	if err := rcpt.Verify([]age.Identity{identity}); err == nil {
		t.Fatal("expected verification to fail on tampered receipt")
	}

	// VerifyWithResult should return VerifyInvalid
	result, _ := rcpt.VerifyWithResult([]age.Identity{identity})
	if result != VerifyInvalid {
		t.Errorf("expected VerifyInvalid, got %v", result)
	}
}

func TestVerifyLegacyReceipt(t *testing.T) {
	// A legacy receipt loaded and converted still has Version "4"
	// but if it had no signature originally, IsLegacy tests against v1/v2
	rcpt := &Receipt{
		Version: "2", // Simulating a pre-conversion check
	}

	// Legacy receipts should be allowed
	if !rcpt.IsLegacy() {
		t.Error("expected v2 receipt to be considered legacy")
	}

	// Generate identity for verification
	identity, _ := age.GenerateX25519Identity()

	result, err := rcpt.VerifyWithResult([]age.Identity{identity})
	if err != nil {
		t.Fatalf("verify legacy receipt: %v", err)
	}
	if result != VerifyLegacy {
		t.Errorf("expected VerifyLegacy, got %v", result)
	}
}

func TestVerifyWrongIdentity(t *testing.T) {
	// Generate two different identities
	signingIdentity, _ := age.GenerateX25519Identity()
	wrongIdentity, _ := age.GenerateX25519Identity()

	// Create and sign with first identity
	rcpt := &Receipt{
		Version:   CurrentVersion,
		Format:    "graph",
		Timestamp: time.Now(),
		Tool:      "writ",
		Platform:  Platform{OS: "linux", Arch: "amd64"},
		Context: WritContext{
			SourceRoot: "/home/user/environment",
			TargetRoot: "/home/user",
			Projects:   []string{"all"},
			Segments:   map[string]string{},
		},
		Roots: []string{"all"},
		Nodes: []Node{},
	}

	if err := rcpt.Sign(signingIdentity); err != nil {
		t.Fatalf("sign receipt: %v", err)
	}

	// Try to verify with wrong identity - should fail
	if err := rcpt.Verify([]age.Identity{wrongIdentity}); err == nil {
		t.Fatal("expected verification to fail with wrong identity")
	}
}

func TestIsSigned(t *testing.T) {
	rcpt := &Receipt{Version: CurrentVersion}

	if rcpt.IsSigned() {
		t.Error("expected unsigned receipt to return false")
	}

	rcpt.Signature = &Signature{Method: "age", Value: "test", Recipient: "age1..."}

	if !rcpt.IsSigned() {
		t.Error("expected signed receipt to return true")
	}
}

func TestVerifyResultString(t *testing.T) {
	tests := []struct {
		result   VerifyResult
		expected string
	}{
		{VerifyOK, "signature valid"},
		{VerifyLegacy, "unsigned (legacy)"},
		{VerifyInvalid, "signature invalid"},
		{VerifyMissing, "signature missing"},
	}

	for _, tt := range tests {
		if got := tt.result.String(); got != tt.expected {
			t.Errorf("VerifyResult(%d).String() = %q, want %q", tt.result, got, tt.expected)
		}
	}
}
