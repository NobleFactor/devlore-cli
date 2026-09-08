// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// The fixtures below are the three shapes the contract refuses, and one it accepts. None is announced for real: the
// refusals happen before anything reaches the registry, and the accepting one is checked directly.

// ExportedStructFixture is a resource announced as a bare exported struct -- the shape every provider had before
// sealing (#625).
type ExportedStructFixture struct{ ResourceBase }

// unsealedFixture is an interface over a resource with no unexported method, so any type could satisfy it.
type unsealedFixture interface{ Resource }

// unsealedFixtureImpl is unsealedFixture's struct.
type unsealedFixtureImpl struct{ ResourceBase }

// sealedOverExportedFixture is sealed, but the struct registered behind it is exported.
type sealedOverExportedFixture interface {
	Resource
	sealedFixture()
}

// SealedOverExportedImpl is sealedOverExportedFixture's struct, exported by mistake.
type SealedOverExportedImpl struct{ ResourceBase }

func (*SealedOverExportedImpl) sealedFixture() {}

// conformingFixture has the shape: an interface embedding Resource, sealed, over an unexported struct.
type conformingFixture interface {
	Resource
	sealedConforming()
}

// conformingFixtureImpl is conformingFixture's struct.
type conformingFixtureImpl struct{ ResourceBase }

func (*conformingFixtureImpl) sealedConforming() {}

func init() {
	RegisterResourceImplementation(reflect.TypeFor[unsealedFixture](), reflect.TypeFor[unsealedFixtureImpl]())
	RegisterResourceImplementation(reflect.TypeFor[sealedOverExportedFixture](), reflect.TypeFor[SealedOverExportedImpl]())
	RegisterResourceImplementation(reflect.TypeFor[conformingFixture](), reflect.TypeFor[conformingFixtureImpl]())
}

// TestAnnounceResource_ANonConformingShapeIsRefused is #646's judgment scenario (the feature plan's scenario 8): a
// deliberately non-conforming fixture fails the announcement, and the refusal names the type and the rule it broke.
// Nothing reaches the registry -- the check runs before registration.
func TestAnnounceResource_ANonConformingShapeIsRefused(t *testing.T) {

	for _, testCase := range []struct {
		name      string
		announced reflect.Type
		wants     []string
	}{
		{"an exported struct, no interface", reflect.TypeFor[ExportedStructFixture](),
			[]string{"ExportedStructFixture", "announced as a struct", "sealed interface"}},
		{"an interface without a seal", reflect.TypeFor[unsealedFixture](),
			[]string{"unsealedFixture", "not sealed", "unexported method"}},
		{"a sealed interface over an exported struct", reflect.TypeFor[sealedOverExportedFixture](),
			[]string{"sealedOverExportedFixture", "SealedOverExportedImpl", "is exported"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			refusal := announcementRefusal(t, func() {
				AnnounceResource(testCase.announced, func(*RuntimeEnvironment, any) (Resource, error) { return nil, nil }, nil)
			})
			for _, want := range testCase.wants {
				if !strings.Contains(refusal, want) {
					t.Errorf("refusal %q does not name %q", refusal, want)
				}
			}
			if _, registered := ReceiverRegistry().TypeByReflection(testCase.announced); registered {
				t.Errorf("%v reached the registry despite the refusal", testCase.announced)
			}
		})
	}
}

// TestCheckSealedShape_TheContractHolds pins the accepting side: an interface embedding Resource, sealed by an
// unexported method, over an unexported struct in the same package whose pointer satisfies it.
func TestCheckSealedShape_TheContractHolds(t *testing.T) {

	if err := CheckSealedShape(reflect.TypeFor[conformingFixture](), reflect.TypeFor[conformingFixtureImpl]()); err != nil {
		t.Fatalf("CheckSealedShape(conforming) = %v; want nil", err)
	}
}

// announcementRefusal runs `announce` and returns the refusal it panicked with, failing the test if it did not panic.
//
// Parameters:
//   - `t`: the test.
//   - `announce`: the announcement expected to be refused.
//
// Returns:
//   - `string`: the panic's text.
func announcementRefusal(t *testing.T, announce func()) (refusal string) {
	t.Helper()
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("the announcement was accepted; want a refusal")
		}
		refusal = fmt.Sprint(recovered)
	}()
	announce()
	return ""
}
