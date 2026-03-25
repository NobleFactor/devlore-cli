// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow_test

import (
	"os"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/flow"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/plan"
)

func TestMain(m *testing.M) {
	op.InitAll(op.NewActionRegistry(), op.Context{})
	os.Exit(m.Run())
}

// region Starlark plan integration

func TestStarlark_PlanReceiver(t *testing.T) {
	reg := op.NewActionRegistry()
	reg.Register(&flow.Complete{})
	reg.Register(&flow.Degraded{})
	reg.Register(&flow.Fatal{})
	op.RegisterReceiverParams(plan.Factory, plan.Params)

	graph := op.NewGraph("flow-integration")
	receiver := plan.Factory.(op.PlanReceiverFactory).NewExecuting(graph, "testproject", reg)

	globals := starlark.StringDict{"plan": receiver}

	thread := &starlark.Thread{
		Name:  "flow-integration",
		Print: func(_ *starlark.Thread, msg string) { t.Logf("[star] %s", msg) },
	}

	data, err := os.ReadFile("testdata/integration.star")
	if err != nil {
		t.Fatalf("reading script: %v", err)
	}

	opts := &syntax.FileOptions{Set: true, GlobalReassign: true, TopLevelControl: true}
	result, err := starlark.ExecFileOptions(opts, thread, "testdata/integration.star", data, globals)
	if err != nil {
		t.Fatalf("exec script: %v", err)
	}

	if result["result_done"] != starlark.True {
		t.Fatal("script did not complete")
	}

	// Verify graph nodes were created.
	if len(graph.Nodes) != 4 {
		t.Fatalf("graph has %d nodes, want 4 (2 complete + 1 degraded + 1 fatal)", len(graph.Nodes))
	}

	// Verify action names.
	wantActions := []string{"flow.complete", "flow.complete", "flow.degraded", "flow.fatal"}
	for i, want := range wantActions {
		if graph.Nodes[i].Action.Name() != want {
			t.Errorf("node[%d].Action.Name() = %q, want %q", i, graph.Nodes[i].Action.Name(), want)
		}
	}

	// Verify project is set on all nodes.
	for i, node := range graph.Nodes {
		if node.Project != "testproject" {
			t.Errorf("node[%d].Project = %q, want 'testproject'", i, node.Project)
		}
	}

	// Verify complete with output has the output slot.
	outputSlot := graph.Nodes[1].GetSlot("output")
	if outputSlot != "done" {
		t.Errorf("complete output slot = %v, want 'done'", outputSlot)
	}

	// Verify degraded has format slot.
	formatSlot := graph.Nodes[2].GetSlot("format")
	if formatSlot != "service %s is slow" {
		t.Errorf("degraded format slot = %v, want 'service %%s is slow'", formatSlot)
	}

	// Verify degraded has args.
	argsLen := graph.Nodes[2].GetSlot("args.len")
	if argsLen != 1 {
		t.Errorf("degraded args.len = %v, want 1", argsLen)
	}
	arg0 := graph.Nodes[2].GetSlot("args[0]")
	if arg0 != "auth" {
		t.Errorf("degraded args[0] = %v, want 'auth'", arg0)
	}

	// Verify fatal has format slot.
	fatalFormat := graph.Nodes[3].GetSlot("format")
	if fatalFormat != "disk full on %s" {
		t.Errorf("fatal format slot = %v, want 'disk full on %%s'", fatalFormat)
	}

	// Starlark results should be Promise promises.
	for _, key := range []string{"result_complete", "result_complete_out", "result_degraded", "result_fatal"} {
		v, ok := result[key]
		if !ok {
			t.Errorf("missing global %q", key)
			continue
		}
		if _, ok := v.(*op.Promise); !ok {
			t.Errorf("%s type = %T, want *op.Promise", key, v)
		}
	}
}

// endregion

// region Action dispatch (plan → execute round-trip)

func TestActions_Complete_RoundTrip(t *testing.T) {
	reg := op.NewActionRegistry()
	reg.Register(&flow.Complete{})

	graph := op.NewGraph("roundtrip")
	p := plan.NewProvider(graph, "proj", reg)

	promise, err := p.Complete("hello")
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if promise == nil {
		t.Fatal("Complete() returned nil promise")
	}

	// Execute: call Do() on the created node.
	node := graph.Nodes[0]
	slots := node.ResolvedSlots(nil)
	result, _, doErr := node.Action.Do(nil, slots)
	if doErr != nil {
		t.Fatalf("Do() error: %v", doErr)
	}
	if result != "hello" {
		t.Errorf("result = %v, want 'hello'", result)
	}
}

func TestActions_Degraded_RoundTrip(t *testing.T) {
	reg := op.NewActionRegistry()
	reg.Register(&flow.Degraded{})

	graph := op.NewGraph("roundtrip")
	p := plan.NewProvider(graph, "proj", reg)

	promise, err := p.Degraded("timeout on %s", []any{"db"}, nil)
	if err != nil {
		t.Fatalf("Degraded() error: %v", err)
	}
	if promise == nil {
		t.Fatal("Degraded() returned nil promise")
	}

	// Execute: call Do() on the created node.
	node := graph.Nodes[0]
	slots := node.ResolvedSlots(nil)
	result, _, doErr := node.Action.Do(nil, slots)
	if doErr != nil {
		t.Fatalf("Do() error: %v", doErr)
	}
	// Degraded returns the rendered message as result, nil error.
	if result == nil {
		t.Fatal("result = nil, want rendered message")
	}
}

func TestActions_Fatal_RoundTrip(t *testing.T) {
	reg := op.NewActionRegistry()
	reg.Register(&flow.Fatal{})

	graph := op.NewGraph("roundtrip")
	p := plan.NewProvider(graph, "proj", reg)

	promise, err := p.Fatal("out of memory", nil, nil)
	if err != nil {
		t.Fatalf("Fatal() error: %v", err)
	}
	if promise == nil {
		t.Fatal("Fatal() returned nil promise")
	}

	// Execute: call Do() on the created node.
	node := graph.Nodes[0]
	slots := node.ResolvedSlots(nil)
	_, _, doErr := node.Action.Do(nil, slots)
	if doErr == nil {
		t.Fatal("expected FatalError, got nil")
	}
	fatalErr, ok := doErr.(*op.FatalError)
	if !ok {
		t.Fatalf("error type = %T, want *op.FatalError", doErr)
	}
	if fatalErr.Message != "out of memory" {
		t.Errorf("message = %q, want 'out of memory'", fatalErr.Message)
	}
}

// endregion
