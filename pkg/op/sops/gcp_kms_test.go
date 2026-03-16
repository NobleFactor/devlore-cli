// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package sops

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

// --- verifyGCPSignature ---

func TestVerifyGCPSignature_RSA_PSS_SHA256(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	data := []byte("test data for signing")
	hash := sha256.Sum256(data)

	signature, err := rsa.SignPSS(rand.Reader, privateKey, crypto.SHA256, hash[:], nil)
	if err != nil {
		t.Fatalf("failed to sign: %v", err)
	}

	err = verifyGCPSignature(&privateKey.PublicKey, kmspb.CryptoKeyVersion_RSA_SIGN_PSS_2048_SHA256, data, signature)
	if err != nil {
		t.Errorf("verification should succeed: %v", err)
	}

	wrongData := []byte("wrong data")
	err = verifyGCPSignature(&privateKey.PublicKey, kmspb.CryptoKeyVersion_RSA_SIGN_PSS_2048_SHA256, wrongData, signature)
	if err == nil {
		t.Error("verification should fail with wrong data")
	}

	tamperedSig := make([]byte, len(signature))
	copy(tamperedSig, signature)
	tamperedSig[0] ^= 0xFF
	err = verifyGCPSignature(&privateKey.PublicKey, kmspb.CryptoKeyVersion_RSA_SIGN_PSS_2048_SHA256, data, tamperedSig)
	if err == nil {
		t.Error("verification should fail with tampered signature")
	}
}

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

	err = verifyGCPSignature(&privateKey.PublicKey, kmspb.CryptoKeyVersion_RSA_SIGN_PSS_2048_SHA256, data, signature)
	if err == nil {
		t.Error("verification should fail with wrong algorithm")
	}
}

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

	err = verifyGCPSignature(&privateKey.PublicKey, kmspb.CryptoKeyVersion_RSA_SIGN_PKCS1_2048_SHA256, []byte("wrong"), signature)
	if err == nil {
		t.Error("verification should fail with wrong data")
	}
}

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

	err = verifyGCPSignature(&privateKey.PublicKey, kmspb.CryptoKeyVersion_EC_SIGN_P256_SHA256, []byte("wrong"), signature)
	if err == nil {
		t.Error("verification should fail with wrong data")
	}

	tamperedSig := make([]byte, len(signature))
	copy(tamperedSig, signature)
	tamperedSig[len(tamperedSig)-1] ^= 0xFF
	err = verifyGCPSignature(&privateKey.PublicKey, kmspb.CryptoKeyVersion_EC_SIGN_P256_SHA256, data, tamperedSig)
	if err == nil {
		t.Error("verification should fail with tampered signature")
	}
}

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

func TestVerifyGCPSignature_WrongKeyType(t *testing.T) {
	rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	ecdsaKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	data := []byte("test data")

	err := verifyGCPSignature(&rsaKey.PublicKey, kmspb.CryptoKeyVersion_EC_SIGN_P256_SHA256, data, []byte("sig"))
	if err == nil {
		t.Error("should reject RSA key for ECDSA algorithm")
	}

	err = verifyGCPSignature(&ecdsaKey.PublicKey, kmspb.CryptoKeyVersion_RSA_SIGN_PSS_2048_SHA256, data, []byte("sig"))
	if err == nil {
		t.Error("should reject ECDSA key for RSA algorithm")
	}
}

func TestVerifyGCPSignature_UnsupportedAlgorithm(t *testing.T) {
	rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	data := []byte("test data")

	err := verifyGCPSignature(&rsaKey.PublicKey, kmspb.CryptoKeyVersion_CRYPTO_KEY_VERSION_ALGORITHM_UNSPECIFIED, data, []byte("sig"))
	if err == nil {
		t.Error("should reject unspecified algorithm")
	}
}

// --- verifyGCPKMS ---

func TestVerifyGCPKMS_WrongMethod(t *testing.T) {
	sig := &Signature{
		Method: "aws_kms",
		Value:  base64.StdEncoding.EncodeToString([]byte("sig")),
		KeyID:  "projects/test/locations/test/keyRings/test/cryptoKeys/test/cryptoKeyVersions/1",
	}

	err := verifyGCPKMS([]byte("data"), sig)
	if err == nil {
		t.Error("should reject wrong method")
	}

	var verifyErr *VerifyError
	if !errors.As(err, &verifyErr) || !errors.Is(verifyErr.Err, ErrWrongMethod) {
		t.Errorf("expected ErrWrongMethod, got: %v", err)
	}
}

func TestVerifyGCPKMS_InvalidBase64(t *testing.T) {
	sig := &Signature{
		Method: "gcp_kms",
		Value:  "not-valid-base64!!!",
		KeyID:  "projects/test/locations/test/keyRings/test/cryptoKeys/test/cryptoKeyVersions/1",
	}

	err := verifyGCPKMS([]byte("data"), sig)
	if err == nil {
		t.Error("should reject invalid base64")
	}
}

// --- newGCPKMSSigner ---

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
			signer := newGCPKMSSigner(tt.input)
			if len(signer.keyNames) != tt.wantKeys {
				t.Errorf("newGCPKMSSigner() got %d keys, want %d", len(signer.keyNames), tt.wantKeys)
			}
		})
	}
}

func TestGCPKMSSigner_Name(t *testing.T) {
	signer := newGCPKMSSigner("projects/p/locations/l/keyRings/r/cryptoKeys/k")
	if signer.name() != "gcp_kms" {
		t.Errorf("name() = %q, want %q", signer.name(), "gcp_kms")
	}
}

// --- ensureKeyVersion ---

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

func TestGCPKMSSigner_Available_NoKeys(t *testing.T) {
	signer := newGCPKMSSigner("")
	if signer.available() {
		t.Error("available() should return false when no keys configured")
	}
}
