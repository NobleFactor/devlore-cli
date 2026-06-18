// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package plan_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/plan"

	// Register the file and flow providers in the process-wide receiver registry, exactly as a host
	// binary (writ/lore) does via its inventory blank-imports.
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/flow/gen"
)

// TestGatherFailureUnwind_ViaPublicAPI plans a graph with a gather and runs it, using ONLY the public
// plan.Provider Go API a host (writ/lore) would use — Plan -> Assemble -> Spec -> Run. The gather's body
// writes one file per item; the last item's path is unwritable, so that iteration fails. The completed
// iterations' writes must be compensated (files removed) — the LIFO failure-unwind contract step 11 owns.
func TestGatherFailureUnwind_ViaPublicAPI(t *testing.T) {
	tmp := t.TempDir()

	root, err := fsroot.OpenConfined(tmp)
	if err != nil {
		t.Fatalf("fsroot.OpenConfined: %v", err)
	}

	env := op.NewRuntimeEnvironment(context.Background(), op.NewRuntimeEnvironmentSpec("test").
		WithRoot(root).
		WithApplication(&application.Application{Name: "test"}))

	planProvider := plan.NewProvider(env)

	// Body: write_text whose destination is the per-iteration item binding.
	itemVar := planProvider.Variable("item", nil)
	writeInv, err := planProvider.Plan("file.write_text", nil, map[string]any{
		"destination_path": itemVar,
		"content":          "x",
		"chmod":            os.FileMode(0o644),
	})
	if err != nil {
		t.Fatalf("Plan(file.write_text): %v", err)
	}

	okA := filepath.Join(tmp, "okA.txt")
	okB := filepath.Join(tmp, "okB.txt")
	// Guaranteed-unwritable: boom's parent is a regular file, not a directory.
	blocker := filepath.Join(tmp, "blocker")
	if writeErr := os.WriteFile(blocker, []byte("x"), 0o644); writeErr != nil {
		t.Fatalf("seed blocker: %v", writeErr)
	}
	boom := filepath.Join(blocker, "boom.txt")

	gatherInv, err := planProvider.Plan("flow.gather", nil, map[string]any{
		"items": []any{okA, okB, boom},
		"limit": int64(1), // serial: okA, okB complete before boom fails
		"body":  []any{writeInv},
	})
	if err != nil {
		t.Fatalf("Plan(flow.gather): %v", err)
	}

	graph, err := planProvider.Assemble([]*op.Invocation{gatherInv}, nil, nil, nil, planProvider.Origin("test"))
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	spec, err := planProvider.Spec("test", tmp, nil)
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}

	_, runErr := planProvider.Run(graph, spec)

	// Dispatch proof: the error must name the failing write, so the body ran all the way to the last
	// item. With limit=1 the iterations dispatch in order, so okA and okB completed (and created their
	// files) before boom failed — closing the "okA/okB never existed" loophole on the unwind assertion.
	if runErr == nil {
		t.Fatal("Run: expected a gather failure (unwritable item), got nil error")
	}
	if !strings.Contains(runErr.Error(), "boom.txt") {
		t.Fatalf("Run error %q does not name the failing write; the gather body may not have dispatched", runErr)
	}

	// Unwind proof: the completed iterations' writes must have been compensated (files removed) LIFO.
	exists := func(p string) bool { _, e := os.Stat(p); return e == nil }
	for _, completed := range []string{okA, okB} {
		if exists(completed) {
			t.Errorf("%s still exists after the failed gather; expected LIFO compensation to remove it",
				filepath.Base(completed))
		}
	}
}
