// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package provider

// AccessType defines when a provider's methods are available.
type AccessType string

// Access level constants define when a provider method is available.
const (
	Immediate AccessType = "immediate" // direct call during plan construction
	Planned   AccessType = "planned"   // graph node only — executed at runtime
	Both      AccessType = "both"      // available in both projections
)
