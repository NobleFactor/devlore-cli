// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package signing

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
)

// AWSKMSSigner signs using AWS KMS.
type AWSKMSSigner struct {
	keyARNs []string
}

// NewAWSKMSSigner creates an AWS KMS signer with the given key ARNs.
// The keyARNs string can contain multiple ARNs separated by commas or newlines.
func NewAWSKMSSigner(keyARNs string) *AWSKMSSigner {
	var arns []string
	for _, line := range strings.Split(keyARNs, "\n") {
		for _, arn := range strings.Split(line, ",") {
			arn = strings.TrimSpace(arn)
			if arn != "" {
				arns = append(arns, arn)
			}
		}
	}
	return &AWSKMSSigner{keyARNs: arns}
}

// Name returns "aws_kms".
func (a *AWSKMSSigner) Name() string {
	return "aws_kms"
}

// Available returns true if AWS credentials are configured and
// we can access at least one of the configured keys.
func (a *AWSKMSSigner) Available() bool {
	if len(a.keyARNs) == 0 {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try to load AWS config
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return false
	}

	// Check if we can describe at least one key
	for _, arn := range a.keyARNs {
		region := extractRegionFromARN(arn)
		if region == "" {
			continue
		}

		client := kms.NewFromConfig(cfg, func(o *kms.Options) {
			o.Region = region
		})

		// Try to describe the key to verify access
		_, err := client.DescribeKey(ctx, &kms.DescribeKeyInput{
			KeyId: &arn,
		})
		if err == nil {
			return true
		}
	}

	return false
}

// extractRegionFromARN extracts the AWS region from a KMS key ARN.
// ARN format: arn:aws:kms:REGION:ACCOUNT:key/KEY-ID
func extractRegionFromARN(arn string) string {
	// arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012
	re := regexp.MustCompile(`arn:aws:kms:([^:]+):`)
	matches := re.FindStringSubmatch(arn)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// findAvailableKey returns the first key ARN that we can use for signing.
func (a *AWSKMSSigner) findAvailableKey(ctx context.Context, cfg aws.Config) (string, *kms.Client) {
	for _, arn := range a.keyARNs {
		region := extractRegionFromARN(arn)
		if region == "" {
			continue
		}

		client := kms.NewFromConfig(cfg, func(o *kms.Options) {
			o.Region = region
		})

		// Verify we can use this key for signing
		describeOut, err := client.DescribeKey(ctx, &kms.DescribeKeyInput{
			KeyId: &arn,
		})
		if err != nil {
			continue
		}

		// Check if key can be used for signing
		keyUsage := describeOut.KeyMetadata.KeyUsage
		if keyUsage != types.KeyUsageTypeSignVerify {
			// This key is for encrypt/decrypt, not signing
			continue
		}

		return arn, client
	}
	return "", nil
}

// Sign signs the data using AWS KMS.
func (a *AWSKMSSigner) Sign(data []byte) (*Signature, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, &SignError{Backend: "aws_kms", Err: err}
	}

	keyARN, client := a.findAvailableKey(ctx, cfg)
	if client == nil {
		return nil, &SignError{Backend: "aws_kms", Err: ErrNoKeyAvailable}
	}

	// Hash the data first (KMS has message size limits)
	hash := sha256.Sum256(data)

	// Sign the hash
	signOut, err := client.Sign(ctx, &kms.SignInput{
		KeyId:            &keyARN,
		Message:          hash[:],
		MessageType:      types.MessageTypeDigest,
		SigningAlgorithm: types.SigningAlgorithmSpecRsassaPssSha256,
	})
	if err != nil {
		return nil, &SignError{Backend: "aws_kms", Err: err}
	}

	return &Signature{
		Method: "aws_kms",
		Value:  base64.StdEncoding.EncodeToString(signOut.Signature),
		KeyID:  keyARN,
	}, nil
}

// VerifyAWSKMS verifies an AWS KMS signature.
func VerifyAWSKMS(data []byte, sig *Signature) error {
	if sig.Method != "aws_kms" {
		return &VerifyError{Backend: "aws_kms", Err: ErrWrongMethod}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return &VerifyError{Backend: "aws_kms", Err: err}
	}

	region := extractRegionFromARN(sig.KeyID)
	if region == "" {
		return &VerifyError{Backend: "aws_kms", Err: ErrNoKeyAvailable, Details: "invalid key ARN"}
	}

	client := kms.NewFromConfig(cfg, func(o *kms.Options) {
		o.Region = region
	})

	sigBytes, err := base64.StdEncoding.DecodeString(sig.Value)
	if err != nil {
		return &VerifyError{Backend: "aws_kms", Err: err}
	}

	hash := sha256.Sum256(data)

	_, err = client.Verify(ctx, &kms.VerifyInput{
		KeyId:            &sig.KeyID,
		Message:          hash[:],
		MessageType:      types.MessageTypeDigest,
		Signature:        sigBytes,
		SigningAlgorithm: types.SigningAlgorithmSpecRsassaPssSha256,
	})
	if err != nil {
		return &VerifyError{Backend: "aws_kms", Err: err}
	}

	return nil
}
