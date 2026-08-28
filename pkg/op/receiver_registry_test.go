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

// init announces the PromotedProviders partition fixtures before any test can materialize the registry singleton —
// [ReceiverRegistry] is a sync.OnceValue that snapshots the announcements at its first call, so staging must happen
// at package init.
func init() {
	AnnounceProvider(reflect.TypeFor[roleZoneRootedProvider](), NewProviderFlags(SurfaceWorkflow, PlacementPromoted), trivialProviderConstructor, nil)
	AnnounceProvider(reflect.TypeFor[roleZoneUnrootedProvider](), NewProviderFlags(SurfaceWorkflow, PlacementQualified), trivialProviderConstructor, nil)
}

func TestAnnounceProvider_PanicsWithoutASurface(t *testing.T) {

	defer func() {
		if recover() == nil {
			t.Error("AnnounceProvider with no surface did not panic; at least one surface must be required")
		}
	}()

	AnnounceProvider(reflect.TypeFor[roleZonePlacementOnlyProvider](),
		NewProviderFlags(0, PlacementPromoted), trivialProviderConstructor, nil)
}

func TestAnnounceProvider_AcceptsASurface(t *testing.T) {

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("AnnounceProvider with a dispatch bit panicked: %v", r)
		}
	}()

	AnnounceProvider(reflect.TypeFor[roleZoneModuleProvider](), NewProviderFlags(SurfaceScript, PlacementQualified), trivialProviderConstructor, nil)
}

func TestReceiverRegistry_PromotedProviders_ReturnsPromotedOnly(t *testing.T) {

	names := make(map[string]bool)
	for _, provider := range ReceiverRegistry().PromotedProviders() {
		names[provider.Name()] = true
	}

	if !names["roleZoneRootedProvider"] {
		t.Error("PromotedProviders() is missing the promoted fixture roleZoneRootedProvider")
	}
	if names["roleZoneUnrootedProvider"] {
		t.Error("PromotedProviders() includes the non-root fixture roleZoneUnrootedProvider")
	}
}

func TestPlacementDirective_ThreadsPlacementPromotedThroughAnnounce(t *testing.T) {

	// The +devlore:placement=promoted directive on flow.Provider must thread PlacementPromoted into the generated call
	// (flow/gen/provider.gen.go, linked into this test binary by the op_test announce shim). Inspecting the announced
	// roles proves the directive end to end: codegen emitted it, announce carried it, and the registry placed flow at
	// the root. (Renamed twice since the step-2 matrix named it, because no
	// generation runs here — the checked-in generated announce is the subject.)
	var flow ProviderReceiverType
	for _, provider := range ReceiverRegistry().PromotedProviders() {
		if provider.Name() == "flow" {
			flow = provider
		}
	}

	if flow == nil {
		t.Fatal("flow is not among PromotedProviders(); +devlore:placement=promoted did not thread through the announce")
	}
	if got := flow.Flags().Surfaces(); got != SurfaceWorkflow {
		t.Errorf("flow Flags().Surfaces() = %#x, want SurfaceWorkflow", uint(got))
	}
	if got := flow.Flags().Placement(); got != PlacementPromoted {
		t.Errorf("flow Flags().Placement() = %#x, want PlacementPromoted", uint(got))
	}
}

// --- Step 4: flow declared a root action provider ---

func TestFlowProvider_ReachesWorkflowSurfaceAndIsPromoted(t *testing.T) {

	rt, ok := ReceiverRegistry().Type("flow")
	if !ok || rt == nil {
		t.Fatal(`ReceiverRegistry().Type("flow") not found (announced by the op_test shim import)`)
	}

	provider, ok := rt.(ProviderReceiverType)
	if !ok {
		t.Fatalf("flow registered as %T, want ProviderReceiverType", rt)
	}

	if got := provider.Flags(); got != NewProviderFlags(SurfaceWorkflow, PlacementPromoted) {
		t.Errorf("flow Flags().Placement() = %#x, want PlacementPromoted", uint(got))
	}
	if got := provider.Flags().Surfaces(); got != SurfaceWorkflow {
		t.Errorf("Flags().Surfaces() = %#x, want SurfaceWorkflow", uint(got))
	}
	if got := provider.Flags().Placement(); got != PlacementPromoted {
		t.Errorf("flow Placement() = %#x, want PlacementPromoted", uint(got))
	}
}

func TestPromotedProviders_IncludesFlow(t *testing.T) {

	for _, provider := range ReceiverRegistry().PromotedProviders() {
		if provider.Name() == "flow" {
			return
		}
	}

	t.Error("PromotedProviders() does not include flow; the +devlore:root=true directive did not reach the registry")
}
