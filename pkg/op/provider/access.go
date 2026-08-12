// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package provider holds the shared provider-registration surface.
//
// [AccessType] declares when a provider's methods are available (plan construction, execution, or
// both), and the instance plumbing hands tools their provider singletons.
package provider

// AccessType defines when a provider's methods are available.
type AccessType string

// The access modes.
const (
	Immediate AccessType = "immediate" // direct call during plan construction
	Planned   AccessType = "planned"   // graph node only — executed at runtime
	Both      AccessType = "both"      // available in both projections
)
