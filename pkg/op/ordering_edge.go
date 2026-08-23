// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import "reflect"

// OrderingEdge is the pure ordering edge's parameter type (ruled at phase 4 PR 3/#611): a slot of this
// type consumes an upstream invocation's promise solely for sequencing — the promise edge orders the
// consumer after its producer, and the delivered value is discarded BY TYPE: every source converts to the
// empty OrderingEdge through the [TargetConverter] contract. The parameter's type is the declaration
// (4-resource-management.md §3): no directive, no value semantics, no nil-promise machinery.
//
// The type exists because `any` cannot carry the contract: [ExecutableUnit] is assignable to `any`, so an
// invocation bound to an any-typed parameter captures the flow-combinator convention (the unit itself)
// instead of its promise — an OrderingEdge-typed parameter takes the promise, which is the edge.
//
// Authoring: `after=<invocation>`; nil (the default) means no edge.
type OrderingEdge struct{}

// CanConvertFrom reports that every source converts — the discard-by-type half of the contract.
//
// Cheap-probe contract: safe on a zero-value receiver.
//
// Parameters:
//   - `_`: the candidate source type; every type is absorbable.
//
// Returns:
//   - `bool`: always true.
func (*OrderingEdge) CanConvertFrom(_ reflect.Type) bool { return true }

// ConvertFrom discards `value` and returns the empty edge — the delivered promise value carries no
// meaning here; only the edge it rode in on does.
//
// Parameters:
//   - `_`: the delivered value, discarded.
//
// Returns:
//   - `any`: the empty [OrderingEdge].
//   - `error`: always nil.
func (*OrderingEdge) ConvertFrom(_ any) (any, error) { return OrderingEdge{}, nil }
