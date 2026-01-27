// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package registry

import _ "embed"

// LifecycleSchema is the JSON schema for lifecycle.yaml files.
// These files define lore package metadata. Phase scripts are discovered
// from the directory structure (see RFC Section 9.3).
//
//go:embed lifecycle.json
var LifecycleSchema []byte
