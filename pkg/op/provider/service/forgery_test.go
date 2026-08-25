// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package service

import (
	"reflect"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// TestConvert_ADictCannotForgeAResource is the judgment scenario for the sealed-resource feature (#625).
//
// The threat is not a struct literal someone might write — none exists outside its own package anywhere in the
// tree. It is a path the framework already takes: [op.Convert]'s generic map-to-struct hydration admits any
// conversion whose source is a string-keyed map and whose target has concrete kind [reflect.Struct]. A resource
// slot satisfies that while the resource is a struct, so an authored dict reflectively mints one and fills its
// exported fields.
//
// The minted value carries no identity. [op.ResourceBase] has only unexported fields, so it stays zero and URI()
// returns "" — the resource is not merely unclaimed, it was never issued by any catalog. It then reaches a
// provider method that acts on the host with it.
//
// Sealing removes the path rather than policing it: the slot type becomes an interface, whose kind is not
// [reflect.Struct], so hydration declines and conversion falls through to the registered constructor, which
// interns through the catalog and refuses a map outright.
func TestConvert_ADictCannotForgeAResource(t *testing.T) {

	runtimeEnvironment := newTestRuntimeEnvironment(t)

	forged, err := op.Convert(
		runtimeEnvironment,
		map[string]any{"Name": "nginx"},
		reflect.TypeFor[Resource](),
	)

	if err == nil {
		t.Fatalf(
			"op.Convert(map) = %#v, want an error — a map must never construct a resource; "+
				"identity comes from the catalog, not from author-supplied fields",
			forged,
		)
	}
}

// TestConvert_EnvlessStringNoLongerReachesConvertFrom records the observation #649 was deferred pending.
//
// [op.Convert] step 7 reaches a resource's [op.TargetConverter] by probing `reflect.New(target)`. While the
// resource was a struct that probe produced a *Resource, which implements the interface, so ConvertFrom ran and
// returned a value with no URI. Now that the slot type is an interface, the probe produces a POINTER TO
// INTERFACE, whose method set is empty — the probe fails and the step declines.
//
// Env-less is the case that isolates it: with a runtime environment present, step 6's registered constructor
// wins first and interns through the catalog, so step 7 never runs either way.
//
// If this ever goes green by returning a resource again, #649's premise is wrong and its decision must be
// revisited.
func TestConvert_EnvlessStringNoLongerReachesConvertFrom(t *testing.T) {

	converted, err := op.Convert(nil, "nginx", reflect.TypeFor[Resource]())

	if err == nil {
		t.Fatalf(
			"op.Convert(env-less string) = %#v, want an error — with the slot sealed, step 7's probe cannot "+
				"satisfy TargetConverter, so no identity-less resource can be minted here",
			converted,
		)
	}
}
