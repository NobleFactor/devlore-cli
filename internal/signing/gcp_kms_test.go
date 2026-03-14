// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package signing

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"testing"

	"cloud.google.com/go/kms/apiv1/kmspb"
)

// TestVerifyGCPSignature_RSA_PSS_SHA256 tests RSA-PSS SHA256 signature verification.
func TestVerifyGCPSignature_RSA_PSS_SHA256(t *testing.T) {
	// Generate test RSA key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	data := []byte("test data for signing")
	hash := sha256.Sum256(data)

	// Sign with RSA-PSS
	signature, err := rsa.SignPSS(rand.Reader, privateKey, crypto.SHA256, hash[:], nil)
	if err != nil {
		t.Fatalf("failed to sign: %v", err)
	}

	// Test verification succeeds with correct signature
	err = verifyGCPSignature(&privateKey.PublicKey, kmspb.CryptoKeyVersion_RSA_SIGN_PSS_2048_SHA256, data, signature)
	if err != nil {
		t.Errorf("verification should succeed: %v", err)
	}

	// Test verification fails with wrong data
	wrongData := []byte("wrong data")
	err = verifyGCPSignature(&privateKey.PublicKey, kmspb.CryptoKeyVersion_RSA_SIGN_PSS_2048_SHA256, wrongData, signature)
	if err == nil {
		t.Error("verification should fail with wrong data")
	}

	// Test verification fails with tampered signature
	tamperedSig := make([]byte, len(signature))
	copy(tamperedSig, signature)
	tamperedSig[0] ^= 0xFF
	err = verifyGCPSignature(&privateKey.PublicKey, kmspb.CryptoKeyVersion_RSA_SIGN_PSS_2048_SHA256, data, tamperedSig)
	if err == nil {
		t.Error("verification should fail with tampered signature")
	}
}

// TestVerifyGCPSignature_RSA_PSS_SHA512 tests RSA-PSS SHA512 signature verification.
func TestVerifyGCPSignature_RSA_PSS_SHA512(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	data := []byte("test data for signing with SHA512")
	hash := sha512.Sum512(data)

	signature, err := rsa.SignPSS(rand.Reader, privateKey, crypto.SHA512, hash[:], nil)
	if err != nil {
		t.Fatalf("failed to sign: %v", err)
	}

	err = verifyGCPSignature(&privateKey.PublicKey, kmspb.CryptoKeyVersion_RSA_SIGN_PSS_4096_SHA512, data, signature)
	if err != nil {
		t.Errorf("verification should succeed: %v", err)
	}

	// Test with wrong algorithm
	err = verifyGCPSignature(&privateKey.PublicKey, kmspb.CryptoKeyVersion_RSA_SIGN_PSS_2048_SHA256, data, signature)
	if err == nil {
		t.Error("verification should fail with wrong algorithm")
	}
}

// TestVerifyGCPSignature_RSA_PKCS1_SHA256 tests RSA PKCS1v15 SHA256 signature verification.
func TestVerifyGCPSignature_RSA_PKCS1_SHA256(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	data := []byte("test data for PKCS1 signing")
	hash := sha256.Sum256(data)

	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatalf("failed to sign: %v", err)
	}

	err = verifyGCPSignature(&privateKey.PublicKey, kmspb.CryptoKeyVersion_RSA_SIGN_PKCS1_2048_SHA256, data, signature)
	if err != nil {
		t.Errorf("verification should succeed: %v", err)
	}

	// Test failure with wrong data
	err = verifyGCPSignature(&privateKey.PublicKey, kmspb.CryptoKeyVersion_RSA_SIGN_PKCS1_2048_SHA256, []byte("wrong"), signature)
	if err == nil {
		t.Error("verification should fail with wrong data")
	}
}

// TestVerifyGCPSignature_ECDSA_P256 tests ECDSA P256 signature verification.
func TestVerifyGCPSignature_ECDSA_P256(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ECDSA key: %v", err)
	}

	data := []byte("test data for ECDSA signing")
	hash := sha256.Sum256(data)

	signature, err := ecdsa.SignASN1(rand.Reader, privateKey, hash[:])
	if err != nil {
		t.Fatalf("failed to sign: %v", err)
	}

	err = verifyGCPSignature(&privateKey.PublicKey, kmspb.CryptoKeyVersion_EC_SIGN_P256_SHA256, data, signature)
	if err != nil {
		t.Errorf("verification should succeed: %v", err)
	}

	// Test failure with wrong data
	err = verifyGCPSignature(&privateKey.PublicKey, kmspb.CryptoKeyVersion_EC_SIGN_P256_SHA256, []byte("wrong"), signature)
	if err == nil {
		t.Error("verification should fail with wrong data")
	}

	// Test failure with tampered signature
	tamperedSig := make([]byte, len(signature))
	copy(tamperedSig, signature)
	tamperedSig[len(tamperedSig)-1] ^= 0xFF
	err = verifyGCPSignature(&privateKey.PublicKey, kmspb.CryptoKeyVersion_EC_SIGN_P256_SHA256, data, tamperedSig)
	if err == nil {
		t.Error("verification should fail with tampered signature")
	}
}

// TestVerifyGCPSignature_ECDSA_P384 tests ECDSA P384 signature verification.
func TestVerifyGCPSignature_ECDSA_P384(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ECDSA key: %v", err)
	}

	data := []byte("test data for ECDSA P384 signing")
	hash := sha512.Sum384(data)

	signature, err := ecdsa.SignASN1(rand.Reader, privateKey, hash[:])
	if err != nil {
		t.Fatalf("failed to sign: %v", err)
	}

	err = verifyGCPSignature(&privateKey.PublicKey, kmspb.CryptoKeyVersion_EC_SIGN_P384_SHA384, data, signature)
	if err != nil {
		t.Errorf("verification should succeed: %v", err)
	}
}

// TestVerifyGCPSignature_WrongKeyType tests that wrong key types are rejected.
func TestVerifyGCPSignature_WrongKeyType(t *testing.T) {
	rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	ecdsaKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	data := []byte("test data")

	// Try to verify ECDSA algorithm with RSA key
	err := verifyGCPSignature(&rsaKey.PublicKey, kmspb.CryptoKeyVersion_EC_SIGN_P256_SHA256, data, []byte("sig"))
	if err == nil {
		t.Error("should reject RSA key for ECDSA algorithm")
	}

	// Try to verify RSA algorithm with ECDSA key
	err = verifyGCPSignature(&ecdsaKey.PublicKey, kmspb.CryptoKeyVersion_RSA_SIGN_PSS_2048_SHA256, data, []byte("sig"))
	if err == nil {
		t.Error("should reject ECDSA key for RSA algorithm")
	}
}

// TestVerifyGCPSignature_UnsupportedAlgorithm tests that unsupported algorithms are rejected.
func TestVerifyGCPSignature_UnsupportedAlgorithm(t *testing.T) {
	rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	data := []byte("test data")

	err := verifyGCPSignature(&rsaKey.PublicKey, kmspb.CryptoKeyVersion_CRYPTO_KEY_VERSION_ALGORITHM_UNSPECIFIED, data, []byte("sig"))
	if err == nil {
		t.Error("should reject unspecified algorithm")
	}
}

// TestVerifyGCPKMS_WrongMethod tests that wrong signature method is rejected.
func TestVerifyGCPKMS_WrongMethod(t *testing.T) {
	sig := &Signature{
		Method: "aws_kms",
		Value:  base64.StdEncoding.EncodeToString([]byte("sig")),
		KeyID:  "projects/test/locations/test/keyRings/test/cryptoKeys/test/cryptoKeyVersions/1",
	}

	err := VerifyGCPKMS([]byte("data"), sig)
	if err == nil {
		t.Error("should reject wrong method")
	}

	var verifyErr *VerifyError
	if !errors.As(err, &verifyErr) || !errors.Is(verifyErr.Err, ErrWrongMethod) {
		t.Errorf("expected ErrWrongMethod, got: %v", err)
	}
}

// TestVerifyGCPKMS_InvalidBase64 tests that invalid base64 signature is rejected.
func TestVerifyGCPKMS_InvalidBase64(t *testing.T) {
	sig := &Signature{
		Method: "gcp_kms",
		Value:  "not-valid-base64!!!",
		KeyID:  "projects/test/locations/test/keyRings/test/cryptoKeys/test/cryptoKeyVersions/1",
	}

	err := VerifyGCPKMS([]byte("data"), sig)
	if err == nil {
		t.Error("should reject invalid base64")
	}
}

// TestNewGCPKMSSigner tests the signer constructor.
func TestNewGCPKMSSigner(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantKeys int
	}{
		{
			name:     "single key",
			input:    "projects/p/locations/l/keyRings/r/cryptoKeys/k",
			wantKeys: 1,
		},
		{
			name:     "comma separated",
			input:    "projects/p1/locations/l/keyRings/r/cryptoKeys/k1,projects/p2/locations/l/keyRings/r/cryptoKeys/k2",
			wantKeys: 2,
		},
		{
			name:     "newline separated",
			input:    "projects/p1/locations/l/keyRings/r/cryptoKeys/k1\nprojects/p2/locations/l/keyRings/r/cryptoKeys/k2",
			wantKeys: 2,
		},
		{
			name:     "mixed with whitespace",
			input:    "  projects/p1/locations/l/keyRings/r/cryptoKeys/k1  ,  projects/p2/locations/l/keyRings/r/cryptoKeys/k2  \n  projects/p3/locations/l/keyRings/r/cryptoKeys/k3  ",
			wantKeys: 3,
		},
		{
			name:     "empty",
			input:    "",
			wantKeys: 0,
		},
		{
			name:     "whitespace only",
			input:    "   \n   ",
			wantKeys: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signer := NewGCPKMSSigner(tt.input)
			if len(signer.keyNames) != tt.wantKeys {
				t.Errorf("NewGCPKMSSigner() got %d keys, want %d", len(signer.keyNames), tt.wantKeys)
			}
		})
	}
}

// TestGCPKMSSigner_Name tests the signer name.
func TestGCPKMSSigner_Name(t *testing.T) {
	signer := NewGCPKMSSigner("projects/p/locations/l/keyRings/r/cryptoKeys/k")
	if signer.Name() != "gcp_kms" {
		t.Errorf("ReceiverName() = %q, want %q", signer.Name(), "gcp_kms")
	}
}

// TestEnsureKeyVersion tests the key version helper.
func TestEnsureKeyVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			input: "projects/p/locations/l/keyRings/r/cryptoKeys/k",
			want:  "projects/p/locations/l/keyRings/r/cryptoKeys/k/cryptoKeyVersions/1",
		},
		{
			input: "projects/p/locations/l/keyRings/r/cryptoKeys/k/cryptoKeyVersions/1",
			want:  "projects/p/locations/l/keyRings/r/cryptoKeys/k/cryptoKeyVersions/1",
		},
		{
			input: "projects/p/locations/l/keyRings/r/cryptoKeys/k/cryptoKeyVersions/5",
			want:  "projects/p/locations/l/keyRings/r/cryptoKeys/k/cryptoKeyVersions/5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ensureKeyVersion(tt.input)
			if got != tt.want {
				t.Errorf("ensureKeyVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestGCPKMSSigner_Available_NoKeys tests that Available returns false with no keys.
func TestGCPKMSSigner_Available_NoKeys(t *testing.T) {
	signer := NewGCPKMSSigner("")
	if signer.Available() {
		t.Error("Available() should return false when no keys configured")
	}
}
