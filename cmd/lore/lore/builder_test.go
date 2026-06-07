// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package lore

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/NobleFactor/devlore-cli/internal/lorepackage"
	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/op"

	// Announce every op provider (pkg, plan, flow, …) so the registry resolves them — production wires this via
	// pkg/op/inventory, which the lore binary does not yet import (see the build note).
	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
)

// TestBuildPackage_NativePackageProducesParentedPhaseSubgraph exercises the Part B rewrite at the package level: a
// native package's phase action registers into the shared plan provider via pp.Plan, and buildPackage's parentless
// sweep groups it under a named, annotated phase subgraph whose children are stamped with its parent ID.
func TestBuildPackage_NativePackageProducesParentedPhaseSubgraph(t *testing.T) {

	registry := op.NewReceiverRegistry()
	sharedEnv := op.NewRuntimeEnvironment(context.Background(), op.NewRuntimeEnvironmentSpec("lore", registry).
		WithModules(registry.Modules()...).
		WithApplication(&application.Application{Name: "lore"}))

	provider, err := sharedProvider(sharedEnv)
	if err != nil {
		t.Fatalf("sharedProvider: %v", err)
	}

	planner := &Planner{Platform: "Linux.Debian", ActionRegistry: registry}
	release := &lorepackage.Release{Name: "git", Version: "latest", Source: lorepackage.SourceApt, NativeName: "git"}

	phases, err := planner.buildPackage(provider, sharedEnv, release, "Linux.Debian", BuildConfig{}, registry)
	if err != nil {
		t.Fatalf("buildPackage: %v", err)
	}

	if len(phases) == 0 {
		t.Fatal("buildPackage produced no phase subgraphs")
	}

	subgraph, ok := phases[0].(*op.Subgraph)
	if !ok {
		t.Fatalf("phase 0 is %T, want *op.Subgraph", phases[0])
	}

	// lore names and IDs the phase subgraph consistently and stamps its provenance annotation.
	if subgraph.Name == "" {
		t.Error("phase subgraph Name is empty")
	}
	if want := "subgraph.git." + subgraph.Name; subgraph.ID() != want {
		t.Errorf("phase subgraph ID = %q, want %q", subgraph.ID(), want)
	}
	if pkg, _ := subgraph.Annotations().Get("package"); pkg != "git" {
		t.Errorf(`phase subgraph annotation "package" = %v, want "git"`, pkg)
	}

	// The parentless sweep grouped the registered native-install node under the phase subgraph, stamping its parent.
	children := subgraph.Children()
	if len(children) == 0 {
		t.Fatal("phase subgraph has no children — the parentless sweep captured nothing")
	}
	for _, child := range children {
		if child.ParentID() != subgraph.ID() {
			t.Errorf("child %q parentID = %q, want %q", child.ID(), child.ParentID(), subgraph.ID())
		}
	}

	// After the sweep, nothing in the ledger is parentless — every node is owned by a phase subgraph.
	if leftover := parentlessTargets(provider); len(leftover) != 0 {
		t.Errorf("after build, %d parentless invocations remain, want 0", len(leftover))
	}
}

// TestLoreOrigin_RoundTripSurvivesJSON proves the lore.Origin view still projects the lore-stamped annotation keys
// after a JSON round-trip, which decodes []string as []any and map[string]string as map[string]any.
func TestLoreOrigin_RoundTripSurvivesJSON(t *testing.T) {

	original := op.NewOriginBase("lore", "git+vim", op.NewAnnotationMap(map[string]any{
		"packages": []string{"git", "vim"},
		"platform": "Linux.Debian",
		"features": []string{"rootless"},
		"settings": map[string]string{"editor": "vim"},
	}))

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded op.OriginBase
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	view := NewOrigin(decoded)

	if got := view.Packages(); len(got) != 2 || got[0] != "git" || got[1] != "vim" {
		t.Errorf("Packages() = %v, want [git vim]", got)
	}
	if got := view.Platform(); got != "Linux.Debian" {
		t.Errorf("Platform() = %q, want Linux.Debian", got)
	}
	if got := view.Features(); len(got) != 1 || got[0] != "rootless" {
		t.Errorf("Features() = %v, want [rootless]", got)
	}
	if got := view.Settings(); got["editor"] != "vim" {
		t.Errorf("Settings()[editor] = %q, want vim", got["editor"])
	}
}