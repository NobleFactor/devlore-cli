// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package schema provides embedded JSON schemas and default configs for the devlore ecosystem.
package schema

import _ "embed"

// DevloreSchema is the shared JSON schema for the devlore config file.
// Both lore and writ read from ~/.config/devlore/config.yaml.
//
//go:embed devlore-config.json
var DevloreSchema []byte

// SharedDefaultConfig is the default shared configuration (secrets, etc.).
// Installed to ~/.config/devlore/config.yaml
//
//go:embed defaults/devlore-shared.yaml
var SharedDefaultConfig []byte

// LoreDefaultConfig is the default lore-specific configuration.
// Installed to ~/.config/devlore/config.d/lore.yaml
//
//go:embed defaults/lore.yaml
var LoreDefaultConfig []byte

// WritDefaultConfig is the default writ-specific configuration.
// Installed to ~/.config/devlore/config.d/writ.yaml
//
//go:embed defaults/writ.yaml
var WritDefaultConfig []byte

// PackagesManifestSchema is the JSON schema for packages-manifest.{json,yaml} files.
// These files declare software dependencies for writ projects.
//
//go:embed packages-manifest.json
var PackagesManifestSchema []byte

// LifecycleSchema is provided by internal/lorepackage for backward compatibility.
// Prefer using lorepackage.LifecycleSchema directly.
// TODO: Remove this re-export once callers are updated.
//
//go:embed lifecycle.json
var LifecycleSchema []byte

// Legacy aliases for backward compatibility.
var (
	LoreSchema = DevloreSchema
	WritSchema = DevloreSchema
)
