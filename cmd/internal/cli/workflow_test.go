// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// indexFixture is a run index with two programs, two scopes, and a definition run three times, so the fold and
// the selection have something to disagree about.
func indexFixture() []IndexEntry {

	at := func(hour int) time.Time { return time.Date(2026, 9, 11, hour, 0, 0, 0, time.UTC) }

	return []IndexEntry{
		{At: at(1), Event: IndexEventGraph, Tool: "writ", Scope: "home", GraphChecksum: "sha256:aaaa"},
		{At: at(2), Event: IndexEventTrace, GraphChecksum: "sha256:aaaa", TraceFile: "t1.yaml"},
		{At: at(3), Event: IndexEventGraph, Tool: "writ", Scope: "system", GraphChecksum: "sha256:bbbb"},
		{At: at(4), Event: IndexEventTrace, GraphChecksum: "sha256:aaaa", TraceFile: "t2.yaml"},
		{At: at(5), Event: IndexEventGraph, Tool: "lore", Scope: "", GraphChecksum: "sha256:cccc"},
		{At: at(6), Event: IndexEventTrace, GraphChecksum: "sha256:cccc", TraceFile: "t3.yaml"},
		{At: at(7), Event: IndexEventTrace, GraphChecksum: "sha256:aaaa", TraceFile: "t3.yaml"},
		{At: at(8), Event: IndexEventTrace, GraphChecksum: "sha256:zzzz", TraceFile: "orphan.yaml"},
	}
}

// TestListWorkflows_FoldsTheIndex pins `workflow list`: one record per definition, runs counted, the newest run
// kept, a trace with no definition ignored, and the sort by tool, scope, then latest run.
func TestListWorkflows_FoldsTheIndex(t *testing.T) {

	records := listWorkflows(indexFixture(), "")

	at := func(hour int) time.Time { return time.Date(2026, 9, 11, hour, 0, 0, 0, time.UTC) }
	want := []WorkflowRecord{
		{Tool: "lore", Scope: "", Definition: "sha256:cccc", Runs: 1, LatestRun: at(6)},
		{Tool: "writ", Scope: "home", Definition: "sha256:aaaa", Runs: 3, LatestRun: at(7)},
		{Tool: "writ", Scope: "system", Definition: "sha256:bbbb", Runs: 0},
	}
	if !reflect.DeepEqual(records, want) {
		t.Errorf("listWorkflows =\n%+v\nwant\n%+v", records, want)
	}
}

// TestListWorkflows_ToolSelects pins the selector: `--tool` keeps one program's workflows, and a program with none
// yields an empty result rather than nil (§8, S8).
func TestListWorkflows_ToolSelects(t *testing.T) {

	if got := listWorkflows(indexFixture(), "writ"); len(got) != 2 || got[0].Tool != "writ" || got[1].Tool != "writ" {
		t.Errorf("--tool writ = %+v; want writ's two", got)
	}
	if got := listWorkflows(indexFixture(), "star"); got == nil || len(got) != 0 {
		t.Errorf("--tool star = %#v; want an empty, non-nil result", got)
	}
}

// TestSelectDocuments_ResolvesByName pins `workflow verify`'s selection: a scope's definition and every trace, the
// kinds narrowed by --kind, the newest trace alone under --latest, and paths under the store's directories.
func TestSelectDocuments_ResolvesByName(t *testing.T) {

	t.Setenv("XDG_STATE_HOME", t.TempDir())
	definition := filepath.Join(GraphsDir(), "sha256-aaaa.yaml")
	trace := func(name string) string { return filepath.Join(TracesDir(), "sha256-aaaa", name) }

	cases := []struct {
		name      string
		selection workflowSelection
		want      []string
	}{
		{"a scope: definition then every trace", workflowSelection{tool: "writ", scope: "home"},
			[]string{definition, trace("t1.yaml"), trace("t2.yaml"), trace("t3.yaml")}},
		{"kind definition", workflowSelection{tool: "writ", scope: "home", kind: "definition"},
			[]string{definition}},
		{"kind trace, latest", workflowSelection{tool: "writ", scope: "home", kind: "trace", latest: true},
			[]string{trace("t3.yaml")}},
		{"every scope of a tool", workflowSelection{tool: "writ", kind: "definition"},
			[]string{definition, filepath.Join(GraphsDir(), "sha256-bbbb.yaml")}},
		{"nothing matches: an empty result, never nil", workflowSelection{tool: "star"}, []string{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := selectDocuments(indexFixture(), c.selection); !reflect.DeepEqual(got, c.want) {
				t.Errorf("selectDocuments = %v; want %v", got, c.want)
			}
		})
	}
}

// TestVerifyPaths_OperandsAndSelectorsDoNotMix pins the refusal: a path operand beside a selector is refused
// naming both, and an unknown --kind is refused naming the two it accepts.
func TestVerifyPaths_OperandsAndSelectorsDoNotMix(t *testing.T) {

	leaf := newWorkflowVerifyCmd("writ")
	if err := leaf.Flags().Set("scope", "home"); err != nil {
		t.Fatal(err)
	}

	_, err := verifyPaths(leaf, []string{"shared.yaml"})
	if err == nil || !strings.Contains(err.Error(), "--scope") || !strings.Contains(err.Error(), "1 document") {
		t.Errorf("mixing = %v; want a refusal naming --scope and the operand", err)
	}

	paths, err := verifyPaths(newWorkflowVerifyCmd("writ"), []string{"a.yaml", "b.yaml"})
	if err != nil || !reflect.DeepEqual(paths, []string{"a.yaml", "b.yaml"}) {
		t.Errorf("operands alone = %v, %v; want them as given", paths, err)
	}

	bad := newWorkflowVerifyCmd("writ")
	if err := bad.Flags().Set("kind", "graph"); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyPaths(bad, nil); err == nil || !strings.Contains(err.Error(), "definition or trace") {
		t.Errorf("--kind graph = %v; want a refusal naming definition or trace", err)
	}
}

// TestNewWorkflowCmd_ToolDefaultsToTheProgram pins §6's rule: `writ workflow list` lists writ's, because --tool
// defaults to the invoking program on both leaves; and the group itself takes no action (§3).
func TestNewWorkflowCmd_ToolDefaultsToTheProgram(t *testing.T) {

	root := NewRootCmd(RootConfig{Name: "probe"})

	var group *cobra.Command
	for _, c := range root.Commands() {
		if c.Name() == "workflow" {
			group = c
		}
	}
	if group == nil {
		t.Fatal("the shared root has no workflow group")
	}
	if group.RunE != nil || group.Run != nil {
		t.Error("the workflow group takes an action; a group prints help")
	}

	for _, name := range []string{"list", "verify"} {
		leaf, _, err := group.Find([]string{name})
		if err != nil || leaf == nil || leaf.Name() != name {
			t.Fatalf("workflow %s: %v", name, err)
		}
		if flag := leaf.Flags().Lookup("tool"); flag == nil || flag.DefValue != "probe" {
			t.Errorf("workflow %s --tool defaults to %v; want the program's name", name, flag)
		}
	}
}
