// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// The boot-discipline test that walks every announced Resource type and asserts
// `Addressing() != AddressingUnknown` lives in pkg/op/inventory/discipline_test.go.
// It can't live here in package op because the test must blank-import every
// provider's gen package to populate the receiver registry, and the providers
// import op — a cycle that Go's test compiler rejects.
//
// The inventory package already blank-imports every provider for inventory-generation
// purposes, so it's the natural home for the discipline test.

package op