// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package writ_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/deploy"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"

	// Blank-import the op inventory so provider registration runs for planning and definition loading.
	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
)

// storeFixture deploys once in a hermetic sandbox (generating the signing key), seeds allowed_signers from the
// generated .pub, and returns the store's definition and trace paths. The deploy is writ's, which is why this test
// lives here and not beside the verifier in cmd/internal/cli: the cli package cannot import writ.
func storeFixture(t *testing.T) (definitionPath, tracePath string) {

	t.Helper()

	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))

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

	if _, err := deploy.Execute(context.Background(), &deploy.Config{
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

	definitions, err := filepath.Glob(filepath.Join(cli.GraphsDir(), "*.yaml"))
	if err != nil || len(definitions) != 1 {
		t.Fatalf("definitions = %v (err %v), want exactly one", definitions, err)
	}
	traces, err := filepath.Glob(filepath.Join(cli.TracesDir(), "*", "2*.yaml"))
	if err != nil || len(traces) != 1 {
		t.Fatalf("traces = %v (err %v), want exactly one", traces, err)
	}

	return definitions[0], traces[0]
}

// runWorkflow runs `writ workflow ...` through writ's root and returns stdout and the error.
func runWorkflow(t *testing.T, args ...string) (string, error) {

	t.Helper()

	root := writ.NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(new(bytes.Buffer))
	root.SetArgs(append([]string{"workflow"}, args...))
	err := root.Execute()
	return out.String(), err
}

// reportsOf decodes the JSON result of `workflow verify`.
func reportsOf(t *testing.T, out string) []cli.VerifyReport {

	t.Helper()

	var reports []cli.VerifyReport
	if err := json.Unmarshal([]byte(out), &reports); err != nil {
		t.Fatalf("result is not a JSON report list: %v\n%s", err, out)
	}
	return reports
}

// TestWorkflowVerify_ByPath pins the signed store end to end, the operand form: the deploy's definition and
// trace verify valid for the trusted principal under the rejecting tier, and each report names its kind in the
// user's vocabulary.
func TestWorkflowVerify_ByPath(t *testing.T) {

	definitionPath, tracePath := storeFixture(t)

	out, err := runWorkflow(t, "verify", "--signing-policy", "reject", definitionPath, tracePath)
	if err != nil {
		t.Fatalf("workflow verify under reject over the signed store: %v", err)
	}

	reports := reportsOf(t, out)
	if len(reports) != 2 || reports[0].Kind != "definition" || reports[1].Kind != "trace" {
		t.Errorf("reports = %+v; want a definition then a trace", reports)
	}
	for _, report := range reports {
		if report.Outcome != "valid" || report.External {
			t.Errorf("%s: outcome %s external %v; want valid, own-store", report.Path, report.Outcome, report.External)
		}
	}
}

// TestWorkflowVerify_BySelection pins the selection form: with no path given, `--tool` defaulting to writ
// resolves the deploy's definition and its trace from the index, `--kind trace --latest` narrows to the trace
// alone, and a scope with no workflow is an empty result. The fixture is a single-source deploy, which records no
// scope; a layered deploy records its scope name, and `--scope home` is the everyday form.
func TestWorkflowVerify_BySelection(t *testing.T) {

	definitionPath, tracePath := storeFixture(t)

	out, err := runWorkflow(t, "verify", "--signing-policy", "reject")
	if err != nil {
		t.Fatalf("workflow verify by the tool default: %v", err)
	}
	reports := reportsOf(t, out)
	if len(reports) != 2 || reports[0].Path != definitionPath || reports[1].Path != tracePath {
		t.Errorf("selected = %+v; want the definition then the trace, by name", reports)
	}

	out, err = runWorkflow(t, "verify", "--kind", "trace", "--latest")
	if err != nil {
		t.Fatalf("workflow verify --kind trace --latest: %v", err)
	}
	if reports := reportsOf(t, out); len(reports) != 1 || reports[0].Path != tracePath {
		t.Errorf("selected = %+v; want the trace alone", reports)
	}

	out, err = runWorkflow(t, "verify", "--scope", "elsewhere")
	if err != nil {
		t.Fatalf("workflow verify over a scope with no workflow: %v", err)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Errorf("an empty selection rendered %q; want an empty result, exit 0", out)
	}
}

// TestWorkflowList_ReadsTheIndex pins `workflow list`: the deploy's one workflow, writ's, with its one run. A
// single-source deploy records no scope.
func TestWorkflowList_ReadsTheIndex(t *testing.T) {

	storeFixture(t)

	out, err := runWorkflow(t, "list")
	if err != nil {
		t.Fatalf("workflow list: %v", err)
	}

	var records []cli.WorkflowRecord
	if err := json.Unmarshal([]byte(out), &records); err != nil {
		t.Fatalf("result is not a JSON record list: %v\n%s", err, out)
	}
	if len(records) != 1 || records[0].Tool != "writ" || records[0].Scope != "" || records[0].Runs != 1 {
		t.Errorf("records = %+v; want writ's one workflow with one run", records)
	}
}

// TestWorkflowVerify_TamperedExternalDocument pins invalidity plus externality: an altered copy outside the store
// is rejected under reject_external and merely reported under report.
func TestWorkflowVerify_TamperedExternalDocument(t *testing.T) {

	_, tracePath := storeFixture(t)

	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(data), "healthy", "degraded", 1)
	external := filepath.Join(t.TempDir(), "shared-trace.yaml")
	if err := os.WriteFile(external, []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = runWorkflow(t, "verify", "--signing-policy", "reject_external", external)
	if err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("a tampered external document = %v, want the policy rejection", err)
	}

	if _, err := runWorkflow(t, "verify", "--signing-policy", "report", external); err != nil {
		t.Errorf("the same document under report = %v, want reported-not-rejected", err)
	}
}

// TestWorkflowVerify_UnsignedIsAFinding pins the unsigned outcome: a hand-written, never-signed trace reports under
// the floor and rejects under reject.
func TestWorkflowVerify_UnsignedIsAFinding(t *testing.T) {

	storeFixture(t)

	unsigned := filepath.Join(t.TempDir(), "unsigned-trace.yaml")
	body := "graph_checksum: sha256:0000\nrun_status:\n    phase: completed\n    condition: healthy\nstack: null\n"
	if err := os.WriteFile(unsigned, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := runWorkflow(t, "verify", "--signing-policy", "report", unsigned); err != nil {
		t.Errorf("unsigned under report = %v, want reported-not-rejected", err)
	}

	_, err := runWorkflow(t, "verify", "--signing-policy", "reject", unsigned)
	if err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Errorf("unsigned under reject = %v, want the policy rejection", err)
	}
}
