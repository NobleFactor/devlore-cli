// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package inventory

import (
	"reflect"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// TestBootDiscipline_EveryResourceTypeOverridesAddressing walks every announced Resource type via the
// receiver registry and asserts that its Addressing() returns a non-Unknown mode.
//
// The contract from op.ResourceBase.Addressing is explicit: every concrete Resource type must override the
// default sentinel. This test catches any Resource type added later that forgets to override — that type
// would inherit op.AddressingUnknown and the test fires.
//
// The discipline test lives in the inventory package because inventory blank-imports every provider's gen
// package, so by the time tests run, the registry is fully populated. The same test can't live in pkg/op
// itself: pkg/op cannot import the providers (they depend on op, which would create a cycle).
//
// Instantiation strategy: reflect.New on the Resource's reflect.Type yields a zero-value pointer; the test
// asserts Addressing() on it. This relies on the override being a pure constant return — true by design for
// every concrete Resource. If a future Resource's Addressing reads state, the test surfaces that as a panic
// with a clear stack trace, which is also a useful signal.
func TestBootDiscipline_EveryResourceTypeOverridesAddressing(t *testing.T) {

	types := op.SnapshotReceiverTypes()
	if len(types) == 0 {
		t.Fatal("no receiver types announced; expected provider gen packages to register types at init")
	}

	var resourceCount int

	for _, rt := range types {

		rrt, ok := rt.(op.ResourceReceiverType)
		if !ok {
			continue
		}
		resourceCount++

		elemType := rrt.ProviderType()
		if elemType.Kind() != reflect.Struct {
			t.Errorf("%s: ProviderType() returned %s (kind %s), want struct", rrt.Name(), elemType, elemType.Kind())
			continue
		}

		instance, ok := reflect.New(elemType).Interface().(op.Resource)
		if !ok {
			t.Errorf("%s: zero-value *%s does not implement op.Resource", rrt.Name(), elemType.Name())
			continue
		}

		if mode := instance.Addressing(); mode == op.AddressingUnknown {
			t.Errorf("%s: Addressing() = AddressingUnknown — concrete Resource types must override", rrt.Name())
		}

		// The content-⟹-packable invariant: a graph must be immutable and portable across machine boundaries,
		// and a content resource IS its bytes — one that cannot pack them could not cross the boundary or run
		// there. "Content-addressable but not packable" is an illegal resource, not a degraded one.
		if instance.Addressing() == op.AddressingContent {
			if _, ok := instance.(op.Packer); !ok {
				t.Errorf("%s: AddressingContent but does not implement op.Packer — content resources must travel",
					rrt.Name())
			}
			if _, ok := instance.(op.Unpacker); !ok {
				t.Errorf("%s: AddressingContent but does not implement op.Unpacker — content resources must travel",
					rrt.Name())
			}
		}
	}

	if resourceCount == 0 {
		t.Fatal("no Resource types found in receiver registry; expected at least one (file, mem, ...)")
	}
}

// TestBootDiscipline_EveryResourceResultResolvesItsProductType asserts that a receipt can resolve the product
// type it records.
//
// [op.ReceiptBase.Commit] records the id [canonicalIDOf] reports, and restore resolves it through an index
// built from every action method's DECLARED result type. Two different derivations of the same identity, and
// nothing forces them to agree.
//
// While every provider's resource was a struct they agreed by construction — the declared return and the
// value's dynamic type were the same type. Sealing separates them: a method declares the interface while
// returning the unexported struct behind it. A recorded id taken from the Go type would then name something no
// method declares.
//
// **The failure is silent.** `retypeStampedResult` reads:
//
//	productType, ok := ReceiverRegistry().ProductTypeByID(s.resultType)
//	if !ok {
//		return
//	}
//
// so a resumed run keeps the raw URI string instead of a resource. No error, no log, and a resume test that
// only asserts success would pass throughout. This asserts resolution itself.
//
// It lives here for the same reason as the discipline test above: inventory blank-imports every provider's gen
// package, so the registry is fully populated by the time tests run. A provider's own package imports neither
// its gen package nor any other, so nothing is announced in that test binary at all.
func TestBootDiscipline_EveryResourceResultResolvesItsProductType(t *testing.T) {

	types := op.SnapshotReceiverTypes()
	if len(types) == 0 {
		t.Fatal("no receiver types announced; expected provider gen packages to register types at init")
	}

	resourceInterface := reflect.TypeFor[op.Resource]()
	var checked int

	for _, rt := range types {

		provider, isProvider := rt.(op.ProviderReceiverType)
		if !isProvider {
			continue
		}

		for method := range provider.Methods() {

			result := method.ResultType()
			if result == nil || !result.Implements(resourceInterface) {
				continue
			}

			// The id a receipt records for a value of this type: the announced identity, not the Go type.
			// typeIDOf drops any pointer, which is exactly what op.ResourceBase stores at construction.
			recorded := op.CanonicalResourceTypeID(result)

			resolved, ok := op.ReceiverRegistry().ProductTypeByID(recorded)
			if !ok {
				t.Errorf(
					"%s.%s returns %s, whose recorded product id %q does not resolve — a receipt from this "+
						"method would silently fail to retype its result on resume",
					provider.Name(), method.Name(), result, recorded)
				continue
			}

			if !result.AssignableTo(resolved) && !resolved.AssignableTo(result) {
				t.Errorf("%s.%s: id %q resolved to %s, unrelated to the declared %s",
					provider.Name(), method.Name(), recorded, resolved, result)
			}

			checked++
		}
	}

	if checked == 0 {
		t.Fatal("no action method returns a resource; the index this test guards would be empty")
	}
}

// TestBootDiscipline_EveryUnpackerResolvesByTypeID pins phase 1's `UnpackerByTypeID` repair (#641).
//
// A resource whose content lives in the document's content section is rebuilt through
// [op.Unpacker.Unpack], reached by looking the resource up by the type id its URI carries. That lookup
// mints its probe reflectively:
//
//	unpacker, ok := reflect.New(resourceType.ProviderType()).Interface().(Unpacker)
//
// Before #641 it reflected on the ANNOUNCED type. Sealing makes that an interface, `reflect.New` yields a
// pointer-to-interface with an empty method set, and the assertion fails — returning `ok=false`, which every
// caller reads as "this type has no unpacker" rather than as an error. Content rehydration would simply stop,
// quietly. #641 routed the reflection to the implementation; this asserts it, because nothing else does.
//
// **The expected set is named rather than derived.** Deriving it from `ProviderType()` would consult the very
// value the regression corrupts: a reverted fix makes the type stop implementing [op.Unpacker], the derived
// loop skips it, and the test passes while content rehydration is broken. Naming the four means a fifth
// unpacker must be added here deliberately — which is the point of a discipline test.
func TestBootDiscipline_EveryUnpackerResolvesByTypeID(t *testing.T) {

	// Every provider whose resource implements op.Unpacker, by registry name.
	wantUnpackers := map[string]bool{
		"function.Resource": true,
		"json.Resource":     true,
		"mem.Resource":      true,
		"yaml.Resource":     true,
	}

	types := op.SnapshotReceiverTypes()
	if len(types) == 0 {
		t.Fatal("no receiver types announced; expected provider gen packages to register types at init")
	}

	seen := map[string]bool{}

	for _, rt := range types {

		resourceType, isResource := rt.(op.ResourceReceiverType)
		if !isResource || !wantUnpackers[resourceType.Name()] {
			continue
		}

		seen[resourceType.Name()] = true

		unpacker, resolved := op.ReceiverRegistry().UnpackerByTypeID(resourceType.TypeID())
		if !resolved {
			t.Errorf(
				"%s implements Unpack, but UnpackerByTypeID(%q) did not resolve — its content section would "+
					"silently stop rehydrating, with no error and no log",
				resourceType.Name(), resourceType.TypeID())
			continue
		}

		if unpacker == nil {
			t.Errorf("%s: UnpackerByTypeID reported resolved but returned nil", resourceType.Name())
		}
	}

	for name := range wantUnpackers {
		if !seen[name] {
			t.Errorf("%s was never announced — either it lost its resource, or this list is stale", name)
		}
	}
}
