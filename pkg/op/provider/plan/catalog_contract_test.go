// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package plan_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/starlarkbridge"

	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/plan/gen"
)

// TestPlannedCopy_TheStoredCatalogIsInputIntent pins the catalog contract for a planned file.copy, starting
// from Starlark (4-resource-management.md §5, ruled 2026-08-20; judgment scenarios in
// docs/plans/resource-construction.md).
//
// The contract: the graph's resource catalog represents input intent — what must exist when the graph runs.
// file.copy is the exemplar because its signature spans both grammars: `source` is resource-typed (a string
// value mints a pending entry), `destination_path` is a plain string naming a product — a runtime fact with
// no plan-time presence.
//
// Asserted against the STORED document, reloaded the way a later session loads it:
//
//  1. the catalog section exists (mandatory, even when empty)
//  2. the source is present, as pending intent
//  3. the destination is ABSENT — a product, not intent
func TestPlannedCopy_TheStoredCatalogIsInputIntent(t *testing.T) {

	root := t.TempDir()
	sourcePath := filepath.Join(root, "original.txt")
	destinationPath := filepath.Join(root, "duplicate.txt")
	graphPath := filepath.Join(root, "graph.json")

	if err := os.WriteFile(sourcePath, []byte("bytes to copy"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Plan — not run — a one-node copy from Starlark, and save the graph document.
	script := fmt.Sprintf(`
node  = plan.file.copy(source = %q, destination_path = %q, mode = 0o600, user = "", group = "")
graph = plan.assemble_definition([node])
plan.save_definition(graph, %q)
`, sourcePath, destinationPath, graphPath)

	scriptPath := filepath.Join(root, "catalog.star")
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}

	environment, err := op.NewRuntimeEnvironment(context.Background(), op.NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}).
		WithRoot(root))
	if err != nil {
		t.Fatalf("op.NewRuntimeEnvironment: %v", err)
	}
	t.Cleanup(func() { _ = environment.Close() })

	if _, err := starlarkbridge.NewRuntime(environment).Invoke("catalog.star", root); err != nil {
		t.Fatalf("Invoke(catalog.star): %v", err)
	}

	data, err := os.ReadFile(graphPath)
	if err != nil {
		t.Fatalf("read saved graph: %v", err)
	}

	// The artifact itself, verbatim — logged on failure so the judgment is against the end result.
	t.Logf("SAVED GRAPH DOCUMENT (%d bytes):\n%s", len(data), data)

	loaded, err := op.LoadGraph(environment, data, "json")
	if err != nil {
		t.Fatalf("op.LoadGraph: %v", err)
	}

	rows := loaded.ResourceCatalog().IntentEntries()

	// 2 — the source: pending intent in the stored catalog.
	sourceRow, found := rowNaming(rows, filepath.Base(sourcePath))
	if !found {
		t.Errorf("stored catalog carries no entry for the source %q\n  rows: %s", sourcePath, describeRows(rows))
	} else if sourceRow.State != op.Pending {
		t.Errorf("source entry state = %v, want Pending — the stored catalog is intent", sourceRow.State)
	}

	// 3 — the destination: absent, pinning the product ruling in both directions.
	if row, present := rowNaming(rows, filepath.Base(destinationPath)); present {
		t.Errorf("stored catalog carries the destination %q — a product is a runtime fact, not intent: %+v",
			destinationPath, row)
	}
}

// rowNaming returns the first intent row whose URI names `needle`.
func rowNaming(rows []op.LedgerEntrySnapshot, needle string) (op.LedgerEntrySnapshot, bool) {

	for _, row := range rows {
		if strings.Contains(row.URI, needle) {
			return row, true
		}
	}

	return op.LedgerEntrySnapshot{}, false
}

// describeRows renders the rows for a failure message.
func describeRows(rows []op.LedgerEntrySnapshot) string {

	if len(rows) == 0 {
		return "(none)"
	}

	uris := make([]string, 0, len(rows))
	for _, row := range rows {
		uris = append(uris, row.URI)
	}

	return strings.Join(uris, "\n        ")
}
