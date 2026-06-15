// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package elevator is a STUB / PLACEHOLDER for the privilege-elevation provider — the mechanism that fulfills the
// elevation policy ([6.1-privilege-elevation.md]). It scaffolds the two strategies (ProcessSpawn / IdentityAssumption),
// the token-provider broker, and the env-keyed config.
//
// EVERYTHING HERE IS PROVISIONAL. The package name `elevator`, the types `Provider` / `Config` / `Broker` / `Offer` /
// `Requirement`, and the config shape are WORKING NAMES, to be settled (see 6.1's "Open design work"). Method bodies
// are not implemented; the provider is not yet announced (no codegen / inventory ride).
//
// [6.1-privilege-elevation.md]: ../../../../docs/architecture/6.1-privilege-elevation.md
package elevator

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// errNotImplemented marks the stub surface; the real mechanism is the subject of docs/architecture/6.1-privilege-elevation.md.
var errNotImplemented = fmt.Errorf("elevator: not implemented (stub) — see docs/architecture/6.1-privilege-elevation.md")

// Provider is the privilege-elevation provider (WORKING NAME).
//
// It fulfills the elevation policy via two strategies — `ProcessSpawn` (a privileged worker) and `IdentityAssumption`
// (just-in-time token minting through the [Broker]). STUB: the methods are unimplemented and the provider is not yet
// announced.
//
// +devlore:access=planned
type Provider struct {
	op.ProviderBase
}

// NewProvider constructs the elevator [Provider] bound to the runtime environment. STUB.
//
// Parameters:
//   - `runtimeEnvironment`: the runtime environment supplying the platform abstraction, the status sink, and (once
//     wired) the [Broker] built from `Application.Config`.
//
// Returns:
//   - `*Provider`: the constructed provider.
func NewProvider(runtimeEnvironment *op.RuntimeEnvironment) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(runtimeEnvironment)}
}

// region EXPORTED METHODS

// region Behaviors

// Compensable actions

// Elevate acquires the elevation a [Requirement] asks for, returning the minted [*SecurityToken] plus a [*Lease] for
// release on undo. STUB.
//
// TODO(elevation): resolve the named offer (`requirement.OfferReferenceID`) against the runtime environment's
// [Config] for the active environment, mint via the [Broker] (`IdentityAssumption`) or acquire the privileged context
// (`ProcessSpawn`), and bridge the token by-value down the outgoing edges (see 6.1).
//
// Parameters:
//   - `requirement`: the plan-time elevation ask carried on the unit/node.
//
// Returns:
//   - `*SecurityToken`: the minted token (nil until implemented).
//   - `*Lease`: the compensation state for [Provider.CompensateElevate] (nil until implemented).
//   - `error`: currently always the stub error.
func (p *Provider) Elevate(requirement Requirement) (token *SecurityToken, lease *Lease, err error) {
	_ = requirement
	return nil, nil, errNotImplemented
}

// CompensateElevate releases the elevation acquired by [Provider.Elevate] — revoking the lease / token. STUB.
//
// Parameters:
//   - `lease`: the [*Lease] returned by [Provider.Elevate].
//
// Returns:
//   - `error`: currently always the stub error.
func (p *Provider) CompensateElevate(lease *Lease) error {
	_ = lease
	return errNotImplemented
}

// endregion

// endregion
