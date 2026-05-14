// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package binding

// region Namespace

// Namespace identifies a property source category. Numeric values ascend with precedence — higher value beats
// lower. Callers can compare Namespaces directly to determine which source would win in a precedence cascade.
type Namespace int

const (
	// NamespaceUnknown is the zero value; should not appear on a Property post-resolve.
	NamespaceUnknown Namespace = iota

	// NamespaceDefault — hardcoded fallback declared on plan.param(name, default=value).
	NamespaceDefault

	// NamespaceConfig — starlark or YAML config files.
	NamespaceConfig

	// NamespaceEnv — environment variables. Resolver checks the program-specific prefix first,
	// then cascades to the global prefix; both hits share this Namespace and are distinguished by Origin.Name.
	NamespaceEnv

	// NamespaceFlag — command-line arguments.
	NamespaceFlag

	// NamespaceOverride — explicit runtime force; programmatic only, not user-typed.
	NamespaceOverride
)

// String returns the canonical lowercase name of the Namespace.
func (n Namespace) String() string {
	return [...]string{"unknown", "default", "config", "env", "flag", "override"}[n]
}

// endregion
