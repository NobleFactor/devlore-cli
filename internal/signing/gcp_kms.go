// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package signing

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"hash/crc32"
	"strings"
	"time"

	kms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// GCPKMSSigner signs using Google Cloud KMS.
type GCPKMSSigner struct {
	keyNames []string
}

// NewGCPKMSSigner creates a GCP KMS signer with the given key resource names.
// The keyNames string can contain multiple names separated by commas or newlines.
// Key format: projects/PROJECT/locations/LOCATION/keyRings/RING/cryptoKeys/KEY/cryptoKeyVersions/VERSION
func NewGCPKMSSigner(keyNames string) *GCPKMSSigner {
	var names []string
	for _, line := range strings.Split(keyNames, "\n") {
		for _, name := range strings.Split(line, ",") {
			name = strings.TrimSpace(name)
			if name != "" {
				names = append(names, name)
			}
		}
	}
	return &GCPKMSSigner{keyNames: names}
}

// Name returns "gcp_kms".
func (g *GCPKMSSigner) Name() string {
	return "gcp_kms"
}

// Available returns true if GCP credentials are configured and
// we can access at least one of the configured keys.
func (g *GCPKMSSigner) Available() bool {
	if len(g.keyNames) == 0 {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := kms.NewKeyManagementClient(ctx)
	if err != nil {
		return false
	}
	defer func() { _ = client.Close() }()

	// Check if we can get at least one key
	for _, name := range g.keyNames {
		// Ensure we have a version suffix for signing
		keyVersionName := ensureKeyVersion(name)

		_, err := client.GetCryptoKeyVersion(ctx, &kmspb.GetCryptoKeyVersionRequest{
			Name: keyVersionName,
		})
		if err == nil {
			return true
		}
	}

	return false
}

// ensureKeyVersion adds /cryptoKeyVersions/1 if not present.
func ensureKeyVersion(name string) string {
	if strings.Contains(name, "/cryptoKeyVersions/") {
		return name
	}
	return name + "/cryptoKeyVersions/1"
}

// findAvailableKey returns the first key that we can use for signing.
func (g *GCPKMSSigner) findAvailableKey(ctx context.Context, client *kms.KeyManagementClient) string {
	for _, name := range g.keyNames {
		keyVersionName := ensureKeyVersion(name)

		resp, err := client.GetCryptoKeyVersion(ctx, &kmspb.GetCryptoKeyVersionRequest{
			Name: keyVersionName,
		})
		if err != nil {
			continue
		}

		// Check if key can be used for signing
		if resp.Algorithm == kmspb.CryptoKeyVersion_CRYPTO_KEY_VERSION_ALGORITHM_UNSPECIFIED {
			continue
		}

		// Asymmetric sign algorithms
		switch resp.Algorithm {
		case kmspb.CryptoKeyVersion_RSA_SIGN_PSS_2048_SHA256,
			kmspb.CryptoKeyVersion_RSA_SIGN_PSS_3072_SHA256,
			kmspb.CryptoKeyVersion_RSA_SIGN_PSS_4096_SHA256,
			kmspb.CryptoKeyVersion_RSA_SIGN_PSS_4096_SHA512,
			kmspb.CryptoKeyVersion_RSA_SIGN_PKCS1_2048_SHA256,
			kmspb.CryptoKeyVersion_RSA_SIGN_PKCS1_3072_SHA256,
			kmspb.CryptoKeyVersion_RSA_SIGN_PKCS1_4096_SHA256,
			kmspb.CryptoKeyVersion_RSA_SIGN_PKCS1_4096_SHA512,
			kmspb.CryptoKeyVersion_EC_SIGN_P256_SHA256,
			kmspb.CryptoKeyVersion_EC_SIGN_P384_SHA384,
			kmspb.CryptoKeyVersion_EC_SIGN_SECP256K1_SHA256:
			return keyVersionName
		}
	}
	return ""
}

// Sign signs the data using GCP KMS.
func (g *GCPKMSSigner) Sign(data []byte) (*Signature, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := kms.NewKeyManagementClient(ctx)
	if err != nil {
		return nil, &SignError{Backend: "gcp_kms", Err: err}
	}
	defer func() { _ = client.Close() }()

	keyVersionName := g.findAvailableKey(ctx, client)
	if keyVersionName == "" {
		return nil, &SignError{Backend: "gcp_kms", Err: ErrNoKeyAvailable}
	}

	// Hash the data
	hash := sha256.Sum256(data)

	// Calculate CRC32C for integrity verification
	crc32c := crc32.MakeTable(crc32.Castagnoli)
	digestCRC32C := crc32.Checksum(hash[:], crc32c)

	// Sign the digest
	resp, err := client.AsymmetricSign(ctx, &kmspb.AsymmetricSignRequest{
		Name: keyVersionName,
		Digest: &kmspb.Digest{
			Digest: &kmspb.Digest_Sha256{
				Sha256: hash[:],
			},
		},
		DigestCrc32C: wrapperspb.Int64(int64(digestCRC32C)),
	})
	if err != nil {
		return nil, &SignError{Backend: "gcp_kms", Err: err}
	}

	// Verify response integrity
	if !resp.VerifiedDigestCrc32C {
		return nil, &SignError{Backend: "gcp_kms", Err: fmt.Errorf("digest CRC32C verification failed")}
	}

	return &Signature{
		Method: "gcp_kms",
		Value:  base64.StdEncoding.EncodeToString(resp.Signature),
		KeyID:  keyVersionName,
	}, nil
}

// VerifyGCPKMS verifies a GCP KMS signature.
func VerifyGCPKMS(data []byte, sig *Signature) error {
	if sig.Method != "gcp_kms" {
		return &VerifyError{Backend: "gcp_kms", Err: ErrWrongMethod}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := kms.NewKeyManagementClient(ctx)
	if err != nil {
		return &VerifyError{Backend: "gcp_kms", Err: err}
	}
	defer func() { _ = client.Close() }()

	sigBytes, err := base64.StdEncoding.DecodeString(sig.Value)
	if err != nil {
		return &VerifyError{Backend: "gcp_kms", Err: err}
	}

	// Get the public key and algorithm info
	pubKeyResp, err := client.GetPublicKey(ctx, &kmspb.GetPublicKeyRequest{
		Name: sig.KeyID,
	})
	if err != nil {
		return &VerifyError{Backend: "gcp_kms", Err: err}
	}

	// Parse the PEM-encoded public key
	block, _ := pem.Decode([]byte(pubKeyResp.Pem))
	if block == nil {
		return &VerifyError{Backend: "gcp_kms", Err: fmt.Errorf("failed to parse PEM public key")}
	}

	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return &VerifyError{Backend: "gcp_kms", Err: fmt.Errorf("failed to parse public key: %w", err)}
	}

	// Verify based on algorithm
	if err := verifyGCPSignature(pubKey, pubKeyResp.Algorithm, data, sigBytes); err != nil {
		return &VerifyError{Backend: "gcp_kms", Err: err}
	}

	return nil
}

// verifyGCPSignature verifies a signature using the appropriate algorithm.
func verifyGCPSignature(pubKey any, algorithm kmspb.CryptoKeyVersion_CryptoKeyVersionAlgorithm, data, sig []byte) error {
	switch algorithm {
	// RSA-PSS algorithms
	case kmspb.CryptoKeyVersion_RSA_SIGN_PSS_2048_SHA256,
		kmspb.CryptoKeyVersion_RSA_SIGN_PSS_3072_SHA256,
		kmspb.CryptoKeyVersion_RSA_SIGN_PSS_4096_SHA256:
		rsaKey, ok := pubKey.(*rsa.PublicKey)
		if !ok {
			return fmt.Errorf("expected RSA public key")
		}
		hash := sha256.Sum256(data)
		return rsa.VerifyPSS(rsaKey, crypto.SHA256, hash[:], sig, nil)

	case kmspb.CryptoKeyVersion_RSA_SIGN_PSS_4096_SHA512:
		rsaKey, ok := pubKey.(*rsa.PublicKey)
		if !ok {
			return fmt.Errorf("expected RSA public key")
		}
		hash := sha512.Sum512(data)
		return rsa.VerifyPSS(rsaKey, crypto.SHA512, hash[:], sig, nil)

	// RSA PKCS1 algorithms
	case kmspb.CryptoKeyVersion_RSA_SIGN_PKCS1_2048_SHA256,
		kmspb.CryptoKeyVersion_RSA_SIGN_PKCS1_3072_SHA256,
		kmspb.CryptoKeyVersion_RSA_SIGN_PKCS1_4096_SHA256:
		rsaKey, ok := pubKey.(*rsa.PublicKey)
		if !ok {
			return fmt.Errorf("expected RSA public key")
		}
		hash := sha256.Sum256(data)
		return rsa.VerifyPKCS1v15(rsaKey, crypto.SHA256, hash[:], sig)

	case kmspb.CryptoKeyVersion_RSA_SIGN_PKCS1_4096_SHA512:
		rsaKey, ok := pubKey.(*rsa.PublicKey)
		if !ok {
			return fmt.Errorf("expected RSA public key")
		}
		hash := sha512.Sum512(data)
		return rsa.VerifyPKCS1v15(rsaKey, crypto.SHA512, hash[:], sig)

	// ECDSA algorithms
	case kmspb.CryptoKeyVersion_EC_SIGN_P256_SHA256,
		kmspb.CryptoKeyVersion_EC_SIGN_SECP256K1_SHA256:
		ecKey, ok := pubKey.(*ecdsa.PublicKey)
		if !ok {
			return fmt.Errorf("expected ECDSA public key")
		}
		hash := sha256.Sum256(data)
		if !ecdsa.VerifyASN1(ecKey, hash[:], sig) {
			return fmt.Errorf("ECDSA signature verification failed")
		}
		return nil

	case kmspb.CryptoKeyVersion_EC_SIGN_P384_SHA384:
		ecKey, ok := pubKey.(*ecdsa.PublicKey)
		if !ok {
			return fmt.Errorf("expected ECDSA public key")
		}
		hash := sha512.Sum384(data)
		if !ecdsa.VerifyASN1(ecKey, hash[:], sig) {
			return fmt.Errorf("ECDSA signature verification failed")
		}
		return nil

	default:
		return fmt.Errorf("unsupported algorithm: %v", algorithm)
	}
}
