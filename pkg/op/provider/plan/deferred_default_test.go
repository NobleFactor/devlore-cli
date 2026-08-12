// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package plan_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/plan"

	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/flow/gen"
)

// TestPlannedDeferredDefault_ResolvesAtDispatch pins the phase-8 step-47 fix.
//
// A PLANNED invocation that omits a defaulted optional parameter dispatches successfully, with the deferred
// `{{ umask ... }}` default resolved against the live run.
//
// The planner stuffs the parsed-but-unresolved [op.DeferredDefault] into the omitted slot; before the fix,
// dispatch handed it straight to [op.Convert] ("*op.treeDefault value is neither assignable nor convertible to
// fs.FileMode"). The starlark bridge's direct-invocation path always resolved deferred defaults, and every
// existing plan-path fixture passed `chmod` explicitly — this is the first planned omission under test.
func TestPlannedDeferredDefault_ResolvesAtDispatch(t *testing.T) {

	tmp := t.TempDir()
	destination := filepath.Join(tmp, "out.txt")

	spec := func() *op.RuntimeEnvironmentSpec {
		root, err := fsroot.OpenConfined(tmp)
		if err != nil {
			t.Fatalf("fsroot.OpenConfined: %v", err)
		}
		return op.NewRuntimeEnvironmentSpec("test").
			WithRoot(root).
			WithApplication(&application.Application{Name: "test"})
	}

	graph, err := op.Plan(context.Background(), spec(), func(env *op.RuntimeEnvironment) (*op.Graph, error) {
		planProvider := plan.NewProvider(env)
		// chmod and chown deliberately omitted: chmod carries the deferred {{ umask 0o666 }} default, chown a
		// literal "".
		if _, err := planProvider.Plan(file.WriteText, nil, map[string]any{
			"destination_path": destination,
			"content":          "defaulted",
		}); err != nil {
			return nil, err
		}
		return planProvider.AssembleDefinition(
			collectInvocations(planProvider), nil, nil, nil, nil, nil, planProvider.Origin(""))
	})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	executor := op.NewGraphExecutor(graph, spec())
	if _, err := executor.Run(context.Background(), nil); err != nil {
		t.Fatalf("run: %v (deferred default not resolved at dispatch)", err)
	}

	info, err := os.Stat(destination)
	if err != nil {
		t.Fatalf("stat %s: %v", destination, err)
	}

	mask := testProcessUmask()
	want := os.FileMode(0o666) &^ mask

	if info.Mode().Perm() != want {
		t.Errorf("mode = %v, want %v (0o666 through the process umask)", info.Mode().Perm(), want)
	}
	if content, err := os.ReadFile(destination); err != nil || string(content) != "defaulted" {
		t.Errorf("content = %q (err %v), want %q", content, err, "defaulted")
	}
}

// collectInvocations drains the provider's registered, parentless invocations for assembly.
func collectInvocations(planProvider *plan.Provider) []*op.Invocation {

	var invocations []*op.Invocation
	for _, invocation := range planProvider.InvocationRegistry().All() {
		if invocation.Target.ParentID() == "" {
			invocations = append(invocations, invocation)
		}
	}
	return invocations
}
