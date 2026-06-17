// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/starlarkbridge"
	"go.starlark.net/starlark"

	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/flow/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/plan/gen"
)

// resultFlowScript plans a subgraph-of-complete entirely in Starlark and runs it in-script.
//
// `plan.subgraph(body=[plan.complete(output=...)])` materializes through SubgraphPlanner into a
// subgraph BOUND to the flow.subgraph action (planners.go:279) wrapping a flow.complete leaf. The
// run result — the leaf's output — must flow out of plan.run and into the `result` global. Reading
// `result` back as a starlark.String is the Starlark-side mirror of the Go API guard.
const resultFlowScript = `
leaf     = plan.complete(output = "` + sentinelOutput + `")
body     = plan.subgraph(body = [leaf])
graph    = plan.assemble([body])
result   = plan.run(graph, plan.spec())
`

// TestSubgraphBoundAction_FlowsLeafResult_Starlark proves the same result-flow as the Go API guard,
// but planned and executed through the Starlark bridge.
//
// The whole pipeline — plan.subgraph (flow.subgraph-bound) wrapping plan.complete, plan.assemble,
// plan.run — runs inside the `.star` script; only the final scalar crosses back to Go. The script's
// `result` global must equal [sentinelOutput], proving the bug fix holds through the bridge planner.
//
// Reachable without the op inventory: it blank-imports only flow/gen and plan/gen (both build clean),
// and plan.Provider discovers flow's root-planned methods via the receiver registry.
func TestSubgraphBoundAction_FlowsLeafResult_Starlark(t *testing.T) {

	root := t.TempDir()

	scriptPath := filepath.Join(root, "result_flow.star")
	if err := os.WriteFile(scriptPath, []byte(resultFlowScript), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}

	confinedRoot, err := fsroot.OpenConfined(root)
	if err != nil {
		t.Fatalf("fsroot.OpenConfined: %v", err)
	}

	spec := op.NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}).
		WithRoot(confinedRoot)

	environment := op.NewRuntimeEnvironment(context.Background(), spec)
	t.Cleanup(func() { _ = environment.Close() })

	runtime := starlarkbridge.NewRuntime(environment)

	globals, err := runtime.Invoke("result_flow.star", root)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	result, ok := globals["result"]
	if !ok {
		t.Fatal("script did not bind a `result` global")
	}

	got, ok := result.(starlark.String)
	if !ok {
		t.Fatalf("result global = %T (%v), want starlark.String "+
			"(flow.subgraph dropped its child's terminal result)", result, result)
	}

	if string(got) != sentinelOutput {
		t.Errorf("result = %q, want %q", string(got), sentinelOutput)
	}
}
