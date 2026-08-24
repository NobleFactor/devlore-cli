// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"reflect"
	"sync"
)

var (
	// resourceMintsMu guards resourceMints. Registration happens in provider init functions; lookups run
	// on every plan-time bind of an authored string to an interface-typed resource parameter.
	resourceMintsMu sync.RWMutex

	// resourceMints maps a provider's resource INTERFACE to the concrete type an authored string claims
	// as. One entry per interface — the designation belongs to the interface, never to a parameter.
	resourceMints = make(map[reflect.Type]reflect.Type)
)

// RegisterResourceMint designates `mint` as the concrete type an authored string claims as when it is
// bound to a parameter typed by the resource interface `resourceInterface`
// (docs/architecture/4-resource-management.md §5.7 rule 6, amended 2026-08-23).
//
// A claim asserts a kind — "claims are true when made" needs a kind to be true about — and an interface
// asserts none, which is why an authored string bound to one is refused by default. A scheme with a kind
// axis resolves that by naming the claim that deliberately asserts nothing: `file` designates
// `*file.Any`, whose assertion is existence alone and which resolves to the observed kind at activation.
//
// **The designation lives on the interface, once.** Not per parameter: two methods taking the same slot
// type must claim the same way, or the same authored path would mean different intent depending on which
// method received it. An interface with no designation keeps the refusal — the author states a kind or
// feeds a discovery.
//
// Called from provider init alongside the type's announcement.
//
// Parameters:
//   - `resourceInterface`: the provider's resource interface type.
//   - `mint`: the concrete resource type an authored string claims as.
func RegisterResourceMint(resourceInterface, mint reflect.Type) {

	resourceMintsMu.Lock()
	defer resourceMintsMu.Unlock()

	resourceMints[resourceInterface] = mint
}

// resourceMintFor returns the concrete type designated for the resource interface `t`.
//
// Parameters:
//   - `t`: the parameter's reflect.Type.
//
// Returns:
//   - `reflect.Type`: the designated mint type, or nil.
//   - `bool`: true when `t` is an interface with a designation.
func resourceMintFor(t reflect.Type) (reflect.Type, bool) {

	resourceMintsMu.RLock()
	defer resourceMintsMu.RUnlock()

	mint, ok := resourceMints[t]

	return mint, ok
}
