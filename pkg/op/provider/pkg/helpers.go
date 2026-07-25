// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package pkg provides package management actions for the operation graph.
package pkg

import (
	"github.com/NobleFactor/devlore-cli/pkg/platform"
)

// resolveType maps a resource's manager prefix to the canonical purl type for the platform.
//
// A non-empty `prefix` resolves through [platform.Platform.ResolvePurlType]; an unknown prefix falls back to the
// platform default rather than failing (the resource already carries a parsed type from construction). An empty
// prefix yields the platform's default native purl type.
//
// Parameters:
//   - `plat`: the target platform.
//   - `prefix`: the resource's purl type / manager prefix; empty for the default.
//
// Returns:
//   - `string`: the canonical purl type.
func resolveType(plat platform.Platform, prefix string) string {

	if prefix == "" {
		return plat.DefaultPurlType()
	}

	if resolved, ok := plat.ResolvePurlType(prefix); ok {
		return resolved
	}

	return plat.DefaultPurlType()
}

// toPURL projects a [*Resource] into a routable [platform.PURL], carrying the requested version.
//
// Parameters:
//   - `plat`: the target platform, for type resolution.
//   - `resource`: the resource to project.
//
// Returns:
//   - `platform.PURL`: the purl with the canonical type, name, and requested version.
func toPURL(plat platform.Platform, resource *Resource) platform.PURL {
	return platform.PURL{
		Type:    resolveType(plat, resource.Type),
		Name:    resource.Name,
		Version: resource.Version,
	}
}

// toPURLs projects each [*Resource] into a [platform.PURL], preserving input order.
//
// The router batches the returned slice by purl type and returns one receipt per package in this order, so the
// caller correlates receipts to resources by index.
//
// Parameters:
//   - `plat`: the target platform, for type resolution.
//   - `resources`: the resources to project.
//
// Returns:
//   - `[]platform.PURL`: the projected purls, in input order.
func toPURLs(plat platform.Platform, resources []*Resource) []platform.PURL {

	purls := make([]platform.PURL, len(resources))

	for i, resource := range resources {
		purls[i] = toPURL(plat, resource)
	}

	return purls
}
