// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package elevator

import (
	"context"
	"fmt"
	"time"
)

// SecurityToken is the unified, pass-by-value token bridged down graph edges into consumer input slots. PLACEHOLDER
// SHAPE (see 6.1's token bridge).
type SecurityToken struct {

	// Value is the raw secret or signed JWT payload.
	Value string

	// Mechanism names the credential shape ("BEARER" / "AWS_CREDS" / "KUBECONFIG").
	Mechanism string

	// ExpiresAt is the strict clock limit, for fail-fast node evaluation.
	ExpiresAt time.Time

	// MaskedVars are env vars to inject into the target node, masked in logs.
	MaskedVars map[string]string
}

// IsExpired reports whether the token is past its ExpiresAt.
//
// Returns:
//   - `bool`: true when the token is no longer viable.
func (t *SecurityToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// Lease is the compensation state for an acquired elevation — what [Provider.CompensateElevate] revokes on undo.
// PLACEHOLDER SHAPE (the "model leases, not just TTL" lesson — see 6.1's prior art).
type Lease struct {

	// Provider is the token-provider that granted the elevation.
	Provider string

	// Handle is the lease / JTI handle used to revoke the grant.
	Handle string

	// ExpiresAt is when the grant lapses on its own.
	ExpiresAt time.Time
}

// TokenProvider is the pluggable minting-driver contract every concrete provider (AWS STS, Kubernetes, Vault,
// SPIFFE/SPIRE, OIDC) implements. STUB.
type TokenProvider interface {

	// Configure initializes the driver from untyped config properties.
	Configure(properties map[string]any) error

	// MintToken creates an environment-specific token for the requested TTL.
	MintToken(ctx context.Context, ttl time.Duration) (*SecurityToken, error)

	// Type returns the unique driver key (e.g. "aws_sts_assume_role").
	Type() string
}

// Broker orchestrates the token-provider drivers (WORKING NAME — it resolves the `ElevationProvider` collision with
// §6's privileged-process executor). It registers drivers, instantiates the enabled providers from a
// [EnvironmentConfig], and mints on request. STUB.
type Broker struct {
	registry        map[string]func() TokenProvider
	activeProviders map[string]TokenProvider
}

// NewBroker constructs an empty [Broker]. STUB.
//
// Returns:
//   - `*Broker`: a broker with no registered drivers and no active providers.
func NewBroker() *Broker {
	return &Broker{
		registry:        make(map[string]func() TokenProvider),
		activeProviders: make(map[string]TokenProvider),
	}
}

// region EXPORTED METHODS

// region State management

// RegisterDriver binds a driver type key to its factory. STUB.
//
// Parameters:
//   - `driverType`: the unique driver key (e.g. "aws_sts_assume_role").
//   - `factory`: constructs a fresh [TokenProvider] of that type.
func (b *Broker) RegisterDriver(driverType string, factory func() TokenProvider) {
	b.registry[driverType] = factory
}

// endregion

// region Behaviors

// InitializeFromConfig instantiates the enabled token providers for an environment, failing fast on an unsupported
// driver type. STUB.
//
// TODO(elevation): for each enabled provider, call its factory, `Configure` it, and store the instance in
// `activeProviders`; then return nil.
//
// Parameters:
//   - `environment`: one environment's provisioning (its offers and token-provider definitions).
//
// Returns:
//   - `error`: the stub error today; in the target, non-nil on an unsupported or misconfigured driver.
func (b *Broker) InitializeFromConfig(environment EnvironmentConfig) error {

	for _, provider := range environment.TokenProviders {
		if !provider.IsEnabled {
			continue
		}
		if _, supported := b.registry[provider.Type]; !supported {
			return fmt.Errorf("elevator: unsupported token provider type %q", provider.Type)
		}
	}
	return errNotImplemented
}

// RequestElevation looks up the named provider and mints a token. STUB. Called by the elevate node at runtime.
//
// TODO(elevation): return `provider.MintToken(ctx, ttl)`.
//
// Parameters:
//   - `ctx`: the request context.
//   - `providerName`: the provider name an offer resolves to in the active environment.
//   - `ttl`: the requested token lifetime.
//
// Returns:
//   - `*SecurityToken`: the minted token (nil until implemented).
//   - `error`: a clear unavailable error for an unknown provider, else the stub error.
func (b *Broker) RequestElevation(ctx context.Context, providerName string, ttl time.Duration) (*SecurityToken, error) {

	_, available := b.activeProviders[providerName]
	if !available {
		return nil, fmt.Errorf(
			"elevator: no active token provider %q (undefined or disabled in this environment)", providerName)
	}

	_ = ctx
	_ = ttl
	return nil, errNotImplemented
}

// endregion

// endregion
