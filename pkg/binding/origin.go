// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package binding

// region Origin

// Origin records where a resolved property value came from. The Name field carries the literal lookup key
// that matched the resolver's source — for env, the env var name including prefix; for config, the matched
// config-map key (or a file:line marker if the loader tracks it); for flag/override/default, the parameter
// name in its Go/starlark form.
type Origin struct {

	// Namespace identifies the source category. See [Namespace] for the enum.
	Namespace Namespace

	// Name is the literal lookup key that matched. Examples: "DEVLORE_WRIT_ROOT" for an env hit;
	// "target_root" for a flag/config/default hit; "config.star:12" for a config loader that tracks
	// source location.
	Name string
}

// String formats as "<namespace>:<name>". [NamespaceUnknown] renders as "unknown" alone since no name
// is meaningful in that case.
func (o Origin) String() string {
	if o.Namespace == NamespaceUnknown {
		return "unknown"
	}
	return o.Namespace.String() + ":" + o.Name
}

// endregion
