// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package sops

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

// Interface guard.
var _ signer = (*awsKMSSigner)(nil)

// awsKMSSigner signs using AWS KMS.
type awsKMSSigner struct {
	keyARNs []string
}

// newAWSKMSSigner creates an AWS KMS signer with the given key ARNs. The keyARNs string can contain multiple ARNs
// separated by commas or newlines.
//
// Parameters:
//   - keyARNs: comma/newline-separated AWS KMS key ARNs
//
// Returns:
//   - *awsKMSSigner: the configured signer
func newAWSKMSSigner(keyARNs string) *awsKMSSigner {

	var arns []string
	for _, line := range strings.Split(keyARNs, "\n") {
		for _, arn := range strings.Split(line, ",") {
			arn = strings.TrimSpace(arn)
			if arn != "" {
				arns = append(arns, arn)
			}
		}
	}
	return &awsKMSSigner{keyARNs: arns}
}

// region UNEXPORTED METHODS

// region Behaviors

func (a *awsKMSSigner) name() string { return "aws_kms" }

// available returns true if AWS credentials are configured and we can access at least one of the configured keys.
func (a *awsKMSSigner) available() bool {

	if len(a.keyARNs) == 0 {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return false
	}

	for _, arn := range a.keyARNs {
		region := extractRegionFromARN(arn)
		if region == "" {
			continue
		}

		client := kms.NewFromConfig(cfg, func(o *kms.Options) {
			o.Region = region
		})

		_, err := client.DescribeKey(ctx, &kms.DescribeKeyInput{
			KeyId: &arn,
		})
		if err == nil {
			return true
		}
	}

	return false
}

// sign signs the data using AWS KMS.
func (a *awsKMSSigner) sign(data []byte) (*Signature, error) {

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

	hash := sha256.Sum256(data)

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

// findAvailableKey returns the first key ARN that we can use for signing.
func (a *awsKMSSigner) findAvailableKey(ctx context.Context, cfg aws.Config) (string, *kms.Client) {

	for _, arn := range a.keyARNs {
		region := extractRegionFromARN(arn)
		if region == "" {
			continue
		}

		client := kms.NewFromConfig(cfg, func(o *kms.Options) {
			o.Region = region
		})

		describeOut, err := client.DescribeKey(ctx, &kms.DescribeKeyInput{
			KeyId: &arn,
		})
		if err != nil {
			continue
		}

		if describeOut.KeyMetadata.KeyUsage != types.KeyUsageTypeSignVerify {
			continue
		}

		return arn, client
	}
	return "", nil
}

// endregion

// endregion

// extractRegionFromARN extracts the AWS region from a KMS key ARN.
// ARN format: arn:aws:kms:REGION:ACCOUNT:key/KEY-ID
//
// Parameters:
//   - arn: AWS KMS key ARN
//
// Returns:
//   - string: AWS region, or empty if not parseable
func extractRegionFromARN(arn string) string {

	re := regexp.MustCompile(`arn:aws:kms:([^:]+):`)
	matches := re.FindStringSubmatch(arn)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// verifyAWSKMS verifies an AWS KMS signature.
//
// Parameters:
//   - data: original content
//   - sig: signature to verify
//
// Returns:
//   - error: verification error
func verifyAWSKMS(data []byte, sig *Signature) error {

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
