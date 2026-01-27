// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

// Package schema provides embedded JSON schemas and default configs for the devlore ecosystem.
package schema

import _ "embed"

// DevloreSchema is the shared JSON schema for the devlore config file.
// Both lore and writ read from ~/.config/devlore/config.yaml.
//
//go:embed devlore-config.json
var DevloreSchema []byte

// DevloreDefaultConfig is the shared default configuration.
//
//go:embed defaults/devlore-config.yaml
var DevloreDefaultConfig []byte

// PackagesManifestSchema is the JSON schema for packages-manifest.{json,yaml} files.
// These files declare software dependencies for writ projects.
//
//go:embed packages-manifest.json
var PackagesManifestSchema []byte

// LifecycleSchema is provided by internal/registry for backward compatibility.
// Prefer using registry.LifecycleSchema directly.
// TODO: Remove this re-export once callers are updated.
//
//go:embed lifecycle.json
var LifecycleSchema []byte

// Legacy aliases for backward compatibility.
// These point to the shared devlore schema and config.
var (
	LoreSchema        = DevloreSchema
	LoreDefaultConfig = DevloreDefaultConfig
	WritSchema        = DevloreSchema
	WritDefaultConfig = DevloreDefaultConfig
)
