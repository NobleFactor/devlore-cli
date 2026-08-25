// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"reflect"
	"sync"
)

var (
	// resourceImplementationsMu guards resourceImplementations. Registration happens in provider init
	// functions; lookups run once per announcement.
	resourceImplementationsMu sync.RWMutex

	// resourceImplementations maps a provider's sealed resource INTERFACE to the unexported struct behind
	// it. One entry per interface.
	resourceImplementations = make(map[reflect.Type]reflect.Type)
)

// RegisterResourceImplementation designates `implementation` as the concrete struct behind the sealed
// resource interface `resourceInterface`.
//
// A sealed resource is an exported interface over an unexported struct, so nothing outside the provider's
// package can name the struct — including the generated announcement, which lives in a sibling package and
// can reach only exported identifiers. This is how the struct crosses that boundary: the provider package
// registers it from its own init, and [AnnounceResource] resolves it when the interface is announced.
//
// **Ordering is guaranteed by the language, not by convention.** The generated package imports the provider
// package, and Go initializes imported packages first, so this registration always precedes the
// announcement that consumes it.
//
// The two types serve different roles and cannot be collapsed. The interface supplies the canonical type id
// — the URI fragment a saved document carries — while the struct supplies everything reflection needs: the
// method set, the dispatch target, and the key `marshalReflect` looks up when it wraps a returned value.
//
// Distinct from [RegisterResourceMint], which answers a different question: what a bare authored string
// claims as. The two coincide only for an interface with exactly one implementation. `file` designates
// `*file.AnyKind` as its mint while having four implementations, so conflating them would be wrong there.
//
// Parameters:
//   - `resourceInterface`: the provider's sealed resource interface type.
//   - `implementation`: the concrete pointer type implementing it.
func RegisterResourceImplementation(resourceInterface, implementation reflect.Type) {

	resourceImplementationsMu.Lock()
	defer resourceImplementationsMu.Unlock()

	resourceImplementations[resourceInterface] = implementation
}

// region HELPER FUNCTIONS

// resourceImplementationFor returns the concrete type reflection should use for `announced`.
//
// A non-interface announcement is its own implementation — the shape every provider had before sealing, and
// the reason this is a no-op for them.
//
// Parameters:
//   - `announcedType`: the type passed to [AnnounceResource]. Named in full because `announced` is the
//     package-level registry variable, and shadowing it here would be a trap for the next reader.
//
// Returns:
//   - `reflect.Type`: the concrete type, or nil when an interface was announced without a registration.
func resourceImplementationFor(announcedType reflect.Type) reflect.Type {

	if announcedType == nil || announcedType.Kind() != reflect.Interface {
		return announcedType
	}

	resourceImplementationsMu.RLock()
	defer resourceImplementationsMu.RUnlock()

	return resourceImplementations[announcedType]
}

// endregion
