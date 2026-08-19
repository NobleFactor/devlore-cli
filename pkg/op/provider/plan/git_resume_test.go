// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package plan_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/document"
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/git"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/plan"

	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/git/gen"
)

// TestGitCloneResumeThenFail_RollsBack_ViaPublicAPI is the step-44 executor-level proof.
//
// The git counterpart of TestGraphResumeThenFail_RollsBack_ViaPublicAPI: a run clones a repo, is saved and reloaded,
// and the resumed run fails at the un-run clone — compensation of the re-armed pre-pause git.Receipt removes the
// cloned tree. The receipt's
// Resource is reconstructed by git.Receipt.RestoreEncoded from the catalog rehydrated at resume, so this exercises the
// full save -> reload -> rearm -> RestoreEncoded -> rollback path for a catalog-URI-resolved receipt, in both document
// formats. Uses a local bare repo (no network) and skips when the git binary is absent.
func TestGitCloneResumeThenFail_RollsBack_ViaPublicAPI(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) { gitCloneResumeThenFail(t, format) })
	}
}

// gitCloneResumeThenFail runs the resume-then-fail rollback scenario for git.clone with the trace saved and reloaded in
// `format`.
func gitCloneResumeThenFail(t *testing.T, format string) {
	t.Helper()

	tmp := t.TempDir()

	// A local bare repo to clone from (no network); an empty bare repo clones fine.
	bare := filepath.Join(tmp, "bare.git")
	if out, err := exec.Command("git", "init", "--bare", bare).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}

	environment, err := op.NewRuntimeEnvironment(context.Background(), op.NewRuntimeEnvironmentSpec("test").
		WithRoot(tmp, fsroot.ModeConfined).
		WithApplication(&application.Application{Name: "test"}))
	if err != nil {
		t.Fatalf("op.NewRuntimeEnvironment: %v", err)
	}
	t.Cleanup(func() { _ = environment.Close() })
	planProvider := plan.NewProvider(environment)

	dirA := filepath.Join(tmp, "a")
	dirB := filepath.Join(tmp, "b")
	inv1, err := planProvider.Plan(git.Clone, nil, map[string]any{"repository": bare, "directory": dirA})
	if err != nil {
		t.Fatalf("Plan(a): %v", err)
	}
	inv2, err := planProvider.Plan(git.Clone, nil, map[string]any{"repository": bare, "directory": dirB})
	if err != nil {
		t.Fatalf("Plan(b): %v", err)
	}
	graph, err := planProvider.AssembleDefinition(
		[]*op.Invocation{inv1, inv2}, nil, nil, nil, nil, nil, planProvider.Origin("test"))
	if err != nil {
		t.Fatalf("AssembleDefinition: %v", err)
	}

	spec, err := planProvider.Spec("test", tmp, nil)
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}

	executor := op.NewGraphExecutor(graph, spec)
	hooks := op.NewHookRegistry()
	hooks.Register(&pauseAfterFirstNode{executor: executor})
	executor.SetHooks(hooks)

	if _, runErr := executor.Run(context.Background(), nil); !errors.Is(runErr, op.ErrPaused) {
		t.Fatalf("first Run: err = %v, want ErrPaused", runErr)
	}

	// One repo was cloned before the pause; the other is the un-run frontier resumed next.
	ranPath, unrunPath := dirA, dirB
	if !dirExists(dirA) {
		ranPath, unrunPath = dirB, dirA
	}
	if !dirExists(ranPath) {
		t.Fatalf("after pause: expected exactly one clone, got a=%v b=%v", dirExists(dirA), dirExists(dirB))
	}

	tracePath := filepath.Join(tmp, "trace."+format)
	if writeErr := document.Write(tracePath, executor.Trace()); writeErr != nil {
		t.Fatalf("document.Write(trace): %v", writeErr)
	}
	reloaded, err := document.ReadFile[op.Trace](tracePath)
	if err != nil {
		t.Fatalf("document.ReadFile(trace): %v", err)
	}

	// Make the un-run clone fail on resume by occupying its destination with a regular file.
	if writeErr := os.WriteFile(unrunPath, []byte("conflict"), 0o644); writeErr != nil {
		t.Fatalf("seed conflict file: %v", writeErr)
	}

	resumedSpec, err := planProvider.Spec("test", tmp, nil)
	if err != nil {
		t.Fatalf("Spec (resume): %v", err)
	}
	resumed, err := op.ResumeExecutor(graph, resumedSpec, reloaded)
	if err != nil {
		t.Fatalf("ResumeExecutor: %v", err)
	}
	if _, runErr := resumed.Run(context.Background(), nil); runErr == nil || errors.Is(runErr, op.ErrPaused) {
		t.Fatalf("resumed Run: want a failure, got %v", runErr)
	}

	// Compensation of the re-armed pre-pause git.Receipt — whose Resource git.Receipt.RestoreEncoded resolved from the
	// rehydrated catalog — must have removed the cloned tree.
	if dirExists(ranPath) {
		t.Fatalf("resume-then-fail: pre-pause clone %q was not rolled back", ranPath)
	}
}
