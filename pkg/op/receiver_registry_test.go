// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"reflect"
	"testing"
)

// Step-2 role-zone fixtures: dummy provider types announced (or refused) by the tests below. Their bare type names
// are their registry names (receiverName's default branch), chosen to collide with nothing real.
type roleZoneRootedProvider struct{ ProviderBase }

type roleZoneUnrootedProvider struct{ ProviderBase }

type roleZoneModuleProvider struct{ ProviderBase }

type roleZonePlacementOnlyProvider struct{ ProviderBase }

// trivialProviderConstructor satisfies [ProviderConstructor] for fixtures that are never instantiated.
func trivialProviderConstructor(*RuntimeEnvironment) (any, error) { return nil, nil }

// init announces the RootProviders partition fixtures before any test can materialize the registry singleton —
// [ReceiverRegistry] is a sync.OnceValue that snapshots the announcements at its first call, so staging must happen
// at package init.
func init() {
	AnnounceProvider(reflect.TypeFor[roleZoneRootedProvider](), RoleAction|RoleRoot, trivialProviderConstructor, nil)
	AnnounceProvider(reflect.TypeFor[roleZoneUnrootedProvider](), RoleAction, trivialProviderConstructor, nil)
}

func TestAnnounceProvider_PanicsWithoutDispatchBit(t *testing.T) {

	defer func() {
		if recover() == nil {
			t.Error("AnnounceProvider(RoleRoot only) did not panic; a dispatch-zone bit must be required")
		}
	}()

	AnnounceProvider(reflect.TypeFor[roleZonePlacementOnlyProvider](), RoleRoot, trivialProviderConstructor, nil)
}

func TestAnnounceProvider_AcceptsDispatchBit(t *testing.T) {

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("AnnounceProvider with a dispatch bit panicked: %v", r)
		}
	}()

	AnnounceProvider(reflect.TypeFor[roleZoneModuleProvider](), RoleModule, trivialProviderConstructor, nil)
}

func TestReceiverRegistry_RootProviders_ReturnsPlacementRootOnly(t *testing.T) {

	names := make(map[string]bool)
	for _, provider := range ReceiverRegistry().RootProviders() {
		names[provider.Name()] = true
	}

	if !names["roleZoneRootedProvider"] {
		t.Error("RootProviders() is missing the RoleRoot-placed fixture roleZoneRootedProvider")
	}
	if names["roleZoneUnrootedProvider"] {
		t.Error("RootProviders() includes the non-root fixture roleZoneUnrootedProvider")
	}
}

func TestRootDirective_ThreadsRoleRootThroughAnnounce(t *testing.T) {

	// The +devlore:root=true directive on flow.Provider must thread RoleRoot into the generated AnnounceProvider call
	// (flow/gen/provider.gen.go, linked into this test binary by the op_test announce shim). Inspecting the announced
	// roles proves the directive end to end: codegen emitted it, announce carried it, and the registry placed flow at
	// the root. (The step-2 matrix named this TestGenerate_RootDirective_ThreadsRoleRoot; renamed because no
	// generation runs here — the checked-in generated announce is the subject.)
	var flow ProviderReceiverType
	for _, provider := range ReceiverRegistry().RootProviders() {
		if provider.Name() == "flow" {
			flow = provider
		}
	}

	if flow == nil {
		t.Fatal("flow is not among RootProviders(); +devlore:root=true did not thread RoleRoot through the announce")
	}
	if got := flow.Roles().Dispatch(); got != RoleAction {
		t.Errorf("flow Roles().Dispatch() = %#x, want RoleAction", uint(got))
	}
	if got := flow.Roles().Placement(); got != RoleRoot {
		t.Errorf("flow Roles().Placement() = %#x, want RoleRoot", uint(got))
	}
}

// --- Step 4: flow declared a root action provider ---

func TestFlowProvider_RegisteredAsActionRoot(t *testing.T) {

	rt, ok := ReceiverRegistry().Type("flow")
	if !ok || rt == nil {
		t.Fatal(`ReceiverRegistry().Type("flow") not found (announced by the op_test shim import)`)
	}

	provider, ok := rt.(ProviderReceiverType)
	if !ok {
		t.Fatalf("flow registered as %T, want ProviderReceiverType", rt)
	}

	if got := provider.Roles(); got != RoleAction|RoleRoot {
		t.Errorf("flow Roles() = %#x, want RoleAction|RoleRoot (0x102)", uint(got))
	}
	if got := provider.Roles().Dispatch(); got != RoleAction {
		t.Errorf("flow Dispatch() = %#x, want RoleAction", uint(got))
	}
	if got := provider.Roles().Placement(); got != RoleRoot {
		t.Errorf("flow Placement() = %#x, want RoleRoot", uint(got))
	}
}

func TestRootProviders_IncludesFlow(t *testing.T) {

	for _, provider := range ReceiverRegistry().RootProviders() {
		if provider.Name() == "flow" {
			return
		}
	}

	t.Error("RootProviders() does not include flow; the +devlore:root=true directive did not reach the registry")
}
