// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"go.starlark.net/starlark"
)

// Interface guards.
var (
	_ starlark.Value    = (*Output)(nil)
	_ starlark.HasAttrs = (*Output)(nil)
	_ fmt.Stringer      = (*Output)(nil)
)

// --- Helpers ---

// makeTestGraph creates a minimal graph for testing.
func makeTestGraph() *Graph {
	return &Graph{Version: "1", Tool: "test", Catalog: NewResourceCatalog()}
}

// makeTestNode creates a node with the given ID and an optional stub action.
func makeTestNode(id, action string) *Node {
	n := &Node{ID: id}
	if action != "" {
		n.Action = StubAction(action)
	}
	return n
}

// resolveInput extracts an *Output from a Starlark value. Test helper only.
func resolveInput(value starlark.Value) (*Output, error) {
	if output, ok := value.(*Output); ok {
		return output, nil
	}
	return nil, fmt.Errorf("expected Output, got %s", value.Type())
}

// --- Output tests ---

func TestNewOutput(t *testing.T) {
	g := makeTestGraph()
	n := makeTestNode("n1", "file.copy")
	out := NewOutput(n, g, "result")

	if out.Node() != n {
		t.Errorf("Node() = %v, want %v", out.Node(), n)
	}
	if out.Graph() != g {
		t.Errorf("Graph() = %v, want %v", out.Graph(), g)
	}
	if out.Slot() != "result" {
		t.Errorf("Slot() = %q, want %q", out.Slot(), "result")
	}
}

func TestOutputString(t *testing.T) {
	tests := []struct {
		name   string
		nodeID string
		want   string
	}{
		{"simple id", "n1", "Output(n1)"},
		{"dotted id", "file.copy.1", "Output(file.copy.1)"},
		{"path id", "/tmp/foo", "Output(/tmp/foo)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := NewOutput(makeTestNode(tt.nodeID, ""), makeTestGraph(), "")
			if got := out.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOutputType(t *testing.T) {
	out := NewOutput(makeTestNode("n1", ""), makeTestGraph(), "")
	if got := out.Type(); got != "Output" {
		t.Errorf("ProviderType() = %q, want %q", got, "Output")
	}
}

func TestOutputTruth(t *testing.T) {
	out := NewOutput(makeTestNode("n1", ""), makeTestGraph(), "")
	if got := out.Truth(); got != true {
		t.Errorf("Truth() = %v, want true", got)
	}
}

func TestOutputHash(t *testing.T) {
	out := NewOutput(makeTestNode("n1", ""), makeTestGraph(), "")
	_, err := out.Hash()
	if err == nil {
		t.Fatal("Hash() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unhashable") {
		t.Errorf("Hash() error = %q, want to contain %q", err.Error(), "unhashable")
	}
}

func TestOutputNodeGraphSlot(t *testing.T) {
	g := makeTestGraph()
	n := makeTestNode("abc", "file.link")
	out := NewOutput(n, g, "out-slot")

	if out.Node() != n {
		t.Error("Node() returned wrong node")
	}
	if out.Graph() != g {
		t.Error("Graph() returned wrong graph")
	}
	if out.Slot() != "out-slot" {
		t.Errorf("Slot() = %q, want %q", out.Slot(), "out-slot")
	}
}

func TestOutputFillSlot(t *testing.T) {
	g := makeTestGraph()
	producer := makeTestNode("producer", "file.copy")
	consumer := makeTestNode("consumer", "file.link")
	out := NewOutput(producer, g, "default")

	out.FillSlot(consumer, "src")

	// Check edge was created.
	if len(g.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(g.Edges))
	}
	edge := g.Edges[0]
	if edge.From != "producer" || edge.To != "consumer" {
		t.Errorf("edge = %v -> %v, want producer -> consumer", edge.From, edge.To)
	}

	// Check promise slot was set on consumer.
	sv, ok := consumer.Slots["src"]
	if !ok {
		t.Fatal("consumer.Slots[\"src\"] not set")
	}
	if !sv.IsPromise() {
		t.Error("slot should be a promise")
	}
	if sv.NodeRef != "producer" {
		t.Errorf("NodeRef = %q, want %q", sv.NodeRef, "producer")
	}
	if sv.Slot != "default" {
		t.Errorf("Slot = %q, want %q", sv.Slot, "default")
	}
}

func TestOutputPath(t *testing.T) {
	tests := []struct {
		name  string
		slots map[string]SlotValue
		want  string
	}{
		{
			name:  "path present",
			slots: map[string]SlotValue{"path": {Immediate: "/tmp/file"}},
			want:  "/tmp/file",
		},
		{
			name:  "path missing",
			slots: nil,
			want:  "",
		},
		{
			name:  "path not string",
			slots: map[string]SlotValue{"path": {Immediate: 42}},
			want:  "",
		},
		{
			name:  "path is promise",
			slots: map[string]SlotValue{"path": {NodeRef: "other"}},
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := makeTestNode("n1", "")
			n.Slots = tt.slots
			out := NewOutput(n, makeTestGraph(), "")
			if got := out.Path(); got != tt.want {
				t.Errorf("Path() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOutputDependOn(t *testing.T) {
	g := makeTestGraph()
	producer := makeTestNode("producer", "file.copy")
	consumer := makeTestNode("consumer", "file.link")
	out := NewOutput(producer, g, "")

	out.DependOn(consumer)

	if len(g.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(g.Edges))
	}
	edge := g.Edges[0]
	if edge.From != "producer" || edge.To != "consumer" {
		t.Errorf("edge = %v -> %v, want producer -> consumer", edge.From, edge.To)
	}

	// Ensure no slots were set on consumer (DependOn creates edge only).
	if len(consumer.Slots) != 0 {
		t.Errorf("DependOn should not set slots, got %v", consumer.Slots)
	}
}

func TestOutputAttr(t *testing.T) {
	n := makeTestNode("test-node", "file.copy")
	n.SetSlotImmediate("path", "/tmp/out")
	g := makeTestGraph()
	out := NewOutput(n, g, "my-slot")

	tests := []struct {
		name    string
		attr    string
		want    string
		wantErr bool
	}{
		{"node_id", "node_id", "test-node", false},
		{"slot", "slot", "my-slot", false},
		{"slot value path", "path", "/tmp/out", false},
		{"unknown attr", "nonexistent", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := out.Attr(tt.attr)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got, ok := starlark.AsString(val)
			if !ok {
				t.Fatalf("expected string value, got %s", val.Type())
			}
			if got != tt.want {
				t.Errorf("Attr(%q) = %q, want %q", tt.attr, got, tt.want)
			}
		})
	}
}

func TestOutputAttrRetry(t *testing.T) {
	n := makeTestNode("r1", "appnet.download")
	g := makeTestGraph()
	out := NewOutput(n, g, "")

	val, err := out.Attr("retry")
	if err != nil {
		t.Fatalf("Attr(\"retry\") error: %v", err)
	}
	if val.Type() != "builtin_function_or_method" {
		t.Errorf("Attr(\"retry\") type = %q, want builtin", val.Type())
	}
}

func TestOutputAttrNames(t *testing.T) {
	n := makeTestNode("n1", "")
	n.SetSlotImmediate("path", "/tmp")
	n.SetSlotImmediate("mode", "0644")
	out := NewOutput(n, makeTestGraph(), "")

	names := out.AttrNames()
	sort.Strings(names)

	// Must contain the base names plus slot names.
	expected := []string{"mode", "node_id", "path", "retry", "slot"}
	sort.Strings(expected)

	if len(names) != len(expected) {
		t.Fatalf("AttrNames() = %v, want %v", names, expected)
	}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("AttrNames()[%d] = %q, want %q", i, name, expected[i])
		}
	}
}

// --- FillSlot function tests ---

func TestFillSlotOutput(t *testing.T) {
	g := makeTestGraph()
	producer := makeTestNode("producer", "file.copy")
	consumer := makeTestNode("consumer", "file.link")
	out := NewOutput(producer, g, "default")

	if err := FillSlot(consumer, g, "input", out); err != nil {
		t.Fatalf("FillSlot() error: %v", err)
	}

	// Verify edge.
	if len(g.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(g.Edges))
	}
	if g.Edges[0].From != "producer" || g.Edges[0].To != "consumer" {
		t.Errorf("edge = %v -> %v, want producer -> consumer", g.Edges[0].From, g.Edges[0].To)
	}

	// Verify promise slot.
	sv := consumer.Slots["input"]
	if !sv.IsPromise() || sv.NodeRef != "producer" {
		t.Errorf("slot = %+v, want promise to producer", sv)
	}
}

func TestFillSlotOutputList(t *testing.T) {
	g := makeTestGraph()
	o1 := NewOutput(makeTestNode("a", ""), g, "")
	o2 := NewOutput(makeTestNode("b", ""), g, "")
	list := starlark.NewList([]starlark.Value{o1, o2})
	consumer := makeTestNode("consumer", "")

	if err := FillSlot(consumer, g, "deps", list); err != nil {
		t.Fatalf("FillSlot() error: %v", err)
	}

	if len(g.Edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(g.Edges))
	}

	// Verify indexed slots exist.
	if _, ok := consumer.Slots["deps[0]"]; !ok {
		t.Error("expected deps[0] slot")
	}
	if _, ok := consumer.Slots["deps[1]"]; !ok {
		t.Error("expected deps[1] slot")
	}

	// Verify length slot.
	lenSlot, ok := consumer.Slots["deps.len"]
	if !ok {
		t.Fatal("deps.len not set")
	}
	if lenSlot.Immediate != 2 {
		t.Errorf("deps.len = %v, want 2", lenSlot.Immediate)
	}
}

func TestFillSlotMixedListFallsThrough(t *testing.T) {
	g := makeTestGraph()
	// A list containing non-Output values should fall through to immediate handling.
	list := starlark.NewList([]starlark.Value{starlark.String("a"), starlark.String("b")})
	consumer := makeTestNode("consumer", "")

	if err := FillSlot(consumer, g, "items", list); err != nil {
		t.Fatalf("FillSlot() error: %v", err)
	}

	// No edges — not a list of Outputs.
	if len(g.Edges) != 0 {
		t.Fatalf("expected 0 edges, got %d", len(g.Edges))
	}

	// Should be stored as immediate.
	sv, ok := consumer.Slots["items"]
	if !ok {
		t.Fatal("items slot not set")
	}
	if !sv.IsImmediate() {
		t.Error("expected immediate slot for non-Output list")
	}
}

func TestFillSlotString(t *testing.T) {
	g := makeTestGraph()
	n := makeTestNode("n1", "")

	if err := FillSlot(n, g, "path", starlark.String("/tmp/foo")); err != nil {
		t.Fatalf("FillSlot() error: %v", err)
	}

	sv := n.Slots["path"]
	if !sv.IsImmediate() {
		t.Error("expected immediate slot")
	}
	if sv.Immediate != "/tmp/foo" {
		t.Errorf("Immediate = %v, want %q", sv.Immediate, "/tmp/foo")
	}
}

func TestFillSlotInt(t *testing.T) {
	g := makeTestGraph()
	n := makeTestNode("n1", "")

	if err := FillSlot(n, g, "count", starlark.MakeInt(42)); err != nil {
		t.Fatalf("FillSlot() error: %v", err)
	}

	sv := n.Slots["count"]
	if !sv.IsImmediate() {
		t.Error("expected immediate slot")
	}
	if sv.Immediate != 42 {
		t.Errorf("Immediate = %v, want 42", sv.Immediate)
	}
}

func TestFillSlotBool(t *testing.T) {
	g := makeTestGraph()
	n := makeTestNode("n1", "")

	if err := FillSlot(n, g, "force", starlark.Bool(true)); err != nil {
		t.Fatalf("FillSlot() error: %v", err)
	}

	sv := n.Slots["force"]
	if !sv.IsImmediate() {
		t.Error("expected immediate slot")
	}
	if sv.Immediate != true {
		t.Errorf("Immediate = %v, want true", sv.Immediate)
	}
}

func TestFillSlotFloat(t *testing.T) {
	g := makeTestGraph()
	n := makeTestNode("n1", "")

	if err := FillSlot(n, g, "ratio", starlark.Float(3.14)); err != nil {
		t.Fatalf("FillSlot() error: %v", err)
	}

	sv := n.Slots["ratio"]
	if !sv.IsImmediate() {
		t.Error("expected immediate slot")
	}
	if sv.Immediate != 3.14 {
		t.Errorf("Immediate = %v, want 3.14", sv.Immediate)
	}
}

func TestFillSlotNone(t *testing.T) {
	g := makeTestGraph()
	n := makeTestNode("n1", "")

	if err := FillSlot(n, g, "optional", starlark.None); err != nil {
		t.Fatalf("FillSlot() error: %v", err)
	}

	// None is a no-op: slot should not be set.
	if n.Slots != nil {
		if _, ok := n.Slots["optional"]; ok {
			t.Error("None should not create a slot entry")
		}
	}
}

func TestFillSlotList(t *testing.T) {
	g := makeTestGraph()
	n := makeTestNode("n1", "")

	list := starlark.NewList([]starlark.Value{
		starlark.String("alpha"),
		starlark.String("beta"),
		starlark.String("gamma"),
	})

	if err := FillSlot(n, g, "items", list); err != nil {
		t.Fatalf("FillSlot() error: %v", err)
	}

	sv := n.Slots["items"]
	if !sv.IsImmediate() {
		t.Error("expected immediate slot")
	}
	got, ok := sv.Immediate.([]string)
	if !ok {
		t.Fatalf("expected []string, got %T", sv.Immediate)
	}
	if len(got) != 3 || got[0] != "alpha" || got[1] != "beta" || got[2] != "gamma" {
		t.Errorf("Immediate = %v, want [alpha beta gamma]", got)
	}
}

func TestFillSlotDict(t *testing.T) {
	g := makeTestGraph()
	n := makeTestNode("n1", "")

	dict := starlark.NewDict(2)
	_ = dict.SetKey(starlark.String("key1"), starlark.String("val1"))
	_ = dict.SetKey(starlark.String("key2"), starlark.MakeInt(99))

	if err := FillSlot(n, g, "env", dict); err != nil {
		t.Fatalf("FillSlot() error: %v", err)
	}

	sv := n.Slots["env"]
	if !sv.IsImmediate() {
		t.Error("expected immediate slot")
	}
	got, ok := sv.Immediate.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", sv.Immediate)
	}
	if got["key1"] != "val1" {
		t.Errorf("env[\"key1\"] = %v, want %q", got["key1"], "val1")
	}
	if got["key2"] != 99 {
		t.Errorf("env[\"key2\"] = %v, want 99", got["key2"])
	}
}

func TestFillSlotUnsupportedType(t *testing.T) {
	g := makeTestGraph()
	n := makeTestNode("n1", "")

	// starlark.Tuple is not handled by FillSlot.
	tuple := starlark.Tuple{starlark.String("a"), starlark.String("b")}
	err := FillSlot(n, g, "bad", tuple)
	if err == nil {
		t.Fatal("expected error for unsupported type, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "unsupported")
	}
}

// --- FillSlot implicit resource edges ---

// testFileResource embeds op.ResourceBase for testing implicit edge creation.
type testFileResource struct {
	ResourceBase
	SourcePath string
}

func newTestFileResource(path string) *testFileResource {
	r := &testFileResource{SourcePath: path}
	r.SetURI("file://" + path)
	return r
}

func TestFillSlotImplicitEdge_ResourceWithOrigin(t *testing.T) {
	g := makeTestGraph()
	consumer := makeTestNode("reader", "file.read")

	// A resource produced by "writer" node — FillSlot should create an implicit edge.
	res := newTestFileResource("/foo")
	if _, err := g.Catalog.Shadow(res, "writer"); err != nil {
		t.Fatalf("Shadow error: %v", err)
	}

	val, err := Marshal(res)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	if err := FillSlot(consumer, g, "source_file", val); err != nil {
		t.Fatalf("FillSlot error: %v", err)
	}

	// Should create an implicit edge from writer → reader.
	if len(g.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(g.Edges))
	}
	if g.Edges[0].From != "writer" || g.Edges[0].To != "reader" {
		t.Errorf("edge = %v → %v, want writer → reader", g.Edges[0].From, g.Edges[0].To)
	}
}

func TestFillSlotImplicitEdge_ResourceWithoutOrigin(t *testing.T) {
	g := makeTestGraph()
	consumer := makeTestNode("reader", "file.read")

	// A discovered resource (no origin) — FillSlot should NOT create an edge.
	res := newTestFileResource("/bar")
	// Resolve creates a discovery entry with no origin.
	g.Catalog.Resolve("file:///bar")

	val, err := Marshal(res)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	if err := FillSlot(consumer, g, "source_file", val); err != nil {
		t.Fatalf("FillSlot error: %v", err)
	}

	if len(g.Edges) != 0 {
		t.Errorf("expected 0 edges for resource without origin, got %d", len(g.Edges))
	}
}

func TestFillSlotImplicitEdge_PlainResource(t *testing.T) {
	g := makeTestGraph()
	consumer := makeTestNode("reader", "file.read")

	// A plain ResourceBase with origin.
	res := new(NewResourceBase("file:///baz"))
	if _, err := g.Catalog.Shadow(res, "producer"); err != nil {
		t.Fatalf("Shadow error: %v", err)
	}

	val, err := Marshal(res)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	if err := FillSlot(consumer, g, "input", val); err != nil {
		t.Fatalf("FillSlot error: %v", err)
	}

	if len(g.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(g.Edges))
	}
	if g.Edges[0].From != "producer" || g.Edges[0].To != "reader" {
		t.Errorf("edge = %v → %v, want producer → reader", g.Edges[0].From, g.Edges[0].To)
	}
}

// --- resolveInput ---

func TestResolveInput(t *testing.T) {
	out := NewOutput(makeTestNode("n1", ""), makeTestGraph(), "")

	got, err := resolveInput(out)
	if err != nil {
		t.Fatalf("resolveInput() error: %v", err)
	}
	if got != out {
		t.Error("resolveInput() returned wrong output")
	}
}

func TestResolveInputNonOutput(t *testing.T) {
	_, err := resolveInput(starlark.String("not an output"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "expected Output") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "expected Output")
	}
}

// --- retryBuiltin tests ---

func TestOutputRetryBuiltin(t *testing.T) {
	tests := []struct {
		name        string
		kwargs      []starlark.Tuple
		wantMax     int
		wantBackoff BackoffStrategy
		wantErr     bool
	}{
		{
			name: "basic retry",
			kwargs: []starlark.Tuple{
				{starlark.String("max_attempts"), starlark.MakeInt(3)},
			},
			wantMax: 3,
		},
		{
			name: "with backoff",
			kwargs: []starlark.Tuple{
				{starlark.String("max_attempts"), starlark.MakeInt(5)},
				{starlark.String("backoff"), starlark.String("exponential")},
			},
			wantMax:     5,
			wantBackoff: BackoffExponential,
		},
		{
			name: "linear backoff with delays",
			kwargs: []starlark.Tuple{
				{starlark.String("max_attempts"), starlark.MakeInt(2)},
				{starlark.String("backoff"), starlark.String("linear")},
				{starlark.String("initial_delay"), starlark.String("1s")},
				{starlark.String("max_delay"), starlark.String("10s")},
			},
			wantMax:     2,
			wantBackoff: BackoffLinear,
		},
		{
			name: "negative max_attempts",
			kwargs: []starlark.Tuple{
				{starlark.String("max_attempts"), starlark.MakeInt(-1)},
			},
			wantErr: true,
		},
		{
			name: "unknown backoff strategy",
			kwargs: []starlark.Tuple{
				{starlark.String("max_attempts"), starlark.MakeInt(1)},
				{starlark.String("backoff"), starlark.String("random")},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := makeTestNode("r1", "appnet.download")
			out := NewOutput(n, makeTestGraph(), "")

			val, err := out.Attr("retry")
			if err != nil {
				t.Fatalf("Attr(\"retry\"): %v", err)
			}
			builtin, ok := val.(*starlark.Builtin)
			if !ok {
				t.Fatalf("retry attr is %T, want *starlark.Builtin", val)
			}

			result, err := starlark.Call(
				&starlark.Thread{Name: "test"},
				builtin,
				nil,
				tt.kwargs,
			)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("retry() error: %v", err)
			}

			// Should return the output itself for chaining.
			if result != out {
				t.Error("retry() should return the Output for chaining")
			}

			if n.Retry == nil {
				t.Fatal("Retry policy not set on node")
			}
			if n.Retry.MaxAttempts != tt.wantMax {
				t.Errorf("MaxAttempts = %d, want %d", n.Retry.MaxAttempts, tt.wantMax)
			}
			if tt.wantBackoff != "" && n.Retry.Backoff != tt.wantBackoff {
				t.Errorf("Backoff = %q, want %q", n.Retry.Backoff, tt.wantBackoff)
			}
		})
	}
}

func TestOutputRetryBuiltinDelays(t *testing.T) {
	n := makeTestNode("r1", "appnet.download")
	out := NewOutput(n, makeTestGraph(), "")

	val, err := out.Attr("retry")
	if err != nil {
		t.Fatalf("Attr(\"retry\"): %v", err)
	}
	builtin := val.(*starlark.Builtin)

	_, err = starlark.Call(
		&starlark.Thread{Name: "test"},
		builtin,
		nil,
		[]starlark.Tuple{
			{starlark.String("max_attempts"), starlark.MakeInt(3)},
			{starlark.String("backoff"), starlark.String("linear")},
			{starlark.String("initial_delay"), starlark.String("500ms")},
			{starlark.String("max_delay"), starlark.String("30s")},
		},
	)
	if err != nil {
		t.Fatalf("retry() error: %v", err)
	}

	if n.Retry.InitialDelay != "500ms" {
		t.Errorf("InitialDelay = %q, want %q", n.Retry.InitialDelay, "500ms")
	}
	if n.Retry.MaxDelay != "30s" {
		t.Errorf("MaxDelay = %q, want %q", n.Retry.MaxDelay, "30s")
	}
}

// --- FillSlot edge cases ---

func TestOutputFillSlotMultipleConsumers(t *testing.T) {
	g := makeTestGraph()
	producer := makeTestNode("producer", "file.copy")
	consumer1 := makeTestNode("c1", "file.link")
	consumer2 := makeTestNode("c2", "file.link")
	out := NewOutput(producer, g, "default")

	out.FillSlot(consumer1, "input")
	out.FillSlot(consumer2, "input")

	// Fan-out: should have 2 edges from the same producer.
	if len(g.Edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(g.Edges))
	}
	for i, edge := range g.Edges {
		if edge.From != "producer" {
			t.Errorf("edge[%d].From = %q, want %q", i, edge.From, "producer")
		}
	}
	targets := []string{g.Edges[0].To, g.Edges[1].To}
	sort.Strings(targets)
	if targets[0] != "c1" || targets[1] != "c2" {
		t.Errorf("edge targets = %v, want [c1 c2]", targets)
	}
}

