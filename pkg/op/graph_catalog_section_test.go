// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/application"
)

// The graph document's catalog section is mandatory even when empty, and it is intent — pending rels, what
// must exist when the graph runs (4-resource-management.md §5.4, ruled 2026-08-20). These tests pin the
// section's presence, its refusal when absent, its round trip, and the executor's pre-flight backstop.

// intentProbeResource is an announced resource whose constructor round-trips its URI `specific` — the
// contract [unpackCatalog] relies on (the file provider honors it by stripping its own scheme prefix).
type intentProbeResource struct {
	ResourceBase
}

// newIntentProbeResource constructs the probe from a path-ish string, tolerating its own emitted specific on
// the rehydration round trip.
func newIntentProbeResource(runtimeEnvironment *RuntimeEnvironment, value any) (Resource, error) {

	s, _ := value.(string)
	s = strings.TrimPrefix(s, "probe:")

	base, err := NewResourceBase(runtimeEnvironment, "probe:"+s, reflect.TypeFor[*intentProbeResource]())
	if err != nil {
		return nil, err
	}

	return &intentProbeResource{ResourceBase: base}, nil
}

func init() {
	AnnounceResource(reflect.TypeFor[*intentProbeResource](), newIntentProbeResource, nil)
}

// TestGraphDocument_CatalogSectionIsPresentEvenEmpty pins the mandatory section: a graph with an empty
// catalog still serializes `"resources": []`, loads back, and repacks byte-identically.
func TestGraphDocument_CatalogSectionIsPresentEvenEmpty(t *testing.T) {

	graph := formatIdentityGraph(t, time.Unix(1_700_000_000, 0).UTC())

	document := serializeGraph(t, graph, "json")
	if !bytes.Contains(document, []byte(`"resources": []`)) &&
		!bytes.Contains(document, []byte(`"resources":[]`)) {
		t.Fatalf("document lacks the empty resources section:\n%s", document)
	}

	loaded, err := LoadGraph(formatIdentityEnvironment(t), document, "json")
	if err != nil {
		t.Fatalf("LoadGraph: %v", err)
	}

	repacked := serializeGraph(t, loaded, "json")
	if !bytes.Equal(document, repacked) {
		t.Errorf("pack → unpack → pack is not byte-identical:\n--- first ---\n%s\n--- second ---\n%s",
			document, repacked)
	}
}

// TestLoadGraph_RefusesDocumentWithoutCatalogSection pins the hard gate: a document lacking the resources
// section — every pre-ruling document — does not load.
func TestLoadGraph_RefusesDocumentWithoutCatalogSection(t *testing.T) {

	document := serializeGraph(t, formatIdentityGraph(t, time.Unix(1_700_000_000, 0).UTC()), "json")

	var decoded map[string]any
	if err := json.Unmarshal(document, &decoded); err != nil {
		t.Fatal(err)
	}
	delete(decoded, "resources")
	stripped, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := LoadGraph(formatIdentityEnvironment(t), stripped, "json"); err == nil {
		t.Fatal("LoadGraph = nil error for a document without the resources section, want the refusal")
	} else if !strings.Contains(err.Error(), "resource catalog section") {
		t.Errorf("refusal does not name the section: %v", err)
	}
}

// TestGraphDocument_IntentRowsRoundTrip pins the row shape and the id-preserving reconstruction: entries
// serialize as pending intent and reload with their ids, URIs, and Pending state — and nothing else.
func TestGraphDocument_IntentRowsRoundTrip(t *testing.T) {

	environment, err := NewRuntimeEnvironment(context.Background(),
		NewRuntimeEnvironmentSpec("test").WithApplication(&application.Application{Name: "test"}))
	if err != nil {
		t.Fatalf("NewRuntimeEnvironment: %v", err)
	}

	catalog := NewResourceCatalog()
	for _, name := range []string{"etc/first.conf", "etc/second.conf"} {
		resource, probeErr := newIntentProbeResource(environment, name)
		if probeErr != nil {
			t.Fatalf("probe %s: %v", name, probeErr)
		}
		if _, discoverErr := catalog.Discover(resource.URI(),
			func() (Resource, error) { return resource, nil }); discoverErr != nil {
			t.Fatalf("Discover %s: %v", name, discoverErr)
		}
	}

	registry := ReceiverRegistry()
	completeAction, err := registry.BuildAction("flow.complete")
	if err != nil {
		t.Fatalf("BuildAction: %v", err)
	}
	leaf, err := NewNode(NewNodeSpec().WithID("leaf").WithAction(completeAction))
	if err != nil {
		t.Fatalf("NewNode: %v", err)
	}
	graph, err := NewGraph(NewGraphSpec().WithUnits(leaf).WithResourceCatalog(catalog))
	if err != nil {
		t.Fatalf("NewGraph: %v", err)
	}

	document := serializeGraph(t, graph, "json")

	loaded, err := LoadGraph(environment, document, "json")
	if err != nil {
		t.Fatalf("LoadGraph: %v", err)
	}

	rows := loaded.ResourceCatalog().IntentEntries()
	if len(rows) != 2 {
		t.Fatalf("reloaded catalog carries %d intent rows, want 2: %+v", len(rows), rows)
	}
	for i, want := range []string{"etc/first.conf", "etc/second.conf"} {
		if !strings.Contains(rows[i].URI, want) {
			t.Errorf("row %d URI = %q, want it to name %q", i, rows[i].URI, want)
		}
	}

	// The stateless-row ruling (§5.4, 2026-08-21): an intent row is {id, uri} and nothing else — pending
	// is definitional, so no state field exists to assert. The serialized document must not carry one.
	if strings.Contains(string(document), `"state"`) {
		t.Errorf("the graph document carries a state field — intent rows are {id, uri}:\n%s", document)
	}

	repacked := serializeGraph(t, loaded, "json")
	if !bytes.Equal(document, repacked) {
		t.Errorf("pack → unpack → pack is not byte-identical with intent rows:\n--- first ---\n%s\n--- second ---\n%s",
			document, repacked)
	}
}

// TestRun_CatalogLessGraph_PreflightFailed pins the executor backstop: a graph stripped of its catalog —
// unreachable through NewGraph or LoadGraph, so induced directly — refuses to dispatch.
func TestRun_CatalogLessGraph_PreflightFailed(t *testing.T) {

	graph := formatIdentityGraph(t, time.Unix(1_700_000_000, 0).UTC())
	graph.resourceCatalog = nil

	executor := NewGraphExecutor(graph, NewRuntimeEnvironmentSpec("test").
		WithApplication(&application.Application{Name: "test"}))

	if _, runErr := executor.Run(context.Background(), nil); runErr == nil {
		t.Fatal("Run = nil error for a catalog-less graph, want the pre-flight refusal")
	}

	got := executor.RunStatus()
	if got.Phase != PhaseStopped || got.Condition != ConditionExecutionFailed || got.Reason != ReasonPreflightFailed {
		t.Errorf("RunStatus() = %v, want stopped × execution_failed × preflight_failed", got)
	}
}
