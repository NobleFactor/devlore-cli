// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package op owns the concrete graph data model types shared by
// the execution engine, Starlark layer, and CLI tools.
//
// # Core Types
//
//   - Graph: A directed graph of nodes and edges representing work to be done
//   - Node: A single unit of work with an action to execute
//   - Edge: A dependency relationship between nodes
//
// # Graph Lifecycle
//
// The Graph represents both plans (before execution) and receipts (after execution):
//   - Before Run(): State is "pending", nodes describe what will happen
//   - After Run(): State is "executed", nodes describe what happened
//   - Serialized before execution: "dry-run" or "purchase order"
//   - Serialized after execution: "receipt"
package op

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"runtime"
	"time"

	"gopkg.in/yaml.v3"
)

// GraphFormatVersion is the current graph serialization format version.
const GraphFormatVersion = "6"

// NewGraph creates a Graph with common metadata populated.
// Tool identifies which tool created the graph ("writ" or "lore").
// Callers populate Context and add Nodes/Edges after creation.
func NewGraph(tool string) *Graph {
	return &Graph{
		Version:   GraphFormatVersion,
		Tool:      tool,
		Timestamp: time.Now(),
		State:     StatePending,
		Platform: Platform{
			OS:   runtime.GOOS,
			Arch: runtime.GOARCH,
		},
		Nodes:   make([]*Node, 0),
		Catalog: NewResourceCatalog(),
	}
}

// GraphState represents the execution state of the graph.
type GraphState string

// GraphState constants define the possible states of a graph.
const (
	// StatePending indicates the graph has not yet been executed.
	StatePending GraphState = "pending"
	// StateExecuted indicates the graph executed successfully.
	StateExecuted GraphState = "executed"
	// StateFailed indicates the graph failed during execution.
	StateFailed GraphState = "failed"
)

// NodeStatus represents the execution status of a node.
type NodeStatus string

// NodeStatus constants define the possible statuses of a node.
const (
	// StatusPending indicates the node has not yet been executed.
	StatusPending NodeStatus = "pending"
	// StatusCompleted indicates the node executed successfully.
	StatusCompleted NodeStatus = "completed"
	// StatusSkipped indicates the node was skipped.
	StatusSkipped NodeStatus = "skipped"
	// StatusFailed indicates the node failed during execution.
	StatusFailed NodeStatus = "failed"
)

// GraphContext contains tool-specific metadata stored in the graph.
// Both writ and lore populate this with their relevant context.
type GraphContext struct {
	// Scope identifies the planning scope for this graph.
	// For writ: target scope ("system", "home").
	// For lore: package cache scope (package name or names).
	Scope string `json:"scope,omitempty" yaml:"scope,omitempty"`

	// SourceRoot is the source directory (writ: repo path, lore: registry cache).
	SourceRoot string `json:"source_root,omitempty" yaml:"source_root,omitempty"`

	// TargetRoot is the target directory (typically $HOME).
	TargetRoot string `json:"target_root,omitempty" yaml:"target_root,omitempty"`

	// Projects lists the projects included (writ-specific).
	Projects []string `json:"projects,omitempty" yaml:"projects,omitempty"`

	// Packages lists the packages included (lore-specific).
	Packages []string `json:"packages,omitempty" yaml:"packages,omitempty"`

	// Segments contains platform segment values (writ-specific).
	Segments map[string]string `json:"segments,omitempty" yaml:"segments,omitempty"`

	// Layers lists repository layers used (writ-specific).
	Layers []string `json:"layers,omitempty" yaml:"layers,omitempty"`

	// Platform is the target platform string (lore-specific, e.g., "Darwin", "Linux.Debian").
	TargetPlatform string `json:"target_platform,omitempty" yaml:"target_platform,omitempty"`

	// Features enabled for package installation (lore-specific).
	Features []string `json:"features,omitempty" yaml:"features,omitempty"`

	// Settings for package installation (lore-specific).
	Settings map[string]string `json:"settings,omitempty" yaml:"settings,omitempty"`

	// CommitHashes records the git commit hash for each layer source (writ-specific).
	// Keys are layer names ("base", "team", "personal"); values are full commit hashes.
	CommitHashes map[string]string `json:"commit_hashes,omitempty" yaml:"commit_hashes,omitempty"`
}

// Summary contains execution statistics.
type Summary struct {
	TotalFiles int `json:"total_files,omitempty" yaml:"total_files,omitempty"`
	Links      int `json:"links,omitempty" yaml:"links,omitempty"`
	Copies     int `json:"copies,omitempty" yaml:"copies,omitempty"`
	Templates  int `json:"templates,omitempty" yaml:"templates,omitempty"`
	Secrets    int `json:"secrets,omitempty" yaml:"secrets,omitempty"`
	Packages   int `json:"packages,omitempty" yaml:"packages,omitempty"`
	Skipped    int `json:"skipped,omitempty" yaml:"skipped,omitempty"`
	Failed     int `json:"failed,omitempty" yaml:"failed,omitempty"`
}

// Signature contains the cryptographic signature of a graph.
type Signature struct {
	// Method is the signing method used (gpg, aws_kms, gcp_kms, azure_kv).
	Method string `json:"method" yaml:"method"`

	// Value is the signature data (base64-encoded).
	Value string `json:"value" yaml:"value"`

	// KeyID identifies the key used for signing.
	// For GPG: fingerprint, for KMS: key ARN/ID/URL.
	KeyID string `json:"key_id" yaml:"key_id"`
}

// Collision records a source conflict resolved during tree building (writ-specific).
type Collision struct {
	Target            string `json:"target" yaml:"target"`
	Winner            string `json:"winner" yaml:"winner"`
	WinnerLayer       string `json:"winner_layer,omitempty" yaml:"winner_layer,omitempty"`
	WinnerSpecificity int    `json:"winner_specificity,omitempty" yaml:"winner_specificity,omitempty"`
	Loser             string `json:"loser" yaml:"loser"`
	LoserLayer        string `json:"loser_layer,omitempty" yaml:"loser_layer,omitempty"`
	LoserSpecificity  int    `json:"loser_specificity,omitempty" yaml:"loser_specificity,omitempty"`
}

// Node represents a single unit of work in an execution graph.
type Node struct {
	// ID is the unique identifier (typically relative target path or package name).
	ID string `json:"id" yaml:"id"`

	// Action to perform. Serialized as the action name string; deserialized
	// as a stubAction. The executor calls Do directly.
	Action Action `json:"-" yaml:"-"`

	// Status of this node: pending, completed, skipped, failed.
	Status NodeStatus `json:"status" yaml:"status"`

	// Timestamp is when this action completed.
	Timestamp string `json:"timestamp,omitempty" yaml:"timestamp,omitempty"`

	// Slots holds input values for this node. Each slot can be:
	// - Immediate: value known at analysis time
	// - Promise: reference to another node's output (creates edge)
	Slots map[string]SlotValue `json:"slots,omitempty" yaml:"slots,omitempty"`

	// Project this node belongs to.
	Project string `json:"project,omitempty" yaml:"project,omitempty"`

	// Layer is the repository layer (base, team, personal).
	Layer string `json:"layer,omitempty" yaml:"layer,omitempty"`

	// Error message if status is failed.
	Error string `json:"error,omitempty" yaml:"error,omitempty"`

	// Retry is the retry policy for this node (nil = no retry).
	Retry *RetryPolicy `json:"retry,omitempty" yaml:"retry,omitempty"`

	// Annotations holds extensible metadata (serialized to receipts).
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// StubAction creates a named action stub for testing and receipt deserialization.
// The stub is not executable — the executor replaces stubs via HydrateGraph.
func StubAction(name string) Action { return &stubAction{name: name} }

// IsStubAction reports whether an action is a stub (from deserialization).
func IsStubAction(a Action) bool {
	_, ok := a.(*stubAction)
	return ok
}

// stubAction stores only a name for receipt deserialization.
// Do panics because stubs must be replaced via HydrateGraph before execution.
type stubAction struct{ name string }

func (s *stubAction) Name() string        { return s.name }
func (s *stubAction) Params() []ParamInfo { return nil }
func (s *stubAction) Do(_ *Context, _ map[string]any) (Result, Complement, error) {
	return nil, nil, fmt.Errorf("stub action %q cannot be executed — call HydrateGraph first", s.name)
}

// nodeJSON is the JSON/YAML serialization shape for Node.
// The type alias strips MarshalJSON/UnmarshalJSON to avoid infinite recursion.
type nodeJSON Node

type nodeJSONWire struct {
	Action string `json:"action" yaml:"action"`
	*nodeJSON
}

// MarshalJSON serializes the node with Action as its name string.
func (n *Node) MarshalJSON() ([]byte, error) {
	return json.Marshal(&nodeJSONWire{
		Action:   n.ActionName(),
		nodeJSON: (*nodeJSON)(n),
	})
}

// UnmarshalJSON deserializes a node, creating a stubAction from the action name.
func (n *Node) UnmarshalJSON(data []byte) error {
	aux := &nodeJSONWire{nodeJSON: (*nodeJSON)(n)}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if aux.Action != "" {
		n.Action = &stubAction{name: aux.Action}
	}
	return nil
}

// MarshalYAML serializes the node with Action as its name string.
// Note: we cannot use the nodeJSONWire embedding pattern here because yaml.v3
// panics on unexported concrete types behind interfaces in shadowed embedded
// fields, and fails to decode embedded fields when names collide (unlike
// encoding/json which handles both correctly). We round-trip through JSON instead.
func (n *Node) MarshalYAML() (any, error) {
	data, err := json.Marshal(n)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// UnmarshalYAML deserializes a node, creating a stubAction from the action name.
// Like MarshalYAML, we avoid the embedded struct pattern and decode manually.
func (n *Node) UnmarshalYAML(value *yaml.Node) error {
	// Decode into a raw map to extract the action name separately.
	var raw struct {
		Action string `yaml:"action"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	// Decode remaining fields into the node via the type-alias (strips this method).
	if err := value.Decode((*nodeJSON)(n)); err != nil {
		return err
	}
	if raw.Action != "" {
		n.Action = &stubAction{name: raw.Action}
	}
	return nil
}

// GetSlot returns the resolved value of a slot.
// If the slot is a promise, returns nil (must be resolved by executor).
func (n *Node) GetSlot(name string) any {
	if n.Slots != nil {
		if sv, ok := n.Slots[name]; ok {
			if sv.IsImmediate() {
				return sv.Immediate
			}
		}
	}
	return nil
}

// RequireStringSlot returns the string value of a required slot.
// Returns an error if the slot is not set, or holds a non-string value.
// An empty string is valid — use GetSlot for optional slots where zero value is acceptable.
func (n *Node) RequireStringSlot(name string) (string, error) {
	v := n.GetSlot(name)
	if v == nil {
		return "", fmt.Errorf("slot %q: not set", name)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("slot %q: expected string, got %T", name, v)
	}
	return s, nil
}

// SetSlotImmediate sets a slot to an immediate value.
func (n *Node) SetSlotImmediate(name string, value any) {
	if n.Slots == nil {
		n.Slots = make(map[string]SlotValue)
	}
	n.Slots[name] = SlotValue{Immediate: value}
}

// ResolvedSlots returns all slot values as a flat map.
// Promise slots are resolved from the results map; immediate slots are returned
// directly. Proxy slots are resolved from the optional proxyCtx map (used by
// gather for per-iteration item binding).
// Pass nil for results when all slots are immediate (e.g., in tests).
func (n *Node) ResolvedSlots(results map[string]any, proxyCtx ...map[string]any) map[string]any {
	slots := make(map[string]any, len(n.Slots))
	for name, sv := range n.Slots {
		switch {
		case sv.IsProxy():
			if len(proxyCtx) > 0 && proxyCtx[0] != nil {
				if item, ok := proxyCtx[0][sv.GatherRef]; ok {
					slots[name] = fieldAccess(item, sv.Field)
				}
			}
		case sv.IsPromise():
			if results != nil {
				if val, ok := results[sv.NodeRef]; ok {
					slots[name] = val
				}
			}
		default:
			slots[name] = sv.Immediate
		}
	}
	return slots
}

// fieldAccess extracts a named field from a value.
// Supports map[string]any for structured items.
func fieldAccess(item any, field string) any {
	if field == "" {
		return item
	}
	if m, ok := item.(map[string]any); ok {
		return m[field]
	}
	return nil
}

// SetSlotPromise sets a slot to a promise (reference to another node).
func (n *Node) SetSlotPromise(name, nodeRef, slot string) {
	if n.Slots == nil {
		n.Slots = make(map[string]SlotValue)
	}
	n.Slots[name] = SlotValue{NodeRef: nodeRef, Slot: slot}
}

// SetSlotProxy sets a slot to a gather proxy reference.
func (n *Node) SetSlotProxy(name, gatherRef, field string) {
	if n.Slots == nil {
		n.Slots = make(map[string]SlotValue)
	}
	n.Slots[name] = SlotValue{GatherRef: gatherRef, Field: field}
}

// GetID returns the node's unique identifier.
func (n *Node) GetID() string { return n.ID }

// ActionName returns the action name. Works for both live nodes
// (Action set by executor) and deserialized receipt nodes (stubAction).
func (n *Node) ActionName() string {
	if n.Action != nil {
		return n.Action.Name()
	}
	return ""
}

// GetProject returns the project name.
func (n *Node) GetProject() string { return n.Project }

// Edge represents a dependency relationship between two nodes.
// From must complete before To can begin execution.
type Edge struct {
	From string `json:"from" yaml:"from"`
	To   string `json:"to" yaml:"to"`
}

// SlotValue represents a value that fills a slot in a node.
// Three variants, mutually exclusive:
//   - Immediate: value known at analysis time
//   - Promise: reference to another node's output (NodeRef)
//   - Proxy: reference to a gather iteration item (GatherRef + Field)
type SlotValue struct {
	// Immediate is the direct value (any type, known at analysis time).
	Immediate any `json:"immediate,omitempty" yaml:"immediate,omitempty"`

	// NodeRef is the ID of the node that produces this value (promise).
	NodeRef string `json:"node_ref,omitempty" yaml:"node_ref,omitempty"`

	// Slot is which output slot of the referenced node (empty = default output).
	Slot string `json:"slot,omitempty" yaml:"slot,omitempty"`

	// GatherRef is the gather node ID for proxy resolution.
	GatherRef string `json:"gather_ref,omitempty" yaml:"gather_ref,omitempty"`

	// Field is the field name to access on the proxy item.
	Field string `json:"field,omitempty" yaml:"field,omitempty"`
}

// IsPromise returns true if this slot value is a promise (reference to another node).
func (s SlotValue) IsPromise() bool {
	return s.NodeRef != ""
}

// IsProxy returns true if this slot value is a gather proxy reference.
func (s SlotValue) IsProxy() bool {
	return s.GatherRef != ""
}

// IsImmediate returns true if this slot value is an immediate value.
func (s SlotValue) IsImmediate() bool {
	return !s.IsPromise() && !s.IsProxy()
}

// Graph represents an execution graph containing nodes and edges.
// This is THE graph used by both writ and lore - they differ only in content.
//
// Before Run(): State is "pending", represents the plan
// After Run(): State is "executed", represents the receipt
type Graph struct {
	// Version is the graph format version.
	Version string `json:"version" yaml:"version"`

	// Tool identifies which tool created this graph ("writ" or "lore").
	Tool string `json:"tool" yaml:"tool"`

	// Timestamp is when the graph was created/executed.
	Timestamp time.Time `json:"timestamp" yaml:"timestamp"`

	// State is the execution state (pending, executed, failed).
	State GraphState `json:"state" yaml:"state"`

	// Platform records the OS and architecture.
	Platform Platform `json:"platform" yaml:"platform"`

	// Context contains tool-specific metadata.
	Context GraphContext `json:"context" yaml:"context"`

	// Nodes are the actions to perform/performed.
	Nodes []*Node `json:"nodes" yaml:"nodes"`

	// Edges are the dependencies between nodes.
	Edges []Edge `json:"edges,omitempty" yaml:"edges,omitempty"`

	// Phases defines the ordered lifecycle phases (nil for non-phased graphs).
	// When present, the executor uses phase-aware execution with retry and rollback.
	// When nil, the executor falls back to flat node execution.
	Phases []*Phase `json:"phases,omitempty" yaml:"phases,omitempty"`

	// Collisions records source conflicts resolved during tree building (writ-specific).
	Collisions []Collision `json:"collisions,omitempty" yaml:"collisions,omitempty"`

	// Summary contains execution statistics (populated after Run).
	Summary Summary `json:"summary,omitempty" yaml:"summary,omitempty"`

	// Rollback records compensating actions executed during rollback (populated on failure).
	Rollback []RollbackEntry `json:"rollback,omitempty" yaml:"rollback,omitempty"`

	// Checksum is the git-style integrity hash.
	Checksum string `json:"checksum,omitempty" yaml:"checksum,omitempty"`

	// Signature contains the cryptographic signature (optional).
	Signature *Signature `json:"signature,omitempty" yaml:"signature,omitempty"`

	// Catalog is the append-only resource catalog for planning.
	// One per Graph. Not serialized — planning-only state.
	Catalog *ResourceCatalog `json:"-" yaml:"-"`
}

// String returns a human-readable summary.
func (s Summary) String() string {
	if s.Packages > 0 {
		// Lore summary
		result := fmt.Sprintf("%d packages", s.Packages)
		if s.Skipped > 0 {
			result += fmt.Sprintf(", %d skipped", s.Skipped)
		}
		if s.Failed > 0 {
			result += fmt.Sprintf(", %d failed", s.Failed)
		}
		return result
	}

	// Writ summary
	result := fmt.Sprintf("%d files", s.TotalFiles)
	if s.Links > 0 {
		result += fmt.Sprintf(" (%d links", s.Links)
		if s.Templates > 0 {
			result += fmt.Sprintf(", %d templates", s.Templates)
		}
		if s.Secrets > 0 {
			result += fmt.Sprintf(", %d secrets", s.Secrets)
		}
		if s.Copies > 0 {
			result += fmt.Sprintf(", %d copies", s.Copies)
		}
		result += ")"
	}
	if s.Skipped > 0 {
		result += fmt.Sprintf(", %d skipped", s.Skipped)
	}
	if s.Failed > 0 {
		result += fmt.Sprintf(", %d failed", s.Failed)
	}
	return result
}

// GitStyleChecksum computes a git-style checksum.
// Format: SHA256("<type> <basename> <len>\0<content>")
// Returns format "sha256:<hex>".
func GitStyleChecksum(objectType, basename string, content []byte) string {
	header := fmt.Sprintf("%s %s %d\x00", objectType, basename, len(content))
	hash := sha256.New()
	hash.Write([]byte(header))
	hash.Write(content)
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

// Encoder is the interface for graph serialization.
// Both *json.Encoder and *yaml.Encoder satisfy this interface.
type Encoder interface {
	Encode(v any) error
}

// Serialize writes the graph to the given encoder.
// The checksum is computed before encoding.
//
// Usage:
//
//	enc := yaml.NewEncoder(file)
//	enc.SetIndent(2)
//	defer enc.Close()
//	g.Serialize(enc)
func (g *Graph) Serialize(enc Encoder) error {
	return enc.Encode(g)
}

// Filename returns the standard filename for this graph.
// Format: "<tool>-<timestamp>.yaml" or "<tool>-<scope>-<timestamp>.yaml" when scoped.
func (g *Graph) Filename() string {
	ts := g.Timestamp.Format("2006-01-02T15-04-05")
	if g.Context.Scope != "" {
		return fmt.Sprintf("%s-%s-%s.yaml", g.Tool, g.Context.Scope, ts)
	}
	return fmt.Sprintf("%s-%s.yaml", g.Tool, ts)
}

// CanonicalContent returns the graph serialized as YAML without checksum and signature.
// This is used for computing checksums and verifying signatures.
func (g *Graph) CanonicalContent() ([]byte, error) {
	type canonicalGraph struct {
		Version    string       `yaml:"version"`
		Tool       string       `yaml:"tool"`
		Timestamp  string       `yaml:"timestamp"`
		State      GraphState   `yaml:"state"`
		Platform   Platform     `yaml:"platform"`
		Context    GraphContext `yaml:"context"`
		Phases     []*Phase     `yaml:"phases,omitempty"`
		Nodes      []*Node      `yaml:"nodes"`
		Edges      []Edge       `yaml:"edges,omitempty"`
		Collisions []Collision  `yaml:"collisions,omitempty"`
	}

	canonical := canonicalGraph{
		Version:    g.Version,
		Tool:       g.Tool,
		Timestamp:  g.Timestamp.Format(time.RFC3339),
		State:      g.State,
		Platform:   g.Platform,
		Context:    g.Context,
		Phases:     g.Phases,
		Nodes:      g.Nodes,
		Edges:      g.Edges,
		Collisions: g.Collisions,
	}

	return yaml.Marshal(canonical)
}

// ComputeSummary calculates summary statistics from nodes.
// For phased graphs, node statuses reflect the phase execution outcome
// (nodes in rolled-back phases may show as completed from before rollback).
func (g *Graph) ComputeSummary() {
	g.Summary = Summary{}

	for _, n := range g.Nodes {
		switch n.Status {
		case StatusSkipped:
			g.Summary.Skipped++
			continue
		case StatusFailed:
			g.Summary.Failed++
			continue
		case StatusCompleted:
			// Count by action type below
		default:
			continue
		}

		// Count by action type
		switch n.ActionName() {
		case "file.link":
			g.Summary.TotalFiles++
			g.Summary.Links++
		case "template.render":
			g.Summary.TotalFiles++
			g.Summary.Templates++
		case "encryption.decrypt":
			g.Summary.TotalFiles++
			g.Summary.Secrets++
		case "file.copy":
			g.Summary.TotalFiles++
			g.Summary.Copies++
		case "pkg.install", "pkg.upgrade", "pkg.remove":
			g.Summary.Packages++
		}
	}
}

// PhaseByID returns the phase with the given ID, or nil if not found.
func (g *Graph) PhaseByID(id string) *Phase {
	for _, p := range g.Phases {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// CollectPhaseNodes returns the nodes and intra-phase edges for the given phase.
// Nodes are returned in graph order; edges are filtered to only those between
// phase-internal nodes.
func (g *Graph) CollectPhaseNodes(phase *Phase) ([]*Node, []Edge) {
	nodeSet := make(map[string]bool, len(phase.NodeIDs))
	for _, id := range phase.NodeIDs {
		nodeSet[id] = true
	}

	var nodes []*Node
	for _, n := range g.Nodes {
		if nodeSet[n.ID] {
			nodes = append(nodes, n)
		}
	}

	var edges []Edge
	for _, e := range g.Edges {
		if nodeSet[e.From] && nodeSet[e.To] {
			edges = append(edges, e)
		}
	}

	return nodes, edges
}

// HydrateGraph replaces stub actions on graph nodes with real actions from the registry.
// This enables loaded/deserialized graphs to be executed. Nodes with no action name
// (e.g., nodes that were never serialized with an action) are skipped.
func HydrateGraph(g *Graph, reg *ActionRegistry) error {
	for _, n := range g.Nodes {
		name := n.ActionName()
		if name == "" {
			continue
		}
		action, ok := reg.Get(name)
		if !ok {
			return fmt.Errorf("hydrate: unknown action %q on node %q", name, n.ID)
		}
		n.Action = action
	}
	return nil
}
