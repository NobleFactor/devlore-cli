// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// AccessType defines when a provider's methods are available.
type AccessType string

// Access level constants define when a provider method is available.
const (
	AccessImmediate AccessType = "immediate" // direct call during plan construction
	AccessPlanned   AccessType = "planned"   // graph node only — executed at runtime
	AccessBoth      AccessType = "both"      // available in both projections
)
