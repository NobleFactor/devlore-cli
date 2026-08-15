// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package verify_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/deploy"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/verify"
	"github.com/NobleFactor/devlore-cli/pkg/signing"

	// Blank-import the op inventory so provider registration runs for planning and graph loading.
	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
)

// fixture deploys once in a hermetic sandbox (generating the signing key), seeds allowed_signers from the
// generated .pub, and returns the store's graph and trace paths.
func fixture(t *testing.T) (graphPath, tracePath string) {

	t.Helper()

	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("HOME", root)

	sourceRoot := filepath.Join(root, "src")
	targetRoot := filepath.Join(root, "home-target")
	if err := os.MkdirAll(filepath.Join(sourceRoot, "myproj"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "myproj", ".zshrc"), []byte("zsh"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := deploy.Execute(context.Background(), &deploy.Config{
		SourceRoot: sourceRoot,
		TargetRoot: targetRoot,
		Projects:   []string{"myproj"},
		Segments:   segment.Segments{{Name: "OS", Value: "Darwin"}},
	}); err != nil {
		t.Fatalf("deploy fixture: %v", err)
	}

	// Trust the generated key: seed allowed_signers from the .pub authorized_keys line.
	publicLine, err := os.ReadFile(filepath.Join(root, "config", "devlore", "signing", "ed25519.pub"))
	if err != nil {
		t.Fatalf("read generated .pub: %v", err)
	}
	fields := strings.Fields(string(publicLine))
	trust := "dev@example.com " + fields[0] + " " + fields[1] + "\n"
	if err := os.WriteFile(filepath.Join(root, "config", "devlore", "allowed_signers"), []byte(trust), 0o644); err != nil {
		t.Fatal(err)
	}

	graphs, err := filepath.Glob(filepath.Join(cli.GraphsDir(), "*.yaml"))
	if err != nil || len(graphs) != 1 {
		t.Fatalf("graphs = %v (err %v), want exactly one", graphs, err)
	}
	traces, err := filepath.Glob(filepath.Join(cli.TracesDir(), "*", "2*.yaml"))
	if err != nil || len(traces) != 1 {
		t.Fatalf("traces = %v (err %v), want exactly one", traces, err)
	}

	return graphs[0], traces[0]
}

// TestExecute_StoreDocumentsAreValid pins the signed store end to end: the deploy's graph and trace verify
// valid for the trusted principal, under every rejecting tier.
func TestExecute_StoreDocumentsAreValid(t *testing.T) {

	graphPath, tracePath := fixture(t)

	err := verify.Execute(context.Background(), &verify.Config{
		Paths:  []string{graphPath, tracePath},
		Policy: signing.PolicyReject,
	})
	if err != nil {
		t.Fatalf("Execute under reject over the signed store: %v", err)
	}
}

// TestExecute_TamperedExternalDocument pins invalidity + externality: an altered copy outside the store is
// rejected under reject_external and merely reported under report.
func TestExecute_TamperedExternalDocument(t *testing.T) {

	_, tracePath := fixture(t)

	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(data), "healthy", "degraded", 1)
	external := filepath.Join(t.TempDir(), "shared-trace.yaml")
	if err := os.WriteFile(external, []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}

	err = verify.Execute(context.Background(), &verify.Config{
		Paths:  []string{external},
		Policy: signing.PolicyRejectExternal,
	})
	if err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("Execute over a tampered external document = %v, want the policy rejection", err)
	}

	if err := verify.Execute(context.Background(), &verify.Config{
		Paths:  []string{external},
		Policy: signing.PolicyReport,
	}); err != nil {
		t.Errorf("Execute under report over the same document = %v, want reported-not-rejected", err)
	}
}

// TestExecute_UnsignedIsAFinding pins the unsigned outcome: a hand-written (never-signed) trace reports under
// the floor and rejects under reject.
func TestExecute_UnsignedIsAFinding(t *testing.T) {

	fixture(t)

	unsigned := filepath.Join(t.TempDir(), "unsigned-trace.yaml")
	body := "graph_checksum: sha256:0000\nrun_status:\n    phase: completed\n    condition: healthy\nstack: null\n"
	if err := os.WriteFile(unsigned, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := verify.Execute(context.Background(), &verify.Config{
		Paths:  []string{unsigned},
		Policy: signing.PolicyReport,
	}); err != nil {
		t.Errorf("unsigned under report = %v, want reported-not-rejected", err)
	}

	err := verify.Execute(context.Background(), &verify.Config{
		Paths:  []string{unsigned},
		Policy: signing.PolicyReject,
	})
	if err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Errorf("unsigned under reject = %v, want the policy rejection", err)
	}
}
