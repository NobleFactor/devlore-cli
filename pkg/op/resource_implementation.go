// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"
	"sync"
	"unicode"
	"unicode/utf8"
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
// `file.AnyKind` as its mint while having four implementations, so conflating them would be wrong there.
//
// Parameters:
//   - `resourceInterface`: the provider's sealed resource interface type.
//   - `implementation`: the concrete pointer type implementing it.
func RegisterResourceImplementation(resourceInterface, implementation reflect.Type) {

	resourceImplementationsMu.Lock()
	defer resourceImplementationsMu.Unlock()

	resourceImplementations[resourceInterface] = implementation
}

// CheckSealedShape reports whether `announced` and `implementation` have the sealed-resource shape: an exported
// interface embedding [Resource], sealed by an unexported method, over an unexported struct declared in the same
// package, whose pointer satisfies the interface. It is the contract for announcing a resource
// (docs/architecture/4.3-resource-registration.md §5): [AnnounceResource] refuses a type that fails it, at the one
// place every resource passes, and the inventory's boot-discipline suite reads the same rule back over every
// announced type. The rules run in the order a reader diagnoses in, and each failure names the type and the rule.
//
// Only a resource announcement is held to this shape. A provider ([AnnounceProvider]) or a plain data type
// ([AnnounceType]) is never consulted, so an exported struct remains a data type exactly as before (#646).
//
// Parameters:
//   - `announced`: the type passed to [AnnounceResource].
//   - `implementation`: the struct registered for it by [RegisterResourceImplementation]; for a struct announcement,
//     the struct itself.
//
// Returns:
//   - `error`: nil when the shape holds; otherwise the first rule broken, naming the type.
func CheckSealedShape(announced, implementation reflect.Type) error {

	if announced == nil || announced.Kind() != reflect.Interface {
		return fmt.Errorf("resource %v is announced as a %s; a resource is announced as its sealed interface over an "+
			"unexported struct (4.3-resource-registration.md §5)", announced, kindName(announced))
	}
	if !announced.Implements(resourceInterfaceType) {
		return fmt.Errorf("resource interface %v does not embed op.Resource", announced)
	}
	if !declaresOwnSeal(announced) {
		return fmt.Errorf("resource interface %v is not sealed: it declares no unexported method of its own (op.Resource's do "+
			"not count), so a type outside %s could satisfy it", announced, announced.PkgPath())
	}
	if implementation == nil || implementation.Kind() != reflect.Struct {
		return fmt.Errorf("resource %v: the registered implementation %v is a %s, not a struct",
			announced, implementation, kindName(implementation))
	}
	if isExportedName(implementation.Name()) {
		return fmt.Errorf("resource %v: the struct behind it, %v, is exported; the struct is unexported so nothing "+
			"outside %s can build one", announced, implementation, implementation.PkgPath())
	}
	if implementation.PkgPath() != announced.PkgPath() {
		return fmt.Errorf("resource %v: the struct behind it, %v, is declared in %s, not in the interface's package %s",
			announced, implementation, implementation.PkgPath(), announced.PkgPath())
	}
	if !reflect.PointerTo(implementation).Implements(announced) {
		return fmt.Errorf("resource %v: *%v does not satisfy it", announced, implementation)
	}
	return nil
}

// region HELPER FUNCTIONS

// resourceImplementationFor returns the concrete type reflection should use for `announced`.
//
// A non-interface announcement passes through as its own implementation so that [CheckSealedShape] can refuse it
// by name; since every provider is sealed (#646), nothing announces one any more.
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

// declaresOwnSeal reports whether an interface type declares at least one unexported method of its own -- the seal
// that keeps a type declared outside the interface's package from satisfying it. [Resource]'s own unexported methods
// do not count: every resource interface embeds [Resource], so they would make the rule vacuous (found by the
// judgment scenario's unsealed fixture, #646).
//
// Parameters:
//   - `interfaceType`: an interface type embedding [Resource].
//
// Returns:
//   - `bool`: true when some unexported method is not one of [Resource]'s.
func declaresOwnSeal(interfaceType reflect.Type) bool {

	inherited := make(map[string]bool, resourceInterfaceType.NumMethod())
	for index := 0; index < resourceInterfaceType.NumMethod(); index++ {
		inherited[resourceInterfaceType.Method(index).Name] = true
	}
	for index := 0; index < interfaceType.NumMethod(); index++ {
		method := interfaceType.Method(index)
		if method.PkgPath != "" && !inherited[method.Name] {
			return true
		}
	}
	return false
}

// isExportedName reports whether a Go identifier is exported.
//
// Parameters:
//   - `name`: the identifier; "" for an unnamed type.
//
// Returns:
//   - `bool`: true when the first rune is upper case.
func isExportedName(name string) bool {

	first, _ := utf8.DecodeRuneInString(name)
	return first != utf8.RuneError && unicode.IsUpper(first)
}

// kindName names a type's kind for a refusal, tolerating nil.
//
// Parameters:
//   - `t`: the type, possibly nil.
//
// Returns:
//   - `string`: the kind's name, or "nil".
func kindName(t reflect.Type) string {

	if t == nil {
		return "nil"
	}
	return t.Kind().String()
}

// endregion
