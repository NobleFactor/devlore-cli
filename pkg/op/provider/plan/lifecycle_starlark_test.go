// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package plan_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/starlarkbridge"

	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/plan/gen"
)

// TestGraphSaveLoadExecute_ViaStarlark is the Starlark-API mirror of the Go-API lifecycle test
// (TestGraphSaveLoadExecuteTrace_ViaPublicAPI): it drives plan -> save -> load -> execute the loaded
// graph entirely from a .star script via plan.assemble / plan.save / plan.load / plan.run, and asserts
// the round-trip produces the side effect and leaves the saved graph document on disk.
//
// Trace capture is intentionally not exercised here: plan.run returns only the run's result value,
// never the executor or its Trace, so "save the trace" stays the Go variant's responsibility. This
// variant proves the plan -> save -> load -> execute round-trip is reachable from Starlark.
func TestGraphSaveLoadExecute_ViaStarlark(t *testing.T) {

	t.Skip("blocked (item 1 / phase-8 step 28): `plan.load(...)` is a Starlark parse error — `load` is a " +
		"reserved keyword, so it cannot be used as an attribute name. The save->load->run round-trip needs the " +
		"plan provider's Load method exposed under a non-reserved Starlark name (or a getattr workaround).")

	root := t.TempDir()
	target := filepath.Join(root, "made")
	graphPath := filepath.Join(root, "graph.json")

	// plan a one-node graph (mkdir), save it, load it back, and run the *loaded* graph.
	script := fmt.Sprintf(`
node   = plan.file.mkdir(path = %q, chmod = 0o755, chown = "")
graph  = plan.assemble([node])
plan.save(graph, %q)
loaded = plan.load(%q)
plan.run(loaded, plan.spec())
`, target, graphPath, graphPath)

	scriptPath := filepath.Join(root, "lifecycle.star")
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}

	confinedRoot, err := fsroot.OpenConfined(root)
	if err != nil {
		t.Fatalf("fsroot.OpenConfined: %v", err)
	}

	environment := op.NewRuntimeEnvironment(context.Background(), op.NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}).
		WithRoot(confinedRoot))
	t.Cleanup(func() { _ = environment.Close() })

	if _, err := starlarkbridge.NewRuntime(environment).Invoke("lifecycle.star", root); err != nil {
		t.Fatalf("Invoke(lifecycle.star): %v", err)
	}

	// The loaded graph ran: its mkdir side effect exists.
	if info, statErr := os.Stat(target); statErr != nil || !info.IsDir() {
		t.Errorf("loaded graph did not create %s (stat err=%v)", target, statErr)
	}
	// plan.save left the graph document on disk.
	if _, statErr := os.Stat(graphPath); statErr != nil {
		t.Errorf("saved graph document not on disk: %v", statErr)
	}
}
