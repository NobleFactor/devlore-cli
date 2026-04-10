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
			got := n.SlotByName(tt.slotName)
			if got != tt.want {
				t.Errorf("GetSlot(%q) = %v, want %v", tt.slotName, got, tt.want)
			}
		})
	}
}

func TestNode_GetSlot_NilSlots(t *testing.T) {
	n := &Node{ID: "empty"}
	if got := n.SlotByName("anything"); got != nil {
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

// --- Node.Receiver ---

func TestNode_Receiver(t *testing.T) {
	n := &Node{ID: "test", Receiver: "file.link"}
	if n.Receiver != "file.link" {
		t.Errorf("Receiver = %q, want %q", n.Receiver, "file.link")
	}

	empty := &Node{ID: "test"}
	if empty.Receiver != "" {
		t.Errorf("Receiver = %q, want empty", empty.Receiver)
	}
}

// --- Node JSON marshal/unmarshal ---

func TestNode_JSON_RoundTrip(t *testing.T) {
	original := &Node{
		ID:       "test-node",
		Receiver: "file.link",
		Status:   StatusPending,
		Slots: map[string]SlotValue{
			"source": {Immediate: "/src/file"},
			"target": {Immediate: "/dst/file"},
		},
		Origin: "myproject",
		Layer:  "base",
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
	if decoded.Receiver != "file.link" {
		t.Errorf("Receiver = %q, want %q", decoded.Receiver, "file.link")
	}
	if decoded.Status != original.Status {
		t.Errorf("Status = %q, want %q", decoded.Status, original.Status)
	}
	if decoded.Origin != original.Origin {
		t.Errorf("Origin = %q, want %q", decoded.Origin, original.Origin)
	}
	if decoded.Layer != original.Layer {
		t.Errorf("Layer = %q, want %q", decoded.Layer, original.Layer)
	}
	if decoded.SlotByName("source") != "/src/file" {
		t.Errorf("source slot = %v, want /src/file", decoded.SlotByName("source"))
	}
}

func TestNode_JSON_EmptyReceiver(t *testing.T) {
	data := `{"id":"n1","status":"pending"}`
	var n Node
	if err := json.Unmarshal([]byte(data), &n); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if n.Receiver != "" {
		t.Errorf("Receiver = %q, want empty", n.Receiver)
	}
}

func TestNode_JSON_ReceiverFieldPresent(t *testing.T) {
	n := Node{
		ID:       "n1",
		Receiver: "template.render_bytes",
		Status:   StatusCompleted,
	}
	data, err := json.Marshal(&n)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal raw error: %v", err)
	}
	if raw["receiver"] != "template.render_bytes" {
		t.Errorf("JSON receiver field = %v, want template.render_bytes", raw["receiver"])
	}
}

// --- Node YAML marshal/unmarshal ---

func TestNode_YAML_RoundTrip(t *testing.T) {
	original := &Node{
		ID:       "yaml-node",
		Receiver: "pkg.install",
		Status:   StatusCompleted,
		Slots: map[string]SlotValue{
			"package": {Immediate: "vim"},
		},
		Annotations: map[string]string{
			"provider": "pkg",
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
	if decoded.Receiver != "pkg.install" {
		t.Errorf("Receiver = %q, want %q", decoded.Receiver, "pkg.install")
	}
	if decoded.Status != original.Status {
		t.Errorf("Status = %q, want %q", decoded.Status, original.Status)
	}
	if decoded.SlotByName("package") != "vim" {
		t.Errorf("package slot = %v, want vim", decoded.SlotByName("package"))
	}
}

func TestNode_YAML_EmptyAction(t *testing.T) {
	input := "id: n1\nstatus: pending\n"
	var n Node
	if err := yaml.Unmarshal([]byte(input), &n); err != nil {
		t.Fatalf("YAML Unmarshal error: %v", err)
	}
	if n.Receiver != "" {
		t.Errorf("Receiver = %q, want empty", n.Receiver)
	}
}

// --- Graph.SubgraphByID ---

func TestGraph_SubgraphByID(t *testing.T) {
	g := &Graph{
		Children: []SubgraphChild{
			{Subgraph: &Subgraph{ID: "phase.install", Name: "install"}},
			{Subgraph: &Subgraph{ID: "phase.configure", Name: "configure"}},
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
			p := g.SubgraphByID(tt.phaseID)
			if tt.wantNil {
				if p != nil {
					t.Errorf("SubgraphByID(%q) = %v, want nil", tt.phaseID, p)
				}
				return
			}
			if p == nil {
				t.Fatalf("SubgraphByID(%q) = nil, want subgraph %q", tt.phaseID, tt.want)
			}
			if p.Name != tt.want {
				t.Errorf("SubgraphByID(%q).Name = %q, want %q", tt.phaseID, p.Name, tt.want)
			}
		})
	}
}

func TestGraph_SubgraphByID_NilChildren(t *testing.T) {
	g := &Graph{}
	if p := g.SubgraphByID("any"); p != nil {
		t.Errorf("SubgraphByID on nil Children = %v, want nil", p)
	}
}

// NOTE: collectPhaseNodes tests removed — that method was part of the old Phase model
// and has been superseded by the Subgraph/Children tree structure.

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
		t.Error("different object receiverTypes should produce different checksums")
	}

	c4 := GitStyleChecksum("graph", "a.yaml", []byte("different"))
	if c1 == c4 {
		t.Error("different content should produce different checksums")
	}
}


// --- Graph.Summary ---

func TestGraph_Summary(t *testing.T) {
	g := &Graph{
		Children: []SubgraphChild{
			{Node: &Node{ID: "link1", Receiver: "file.link", Status: StatusCompleted}},
			{Node: &Node{ID: "link2", Receiver: "file.link", Status: StatusCompleted}},
			{Node: &Node{ID: "tmpl1", Receiver: "template.render_bytes", Status: StatusCompleted}},
			{Node: &Node{ID: "sec1", Receiver: "encryption.decrypt", Status: StatusCompleted}},
			{Node: &Node{ID: "copy1", Receiver: "file.copy", Status: StatusCompleted}},
			{Node: &Node{ID: "pkg1", Receiver: "pkg.install", Status: StatusCompleted}},
			{Node: &Node{ID: "pkg2", Receiver: "pkg.upgrade", Status: StatusCompleted}},
			{Node: &Node{ID: "pkg3", Receiver: "pkg.remove", Status: StatusCompleted}},
			{Node: &Node{ID: "skip1", Receiver: "file.link", Status: StatusSkipped}},
			{Node: &Node{ID: "fail1", Receiver: "file.link", Status: StatusFailed}},
			{Node: &Node{ID: "pend1", Receiver: "file.link", Status: StatusPending}},
		},
	}

	s := g.Summary()
	byAction := s.ByAction()

	if s.Completed() != 8 {
		t.Errorf("Completed() = %d, want 8", s.Completed())
	}
	if s.Skipped() != 1 {
		t.Errorf("Skipped() = %d, want 1", s.Skipped())
	}
	if s.Failed() != 1 {
		t.Errorf("Failed() = %d, want 1", s.Failed())
	}
	if s.Total() != 11 {
		t.Errorf("Total() = %d, want 11", s.Total())
	}
	if byAction["file.link"].Completed() != 2 {
		t.Errorf("ByAction[file.link].Completed() = %d, want 2", byAction["file.link"].Completed())
	}
	if byAction["file.link"].Failed() != 1 {
		t.Errorf("ByAction[file.link].Failed() = %d, want 1", byAction["file.link"].Failed())
	}
	if byAction["file.link"].Skipped() != 1 {
		t.Errorf("ByAction[file.link].Skipped() = %d, want 1", byAction["file.link"].Skipped())
	}
	if byAction["file.link"].Total() != 5 {
		t.Errorf("ByAction[file.link].Total() = %d, want 5", byAction["file.link"].Total())
	}
	if byAction["template.render_bytes"].Completed() != 1 {
		t.Errorf("ByAction[template.render_bytes].Completed() = %d, want 1", byAction["template.render_bytes"].Completed())
	}
	if byAction["pkg.install"].Completed() != 1 {
		t.Errorf("ByAction[pkg.install].Completed() = %d, want 1", byAction["pkg.install"].Completed())
	}
}

func TestGraph_Summary_EmptyGraph(t *testing.T) {
	g := &Graph{}
	s := g.Summary()
	if s.Completed() != 0 || s.Failed() != 0 || s.Skipped() != 0 || s.Total() != 0 {
		t.Errorf("empty graph summary should be all zeros")
	}
	if len(s.ByAction()) != 0 {
		t.Errorf("empty graph ByAction should be empty, got %v", s.ByAction())
	}
}

func TestGraph_Summary_ResetsOnRecompute(t *testing.T) {
	g := &Graph{
		Children: []SubgraphChild{
			{Node: &Node{ID: "a", Receiver: "file.link", Status: StatusCompleted}},
		},
	}
	s := g.Summary()
	if s.ByAction()["file.link"].Completed() != 1 {
		t.Fatalf("first compute: file.link.Completed() = %d, want 1", s.ByAction()["file.link"].Completed())
	}

	// Remove node and recompute: summary should reset.
	g.Children = nil
	s = g.Summary()
	if len(s.ByAction()) != 0 {
		t.Errorf("recompute after clearing nodes: ByAction should be empty, got %v", s.ByAction())
	}
}


// --- HydrateGraph ---

// testAction is a minimal FallibleAction implementation for testing HydrateGraph.
type testAction struct {
	name string
}

func (a *testAction) Name() string        { return a.name }
func (a *testAction) Params() []Parameter { return nil }
func (a *testAction) Do(_ *ExecutionContext, _ map[string]any) (Result, Complement, error) {
	return nil, nil, nil
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

	t.Run("unscoped", func(t *testing.T) {
		g := &Graph{Timestamp: ts}
		want := "2025-06-15T10-30-45.yaml"
		if got := g.Filename(); got != want {
			t.Errorf("Filename() = %q, want %q", got, want)
		}
	})

	t.Run("scoped", func(t *testing.T) {
		g := &Graph{
			Timestamp:  ts,
			Provenance: Provenance{Scope: "home"},
		}
		want := "home-2025-06-15T10-30-45.yaml"
		if got := g.Filename(); got != want {
			t.Errorf("Filename() = %q, want %q", got, want)
		}
	})
}

// --- NewGraph resource fields ---

func TestNewGraph_InitializesState(t *testing.T) {
	g := NewGraph(&ExecutionContext{})
	if g.State != StatePending {
		t.Errorf("NewGraph().State = %q, want %q", g.State, StatePending)
	}
	if g.Version != GraphFormatVersion {
		t.Errorf("NewGraph().Version = %q, want %q", g.Version, GraphFormatVersion)
	}
}

// testGraphResource is a minimal Resource for testing catalog behavior without
// depending on a concrete provider.
type testGraphResource struct {
	ResourceBase
}

func newTestGraphResource(uri string) *testGraphResource {
	return &testGraphResource{
		ResourceBase: NewResourceBase(&ExecutionContext{}, uri),
	}
}

func TestGraph_CatalogNotSerialized(t *testing.T) {
	g := NewGraph(&ExecutionContext{})
	g.Catalog = NewResourceCatalog()
	g.Catalog.Resolve(newTestGraphResource("file:///foo"))
	g.Catalog.Resolve(newTestGraphResource("file:///bar"))

	data, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	// Catalog should not appear in JSON.
	if strings.Contains(string(data), "catalog") {
		t.Error("Catalog should not be serialized to JSON")
	}
}
