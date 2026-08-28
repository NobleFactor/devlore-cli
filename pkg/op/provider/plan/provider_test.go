// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package plan

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"

	// ui is promoted and reaches BOTH surfaces; announcing it here is what lets this package assert the
	// workflow half of its promotion. plan imports flow directly, so flow announces itself already.
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/ui/gen"
)

// resolutionProvider builds a plan Provider against the test binary's registry (file + flow + plan announced by the
// API tests' blank imports and the plan_announce_test.go shim).
func resolutionProvider(t *testing.T) *Provider {
	t.Helper()
	return NewProvider(&op.RuntimeEnvironment{})
}

// tierCollisionPromotedProvider is a synthetic promoted provider whose one method ("Variable" → snake "variable") is aimed
// at whatever tier a test seeds into promoteMethods' name sets. Built as a real receiver type via
// [op.NewProviderReceiverType]; never announced, so the process registry stays clean.
type tierCollisionPromotedProvider struct{}

func (tierCollisionPromotedProvider) Variable() string { return "" }

// collisionPromotedReceiverType constructs the synthetic promoted provider's receiver type.
func collisionPromotedReceiverType(t *testing.T) op.ProviderReceiverType {
	t.Helper()

	receiverType, err := op.NewProviderReceiverType(
		reflect.TypeFor[tierCollisionPromotedProvider](),
		func(*op.RuntimeEnvironment) (any, error) { return nil, nil },
		op.NewProviderFlags(op.SurfaceWorkflow, op.PlacementPromoted),
		map[string][]op.Parameter{"Variable": {}},
		nil,
	)
	if err != nil {
		t.Fatalf("NewProviderReceiverType: %v", err)
	}

	return receiverType
}

// wantPanicContaining runs `body` and asserts it panics with a message containing `fragment`.
func wantPanicContaining(t *testing.T, fragment string, body func()) {
	t.Helper()

	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("no panic; want one containing %q", fragment)
			return
		}
		if message, ok := r.(string); !ok || !strings.Contains(message, fragment) {
			t.Errorf("panic = %v, want message containing %q", r, fragment)
		}
	}()

	body()
}

// plannedEnvironmentAt builds a runtime environment rooted at `rootPath`, mirroring how a planning host constructs
// one (confined fsroot + application identity).
func plannedEnvironmentAt(t *testing.T, rootPath string) *op.RuntimeEnvironment {
	t.Helper()

	environment, err := op.NewRuntimeEnvironment(context.Background(), op.NewRuntimeEnvironmentSpec("test").
		WithRoot(rootPath).
		WithApplication(&application.Application{Name: "test"}))
	if err != nil {
		t.Fatalf("op.NewRuntimeEnvironment: %v", err)
	}
	t.Cleanup(func() { _ = environment.Close() })

	return environment
}

// plannedMkdir plans one file.mkdir invocation targeting `path` through [Provider.Plan].
func plannedMkdir(t *testing.T, p *Provider, path string) *op.Invocation {
	t.Helper()

	invocation, err := p.Plan(file.Mkdir, nil, map[string]any{
		"path": path,
		"mode": os.FileMode(0o755),
		"user": "", "group": "",
	})
	if err != nil {
		t.Fatalf("Plan(file.mkdir): %v", err)
	}

	return invocation
}

func TestProvider_ResolveAttr_Tier2_PromotedBuiltin(t *testing.T) {

	p := resolutionProvider(t)

	for _, name := range []string{"choose", "gather", "subgraph", "wait_until"} {
		resolved := p.ResolveAttr(name)
		if _, ok := resolved.(*starlark.Builtin); !ok {
			t.Errorf("ResolveAttr(%q) = %T, want *starlark.Builtin (Tier 2 promoted from flow)", name, resolved)
		}
	}
}

// TestProvider_ResolveAttr_Tier2_PromotedAcrossBothSurfaces pins the half of promotion nothing asserted.
//
// Placement applies to EVERY surface a provider reaches, from one declaration. flow reaches only the workflow
// surface, so its promotion has always been visible here — plan.choose, plan.gather. ui reaches BOTH, so the
// same declaration that makes note() a top-level global in a script also puts plan.note() in a workflow.
//
// That second half was documented in three doc comments and tested nowhere. It is also the half that surprised
// its own author: #708 gave ui its placement after reasoning only about the script surface, and the workflow
// consequence was found days later by reading buildPromotedBuiltins rather than by any failure.
//
// Guarding it here means a future change to placement semantics cannot quietly drop one surface.
func TestProvider_ResolveAttr_Tier2_PromotedAcrossBothSurfaces(t *testing.T) {

	p := resolutionProvider(t)

	// ui reaches both surfaces and is promoted, so all six of its methods answer under plan.* as well.
	for _, name := range []string{"error", "fail", "note", "print", "succeed", "warn"} {
		resolved := p.ResolveAttr(name)
		if _, ok := resolved.(*starlark.Builtin); !ok {
			t.Errorf("ResolveAttr(%q) = %T, want *starlark.Builtin; ui is promoted, and placement applies to "+
				"every surface it reaches — not only the script one", name, resolved)
		}
	}

	// The contrast that makes the above meaningful: a QUALIFIED provider is reached through its own name, so
	// its methods must NOT answer at the plan root. file.copy is not plan.copy.
	if resolved := p.ResolveAttr("copy"); resolved != nil {
		t.Errorf("ResolveAttr(\"copy\") = %T, want nil; file is qualified, so its methods stay under plan.file.*",
			resolved)
	}
}

func TestProvider_ResolveAttr_Tier1_SubNamespaceAdapter(t *testing.T) {

	p := resolutionProvider(t)

	resolved := p.ResolveAttr("file")
	if _, ok := resolved.(*adapter); !ok {
		t.Errorf("ResolveAttr(%q) = %T, want *adapter (Tier 1 sub-namespace)", "file", resolved)
	}
}

func TestProvider_ResolveAttr_Tier3_OwnMethod(t *testing.T) {

	// Tier 3 (plan's own methods) deliberately never reaches ResolveAttr — the bridge's goReceiver resolves them
	// against the announced receiver type first. Prove both halves: ResolveAttr declines the name, and the receiver
	// path can serve it.
	p := resolutionProvider(t)

	if resolved := p.ResolveAttr("assemble_definition"); resolved != nil {
		t.Errorf("ResolveAttr(%q) = %T, want nil (own methods ride the goReceiver path, not ResolveAttr)",
			"assemble_definition", resolved)
	}

	rt, ok := op.ReceiverRegistry().Type("plan")
	if !ok {
		t.Fatal(`registry has no "plan" receiver type (plan_announce_test.go shim missing?)`)
	}
	if _, found := rt.MethodByName("AssembleDefinition"); !found {
		t.Error(`plan receiver type lacks AssembleDefinition; the goReceiver path could not serve plan.assemble_definition`)
	}
}

func TestProvider_ResolveAttr_PromotedProviderExcludedFromTier1(t *testing.T) {

	p := resolutionProvider(t)

	if resolved := p.ResolveAttr("flow"); resolved != nil {
		t.Errorf("ResolveAttr(%q) = %T, want nil (root providers are not nested sub-namespaces)", "flow", resolved)
	}
}

func TestProvider_ResolveAttr_UnknownReturnsNil(t *testing.T) {

	p := resolutionProvider(t)

	if resolved := p.ResolveAttr("no_such_namespace_or_method"); resolved != nil {
		t.Errorf("ResolveAttr(unknown) = %T, want nil", resolved)
	}
}

func TestProvider_ResolveAttr_TierOrder(t *testing.T) {

	// Collisions across tiers panic at construction (invariant I4), so no real name can occupy two tiers. Prove the
	// precedence structurally by injecting a synthetic promoted entry over an existing Tier-1 name on this instance —
	// the promoted map must win before the adapter path is consulted.
	p := resolutionProvider(t)

	p.promotedBuiltins["file"] = starlark.None
	if resolved := p.ResolveAttr("file"); resolved != starlark.None {
		t.Errorf("ResolveAttr(%q) with injected promoted entry = %T, want the promoted value (Tier 2 wins)",
			"file", resolved)
	}
}

func TestProvider_BuildPromotedBuiltins_PanicsOnCollision_PromotedVsOwn(t *testing.T) {

	p := resolutionProvider(t)
	roots := []op.ProviderReceiverType{collisionPromotedReceiverType(t)}

	wantPanicContaining(t, "collides with plan.Provider's own method", func() {
		p.promoteMethods(map[string]struct{}{"variable": {}}, map[string]struct{}{}, roots)
	})
}

func TestProvider_BuildPromotedBuiltins_PanicsOnCollision_PromotedVsSubNamespace(t *testing.T) {

	p := resolutionProvider(t)
	roots := []op.ProviderReceiverType{collisionPromotedReceiverType(t)}

	wantPanicContaining(t, "collides with sub-namespace adapter name", func() {
		p.promoteMethods(map[string]struct{}{}, map[string]struct{}{"variable": {}}, roots)
	})
}

func TestProvider_PromoteMethods_PanicsOnDuplicateMethod(t *testing.T) {

	// Extra-matrix companion to rows 7–8: the third collision case — the same method promoted from two root
	// providers.
	p := resolutionProvider(t)
	duplicate := collisionPromotedReceiverType(t)

	wantPanicContaining(t, "collides with another root provider's method", func() {
		p.promoteMethods(map[string]struct{}{}, map[string]struct{}{},
			[]op.ProviderReceiverType{duplicate, duplicate})
	})
}

func TestAdapter_Attr_RoutesToMethod(t *testing.T) {

	p := resolutionProvider(t)

	fileReceiverType, ok := op.ReceiverRegistry().PlannerByName("file")
	if !ok {
		t.Fatal(`registry has no "file" planner (file/gen announce missing?)`)
	}

	a := p.adapterFor(fileReceiverType)

	attr, err := a.Attr("write_text")
	if err != nil || attr == nil {
		t.Fatalf(`adapter.Attr("write_text") = (%v, %v), want a builtin`, attr, err)
	}
	if _, ok := attr.(*starlark.Builtin); !ok {
		t.Errorf(`adapter.Attr("write_text") = %T, want *starlark.Builtin`, attr)
	}

	missing, err := a.Attr("no_such_method")
	if missing != nil || err == nil {
		t.Errorf("adapter.Attr(unknown) = (%v, %v), want (nil, NoSuchAttrError)", missing, err)
	}
}

func TestNewProvider_BuildsNoGraph(t *testing.T) {

	// Structural half of phase-8 D5: the Provider declares no graph-typed state.
	graphType := reflect.TypeFor[op.Graph]()
	providerType := reflect.TypeFor[Provider]()

	for i := range providerType.NumField() {
		field := providerType.Field(i)
		if field.Type == graphType || (field.Type.Kind() == reflect.Pointer && field.Type.Elem() == graphType) {
			t.Errorf("Provider field %q is graph-typed (%s); plan-time state must stay detached (phase-8 D5)",
				field.Name, field.Type)
		}
	}

	// Behavioral half: a planned invocation stays detached — nothing roots it until AssembleDefinition.
	tmp := t.TempDir()
	p := NewProvider(plannedEnvironmentAt(t, tmp))

	invocation := plannedMkdir(t, p, filepath.Join(tmp, "made"))
	if parentID := invocation.Target.ParentID(); parentID != "" {
		t.Errorf("planned invocation's Target.ParentID() = %q, want empty (detached until AssembleDefinition)",
			parentID)
	}
}

func TestAssembleDefinition_MaterializesGraphFromInvocations(t *testing.T) {

	tmp := t.TempDir()
	p := NewProvider(plannedEnvironmentAt(t, tmp))

	first := plannedMkdir(t, p, filepath.Join(tmp, "one"))
	second := plannedMkdir(t, p, filepath.Join(tmp, "two"))

	graph, err := p.AssembleDefinition([]*op.Invocation{first, second}, nil, nil, nil, nil, nil, p.Origin("test"))
	if err != nil {
		t.Fatalf("AssembleDefinition: %v", err)
	}

	children := graph.Root().Children()
	if len(children) != 2 {
		t.Fatalf("graph root has %d children, want 2 (one per invocation)", len(children))
	}

	childIDs := make(map[string]struct{}, len(children))
	for _, child := range children {
		childIDs[child.ID()] = struct{}{}
	}

	for _, invocation := range []*op.Invocation{first, second} {
		if _, ok := childIDs[invocation.Target.ID()]; !ok {
			t.Errorf("invocation %q's Target %q is not a root child; the graph must materialize from the "+
				"invocation set", invocation.Label, invocation.Target.ID())
		}
		if parentID := invocation.Target.ParentID(); parentID != graph.Root().ID() {
			t.Errorf("invocation %q's Target.ParentID() = %q, want %q (rooted by AssembleDefinition)",
				invocation.Label, parentID, graph.Root().ID())
		}
	}
}

func TestAssembleDefinition_TransfersCatalogOwnership(t *testing.T) {

	tmp := t.TempDir()
	environment := plannedEnvironmentAt(t, tmp)
	p := NewProvider(environment)

	invocation := plannedMkdir(t, p, filepath.Join(tmp, "made"))

	captured := environment.ResourceCatalog
	if captured == nil {
		t.Fatal("runtime environment has no ResourceCatalog before assembly")
	}

	graph, err := p.AssembleDefinition([]*op.Invocation{invocation}, nil, nil, nil, nil, nil, p.Origin("test"))
	if err != nil {
		t.Fatalf("AssembleDefinition: %v", err)
	}

	if environment.ResourceCatalog != nil {
		t.Error("runtime environment retains its ResourceCatalog after assembly; want nil (ownership transferred)")
	}
	if graph.ResourceCatalog() != captured {
		t.Error("graph's ResourceCatalog is not the captured planning catalog; ownership must transfer, not copy")
	}
}

func TestProvider_Spec_DefaultsFromPlanningEnvironment(t *testing.T) {

	tmp := t.TempDir()

	environment, err := op.NewRuntimeEnvironment(context.Background(), op.NewRuntimeEnvironmentSpec("planner").
		WithRoot(tmp).
		WithApplication(&application.Application{Name: "planner", Flags: map[string]any{"dry-run": true}}))
	if err != nil {
		t.Fatalf("op.NewRuntimeEnvironment: %v", err)
	}
	t.Cleanup(func() { _ = environment.Close() })
	p := NewProvider(environment)

	spec, err := p.Spec("", "", nil)
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}

	// Each zero-value argument falls back to the planning environment's corresponding field.
	if spec.ProgramName != "planner" {
		t.Errorf("spec.ProgramName = %q, want %q (defaulted from the planning environment)",
			spec.ProgramName, "planner")
	}
	if spec.Application == nil || spec.Application.Flags == nil || spec.Application.Flags["dry-run"] != true {
		t.Errorf("spec.Application.Flags = %v, want the planning environment's flags (dry-run: true)",
			spec.Application)
	}

	// The spec carries the anchor, never a handle (issue #393): it must anchor at the planning environment's root
	// path in confined mode; each Run's executor mints its own Root from it.
	if spec.RootPath != environment.Root().Name() {
		t.Fatalf("spec.RootPath = %q, want the planning environment's root path %q",
			spec.RootPath, environment.Root().Name())
	}
}

func TestProvider_Run_NilArguments_Error(t *testing.T) {

	p := resolutionProvider(t)

	if _, err := p.Run(nil, &op.RuntimeEnvironmentSpec{}); err == nil {
		t.Error("Run(nil graph) returned no error")
	}
	if _, err := p.Run(&op.Graph{}, nil); err == nil {
		t.Error("Run(nil spec) returned no error")
	}
}

func TestAssembleDefinition_OrphanInvocation_Errors(t *testing.T) {

	tmp := t.TempDir()
	p := NewProvider(plannedEnvironmentAt(t, tmp))

	attached := plannedMkdir(t, p, filepath.Join(tmp, "made"))
	orphan := plannedMkdir(t, p, filepath.Join(tmp, "stray"))

	// `orphan` is registered in the session ledger but deliberately absent from the root set — never rooted by
	// any container.
	graph, err := p.AssembleDefinition([]*op.Invocation{attached}, nil, nil, nil, nil, nil, p.Origin("test"))

	if err == nil {
		t.Fatal("AssembleDefinition with an unattached invocation returned no error; want the orphan error")
	}
	if graph != nil {
		t.Error("AssembleDefinition returned a graph alongside the orphan error; want nil")
	}
	if !strings.Contains(err.Error(), "orphan invocation") || !strings.Contains(err.Error(), orphan.Label) {
		t.Errorf("error %q does not name the orphan invocation %q", err, orphan.Label)
	}
}

// region Call-site naming (#710)

// TestResolveAttr_BuiltinNamesFollowTheCallSite pins what a workflow-surface dispatch error reports.
//
// The rule: where a call site exists, an error names what the author typed. Under plan.*, that means a
// PROMOTED provider's method is plan.choose — never flow.choose, since flow is not a symbol reachable from a
// script — and a QUALIFIED provider's method is plan.file.copy, the whole path the author wrote.
//
// Both are asserted together because a fix that promoted every name would satisfy the first alone.
//
// This governs the STARLARK-FACING name only. A graph node's action name stays provider-qualified: it is the
// node's identity field, carried in the serialized document and the checksum.
func TestResolveAttr_BuiltinNamesFollowTheCallSite(t *testing.T) {

	p := resolutionProvider(t)

	t.Run("promoted method is named for the plan root", func(t *testing.T) {

		builtin, ok := p.ResolveAttr("choose").(*starlark.Builtin)
		if !ok {
			t.Fatalf("ResolveAttr(choose) = %T, want *starlark.Builtin", p.ResolveAttr("choose"))
		}

		if got := builtin.Name(); got != "plan.choose" {
			t.Errorf("builtin name = %q, want %q; flow is not a symbol any script can reach", got, "plan.choose")
		}
	})

	t.Run("qualified method keeps its provider in the path", func(t *testing.T) {

		adapterValue, ok := p.ResolveAttr("file").(*adapter)
		if !ok {
			t.Fatalf("ResolveAttr(file) = %T, want *adapter", p.ResolveAttr("file"))
		}

		attr, err := adapterValue.Attr("copy")
		if err != nil {
			t.Fatalf("Attr(copy): %v", err)
		}

		builtin, ok := attr.(*starlark.Builtin)
		if !ok {
			t.Fatalf("Attr(copy) = %T, want *starlark.Builtin", attr)
		}

		if got := builtin.Name(); got != "plan.file.copy" {
			t.Errorf("builtin name = %q, want %q; the author typed the whole path", got, "plan.file.copy")
		}
	})
}

// TestActionNamesStayProviderQualified pins the other side of the #710 boundary.
//
// The starlark-facing builtin name follows placement; the ACTION name does not. An action name is a node's
// identity field, carried in the serialized document and therefore in the checksum — so tying it to placement
// would rewrite every saved workflow the moment a provider's placement changed.
//
// flow is promoted, so its call site is plan.choose. Its action name is still flow.choose, and the registry
// must still resolve it under that name — a saved workflow says flow.choose and has to load.
func TestActionNamesStayProviderQualified(t *testing.T) {

	for _, actionName := range []string{"flow.choose", "flow.gather", "flow.failed"} {
		if _, err := op.ReceiverRegistry().BuildAction(op.ActionName(actionName)); err != nil {
			t.Errorf("BuildAction(%q): %v; the action name is a node's serialized identity and must not "+
				"follow placement", actionName, err)
		}
	}
}

// endregion
