// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op_test

// Announce the flow provider into the pkg/op test binary's global receiver registry.
//
// Every graph fsroot now binds "flow.subgraph" by name (seeded by NewGraphSpec's inlined fsroot spec), so
// NewGraph / NewGraphSpec validate that name against the global registry ([ReceiverRegistry], populated by each
// provider's package-init AnnounceProvider). pkg/op cannot import a provider (the op → flow layering forbids it), but an
// external op_test file can: Go links internal (package op) and external (package op_test) test files into one test
// binary, so this init() makes flow.subgraph resolvable for every pkg/op test — including the internal-package tests
// that call NewGraph. This is a test-only announcement; the production always-announced guarantee is a later phase.
import _ "github.com/NobleFactor/devlore-cli/pkg/op/provider/flow/gen"
