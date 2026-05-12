// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

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
	}

	if resourceCount == 0 {
		t.Fatal("no Resource types found in receiver registry; expected at least one (file, mem, ...)")
	}
}