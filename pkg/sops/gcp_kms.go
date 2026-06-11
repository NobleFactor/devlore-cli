// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package sops

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

// Interface guard.
var _ signer = (*gcpKMSSigner)(nil)

// gcpKMSSigner signs using Google Cloud KMS.
type gcpKMSSigner struct {
	keyNames []string
}

// newGCPKMSSigner creates a GCP KMS signer with the given key resource names. The keyNames string can contain multiple
// names separated by commas or newlines.
// Key format: projects/PROJECT/locations/LOCATION/keyRings/RING/cryptoKeys/KEY/cryptoKeyVersions/VERSION
//
// Parameters:
//   - keyNames: comma/newline-separated GCP KMS key resource names
//
// Returns:
//   - *gcpKMSSigner: the configured signer
func newGCPKMSSigner(keyNames string) *gcpKMSSigner {

	var names []string
	for _, line := range strings.Split(keyNames, "\n") {
		for _, name := range strings.Split(line, ",") {
			name = strings.TrimSpace(name)
			if name != "" {
				names = append(names, name)
			}
		}
	}
	return &gcpKMSSigner{keyNames: names}
}

// region UNEXPORTED METHODS

// region Behaviors

func (g *gcpKMSSigner) name() string { return "gcp_kms" }

// available returns true if GCP credentials are configured and we can access at least one of the configured keys.
func (g *gcpKMSSigner) available() bool {

	if len(g.keyNames) == 0 {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := kms.NewKeyManagementClient(ctx)
	if err != nil {
		return false
	}
	defer func() { _ = client.Close() }() //nolint:errcheck // Close error not actionable

	for _, name := range g.keyNames {
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

// sign signs the data using GCP KMS.
func (g *gcpKMSSigner) sign(data []byte) (*Signature, error) {

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := kms.NewKeyManagementClient(ctx)
	if err != nil {
		return nil, &SignError{Backend: "gcp_kms", Err: err}
	}
	defer func() { _ = client.Close() }() //nolint:errcheck // Close error not actionable

	keyVersionName := g.findAvailableKey(ctx, client)
	if keyVersionName == "" {
		return nil, &SignError{Backend: "gcp_kms", Err: ErrNoKeyAvailable}
	}

	hash := sha256.Sum256(data)

	crc32c := crc32.MakeTable(crc32.Castagnoli)
	digestCRC32C := crc32.Checksum(hash[:], crc32c)

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

	if !resp.VerifiedDigestCrc32C {
		return nil, &SignError{Backend: "gcp_kms", Err: fmt.Errorf("digest CRC32C verification failed")}
	}

	return &Signature{
		Method: "gcp_kms",
		Value:  base64.StdEncoding.EncodeToString(resp.Signature),
		KeyID:  keyVersionName,
	}, nil
}

// findAvailableKey returns the first key that we can use for signing.
func (g *gcpKMSSigner) findAvailableKey(ctx context.Context, client *kms.KeyManagementClient) string {

	for _, name := range g.keyNames {
		keyVersionName := ensureKeyVersion(name)

		resp, err := client.GetCryptoKeyVersion(ctx, &kmspb.GetCryptoKeyVersionRequest{
			Name: keyVersionName,
		})
		if err != nil {
			continue
		}

		if resp.Algorithm == kmspb.CryptoKeyVersion_CRYPTO_KEY_VERSION_ALGORITHM_UNSPECIFIED {
			continue
		}

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

// endregion

// endregion

// ensureKeyVersion adds /cryptoKeyVersions/1 if not present.
//
// Parameters:
//   - name: GCP KMS key resource name
//
// Returns:
//   - string: key name with version suffix
func ensureKeyVersion(name string) string {

	if strings.Contains(name, "/cryptoKeyVersions/") {
		return name
	}
	return name + "/cryptoKeyVersions/1"
}

// verifyGCPKMS verifies a GCP KMS signature.
//
// Parameters:
//   - data: original content
//   - sig: signature to verify
//
// Returns:
//   - error: verification error
func verifyGCPKMS(data []byte, sig *Signature) error {

	if sig.Method != "gcp_kms" {
		return &VerifyError{Backend: "gcp_kms", Err: ErrWrongMethod}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := kms.NewKeyManagementClient(ctx)
	if err != nil {
		return &VerifyError{Backend: "gcp_kms", Err: err}
	}
	defer func() { _ = client.Close() }() //nolint:errcheck // Close error not actionable

	sigBytes, err := base64.StdEncoding.DecodeString(sig.Value)
	if err != nil {
		return &VerifyError{Backend: "gcp_kms", Err: err}
	}

	pubKeyResp, err := client.GetPublicKey(ctx, &kmspb.GetPublicKeyRequest{
		Name: sig.KeyID,
	})
	if err != nil {
		return &VerifyError{Backend: "gcp_kms", Err: err}
	}

	block, _ := pem.Decode([]byte(pubKeyResp.Pem))
	if block == nil {
		return &VerifyError{Backend: "gcp_kms", Err: fmt.Errorf("failed to parse PEM public key")}
	}

	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return &VerifyError{Backend: "gcp_kms", Err: fmt.Errorf("failed to parse public key: %w", err)}
	}

	if err := verifyGCPSignature(pubKey, pubKeyResp.Algorithm, data, sigBytes); err != nil {
		return &VerifyError{Backend: "gcp_kms", Err: err}
	}

	return nil
}

// verifyGCPSignature verifies a signature using the appropriate algorithm.
//
// Parameters:
//   - pubKey: parsed public key
//   - algorithm: GCP KMS algorithm identifier
//   - data: original content
//   - sig: raw signature bytes
//
// Returns:
//   - error: verification error
func verifyGCPSignature(pubKey any, algorithm kmspb.CryptoKeyVersion_CryptoKeyVersionAlgorithm, data, sig []byte) error { //nolint:cyclop

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
