// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package sops

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
)

// Interface guard.
var _ signer = (*azureKVSigner)(nil)

// azureKVSigner signs using Azure Key Vault.
type azureKVSigner struct {
	keyURLs []string
}

// newAzureKVSigner creates an Azure Key Vault signer with the given key URLs. The keyURLs string can contain multiple
// URLs separated by commas or newlines.
// Key format: https://VAULT.vault.azure.net/keys/KEY/VERSION
//
// Parameters:
//   - keyURLs: comma/newline-separated Azure Key Vault key URLs
//
// Returns:
//   - *azureKVSigner: the configured signer
func newAzureKVSigner(keyURLs string) *azureKVSigner {

	var urls []string
	for _, line := range strings.Split(keyURLs, "\n") {
		for _, u := range strings.Split(line, ",") {
			u = strings.TrimSpace(u)
			if u != "" {
				urls = append(urls, u)
			}
		}
	}
	return &azureKVSigner{keyURLs: urls}
}

// region UNEXPORTED METHODS

// region Behaviors

func (a *azureKVSigner) name() string { return "azure_kv" }

// available returns true if Azure credentials are configured and we can access at least one of the configured keys.
func (a *azureKVSigner) available() bool {

	if len(a.keyURLs) == 0 {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return false
	}

	for _, keyURL := range a.keyURLs {
		vaultURL, keyName, version, err := parseKeyURL(keyURL)
		if err != nil || keyName == "" {
			continue
		}

		client, err := azkeys.NewClient(vaultURL, cred, nil)
		if err != nil {
			continue
		}

		_, err = client.GetKey(ctx, keyName, version, nil)
		if err == nil {
			return true
		}
	}

	return false
}

// sign signs the data using Azure Key Vault.
func (a *azureKVSigner) sign(data []byte) (*Signature, error) {

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, &SignError{Backend: "azure_kv", Err: err}
	}

	keyURL, client, keyName, version := a.findAvailableKey(ctx, cred)
	if client == nil {
		return nil, &SignError{Backend: "azure_kv", Err: ErrNoKeyAvailable}
	}

	hash := sha256.Sum256(data)

	algorithm := azkeys.SignatureAlgorithmRS256
	signResp, err := client.Sign(ctx, keyName, version, azkeys.SignParameters{
		Algorithm: &algorithm,
		Value:     hash[:],
	}, nil)
	if err != nil {
		return nil, &SignError{Backend: "azure_kv", Err: err}
	}

	return &Signature{
		Method: "azure_kv",
		Value:  base64.StdEncoding.EncodeToString(signResp.Result),
		KeyID:  keyURL,
	}, nil
}

// findAvailableKey returns the first key URL that we can use for signing.
func (a *azureKVSigner) findAvailableKey(ctx context.Context, cred *azidentity.DefaultAzureCredential) (vaultURL string, client *azkeys.Client, keyName, version string) {

	for _, keyURL := range a.keyURLs {
		vaultURL, keyName, version, err := parseKeyURL(keyURL)
		if err != nil || keyName == "" {
			continue
		}

		client, err := azkeys.NewClient(vaultURL, cred, nil)
		if err != nil {
			continue
		}

		keyResp, err := client.GetKey(ctx, keyName, version, nil)
		if err != nil {
			continue
		}

		if keyResp.Key != nil && keyResp.Key.Kty != nil {
			kty := *keyResp.Key.Kty
			if kty == azkeys.KeyTypeRSA || kty == azkeys.KeyTypeRSAHSM ||
				kty == azkeys.KeyTypeEC || kty == azkeys.KeyTypeECHSM {
				return keyURL, client, keyName, version
			}
		}
	}
	return "", nil, "", ""
}

// endregion

// endregion

// parseKeyURL extracts vault URL, key name, and version from a key URL.
// URL format: https://VAULT.vault.azure.net/keys/KEY/VERSION
//
// Parameters:
//   - keyURL: Azure Key Vault key URL
//
// Returns:
//   - vaultURL: base vault URL
//   - keyName: key name
//   - version: key version (may be empty)
//   - err: URL parse error
func parseKeyURL(keyURL string) (vaultURL, keyName, version string, err error) {

	u, err := url.Parse(keyURL)
	if err != nil {
		return "", "", "", err
	}

	vaultURL = u.Scheme + "://" + u.Host

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) >= 2 && parts[0] == "keys" {
		keyName = parts[1]
		if len(parts) >= 3 {
			version = parts[2]
		}
	}

	return vaultURL, keyName, version, nil
}

// verifyAzureKV verifies an Azure Key Vault signature.
//
// Parameters:
//   - data: original content
//   - sig: signature to verify
//
// Returns:
//   - error: verification error
func verifyAzureKV(data []byte, sig *Signature) error {

	if sig.Method != "azure_kv" {
		return &VerifyError{Backend: "azure_kv", Err: ErrWrongMethod}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return &VerifyError{Backend: "azure_kv", Err: err}
	}

	vaultURL, keyName, version, err := parseKeyURL(sig.KeyID)
	if err != nil || keyName == "" {
		return &VerifyError{Backend: "azure_kv", Err: ErrNoKeyAvailable, Details: "invalid key URL"}
	}

	client, err := azkeys.NewClient(vaultURL, cred, nil)
	if err != nil {
		return &VerifyError{Backend: "azure_kv", Err: err}
	}

	sigBytes, err := base64.StdEncoding.DecodeString(sig.Value)
	if err != nil {
		return &VerifyError{Backend: "azure_kv", Err: err}
	}

	hash := sha256.Sum256(data)

	algorithm := azkeys.SignatureAlgorithmRS256
	verifyResp, err := client.Verify(ctx, keyName, version, azkeys.VerifyParameters{
		Algorithm: &algorithm,
		Digest:    hash[:],
		Signature: sigBytes,
	}, nil)
	if err != nil {
		return &VerifyError{Backend: "azure_kv", Err: err}
	}

	if verifyResp.Value == nil || !*verifyResp.Value {
		return &VerifyError{Backend: "azure_kv", Err: ErrWrongMethod, Details: "signature verification failed"}
	}

	return nil
}
