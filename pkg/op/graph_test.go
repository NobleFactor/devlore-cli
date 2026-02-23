// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// --- SlotValue ---

func TestSlotValue_IsImmediate(t *testing.T) {
	tests := []struct {
		name string
		sv   SlotValue
		want bool
	}{
		{"immediate string", SlotValue{Immediate: "hello"}, true},
		{"immediate nil", SlotValue{}, true},
		{"promise", SlotValue{NodeRef: "node-1"}, false},
		{"proxy", SlotValue{GatherRef: "gather-1"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sv.IsImmediate(); got != tt.want {
				t.Errorf("IsImmediate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSlotValue_IsPromise(t *testing.T) {
	tests := []struct {
		name string
		sv   SlotValue
		want bool
	}{
		{"promise", SlotValue{NodeRef: "node-1", Slot: "output"}, true},
		{"immediate", SlotValue{Immediate: 42}, false},
		{"proxy", SlotValue{GatherRef: "g-1"}, false},
		{"empty", SlotValue{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sv.IsPromise(); got != tt.want {
				t.Errorf("IsPromise() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSlotValue_IsProxy(t *testing.T) {
	tests := []struct {
		name string
		sv   SlotValue
		want bool
	}{
		{"proxy", SlotValue{GatherRef: "gather-1", Field: "name"}, true},
		{"proxy no field", SlotValue{GatherRef: "gather-1"}, true},
		{"immediate", SlotValue{Immediate: "x"}, false},
		{"promise", SlotValue{NodeRef: "node-1"}, false},
		{"empty", SlotValue{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sv.IsProxy(); got != tt.want {
				t.Errorf("IsProxy() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Node.GetSlot ---

func TestNode_GetSlot(t *testing.T) {
	n := &Node{
		ID: "test-node",
		Slots: map[string]SlotValue{
			"source":  {Immediate: "/path/to/file"},
			"count":   {Immediate: 42},
			"promise": {NodeRef: "upstream"},
			"proxy":   {GatherRef: "gather-1"},
		},
	}

	tests := []struct {
		name     string
		slotName string
		want     any
	}{
		{"immediate string", "source", "/path/to/file"},
		{"immediate int", "count", 42},
		{"promise returns nil", "promise", nil},
		{"proxy returns nil", "proxy", nil},
		{"missing returns nil", "nonexistent", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := n.GetSlot(tt.slotName)
			if got != tt.want {
				t.Errorf("GetSlot(%q) = %v, want %v", tt.slotName, got, tt.want)
			}
		})
	}
}

func TestNode_GetSlot_NilSlots(t *testing.T) {
	n := &Node{ID: "empty"}
	if got := n.GetSlot("anything"); got != nil {
		t.Errorf("GetSlot on nil Slots = %v, want nil", got)
	}
}

// --- Node.RequireStringSlot ---

func TestNode_RequireStringSlot(t *testing.T) {
	n := &Node{
		ID: "test-node",
		Slots: map[string]SlotValue{
			"source":  {Immediate: "/path/to/file"},
			"empty":   {Immediate: ""},
			"count":   {Immediate: 42},
			"promise": {NodeRef: "upstream"},
		},
	}

	tests := []struct {
		name    string
		slot    string
		want    string
		wantErr string
	}{
		{"valid string", "source", "/path/to/file", ""},
		{"empty string is valid", "empty", "", ""},
		{"wrong type", "count", "", "expected string, got int"},
		{"promise slot not set", "promise", "", "not set"},
		{"missing slot", "missing", "", "not set"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := n.RequireStringSlot(tt.slot)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("RequireStringSlot(%q) expected error containing %q, got nil", tt.slot, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("RequireStringSlot(%q) error = %q, want containing %q", tt.slot, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("RequireStringSlot(%q) unexpected error: %v", tt.slot, err)
			}
			if got != tt.want {
				t.Errorf("RequireStringSlot(%q) = %q, want %q", tt.slot, got, tt.want)
			}
		})
	}
}

// --- Node.SetSlotImmediate ---

func TestNode_SetSlotImmediate(t *testing.T) {
	n := &Node{ID: "test-node"}
	n.SetSlotImmediate("source", "/path/to/file")

	if n.Slots == nil {
		t.Fatal("SetSlotImmediate did not initialize Slots map")
	}
	sv, ok := n.Slots["source"]
	if !ok {
		t.Fatal("SetSlotImmediate did not create slot entry")
	}
	if !sv.IsImmediate() {
		t.Error("slot should be immediate")
	}
	if sv.Immediate != "/path/to/file" {
		t.Errorf("slot value = %v, want /path/to/file", sv.Immediate)
	}
}

func TestNode_SetSlotImmediate_Overwrites(t *testing.T) {
	n := &Node{ID: "test-node"}
	n.SetSlotImmediate("key", "old")
	n.SetSlotImmediate("key", "new")

	if n.Slots["key"].Immediate != "new" {
		t.Errorf("overwrite failed: got %v, want new", n.Slots["key"].Immediate)
	}
}

// --- Node.SetSlotPromise ---

func TestNode_SetSlotPromise(t *testing.T) {
	n := &Node{ID: "test-node"}
	n.SetSlotPromise("input", "upstream-node", "output")

	if n.Slots == nil {
		t.Fatal("SetSlotPromise did not initialize Slots map")
	}
	sv := n.Slots["input"]
	if !sv.IsPromise() {
		t.Error("slot should be a promise")
	}
	if sv.NodeRef != "upstream-node" {
		t.Errorf("NodeRef = %q, want %q", sv.NodeRef, "upstream-node")
	}
	if sv.Slot != "output" {
		t.Errorf("Slot = %q, want %q", sv.Slot, "output")
	}
}

// --- Node.SetSlotProxy ---

func TestNode_SetSlotProxy(t *testing.T) {
	n := &Node{ID: "test-node"}
	n.SetSlotProxy("item_name", "gather-1", "name")

	if n.Slots == nil {
		t.Fatal("SetSlotProxy did not initialize Slots map")
	}
	sv := n.Slots["item_name"]
	if !sv.IsProxy() {
		t.Error("slot should be a proxy")
	}
	if sv.GatherRef != "gather-1" {
		t.Errorf("GatherRef = %q, want %q", sv.GatherRef, "gather-1")
	}
	if sv.Field != "name" {
		t.Errorf("Field = %q, want %q", sv.Field, "name")
	}
}

// --- Node.ResolvedSlots ---

func TestNode_ResolvedSlots_ImmediatesOnly(t *testing.T) {
	n := &Node{
		ID: "test",
		Slots: map[string]SlotValue{
			"source": {Immediate: "/src"},
			"target": {Immediate: "/dst"},
		},
	}
	got := n.ResolvedSlots(nil)
	if got["source"] != "/src" {
		t.Errorf("source = %v, want /src", got["source"])
	}
	if got["target"] != "/dst" {
		t.Errorf("target = %v, want /dst", got["target"])
	}
}

func TestNode_ResolvedSlots_PromiseResolution(t *testing.T) {
	n := &Node{
		ID: "test",
		Slots: map[string]SlotValue{
			"data": {NodeRef: "producer"},
		},
	}
	results := map[string]any{
		"producer": "produced-value",
	}
	got := n.ResolvedSlots(results)
	if got["data"] != "produced-value" {
		t.Errorf("data = %v, want produced-value", got["data"])
	}
}

func TestNode_ResolvedSlots_PromiseMissingRef(t *testing.T) {
	n := &Node{
		ID: "test",
		Slots: map[string]SlotValue{
			"data": {NodeRef: "nonexistent"},
		},
	}
	got := n.ResolvedSlots(map[string]any{})
	if _, exists := got["data"]; exists {
		t.Error("expected missing promise to not be in resolved slots")
	}
}

func TestNode_ResolvedSlots_ProxyResolution(t *testing.T) {
	n := &Node{
		ID: "test",
		Slots: map[string]SlotValue{
			"item_name": {GatherRef: "gather-1", Field: "name"},
		},
	}
	proxyCtx := map[string]any{
		"gather-1": map[string]any{"name": "vim", "version": "9.0"},
	}
	got := n.ResolvedSlots(nil, proxyCtx)
	if got["item_name"] != "vim" {
		t.Errorf("item_name = %v, want vim", got["item_name"])
	}
}

func TestNode_ResolvedSlots_ProxyEmptyField(t *testing.T) {
	n := &Node{
		ID: "test",
		Slots: map[string]SlotValue{
			"whole_item": {GatherRef: "gather-1", Field: ""},
		},
	}
	item := map[string]any{"name": "vim"}
	proxyCtx := map[string]any{
		"gather-1": item,
	}
	got := n.ResolvedSlots(nil, proxyCtx)
	gotMap, ok := got["whole_item"].(map[string]any)
	if !ok {
		t.Fatalf("whole_item is %T, want map[string]any", got["whole_item"])
	}
	if gotMap["name"] != "vim" {
		t.Errorf("whole_item.name = %v, want vim", gotMap["name"])
	}
}

func TestNode_ResolvedSlots_ProxyMissingRef(t *testing.T) {
	n := &Node{
		ID: "test",
		Slots: map[string]SlotValue{
			"item": {GatherRef: "nonexistent", Field: "name"},
		},
	}
	got := n.ResolvedSlots(nil, map[string]any{})
	if _, exists := got["item"]; exists {
		t.Error("expected missing proxy to not be in resolved slots")
	}
}

func TestNode_ResolvedSlots_ProxyNilCtx(t *testing.T) {
	n := &Node{
		ID: "test",
		Slots: map[string]SlotValue{
			"item": {GatherRef: "gather-1", Field: "name"},
		},
	}
	got := n.ResolvedSlots(nil)
	if _, exists := got["item"]; exists {
		t.Error("expected proxy with no proxyCtx to not be in resolved slots")
	}
}

func TestNode_ResolvedSlots_Mixed(t *testing.T) {
	n := &Node{
		ID: "test",
		Slots: map[string]SlotValue{
			"imm":     {Immediate: "direct"},
			"promise": {NodeRef: "upstream"},
			"proxy":   {GatherRef: "gather-1", Field: "val"},
		},
	}
	results := map[string]any{"upstream": "resolved-promise"}
	proxyCtx := map[string]any{
		"gather-1": map[string]any{"val": "resolved-proxy"},
	}
	got := n.ResolvedSlots(results, proxyCtx)
	if got["imm"] != "direct" {
		t.Errorf("imm = %v, want direct", got["imm"])
	}
	if got["promise"] != "resolved-promise" {
		t.Errorf("promise = %v, want resolved-promise", got["promise"])
	}
	if got["proxy"] != "resolved-proxy" {
		t.Errorf("proxy = %v, want resolved-proxy", got["proxy"])
	}
}

// --- Node.ActionName ---

func TestNode_ActionName(t *testing.T) {
	tests := []struct {
		name   string
		action Action
		want   string
	}{
		{"with action", &stubAction{name: "file.link"}, "file.link"},
		{"nil action", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &Node{ID: "test", Action: tt.action}
			if got := n.ActionName(); got != tt.want {
				t.Errorf("ActionName() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- Node JSON marshal/unmarshal ---

func TestNode_JSON_RoundTrip(t *testing.T) {
	original := &Node{
		ID:     "test-node",
		Action: StubAction("file.link"),
		Status: StatusPending,
		Slots: map[string]SlotValue{
			"source": {Immediate: "/src/file"},
			"target": {Immediate: "/dst/file"},
		},
		Project: "myproject",
		Layer:   "base",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Node
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, original.ID)
	}
	if decoded.ActionName() != "file.link" {
		t.Errorf("ActionName() = %q, want %q", decoded.ActionName(), "file.link")
	}
	if decoded.Status != original.Status {
		t.Errorf("Status = %q, want %q", decoded.Status, original.Status)
	}
	if decoded.Project != original.Project {
		t.Errorf("Project = %q, want %q", decoded.Project, original.Project)
	}
	if decoded.Layer != original.Layer {
		t.Errorf("Layer = %q, want %q", decoded.Layer, original.Layer)
	}
	if decoded.GetSlot("source") != "/src/file" {
		t.Errorf("source slot = %v, want /src/file", decoded.GetSlot("source"))
	}
}

func TestNode_JSON_EmptyAction(t *testing.T) {
	data := `{"id":"n1","status":"pending"}`
	var n Node
	if err := json.Unmarshal([]byte(data), &n); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if n.ActionName() != "" {
		t.Errorf("ActionName() = %q, want empty", n.ActionName())
	}
}

func TestNode_JSON_ActionFieldPresent(t *testing.T) {
	n := Node{
		ID:     "n1",
		Action: StubAction("template.render"),
		Status: StatusCompleted,
	}
	data, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal raw error: %v", err)
	}
	if raw["action"] != "template.render" {
		t.Errorf("JSON action field = %v, want template.render", raw["action"])
	}
}

// --- Node YAML marshal/unmarshal ---

func TestNode_YAML_RoundTrip(t *testing.T) {
	original := &Node{
		ID:     "yaml-node",
		Action: StubAction("pkg.install"),
		Status: StatusCompleted,
		Slots: map[string]SlotValue{
			"package": {Immediate: "vim"},
		},
		Annotations: map[string]string{
			"backup": "/tmp/backup",
		},
	}

	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("YAML Marshal error: %v", err)
	}

	var decoded Node
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("YAML Unmarshal error: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, original.ID)
	}
	if decoded.ActionName() != "pkg.install" {
		t.Errorf("ActionName() = %q, want %q", decoded.ActionName(), "pkg.install")
	}
	if decoded.Status != original.Status {
		t.Errorf("Status = %q, want %q", decoded.Status, original.Status)
	}
	if decoded.GetSlot("package") != "vim" {
		t.Errorf("package slot = %v, want vim", decoded.GetSlot("package"))
	}
}

func TestNode_YAML_EmptyAction(t *testing.T) {
	input := "id: n1\nstatus: pending\n"
	var n Node
	if err := yaml.Unmarshal([]byte(input), &n); err != nil {
		t.Fatalf("YAML Unmarshal error: %v", err)
	}
	if n.ActionName() != "" {
		t.Errorf("ActionName() = %q, want empty", n.ActionName())
	}
}

// --- Graph.PhaseByID ---

func TestGraph_PhaseByID(t *testing.T) {
	g := &Graph{
		Phases: []*Phase{
			{ID: "phase.install", Name: "install"},
			{ID: "phase.configure", Name: "configure"},
		},
	}

	tests := []struct {
		name    string
		phaseID string
		want    string
		wantNil bool
	}{
		{"found first", "phase.install", "install", false},
		{"found second", "phase.configure", "configure", false},
		{"not found", "phase.nonexistent", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := g.PhaseByID(tt.phaseID)
			if tt.wantNil {
				if p != nil {
					t.Errorf("PhaseByID(%q) = %v, want nil", tt.phaseID, p)
				}
				return
			}
			if p == nil {
				t.Fatalf("PhaseByID(%q) = nil, want phase %q", tt.phaseID, tt.want)
			}
			if p.Name != tt.want {
				t.Errorf("PhaseByID(%q).Name = %q, want %q", tt.phaseID, p.Name, tt.want)
			}
		})
	}
}

func TestGraph_PhaseByID_NilPhases(t *testing.T) {
	g := &Graph{}
	if p := g.PhaseByID("any"); p != nil {
		t.Errorf("PhaseByID on nil Phases = %v, want nil", p)
	}
}

// --- Graph.CollectPhaseNodes ---

func TestGraph_CollectPhaseNodes(t *testing.T) {
	g := &Graph{
		Nodes: []*Node{
			{ID: "a"},
			{ID: "b"},
			{ID: "c"},
			{ID: "d"},
		},
		Edges: []Edge{
			{From: "a", To: "b"},
			{From: "b", To: "c"},
			{From: "c", To: "d"},
			{From: "a", To: "d"},
		},
	}
	phase := &Phase{
		ID:      "phase.1",
		NodeIDs: []string{"a", "b"},
	}

	nodes, edges := g.CollectPhaseNodes(phase)

	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
	if nodes[0].ID != "a" || nodes[1].ID != "b" {
		t.Errorf("nodes = [%q, %q], want [a, b]", nodes[0].ID, nodes[1].ID)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 intra-phase edge, got %d", len(edges))
	}
	if edges[0].From != "a" || edges[0].To != "b" {
		t.Errorf("edge = {%q, %q}, want {a, b}", edges[0].From, edges[0].To)
	}
}

func TestGraph_CollectPhaseNodes_Empty(t *testing.T) {
	g := &Graph{
		Nodes: []*Node{{ID: "a"}},
		Edges: []Edge{{From: "a", To: "b"}},
	}
	phase := &Phase{ID: "empty", NodeIDs: []string{}}

	nodes, edges := g.CollectPhaseNodes(phase)
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(nodes))
	}
	if len(edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(edges))
	}
}

func TestGraph_CollectPhaseNodes_PreservesOrder(t *testing.T) {
	g := &Graph{
		Nodes: []*Node{
			{ID: "z"},
			{ID: "a"},
			{ID: "m"},
		},
	}
	phase := &Phase{
		ID:      "phase.order",
		NodeIDs: []string{"m", "z"},
	}

	nodes, _ := g.CollectPhaseNodes(phase)
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
	// Graph order is z, m (order in g.Nodes), not phase order (m, z).
	if nodes[0].ID != "z" || nodes[1].ID != "m" {
		t.Errorf("nodes = [%q, %q], want [z, m] (graph order)", nodes[0].ID, nodes[1].ID)
	}
}

// --- GitStyleChecksum ---

func TestGitStyleChecksum_Deterministic(t *testing.T) {
	content := []byte("hello world")
	c1 := GitStyleChecksum("graph", "test.yaml", content)
	c2 := GitStyleChecksum("graph", "test.yaml", content)
	if c1 != c2 {
		t.Errorf("checksums differ: %q vs %q", c1, c2)
	}
}

func TestGitStyleChecksum_Format(t *testing.T) {
	checksum := GitStyleChecksum("graph", "test.yaml", []byte("data"))
	if !strings.HasPrefix(checksum, "sha256:") {
		t.Errorf("checksum = %q, want prefix sha256:", checksum)
	}
	hexPart := strings.TrimPrefix(checksum, "sha256:")
	if len(hexPart) != 64 {
		t.Errorf("hex length = %d, want 64", len(hexPart))
	}
}

func TestGitStyleChecksum_DifferentInputs(t *testing.T) {
	c1 := GitStyleChecksum("graph", "a.yaml", []byte("data"))
	c2 := GitStyleChecksum("graph", "b.yaml", []byte("data"))
	if c1 == c2 {
		t.Error("different basenames should produce different checksums")
	}

	c3 := GitStyleChecksum("receipt", "a.yaml", []byte("data"))
	if c1 == c3 {
		t.Error("different object types should produce different checksums")
	}

	c4 := GitStyleChecksum("graph", "a.yaml", []byte("different"))
	if c1 == c4 {
		t.Error("different content should produce different checksums")
	}
}

// --- StubAction ---

func TestStubAction_Name(t *testing.T) {
	a := StubAction("file.link")
	if a.Name() != "file.link" {
		t.Errorf("Name() = %q, want %q", a.Name(), "file.link")
	}
}

func TestStubAction_Do_ReturnsError(t *testing.T) {
	a := StubAction("file.link")
	_, _, err := a.Do(nil, nil)
	if err == nil {
		t.Fatal("StubAction.Do() should return an error")
	}
	if !strings.Contains(err.Error(), "stub action") {
		t.Errorf("error = %q, want containing 'stub action'", err)
	}
	if !strings.Contains(err.Error(), "HydrateGraph") {
		t.Errorf("error = %q, want containing 'HydrateGraph'", err)
	}
}

// --- Graph.ComputeSummary ---

func TestGraph_ComputeSummary(t *testing.T) {
	g := &Graph{
		Nodes: []*Node{
			{ID: "link1", Action: StubAction("file.link"), Status: StatusCompleted},
			{ID: "link2", Action: StubAction("file.link"), Status: StatusCompleted,
				Annotations: map[string]string{"backup": "/tmp/bak"}},
			{ID: "tmpl1", Action: StubAction("template.render"), Status: StatusCompleted},
			{ID: "sec1", Action: StubAction("encryption.decrypt"), Status: StatusCompleted},
			{ID: "copy1", Action: StubAction("file.copy"), Status: StatusCompleted},
			{ID: "pkg1", Action: StubAction("pkg.install"), Status: StatusCompleted},
			{ID: "pkg2", Action: StubAction("pkg.upgrade"), Status: StatusCompleted},
			{ID: "pkg3", Action: StubAction("pkg.remove"), Status: StatusCompleted},
			{ID: "skip1", Action: StubAction("file.link"), Status: StatusSkipped},
			{ID: "fail1", Action: StubAction("file.link"), Status: StatusFailed},
			{ID: "pend1", Action: StubAction("file.link"), Status: StatusPending},
		},
	}

	g.ComputeSummary()
	s := g.Summary

	if s.TotalFiles != 5 {
		t.Errorf("TotalFiles = %d, want 5", s.TotalFiles)
	}
	if s.Links != 2 {
		t.Errorf("Links = %d, want 2", s.Links)
	}
	if s.Templates != 1 {
		t.Errorf("Templates = %d, want 1", s.Templates)
	}
	if s.Secrets != 1 {
		t.Errorf("Secrets = %d, want 1", s.Secrets)
	}
	if s.Copies != 1 {
		t.Errorf("Copies = %d, want 1", s.Copies)
	}
	if s.Packages != 3 {
		t.Errorf("Packages = %d, want 3", s.Packages)
	}
	if s.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", s.Skipped)
	}
	if s.Failed != 1 {
		t.Errorf("Failed = %d, want 1", s.Failed)
	}
	if s.BackedUp != 1 {
		t.Errorf("BackedUp = %d, want 1", s.BackedUp)
	}
}

func TestGraph_ComputeSummary_EmptyGraph(t *testing.T) {
	g := &Graph{}
	g.ComputeSummary()
	s := g.Summary
	if s.TotalFiles != 0 || s.Links != 0 || s.Packages != 0 || s.Skipped != 0 || s.Failed != 0 {
		t.Errorf("empty graph summary should be all zeros, got %+v", s)
	}
}

func TestGraph_ComputeSummary_ResetsOnRecompute(t *testing.T) {
	g := &Graph{
		Nodes: []*Node{
			{ID: "a", Action: StubAction("file.link"), Status: StatusCompleted},
		},
	}
	g.ComputeSummary()
	if g.Summary.Links != 1 {
		t.Fatalf("first compute: Links = %d, want 1", g.Summary.Links)
	}

	// Remove node and recompute: summary should reset.
	g.Nodes = nil
	g.ComputeSummary()
	if g.Summary.Links != 0 {
		t.Errorf("recompute after clearing nodes: Links = %d, want 0", g.Summary.Links)
	}
}

// --- HydrateGraph ---

// testAction is a minimal Action implementation for testing HydrateGraph.
type testAction struct {
	name string
}

func (a *testAction) Name() string { return a.name }
func (a *testAction) Do(_ *Context, _ map[string]any) (result Result, state UndoState, err error) {
	return nil, nil, nil
}

func TestHydrateGraph_ReplacesStubs(t *testing.T) {
	g := &Graph{
		Nodes: []*Node{
			{ID: "n1", Action: StubAction("file.link")},
			{ID: "n2", Action: StubAction("template.render")},
		},
	}
	reg := NewActionRegistry()
	reg.Register(&testAction{name: "file.link"})
	reg.Register(&testAction{name: "template.render"})

	if err := HydrateGraph(g, reg); err != nil {
		t.Fatalf("HydrateGraph error: %v", err)
	}

	// Verify stubs were replaced: Do should not error.
	for _, n := range g.Nodes {
		_, _, err := n.Action.Do(nil, nil)
		if err != nil {
			t.Errorf("node %q: Do() returned error after hydration: %v", n.ID, err)
		}
	}
}

func TestHydrateGraph_MissingAction(t *testing.T) {
	g := &Graph{
		Nodes: []*Node{
			{ID: "n1", Action: StubAction("unknown.action")},
		},
	}
	reg := NewActionRegistry()

	err := HydrateGraph(g, reg)
	if err == nil {
		t.Fatal("HydrateGraph should error on missing action")
	}
	if !strings.Contains(err.Error(), "unknown action") {
		t.Errorf("error = %q, want containing 'unknown action'", err)
	}
	if !strings.Contains(err.Error(), "unknown.action") {
		t.Errorf("error = %q, want containing action name", err)
	}
}

func TestHydrateGraph_SkipsNilAction(t *testing.T) {
	g := &Graph{
		Nodes: []*Node{
			{ID: "n1", Action: nil},
			{ID: "n2", Action: StubAction("file.link")},
		},
	}
	reg := NewActionRegistry()
	reg.Register(&testAction{name: "file.link"})

	if err := HydrateGraph(g, reg); err != nil {
		t.Fatalf("HydrateGraph error: %v", err)
	}
	if g.Nodes[0].Action != nil {
		t.Error("nil action node should remain nil after hydration")
	}
}

func TestHydrateGraph_EmptyGraph(t *testing.T) {
	g := &Graph{}
	reg := NewActionRegistry()
	if err := HydrateGraph(g, reg); err != nil {
		t.Fatalf("HydrateGraph on empty graph should succeed, got: %v", err)
	}
}

// --- fieldAccess ---

func TestFieldAccess(t *testing.T) {
	tests := []struct {
		name  string
		item  any
		field string
		want  any
	}{
		{"empty field returns item", map[string]any{"k": "v"}, "", map[string]any{"k": "v"}},
		{"map field access", map[string]any{"name": "vim"}, "name", "vim"},
		{"map missing field", map[string]any{"name": "vim"}, "version", nil},
		{"non-map with field", "scalar", "name", nil},
		{"empty field scalar", "scalar", "", "scalar"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fieldAccess(tt.item, tt.field)
			// For map comparison, check specific key.
			if tt.field == "" && tt.want != nil {
				if _, isMap := tt.want.(map[string]any); isMap {
					gotMap, ok := got.(map[string]any)
					if !ok {
						t.Fatalf("got %T, want map[string]any", got)
					}
					wantMap := tt.want.(map[string]any)
					if gotMap["k"] != wantMap["k"] {
						t.Errorf("map mismatch: got %v, want %v", gotMap, wantMap)
					}
					return
				}
			}
			if got != tt.want {
				t.Errorf("fieldAccess(%v, %q) = %v, want %v", tt.item, tt.field, got, tt.want)
			}
		})
	}
}

// --- Graph.Filename ---

func TestGraph_Filename(t *testing.T) {
	ts, _ := time.Parse(time.RFC3339, "2025-06-15T10:30:45Z")
	g := &Graph{Tool: "writ", Timestamp: ts}
	want := "writ-2025-06-15T10-30-45.yaml"
	if got := g.Filename(); got != want {
		t.Errorf("Filename() = %q, want %q", got, want)
	}
}

// --- Summary.String ---

func TestSummary_String_Writ(t *testing.T) {
	s := Summary{
		TotalFiles: 10,
		Links:      5,
		Templates:  3,
		Secrets:    1,
		Copies:     1,
		Skipped:    2,
		Failed:     1,
		BackedUp:   3,
	}
	got := s.String()
	if !strings.Contains(got, "10 files") {
		t.Errorf("missing total files in %q", got)
	}
	if !strings.Contains(got, "5 links") {
		t.Errorf("missing links in %q", got)
	}
	if !strings.Contains(got, "3 templates") {
		t.Errorf("missing templates in %q", got)
	}
	if !strings.Contains(got, "1 secrets") {
		t.Errorf("missing secrets in %q", got)
	}
	if !strings.Contains(got, "1 copies") {
		t.Errorf("missing copies in %q", got)
	}
	if !strings.Contains(got, "2 skipped") {
		t.Errorf("missing skipped in %q", got)
	}
	if !strings.Contains(got, "1 failed") {
		t.Errorf("missing failed in %q", got)
	}
	if !strings.Contains(got, "3 backed up") {
		t.Errorf("missing backed up in %q", got)
	}
}

func TestSummary_String_Lore(t *testing.T) {
	s := Summary{
		Packages: 5,
		Skipped:  1,
		Failed:   2,
	}
	got := s.String()
	if !strings.Contains(got, "5 packages") {
		t.Errorf("missing packages in %q", got)
	}
	if !strings.Contains(got, "1 skipped") {
		t.Errorf("missing skipped in %q", got)
	}
	if !strings.Contains(got, "2 failed") {
		t.Errorf("missing failed in %q", got)
	}
}

func TestSummary_String_MinimalWrit(t *testing.T) {
	s := Summary{TotalFiles: 0}
	got := s.String()
	if got != "0 files" {
		t.Errorf("String() = %q, want %q", got, "0 files")
	}
}

// --- Node.GetID / GetProject ---

func TestNode_GetID(t *testing.T) {
	n := &Node{ID: "my-node"}
	if n.GetID() != "my-node" {
		t.Errorf("GetID() = %q, want %q", n.GetID(), "my-node")
	}
}

func TestNode_GetProject(t *testing.T) {
	n := &Node{Project: "myproject"}
	if n.GetProject() != "myproject" {
		t.Errorf("GetProject() = %q, want %q", n.GetProject(), "myproject")
	}
}
