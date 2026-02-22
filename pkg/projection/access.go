// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package projection

// Access level constants define when a provider method is available.
const (
	AccessImmediate = "immediate" // query only — no graph node
	AccessPlanned   = "planned"   // graph node only — no immediate call
	AccessBoth      = "both"      // available in both projections
)
