// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package plan_test

// Announce the plan provider itself into this test binary's global receiver registry, alongside the file and flow
// announcements the API tests already import. The internal-package tests (provider_test.go, package plan) cannot
// import plan/gen — it imports plan, and an internal test file is compiled into plan, so the import would cycle —
// but an external test file can: Go links internal and external test files into one binary, so this blank import
// makes registry.Type("plan") resolvable for the Tier-3 resolution test and populates selfNames during
// buildPromotedBuiltins exactly as a host binary does.
import _ "github.com/NobleFactor/devlore-cli/pkg/op/provider/plan/gen"
