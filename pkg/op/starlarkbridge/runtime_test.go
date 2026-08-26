// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package starlarkbridge

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// region Test fixtures

// stubHasAttrs is a minimal [starlark.HasAttrs] whose attribute set is fixed at construction, used to verify
// filteredReceiver narrowing without standing up a full runtime environment.
type stubHasAttrs struct {
	attrs map[string]starlark.Value
}

// Attr returns the fixed attribute, or (nil, nil) — starlark's "no such attribute" — when absent.
func (s *stubHasAttrs) Attr(name string) (starlark.Value, error) { return s.attrs[name], nil }

// AttrNames returns the fixed attribute names in sorted order.
func (s *stubHasAttrs) AttrNames() []string {

	names := make([]string, 0, len(s.attrs))
	for name := range s.attrs {
		names = append(names, name)
	}

	sort.Strings(names)
	return names
}

func (s *stubHasAttrs) String() string        { return "stub" }
func (s *stubHasAttrs) Type() string          { return "stub" }
func (s *stubHasAttrs) Freeze()               {}
func (s *stubHasAttrs) Truth() starlark.Bool  { return starlark.True }
func (s *stubHasAttrs) Hash() (uint32, error) { return 0, nil }

// endregion

// region Tests

// TestFilteredReceiver_Attr verifies a denied name errors, a retained name delegates, and an absent name passes
// through the "no such attribute" signal unchanged.
func TestFilteredReceiver_Attr(t *testing.T) {

	inner := &stubHasAttrs{attrs: map[string]starlark.Value{
		"keep":   starlark.String("kept"),
		"deny":   starlark.String("denied"),
		"deny_2": starlark.String("also-denied"),
	}}

	receiver := &filteredReceiver{
		HasAttrs: inner,
		global:   "plan",
		denied:   map[string]bool{"deny": true, "deny_2": true},
	}

	t.Run("retained name delegates", func(t *testing.T) {

		value, err := receiver.Attr("keep")
		if err != nil {
			t.Fatalf("Attr(keep): unexpected error: %v", err)
		}

		if value != starlark.String("kept") {
			t.Fatalf("Attr(keep) = %v, want %q", value, "kept")
		}
	})

	t.Run("denied name errors with global-qualified message", func(t *testing.T) {

		_, err := receiver.Attr("deny")
		if err == nil {
			t.Fatal("Attr(deny): expected error, got nil")
		}

		if want := "plan.deny is not available in this runtime"; !strings.Contains(err.Error(), want) {
			t.Fatalf("Attr(deny) error = %q, want it to contain %q", err.Error(), want)
		}
	})

	t.Run("absent name delegates the no-such-attribute signal", func(t *testing.T) {

		value, err := receiver.Attr("missing")
		if err != nil || value != nil {
			t.Fatalf("Attr(missing) = (%v, %v), want (nil, nil)", value, err)
		}
	})
}

// TestFilteredReceiver_AttrNames verifies the denied names are dropped and the rest retained.
func TestFilteredReceiver_AttrNames(t *testing.T) {

	inner := &stubHasAttrs{attrs: map[string]starlark.Value{
		"keep":   starlark.String("kept"),
		"deny":   starlark.String("denied"),
		"deny_2": starlark.String("also-denied"),
	}}

	receiver := &filteredReceiver{
		HasAttrs: inner,
		global:   "plan",
		denied:   map[string]bool{"deny": true, "deny_2": true},
	}

	got := receiver.AttrNames()
	want := []string{"keep"}

	if !equalStrings(got, want) {
		t.Fatalf("AttrNames() = %v, want %v", got, want)
	}
}

// TestDenyAttributes_RecordsDenials verifies the option unions names across calls and keeps globals separate.
func TestDenyAttributes_RecordsDenials(t *testing.T) {

	rt := &Runtime{}

	DenyAttributes("plan", "assemble", "run")(rt)
	DenyAttributes("plan", "save")(rt)
	DenyAttributes("ui", "prompt")(rt)

	plan := sortedKeys(rt.denied["plan"])
	if want := []string{"assemble", "run", "save"}; !equalStrings(plan, want) {
		t.Fatalf("denied[plan] = %v, want %v", plan, want)
	}

	ui := sortedKeys(rt.denied["ui"])
	if want := []string{"prompt"}; !equalStrings(ui, want) {
		t.Fatalf("denied[ui] = %v, want %v", ui, want)
	}
}

// TestRuntime_applyDenials verifies a present global is wrapped, and an absent denied global is skipped.
func TestRuntime_applyDenials(t *testing.T) {

	t.Run("present global is wrapped", func(t *testing.T) {

		inner := &stubHasAttrs{attrs: map[string]starlark.Value{"deny": starlark.String("x")}}
		predeclared := starlark.StringDict{"plan": inner}

		rt := &Runtime{denied: map[string]map[string]bool{"plan": {"deny": true}}}
		rt.applyDenials(predeclared)

		wrapped, ok := predeclared["plan"].(*filteredReceiver)
		if !ok {
			t.Fatalf("predeclared[plan] is %T, want *filteredReceiver", predeclared["plan"])
		}

		if _, err := wrapped.Attr("deny"); err == nil {
			t.Fatal("wrapped Attr(deny): expected error, got nil")
		}
	})

	t.Run("absent denied global is skipped", func(t *testing.T) {

		predeclared := starlark.StringDict{"plan": &stubHasAttrs{}}

		rt := &Runtime{denied: map[string]map[string]bool{"absent": {"x": true}}}
		rt.applyDenials(predeclared)

		if _, present := predeclared["absent"]; present {
			t.Fatal("applyDenials introduced an entry for an absent global")
		}
	})
}

// endregion

// region Test helpers

// equalStrings reports whether two string slices are element-wise equal.
func equalStrings(a, b []string) bool {

	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

// sortedKeys returns the keys of a set in sorted order.
func sortedKeys(set map[string]bool) []string {

	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}

	sort.Strings(keys)
	return keys
}

// endregion

// region Step-6 registration-branch fixtures

// The step-6 fixtures are real announced provider types — NewRuntime resolves module instances through the global
// registry (env.ModuleByName → ReceiverRegistry), so an unannounced fixture would be silently skipped by buildOne
// and prove nothing. Announced at package init (ahead of the registry singleton's sync.OnceValue snapshot); inert to
// every other test because only an explicit env.Modules selection reaches them.

// bridgeModuleFixture is an immediate-mode, non-root module: registers under its own name.
type bridgeModuleFixture struct{ op.ProviderBase }

func (*bridgeModuleFixture) Ping() string { return "pong" }

// bridgeRootModuleFixtureA is an immediate-mode, root-placed module: each method installs as its own global.
type bridgeRootModuleFixtureA struct{ op.ProviderBase }

func (*bridgeRootModuleFixtureA) Greet() string { return "hello" }

// bridgeRootModuleFixtureB collides with A: same method name, so selecting both must panic at registration.
type bridgeRootModuleFixtureB struct{ op.ProviderBase }

func (*bridgeRootModuleFixtureB) Greet() string { return "hi" }

// bridgePlannedFixture is planned-only, non-root: never registered as a global.
type bridgePlannedFixture struct{ op.ProviderBase }

func (*bridgePlannedFixture) Deploy() string { return "" }

// bridgePlannedRootFixture is planned-only, root-placed (the flow shape): never registered as a global either.
type bridgePlannedRootFixture struct{ op.ProviderBase }

func (*bridgePlannedRootFixture) Launch() string { return "" }

// bridgeMixedClaimFixture has one deterministic method and one that claims nothing, so a hermetic runtime must
// admit part of it rather than all or none.
type bridgeMixedClaimFixture struct{ op.ProviderBase }

func (*bridgeMixedClaimFixture) Pure() string { return "pure" }

func (*bridgeMixedClaimFixture) Impure() string { return "impure" }

// bridgeNoClaimFixture claims nothing at all: a hermetic runtime admits none of it, which is a loud failure.
type bridgeNoClaimFixture struct{ op.ProviderBase }

func (*bridgeNoClaimFixture) Reach() string { return "reach" }

func init() {
	op.AnnounceProvider(reflect.TypeFor[bridgeMixedClaimFixture](), op.RoleModule,
		func(re *op.RuntimeEnvironment) (any, error) {
			return &bridgeMixedClaimFixture{ProviderBase: op.NewProviderBase(re)}, nil
		},
		map[string]op.MethodMetadata{
			"Pure":   {ParameterNames: []string{}, Claims: op.ClaimDeterministic},
			"Impure": {ParameterNames: []string{}},
		})

	op.AnnounceProvider(reflect.TypeFor[bridgeNoClaimFixture](), op.RoleModule,
		func(re *op.RuntimeEnvironment) (any, error) {
			return &bridgeNoClaimFixture{ProviderBase: op.NewProviderBase(re)}, nil
		},
		map[string]op.MethodMetadata{"Reach": {ParameterNames: []string{}}})

	announce := func(providerType reflect.Type, roles op.ProviderRole, method string, construct op.ProviderConstructor) {
		op.AnnounceProvider(providerType, roles, construct,
			map[string]op.MethodMetadata{method: {ParameterNames: []string{}}})
	}
	announce(reflect.TypeFor[bridgeModuleFixture](), op.RoleModule, "Ping",
		func(re *op.RuntimeEnvironment) (any, error) {
			return &bridgeModuleFixture{ProviderBase: op.NewProviderBase(re)}, nil
		})
	announce(reflect.TypeFor[bridgeRootModuleFixtureA](), op.RoleModule|op.RoleRoot, "Greet",
		func(re *op.RuntimeEnvironment) (any, error) {
			return &bridgeRootModuleFixtureA{ProviderBase: op.NewProviderBase(re)}, nil
		})
	announce(reflect.TypeFor[bridgeRootModuleFixtureB](), op.RoleModule|op.RoleRoot, "Greet",
		func(re *op.RuntimeEnvironment) (any, error) {
			return &bridgeRootModuleFixtureB{ProviderBase: op.NewProviderBase(re)}, nil
		})
	announce(reflect.TypeFor[bridgePlannedFixture](), op.RoleAction, "Deploy",
		func(re *op.RuntimeEnvironment) (any, error) {
			return &bridgePlannedFixture{ProviderBase: op.NewProviderBase(re)}, nil
		})
	announce(reflect.TypeFor[bridgePlannedRootFixture](), op.RoleAction|op.RoleRoot, "Launch",
		func(re *op.RuntimeEnvironment) (any, error) {
			return &bridgePlannedRootFixture{ProviderBase: op.NewProviderBase(re)}, nil
		})
}

// bridgeFixtureModules resolves announced fixture receiver types by name for an env.Modules selection.
func bridgeFixtureModules(t *testing.T, names ...string) []op.ProviderReceiverType {
	t.Helper()

	modules := make([]op.ProviderReceiverType, 0, len(names))
	for _, name := range names {
		rt, ok := op.ReceiverRegistry().Type(name)
		if !ok {
			t.Fatalf("fixture %q not in the registry", name)
		}
		provider, ok := rt.(op.ProviderReceiverType)
		if !ok {
			t.Fatalf("fixture %q registered as %T, want ProviderReceiverType", name, rt)
		}
		modules = append(modules, provider)
	}
	return modules
}

// endregion

// region Step-6 registration-branch tests

func TestNewRuntime_PlannedOnlyProvider_NotRegistered(t *testing.T) {

	env := &op.RuntimeEnvironment{
		Modules: bridgeFixtureModules(t, "bridgePlannedFixture", "bridgePlannedRootFixture"),
	}

	predeclared := NewRuntime(env).Predeclared()

	for _, absent := range []string{"bridgePlannedFixture", "bridgePlannedRootFixture", "deploy", "launch"} {
		if _, present := predeclared[absent]; present {
			t.Errorf("predeclared contains %q; planned-only providers must not register as globals", absent)
		}
	}
}

func TestNewRuntime_ModuleNonRoot_RegisteredUnderName(t *testing.T) {

	env := &op.RuntimeEnvironment{Modules: bridgeFixtureModules(t, "bridgeModuleFixture")}

	predeclared := NewRuntime(env).Predeclared()

	if _, present := predeclared["bridgeModuleFixture"]; !present {
		t.Error(`predeclared lacks "bridgeModuleFixture"; a RoleModule non-root provider registers under its name`)
	}
	if _, present := predeclared["ping"]; present {
		t.Error(`predeclared contains "ping"; a non-root module's methods must not install as top-level globals`)
	}
}

func TestNewRuntime_ModuleRoot_InstallsEachMethodAndPanicsOnCollision(t *testing.T) {

	env := &op.RuntimeEnvironment{Modules: bridgeFixtureModules(t, "bridgeRootModuleFixtureA")}

	predeclared := NewRuntime(env).Predeclared()

	if _, present := predeclared["greet"]; !present {
		t.Error(`predeclared lacks "greet"; a RoleModule|RoleRoot provider installs each method as its own global`)
	}
	if _, present := predeclared["bridgeRootModuleFixtureA"]; present {
		t.Error(`predeclared contains the provider name; a root module exposes methods, not itself`)
	}

	// The collision half: selecting both root modules declares "greet" twice, which must panic at registration.
	defer func() {
		if recover() == nil {
			t.Error("NewRuntime with colliding root-module globals did not panic")
		}
	}()

	collision := &op.RuntimeEnvironment{
		Modules: bridgeFixtureModules(t, "bridgeRootModuleFixtureA", "bridgeRootModuleFixtureB"),
	}
	NewRuntime(collision)
}

// endregion

// region HERMETIC FILTER

func TestNewRuntime_Hermetic_AdmitsOnlyClaimingMethods(t *testing.T) {

	env := &op.RuntimeEnvironment{Modules: bridgeFixtureModules(t, "bridgeMixedClaimFixture")}

	t.Run("non-hermetic admits every method", func(t *testing.T) {

		receiver := requireGlobal(t, NewRuntime(env).Predeclared(), "bridgeMixedClaimFixture")

		for _, name := range []string{"pure", "impure"} {
			if _, err := receiver.Attr(name); err != nil {
				t.Errorf("Attr(%q): %v; a non-hermetic runtime filters nothing", name, err)
			}
		}
	})

	t.Run("hermetic admits the claiming method only", func(t *testing.T) {

		receiver := requireGlobal(t, NewRuntime(env, Hermetic()).Predeclared(), "bridgeMixedClaimFixture")

		if _, err := receiver.Attr("pure"); err != nil {
			t.Errorf("Attr(pure): %v; a method claiming deterministic must survive the filter", err)
		}

		if _, err := receiver.Attr("impure"); err == nil {
			t.Error("Attr(impure) succeeded; a method claiming nothing must not reach a hermetic surface")
		}
	})
}

func TestNewRuntime_Hermetic_ProviderWithNothingAdmitted_ReportsAtTheCallSite(t *testing.T) {

	env := &op.RuntimeEnvironment{Modules: bridgeFixtureModules(t, "bridgeNoClaimFixture")}

	// Selecting every module is the natural default, and a per-method filter exists so a caller need not curate
	// the list. Refusing a provider whole at construction would make that impossible, so the global is installed
	// with every attribute denied and the author hears about it where they reached for it -- naming the method,
	// which a construction-time failure naming the provider could not.
	receiver := requireGlobal(t, NewRuntime(env, Hermetic()).Predeclared(), "bridgeNoClaimFixture")

	_, err := receiver.Attr("reach")
	if err == nil {
		t.Fatal("Attr(reach) succeeded; a method claiming nothing must not reach a hermetic surface")
	}

	for _, want := range []string{"bridgeNoClaimFixture.reach", "hermetic", "deterministic"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Attr(reach) error %q lacks %q; the message must name the method and say why", err, want)
		}
	}
}

// requireGlobal fetches a predeclared global as a [starlark.HasAttrs], failing the test when it is absent.
func requireGlobal(t *testing.T, predeclared starlark.StringDict, name string) starlark.HasAttrs {
	t.Helper()

	value, present := predeclared[name]
	if !present {
		t.Fatalf("predeclared lacks %q", name)
	}

	receiver, ok := value.(starlark.HasAttrs)
	if !ok {
		t.Fatalf("predeclared[%q] is %T, want starlark.HasAttrs", name, value)
	}

	return receiver
}

// endregion
