// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

// Package schema provides embedded JSON schemas and default configs for lore and writ.
package schema

import _ "embed"

// Lore schema and default config.
var (
	//go:embed lore-config.json
	LoreSchema []byte

	//go:embed defaults/lore-config.yaml
	LoreDefaultConfig []byte
)

// Writ schema and default config.
var (
	//go:embed writ-config.json
	WritSchema []byte

	//go:embed defaults/writ-config.yaml
	WritDefaultConfig []byte
)
