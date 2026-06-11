// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"time"
)

// ElevationOffer acts as the complete metadata block governing elevation rules.
type ElevationOffer struct {

	// Strategy is the mechanism used to acquire elevation (host escalation, interactive challenge, role assumption,
	// or mandated approval).
	Strategy ElevationStrategy `json:"strategy" yaml:"strategy"`

	// Scope is the security domain and the explicit privileges the elevation must grant.
	Scope ElevationScope `json:"scope" yaml:"scope"`

	// Lifespan is the duration and caching semantics of the elevated state.
	Lifespan ElevationLifespan `json:"lifespan" yaml:"lifespan"`

	// Fallback is an optional chainable alternative, attempted when this policy cannot be satisfied.
	Fallback *ElevationOffer `json:"fallback" yaml:"fallback"`
}

// region SUPPORTING TYPES

// ElevationLifespan defines the duration and caching semantics of the elevated state.
type ElevationLifespan struct {

	// Ephemeral, if true, means privileges drop immediately after the action completes.
	Ephemeral bool `json:"ephemeral" yaml:"ephemeral"`

	// CacheDuration defines how long the elevated token/session remains valid before expiring.
	CacheDuration time.Duration `json:"cache_duration" yaml:"cache_duration"`
}

// ElevationScope defines the boundaries of the required privilege.
type ElevationScope struct {

	// Domain specifies the security subsystem (e.g., "OS", "GoogleOAuth", "AWS-IAM").
	Domain string `json:"domain" yaml:"domain"`

	// RequiredPrivileges lists the explicit capabilities needed (e.g., ["fsroot", "repo:write"]).
	RequiredPrivileges []string `json:"required_privileges" yaml:"required_privileges"`
}

// ElevationStrategy defines the mechanical approach used to achieve elevation.
type ElevationStrategy string

const (
	HostEscalation       ElevationStrategy = "host_escalation"       // OS-level escalation (e.g., sudo, runas)
	InteractiveChallenge ElevationStrategy = "interactive_challenge" // Prompting user for password/OTP
	IdentityAssumption   ElevationStrategy = "identity_assumption"   // Assuming a role dynamically (AWS STS, JWT minting)
	MandatedApproval     ElevationStrategy = "mandated_approval"     // Awaiting third-party admin gatekeeper approval
)
