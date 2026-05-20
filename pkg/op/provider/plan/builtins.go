// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package plan

// This file is intentionally empty. It previously held bridge-side starlark.Builtin bodies for
// Provider's Tier-3 methods (Assemble / Run / Save / Load / Clear) when those methods bypassed
// codegen + op.MethodMetadata dispatch. The methods now go through the standard goReceiver
// dispatch path like every other Provider method; the dedicated builtins were unnecessary.
//
// Delete this file when the change lands — keeping it empty here only because the tooling that
// produced this edit cannot rm files in the working tree directly.
