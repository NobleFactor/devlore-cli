// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package plan_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/plan"
)

// region TEST FUNCTIONS

// TestRun_ProductionAtFreshURI_AppendsActiveWithProducer pins the production matrix's first row
// (4-resource-management.md §3) end to end on the run clone: a product at a fresh URI appends an Active
// entry carrying the producing unit's stamp, and the trace tells the story.
func TestRun_ProductionAtFreshURI_AppendsActiveWithProducer(t *testing.T) {

	root := t.TempDir()

	graph := assembleProductionGraph(t, root, func(provider *plan.Provider) []*op.Invocation {

		write, err := provider.Plan(file.WriteText, nil, map[string]any{
			"destination_path": "out.txt", "content": "produced", "mode": os.FileMode(0o600)})
		if err != nil {
			t.Fatalf("Plan(write_text): %v", err)
		}
		return []*op.Invocation{write}
	})

	executor := op.NewGraphExecutor(graph, runSpec(root))
	if _, err := executor.Run(context.Background(), nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	entries := traceEntriesFor(t, executor, "out.txt")
	if len(entries) != 1 {
		t.Fatalf("trace entries for the product = %d, want 1", len(entries))
	}
	if entries[0].State != op.Active {
		t.Errorf("product state = %v, want Active", entries[0].State)
	}
	if !strings.Contains(entries[0].ProducerID, "write_text") {
		t.Errorf("product producer = %q, want the writing unit's stamp", entries[0].ProducerID)
	}

	// Products are runtime facts on the run clone: the planning catalog gains nothing from the run —
	// the pristine pin, extended from claims to production.
	if got := graph.ResourceCatalog().Len(); got != 0 {
		t.Errorf("planning catalog length after the run = %d, want 0 (products never leak backward)", got)
	}
}

// TestRun_ProductionAtClaimedURI_ShadowsWithProducer pins the production matrix's occupied-location row:
// a product at a CLAIMED URI appends a fresh generation with the producer stamp, and the claim's
// activated generation survives as history — graph = intent, trace = the run's story.
func TestRun_ProductionAtClaimedURI_ShadowsWithProducer(t *testing.T) {

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "data.txt"), []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}

	graph := assembleProductionGraph(t, root, func(provider *plan.Provider) []*op.Invocation {

		read, err := provider.Plan(file.ReadText, nil, map[string]any{"resource": "data.txt"})
		if err != nil {
			t.Fatalf("Plan(read_text): %v", err)
		}
		write, err := provider.Plan(file.WriteText, nil, map[string]any{
			"destination_path": "data.txt", "content": read, "mode": os.FileMode(0o600)})
		if err != nil {
			t.Fatalf("Plan(write_text): %v", err)
		}
		return []*op.Invocation{read, write}
	})

	executor := op.NewGraphExecutor(graph, runSpec(root))
	if _, err := executor.Run(context.Background(), nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	entries := traceEntriesFor(t, executor, "data.txt")
	if len(entries) != 2 {
		t.Fatalf("trace entries for the claimed-and-produced URI = %d, want 2 (claim + shadow)", len(entries))
	}

	claim, shadow := entries[0], entries[1]
	if claim.ProducerID != "" || claim.State != op.Active {
		t.Errorf("claim generation = producer %q, state %v; want producerless Active (verified intent)",
			claim.ProducerID, claim.State)
	}
	if !strings.Contains(shadow.ProducerID, "write_text") || shadow.State != op.Active {
		t.Errorf("shadow generation = producer %q, state %v; want the writing unit's Active generation",
			shadow.ProducerID, shadow.State)
	}
}

// TestRun_ProductionAtGoneURI_RevivesByShadow pins the revival row: a destroyed entry stays Gone with the
// destroyer stamp, and a later production at the same URI appends a fresh Active generation.
func TestRun_ProductionAtGoneURI_RevivesByShadow(t *testing.T) {

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "data.txt"), []byte("doomed"), 0o600); err != nil {
		t.Fatal(err)
	}

	graph := assembleProductionGraph(t, root, func(provider *plan.Provider) []*op.Invocation {

		remove, err := provider.Plan(file.Remove, nil, map[string]any{
			"target": "data.txt", "prune": false, "boundary": ""})
		if err != nil {
			t.Fatalf("Plan(remove): %v", err)
		}
		write, err := provider.Plan(file.WriteText, nil, map[string]any{
			"destination_path": "data.txt", "content": "revived", "mode": os.FileMode(0o600)})
		if err != nil {
			t.Fatalf("Plan(write_text): %v", err)
		}
		return []*op.Invocation{remove, write}
	})

	executor := op.NewGraphExecutor(graph, runSpec(root))
	if _, err := executor.Run(context.Background(), nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	entries := traceEntriesFor(t, executor, "data.txt")
	if len(entries) != 2 {
		t.Fatalf("trace entries = %d, want 2 (the Gone generation + the revival)", len(entries))
	}

	gone, revived := entries[0], entries[1]
	if gone.State != op.Gone || !strings.Contains(gone.DestroyedBy, "remove") {
		t.Errorf("first generation = state %v, destroyed_by %q; want Gone with the destroyer stamp",
			gone.State, gone.DestroyedBy)
	}
	if revived.State != op.Active || !strings.Contains(revived.ProducerID, "write_text") {
		t.Errorf("revival = state %v, producer %q; want the writer's Active generation",
			revived.State, revived.ProducerID)
	}
}

// TestImmediateMode_ConstructionAndInterningSurvive pins §5.6's second carve-out (phase 4 step 6): the
// session string path stays — an immediate file operation constructs and interns its product into the
// session catalog, and session-side Convert still constructs a typed resource from a path string (the
// graph-dispatch refusal never fires outside a graph).
func TestImmediateMode_ConstructionAndInterningSurvive(t *testing.T) {

	root := t.TempDir()

	environment, err := op.NewRuntimeEnvironment(context.Background(), op.NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}).
		WithRoot(root))
	if err != nil {
		t.Fatalf("op.NewRuntimeEnvironment: %v", err)
	}
	t.Cleanup(func() { _ = environment.Close() })

	fileProvider := file.NewProvider(environment)

	product, _, err := fileProvider.WriteText(
		&op.ActivationRecord{RuntimeEnvironment: environment, Context: environment.Context},
		"session.txt", "immediate", 0o600, "", "")
	if err != nil {
		t.Fatalf("immediate WriteText: %v", err)
	}
	if id := environment.ResourceCatalog.Current(product.URI()); id == "" {
		t.Error("the immediate product was not interned into the session catalog")
	}

	converted, err := op.Convert(environment, "another.txt", reflect.TypeFor[file.Regular]())
	if err != nil {
		t.Fatalf("session Convert(string → file.Regular): %v — session construction must survive", err)
	}
	regular, ok := converted.(file.Regular)
	if !ok {
		t.Fatalf("session Convert returned %T, want file.Regular", converted)
	}
	if id := environment.ResourceCatalog.Current(regular.URI()); id == "" {
		t.Error("the session-constructed resource was not interned as a claim")
	}
}

// endregion

// region HELPER FUNCTIONS

// assembleProductionGraph plans and assembles a graph under a session environment rooted at `root`,
// with the invocations `build` returns.
func assembleProductionGraph(
	t *testing.T, root string, build func(provider *plan.Provider) []*op.Invocation,
) *op.Graph {

	t.Helper()

	environment, err := op.NewRuntimeEnvironment(context.Background(), op.NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}).
		WithRoot(root))
	if err != nil {
		t.Fatalf("op.NewRuntimeEnvironment: %v", err)
	}
	t.Cleanup(func() { _ = environment.Close() })

	provider := plan.NewProvider(environment)

	graph, err := provider.AssembleDefinition(build(provider), nil, nil, nil, nil, nil, provider.Origin("test"))
	if err != nil {
		t.Fatalf("AssembleDefinition: %v", err)
	}

	return graph
}

// traceEntriesFor returns the trace ledger entries whose URI contains `fragment`, in ledger append order.
func traceEntriesFor(t *testing.T, executor *op.GraphExecutor, fragment string) []op.LedgerEntrySnapshot {

	t.Helper()

	trace := executor.Trace()
	if trace == nil || trace.Catalog == nil {
		t.Fatal("trace carries no ledger snapshot")
	}

	var matched []op.LedgerEntrySnapshot
	for _, entry := range trace.Catalog.Entries {
		if strings.Contains(entry.URI, fragment) {
			matched = append(matched, entry)
		}
	}
	return matched
}

// endregion
