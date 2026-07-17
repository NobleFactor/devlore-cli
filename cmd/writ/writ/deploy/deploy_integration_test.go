// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package deploy_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/deploy"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"
	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/pkg/op"

	// Blank-import the op inventory so every provider's gen package init() runs and registers its
	// ProviderReceiverType; the plan provider resolves actions through the receiver registry.
	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
)

// fixture builds a single-source deploy layout: one plain file (links) and one template (renders), plus the
// state home redirect that keeps the store inside the test sandbox.
func fixture(t *testing.T) (cfg *deploy.Config, sourceRoot, targetRoot string) {

	t.Helper()

	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))

	sourceRoot = filepath.Join(root, "src")
	targetRoot = filepath.Join(root, "home")

	if err := os.MkdirAll(filepath.Join(sourceRoot, "myproj"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(sourceRoot, "myproj", ".zshrc"), []byte("plain zsh"), 0o644); err != nil {
		t.Fatal(err)
	}
	template := []byte("os={{ .Segments.OS }}")
	if err := os.WriteFile(filepath.Join(sourceRoot, "myproj", ".gitconfig.template"), template, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg = &deploy.Config{
		SourceRoot: sourceRoot,
		TargetRoot: targetRoot,
		Projects:   []string{"myproj"},
		Segments: segment.Segments{
			{Name: "OS", Value: "Darwin"},
			{Name: "ARCH", Value: "arm64"},
		},
	}

	return cfg, sourceRoot, targetRoot
}

// TestExecute_SingleSource_LinkAndTemplate deploys a plain file and a template end to end: the plain file
// links, the template renders with segment data, and the store gains one graph, one trace, and index lines.
func TestExecute_SingleSource_LinkAndTemplate(t *testing.T) {

	cfg, sourceRoot, targetRoot := fixture(t)

	if err := deploy.Execute(context.Background(), cfg); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// The plain file linked (the provider may emit a relative symlink; resolve both sides).
	linkPath := filepath.Join(targetRoot, ".zshrc")
	if info, err := os.Lstat(linkPath); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf(".zshrc is not a symlink (err %v)", err)
	}
	resolved, err := filepath.EvalSymlinks(linkPath)
	if err != nil {
		t.Fatalf("resolve .zshrc link: %v", err)
	}
	wantSource, err := filepath.EvalSymlinks(filepath.Join(sourceRoot, "myproj", ".zshrc"))
	if err != nil {
		t.Fatal(err)
	}
	if resolved != wantSource {
		t.Errorf("link resolves to %q, want %q", resolved, wantSource)
	}

	// The template rendered with segment data.
	rendered, err := os.ReadFile(filepath.Join(targetRoot, ".gitconfig"))
	if err != nil {
		t.Fatalf("read rendered .gitconfig: %v", err)
	}
	if got := string(rendered); got != "os=Darwin" {
		t.Errorf("rendered content = %q, want %q", got, "os=Darwin")
	}

	// The store holds the plan, the trace, and the index lines.
	graphs, err := filepath.Glob(filepath.Join(cli.GraphsDir(), "*.yaml"))
	if err != nil || len(graphs) != 1 {
		t.Errorf("graphs = %v (err %v), want exactly one", graphs, err)
	}
	traces, err := filepath.Glob(filepath.Join(cli.ReceiptsDir(), "*", "2*.yaml"))
	if err != nil || len(traces) != 1 {
		t.Errorf("traces = %v (err %v), want exactly one", traces, err)
	}
	entries, err := cli.ReadIndex()
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}
	var graphEvents, traceEvents int
	for _, e := range entries {
		switch e.Event {
		case cli.IndexEventGraph:
			graphEvents++
			if e.Tool != "writ" {
				t.Errorf("graph event tool = %q, want writ", e.Tool)
			}
		case cli.IndexEventTrace:
			traceEvents++
		}
	}
	if graphEvents != 1 || traceEvents != 1 {
		t.Errorf("index events = %d graph / %d trace, want 1 / 1", graphEvents, traceEvents)
	}
}

// TestExecute_DryRun_TouchesNothing pins dry-run: no target mutations, no store writes.
func TestExecute_DryRun_TouchesNothing(t *testing.T) {

	cfg, _, targetRoot := fixture(t)
	cfg.DryRun = true

	if err := deploy.Execute(context.Background(), cfg); err != nil {
		t.Fatalf("Execute --dry-run: %v", err)
	}

	targetEntries, err := os.ReadDir(targetRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(targetEntries) != 0 {
		t.Errorf("target gained entries under dry-run: %v", targetEntries)
	}

	if _, err := os.Stat(cli.IndexPath()); !os.IsNotExist(err) {
		t.Errorf("run index exists after dry-run (err %v)", err)
	}
}

// TestExecute_ForeignOccupantRefusedByDefault pins the step-49 layered enforcement: under the default stop
// policy, an occupied target the readback does not recognize as writ's own refuses the deploy — listing the
// file and naming the flag — and nothing on disk changes.
func TestExecute_ForeignOccupantRefusedByDefault(t *testing.T) {

	cfg, _, targetRoot := fixture(t)

	squatter := filepath.Join(targetRoot, ".zshrc")
	if err := os.WriteFile(squatter, []byte("squatter"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := deploy.Execute(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), squatter) || !strings.Contains(err.Error(), "--conflict") {
		t.Fatalf("Execute over a foreign occupant = %v, want the refusal listing the file and the flag", err)
	}

	content, readErr := os.ReadFile(squatter)
	if readErr != nil || string(content) != "squatter" {
		t.Errorf("foreign occupant disturbed by a refused deploy: %q (err %v)", content, readErr)
	}
	if _, statErr := os.Lstat(filepath.Join(targetRoot, ".gitconfig")); !os.IsNotExist(statErr) {
		t.Errorf("a refused deploy still wrote other targets (err %v)", statErr)
	}
}

// TestExecute_ConflictReplaceArchivesForeignOccupant pins --conflict=replace: the occupant is archived and the
// symlink lands; the run succeeds.
func TestExecute_ConflictReplaceArchivesForeignOccupant(t *testing.T) {

	cfg, _, targetRoot := fixture(t)
	cfg.Conflict = op.ConflictReplace

	if err := os.WriteFile(filepath.Join(targetRoot, ".zshrc"), []byte("squatter"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := deploy.Execute(context.Background(), cfg); err != nil {
		t.Fatalf("Execute --conflict=replace: %v", err)
	}

	info, err := os.Lstat(filepath.Join(targetRoot, ".zshrc"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("occupied target was not replaced by the symlink under replace")
	}
}

// TestExecute_ConflictSkipLeavesForeignOccupant pins --conflict=skip: the occupant survives, the run succeeds,
// and the unoccupied targets still deploy.
func TestExecute_ConflictSkipLeavesForeignOccupant(t *testing.T) {

	cfg, _, targetRoot := fixture(t)
	cfg.Conflict = op.ConflictSkip

	squatter := filepath.Join(targetRoot, ".zshrc")
	if err := os.WriteFile(squatter, []byte("squatter"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := deploy.Execute(context.Background(), cfg); err != nil {
		t.Fatalf("Execute --conflict=skip: %v", err)
	}

	content, err := os.ReadFile(squatter)
	if err != nil || string(content) != "squatter" {
		t.Errorf("skipped occupant disturbed: %q (err %v)", content, err)
	}
	rendered, err := os.ReadFile(filepath.Join(targetRoot, ".gitconfig"))
	if err != nil || string(rendered) != "os=Darwin" {
		t.Errorf("unoccupied target did not deploy under skip: %q (err %v)", rendered, err)
	}
}

// TestExecute_RedeployFlowsUnderDefault pins the layered-enforcement point: a redeploy over writ's own
// unmodified outputs proceeds under the DEFAULT policy — the pre-flight recognizes the link and the recorded
// digest and clears the run — and the changed template re-renders.
func TestExecute_RedeployFlowsUnderDefault(t *testing.T) {

	cfg, sourceRoot, targetRoot := fixture(t)

	if err := deploy.Execute(context.Background(), cfg); err != nil {
		t.Fatalf("first deploy: %v", err)
	}

	template := filepath.Join(sourceRoot, "myproj", ".gitconfig.template")
	if err := os.WriteFile(template, []byte("os={{ .Segments.OS }} v2"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := deploy.Execute(context.Background(), cfg); err != nil {
		t.Fatalf("redeploy under the default policy: %v", err)
	}

	rendered, err := os.ReadFile(filepath.Join(targetRoot, ".gitconfig"))
	if err != nil || string(rendered) != "os=Darwin v2" {
		t.Errorf("redeploy did not refresh the render: %q (err %v)", rendered, err)
	}
}
