// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/pkg/signing"
)

// region SUPPORTING TYPES

// WorkflowRecord is one workflow as the store knows it: the program and scope that planned it, its definition, and
// the runs recorded against that definition.
type WorkflowRecord struct {

	// Tool is the producing program's name, from the definition's origin.
	Tool string `json:"tool"`

	// Scope is the planning scope, from the definition's origin.
	Scope string `json:"scope"`

	// Definition is the definition's canonical "sha256:<hex>" checksum, the key every run ties back to.
	Definition string `json:"definition"`

	// Runs is the number of execution traces recorded against the definition.
	Runs int `json:"runs"`

	// LatestRun is the newest execution trace's time; zero when no run is recorded.
	LatestRun time.Time `json:"latest_run"`
}

// workflowSelection is what `workflow verify` selects from the index: the workflows, and which of their documents.
type workflowSelection struct {
	tool   string // the producing program; "" selects every program
	scope  string // the planning scope; "" selects every scope
	kind   string // "definition", "trace", or "" for both
	latest bool   // keep only the newest trace per definition
}

// endregion

// region EXPORTED FUNCTIONS

// NewWorkflowCmd builds the shared `workflow` group: the store's documents addressed by name
// (10-command-line-interface.md §6, ruled 2026-09-11). A workflow has a definition and the execution traces of its
// runs; the run index is the one place a person's name for either exists, so `list` reads it and `verify` selects
// from it. The group takes no action of its own (§3).
//
// Parameters:
//   - `programName`: the invoking program's name; the default for `--tool`, so `writ workflow list` lists writ's.
//
// Returns:
//   - `*cobra.Command`: the group with its `list` and `verify` leaves.
func NewWorkflowCmd(programName string) *cobra.Command {

	group := &cobra.Command{
		Use:   "workflow",
		Short: "The store's workflows: each a definition and the execution traces of its runs",
		Long: `The store's workflows: each a definition and the execution traces of its runs.

A document has one name on disk, its checksum. The run index carries the name a person
uses -- which program planned it, for which scope, and when it ran -- so these commands
address the store through the index. Selectors default to the invoking program;
--tool asks for another program's workflows.`,
	}

	group.AddCommand(newWorkflowListCmd(programName))
	group.AddCommand(newWorkflowVerifyCmd(programName))

	return group
}

// endregion

// region HELPER FUNCTIONS

// newWorkflowListCmd builds `workflow list`: one record per workflow in the index.
//
// Parameters:
//   - `programName`: the default for `--tool`.
//
// Returns:
//   - `*cobra.Command`: the leaf.
func newWorkflowListCmd(programName string) *cobra.Command {

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the store's workflows: tool, scope, definition, run count, latest run",
		Long: `List the store's workflows from the run index: one record per workflow, with the
program that planned it, the scope, the definition's checksum, how many runs are
recorded against it, and when the newest ran.`,
		Example: `  writ workflow list
  writ workflow list -o table
  writ workflow list --tool lore
  writ workflow list --jq '.[] | select(.scope == "home") | .definition' -o value`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {

			tool, _ := cmd.Flags().GetString("tool") //nolint:errcheck // flag registered below

			entries, err := ReadIndex()
			if err != nil {
				return err
			}

			return Emit(cmd, listWorkflows(entries, tool))
		},
	}

	cmd.Flags().String("tool", programName, "The program whose workflows to list")

	return cmd
}

// newWorkflowVerifyCmd builds `workflow verify`: publisher-signature verification of the selected documents.
//
// Parameters:
//   - `programName`: the default for `--tool`.
//
// Returns:
//   - `*cobra.Command`: the leaf.
func newWorkflowVerifyCmd(programName string) *cobra.Command {

	cmd := &cobra.Command{
		Use:   "verify [<document>...]",
		Short: "Verify the publisher signatures of workflow definitions and execution traces",
		Long: `Verify the publisher signatures of workflow definitions and execution traces.

With no operand, the documents are selected from the run index: every definition and
execution trace of the selected workflows. --scope narrows to one scope, --kind to one
kind of document, --latest to the newest trace per definition. A path operand names a
document from outside the store, such as one shared with you; operands and selectors
do not mix in one invocation.

Each document is re-canonicalized and its raw ssh-ed25519 signature checked over the
namespace-prefixed canonical bytes; the publisher key resolves against the verifier's
allowed_signers trust list. What each outcome does to the exit status is the signing
policy ladder:

  ignore           No verification at all
  report           (default) Report every outcome; never fail
  reject_external  Reject unsigned/invalid/untrusted documents from OUTSIDE this
                   machine's own store; own-store documents only report
  reject           Reject anything that is not valid`,
		Example: `  writ workflow verify
  writ workflow verify --scope home
  writ workflow verify --scope home --kind trace --latest
  writ workflow verify -o table --signing-policy=reject
  writ workflow verify --signing-policy=reject_external ~/Downloads/shared-plan.yaml`,
		RunE: func(cmd *cobra.Command, args []string) error {

			policyValue, _ := cmd.Flags().GetString("signing-policy") //nolint:errcheck // flag registered below
			policy, err := signing.ParsePolicy(policyValue)
			if err != nil {
				return err
			}
			allowedSigners, _ := cmd.Flags().GetString("allowed-signers") //nolint:errcheck // flag registered below

			paths, err := verifyPaths(cmd, args)
			if err != nil {
				return err
			}

			reports, err := VerifyDocuments(cmd.Context(), &VerifyConfig{
				Paths:          paths,
				Policy:         policy,
				AllowedSigners: allowedSigners,
			})

			// The reports are the result and are emitted whether or not the policy rejected: a rejection is the
			// answer to the question, not a reason to withhold it.
			if emitErr := Emit(cmd, reports); emitErr != nil {
				return emitErr
			}

			return err
		},
	}

	cmd.Flags().String("tool", programName, "The program whose workflows to verify")
	cmd.Flags().String("scope", "", "Verify one scope's workflow (default: every scope)")
	cmd.Flags().String("kind", "", "Verify one kind of document: definition or trace (default: both)")
	cmd.Flags().Bool("latest", false, "Verify only the newest execution trace of each definition")
	cmd.Flags().String("signing-policy", "report", "Verification policy: ignore, report, reject_external, reject")
	cmd.Flags().String("allowed-signers", "", "Trust-list path (default: <config>/devlore/allowed_signers)")

	return cmd
}

// verifyPaths resolves what `workflow verify` verifies: the operands as given, or the documents the selectors
// pick from the index. Operands and selectors together are refused, naming both.
//
// Parameters:
//   - `cmd`: the leaf, for its flags.
//   - `args`: the operands.
//
// Returns:
//   - `[]string`: the document paths, in a deterministic order.
//   - `error`: operands and selectors in one invocation, an unknown `--kind`, or an unreadable index.
func verifyPaths(cmd *cobra.Command, args []string) ([]string, error) {

	var selectors []string
	for _, name := range []string{"tool", "scope", "kind", "latest"} {
		if cmd.Flags().Changed(name) {
			selectors = append(selectors, "--"+name)
		}
	}

	if len(args) > 0 {
		if len(selectors) > 0 {
			return nil, fmt.Errorf("operands and selectors do not mix: %s and %d document operand(s); "+
				"name documents by path, or select them from the store, not both",
				strings.Join(selectors, ", "), len(args))
		}
		return args, nil
	}

	kind, _ := cmd.Flags().GetString("kind") //nolint:errcheck // flag registered above
	switch kind {
	case "", "definition", "trace":
	default:
		return nil, fmt.Errorf("--kind %q: want definition or trace", kind)
	}

	tool, _ := cmd.Flags().GetString("tool")   //nolint:errcheck // flag registered above
	scope, _ := cmd.Flags().GetString("scope") //nolint:errcheck // flag registered above
	latest, _ := cmd.Flags().GetBool("latest") //nolint:errcheck // flag registered above

	entries, err := ReadIndex()
	if err != nil {
		return nil, err
	}

	return selectDocuments(entries, workflowSelection{tool: tool, scope: scope, kind: kind, latest: latest}), nil
}

// listWorkflows folds the run index into one record per workflow.
//
// A definition event names the workflow (tool, scope, checksum); each trace event against the same checksum is
// one run. Records sort by tool, then scope, then latest run descending, so the newest of a scope reads first.
//
// Parameters:
//   - `entries`: the run index.
//   - `tool`: the program whose workflows to keep; "" keeps every program's.
//
// Returns:
//   - `[]WorkflowRecord`: the records; empty, never nil, when nothing matches.
func listWorkflows(entries []IndexEntry, tool string) []WorkflowRecord {

	byDefinition := make(map[string]*WorkflowRecord)
	var order []string

	for _, entry := range entries {
		switch entry.Event {
		case IndexEventGraph:
			if _, seen := byDefinition[entry.GraphChecksum]; !seen {
				byDefinition[entry.GraphChecksum] = &WorkflowRecord{
					Tool: entry.Tool, Scope: entry.Scope, Definition: entry.GraphChecksum,
				}
				order = append(order, entry.GraphChecksum)
			}
		case IndexEventTrace:
			record, seen := byDefinition[entry.GraphChecksum]
			if !seen {
				continue
			}
			record.Runs++
			if entry.At.After(record.LatestRun) {
				record.LatestRun = entry.At
			}
		}
	}

	records := make([]WorkflowRecord, 0, len(order))
	for _, checksum := range order {
		record := byDefinition[checksum]
		if tool != "" && record.Tool != tool {
			continue
		}
		records = append(records, *record)
	}

	sort.SliceStable(records, func(i, j int) bool {
		if records[i].Tool != records[j].Tool {
			return records[i].Tool < records[j].Tool
		}
		if records[i].Scope != records[j].Scope {
			return records[i].Scope < records[j].Scope
		}
		return records[i].LatestRun.After(records[j].LatestRun)
	})

	return records
}

// selectDocuments resolves a selection to document paths: for each selected workflow, its definition under
// [GraphsDir] and its traces under [TracesDir], definitions first in index order, then each definition's traces in
// that same order.
//
// Parameters:
//   - `entries`: the run index.
//   - `selection`: what to keep.
//
// Returns:
//   - `[]string`: the paths; empty when nothing matches.
func selectDocuments(entries []IndexEntry, selection workflowSelection) []string {

	definitions, selected := selectDefinitions(entries, selection)
	traces := selectTraces(entries, selected, selection)

	paths := make([]string, 0, len(definitions)+len(traces))
	if selection.kind != "trace" {
		for _, checksum := range definitions {
			paths = append(paths, filepath.Join(GraphsDir(), safeChecksum(checksum)+".yaml"))
		}
	}
	for _, checksum := range definitions {
		for _, ref := range traces[checksum] {
			paths = append(paths, ref.path)
		}
	}

	return paths
}

// traceRef is one execution trace's path and time, for ordering and for `--latest`.
type traceRef struct {
	at   time.Time
	path string
}

// selectDefinitions picks the workflows a selection names: every definition event matching the tool and scope,
// each once, in index order.
//
// Parameters:
//   - `entries`: the run index.
//   - `selection`: the tool and scope to match; "" matches all.
//
// Returns:
//   - `order`: the selected definitions' checksums, in index order.
//   - `selected`: the same set, for membership.
func selectDefinitions(
	entries []IndexEntry, selection workflowSelection,
) (order []string, selected map[string]bool) {

	selected = make(map[string]bool)

	for _, entry := range entries {
		if entry.Event != IndexEventGraph || selected[entry.GraphChecksum] {
			continue
		}
		if selection.tool != "" && entry.Tool != selection.tool {
			continue
		}
		if selection.scope != "" && entry.Scope != selection.scope {
			continue
		}
		selected[entry.GraphChecksum] = true
		order = append(order, entry.GraphChecksum)
	}

	return order, selected
}

// selectTraces collects the traces of the selected definitions, in index order, keeping only the newest per
// definition under `--latest`; none when the selection is definitions alone.
//
// Parameters:
//   - `entries`: the run index.
//   - `selected`: the selected definitions.
//   - `selection`: the kind and the `--latest` switch.
//
// Returns:
//   - `map[string][]traceRef`: each selected definition's traces.
func selectTraces(entries []IndexEntry, selected map[string]bool, selection workflowSelection) map[string][]traceRef {

	traces := make(map[string][]traceRef)
	if selection.kind == "definition" {
		return traces
	}

	for _, entry := range entries {
		if entry.Event != IndexEventTrace || !selected[entry.GraphChecksum] {
			continue
		}
		ref := traceRef{
			at:   entry.At,
			path: filepath.Join(TracesDir(), safeChecksum(entry.GraphChecksum), entry.TraceFile),
		}
		if selection.latest {
			traces[entry.GraphChecksum] = newestOf(traces[entry.GraphChecksum], ref)
			continue
		}
		traces[entry.GraphChecksum] = append(traces[entry.GraphChecksum], ref)
	}

	return traces
}

// newestOf keeps the newer of the held trace and the candidate: a one-element list, so `--latest` and the plain
// case share one shape.
//
// Parameters:
//   - `held`: the trace kept so far; empty on the first.
//   - `candidate`: the trace under consideration.
//
// Returns:
//   - `[]traceRef`: the newer of the two, alone.
func newestOf(held []traceRef, candidate traceRef) []traceRef {

	if len(held) == 0 || candidate.at.After(held[0].at) {
		return []traceRef{candidate}
	}
	return held
}

// endregion
