// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package elevator

import "time"

// Config is the elevator provider's configuration — the **environment-keyed provisioning** of named offers (WORKING
// NAME; the devconfig-section integration and the final shape are to be settled — see 6.1's config outline).
//
// PLACEHOLDER SHAPE. The split it encodes: a [Requirement] (plan-time, saved in the graph) carries a named offer; at
// run time the named offer is resolved against `Config.Environments[<env>].Offers`; and each environment configures
// the [TokenProviderConfig] entries its offers reference. The graph holds *what*; this holds *how*, per environment.
type Config struct {

	// Environments maps an environment name ("dev" / "test" / "stage" / "prod") to its provisioning.
	Environments map[string]EnvironmentConfig `json:"environments" yaml:"environments"`
}

// region SUPPORTING TYPES

// EnvironmentConfig is one environment's offer realizations plus the token-provider definitions they reference.
// PLACEHOLDER SHAPE.
type EnvironmentConfig struct {

	// Offers maps a named offer (the graph's `offer_reference_id`) to its realization in THIS environment.
	Offers map[string]Offer `json:"offers" yaml:"offers"`

	// TokenProviders are the provider definitions the offers reference — provider setup, NOT raw long-lived secrets.
	TokenProviders []TokenProviderConfig `json:"token_providers" yaml:"token_providers"`
}

// Offer is the realization of a named offer in an environment: which strategy, and (for `identity_assumption`) which
// token provider satisfies it. PLACEHOLDER SHAPE.
type Offer struct {

	// Strategy selects the elevation mechanism ("host_escalation" | "identity_assumption" | ...).
	Strategy string `json:"strategy" yaml:"strategy"`

	// TokenProvider names the [TokenProviderConfig] that mints this offer's token (empty for `host_escalation`).
	TokenProvider string `json:"token_provider,omitempty" yaml:"token_provider,omitempty"`
}

// TokenProviderConfig is a token-provider **driver definition** (`aws_sts_assume_role` / `k8s_token_request` /
// `hashicorp_vault` / ...). `ConfigProperties` carry provider setup (role ARNs, addresses, mount paths) — NOT raw
// long-lived credentials; the actual credentials come from the host's credential chain. PLACEHOLDER SHAPE.
type TokenProviderConfig struct {
	Name                     string         `json:"name"                       yaml:"name"`
	Type                     string         `json:"type"                       yaml:"type"`
	IsEnabled                bool           `json:"is_enabled"                 yaml:"is_enabled"`
	ConnectionTimeoutSeconds int            `json:"connection_timeout_seconds" yaml:"connection_timeout_seconds"`
	ConfigProperties         map[string]any `json:"config_properties"          yaml:"config_properties"`
}

// Requirement is the **plan-time** elevation ask carried on a graph unit/node and saved into the signed graph: a named
// offer plus its TTL and context assertions. It is environment-agnostic — the same `OfferReferenceID` resolves to a
// different realization per environment. PLACEHOLDER SHAPE (the home of these fields on the unit/node is to be
// settled).
type Requirement struct {
	OfferReferenceID         string        `json:"offer_reference_id"          yaml:"offer_reference_id"`
	RequestedTTL             time.Duration `json:"requested_ttl"               yaml:"requested_ttl"`
	RequiredContextAssertion []string      `json:"required_context_assertions" yaml:"required_context_assertions"`
}

// endregion
