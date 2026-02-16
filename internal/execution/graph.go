// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package execution provides the core execution graph primitives and executor
// shared by writ (configuration deployment) and lore (package management).
//
// # Core Types
//
//   - Graph: A directed graph of nodes and edges representing work to be done
//   - Node: A single unit of work with an action to execute
//   - Edge: A dependency relationship between nodes
//
// # Execution Model
//
//   - GraphBuilder: Interface for building graphs (implementations in tools)
//   - GraphExecutor: Runs graphs by executing actions on nodes
//   - ActionRegistry: Maps action names to implementations
//
// # Actions
//
// Each action implements Action.Do(ctx, slots) returning (Result, UndoState, error).
// Content flows via Result and promise slots — upstream actions return content as
// Result, downstream actions receive it through a "content" promise slot.
//
// # Graph Lifecycle
//
// The Graph represents both plans (before execution) and receipts (after execution):
//   - Before Run(): State is "pending", nodes describe what will happen
//   - After Run(): State is "executed", nodes describe what happened
//   - Serialized before execution: "dry-run" or "purchase order"
//   - Serialized after execution: "receipt"
package execution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// GraphState represents the execution state of the graph.
type GraphState string

const (
	StatePending  GraphState = "pending"
	StateExecuted GraphState = "executed"
	StateFailed   GraphState = "failed"
)

// NodeStatus represents the execution status of a node.
type NodeStatus string

const (
	StatusPending   NodeStatus = "pending"
	StatusCompleted NodeStatus = "completed"
	StatusSkipped   NodeStatus = "skipped"
	StatusFailed    NodeStatus = "failed"
)

// Platform records the OS and architecture.
type Platform struct {
	OS   string `json:"os" yaml:"os"`
	Arch string `json:"arch" yaml:"arch"`
}

// GraphContext contains tool-specific metadata stored in the graph.
// Both writ and lore populate this with their relevant context.
type GraphContext struct {
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
	BackedUp   int `json:"backed_up,omitempty" yaml:"backed_up,omitempty"`
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

	// Action to perform. Set at construction via SetAction or node.Action = reg.MustGet(...).
	// Serialized as the action name string; deserialized as a stubAction.
	Action Action `json:"-" yaml:"-"`

	// Status of this node: pending, completed, skipped, failed.
	Status NodeStatus `json:"status" yaml:"status"`

	// Timestamp is when this operation completed.
	Timestamp string `json:"timestamp,omitempty" yaml:"timestamp,omitempty"`

	// Slots holds input values for this node. Each slot can be:
	// - Immediate: value known at analysis time
	// - Promise: reference to another node's output (creates edge)
	Slots map[string]SlotValue `json:"slots,omitempty" yaml:"slots,omitempty"`

	// Project this node belongs to.
	Project string `json:"project,omitempty" yaml:"project,omitempty"`

	// Layer is the repository layer (base, team, personal).
	Layer string `json:"layer,omitempty" yaml:"layer,omitempty"`

	// SourceChecksum is the SHA256 of the source file at deploy time.
	SourceChecksum string `json:"source_checksum,omitempty" yaml:"source_checksum,omitempty"`

	// TargetChecksum is the SHA256 of the target file after deployment.
	TargetChecksum string `json:"target_checksum,omitempty" yaml:"target_checksum,omitempty"`

	// Error message if status is failed.
	Error string `json:"error,omitempty" yaml:"error,omitempty"`

	// Retry is the retry policy for this node (nil = no retry).
	Retry *RetryPolicy `json:"retry,omitempty" yaml:"retry,omitempty"`

	// Annotations holds extensible metadata (serialized to receipts).
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// StubAction creates a named Action stub for testing and receipt deserialization.
// Do and Undo return errors — stub actions are not executable.
func StubAction(name string) Action { return &stubAction{name: name} }

// stubAction implements Action with just a name for receipt deserialization.
// Do and Undo return errors — receipt nodes are not executable.
type stubAction struct{ name string }

func (s *stubAction) Name() string { return s.name }
func (s *stubAction) Do(_ *Context, _ map[string]any) (Result, UndoState, error) {
	return nil, nil, fmt.Errorf("stub action %s: not executable", s.name)
}
func (s *stubAction) Undo(_ *Context, _ map[string]any, _ UndoState) error {
	return fmt.Errorf("stub action %s: not executable", s.name)
}

// nodeJSON is the JSON/YAML serialization shape for Node.
// The type alias strips MarshalJSON/UnmarshalJSON to avoid infinite recursion.
type nodeJSON Node

type nodeJSONWire struct {
	Action string `json:"action" yaml:"action"`
	*nodeJSON
}

// MarshalJSON serializes the node with Action as its name string.
func (n Node) MarshalJSON() ([]byte, error) {
	return json.Marshal(&nodeJSONWire{
		Action:   n.ActionName(),
		nodeJSON: (*nodeJSON)(&n),
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
func (n Node) MarshalYAML() (any, error) {
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
// Promise slots are resolved from the results map; immediate slots are returned directly.
// Pass nil for results when all slots are immediate (e.g., in tests).
func (n *Node) ResolvedSlots(results map[string]any) map[string]any {
	slots := make(map[string]any, len(n.Slots))
	for name, sv := range n.Slots {
		if sv.IsPromise() {
			if results != nil {
				if val, ok := results[sv.NodeRef]; ok {
					slots[name] = val
				}
			}
		} else if sv.IsImmediate() {
			slots[name] = sv.Immediate
		}
	}
	return slots
}

// SetSlotPromise sets a slot to a promise (reference to another node).
func (n *Node) SetSlotPromise(name, nodeRef, slot string) {
	if n.Slots == nil {
		n.Slots = make(map[string]SlotValue)
	}
	n.Slots[name] = SlotValue{NodeRef: nodeRef, Slot: slot}
}

// GetID returns the node's unique identifier.
func (n *Node) GetID() string { return n.ID }

// ActionName returns the action name. Works for both live nodes
// (Action interface set) and deserialized receipt nodes (stubAction).
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
// Either Immediate is set (direct value) or NodeRef is set (promise from upstream node).
type SlotValue struct {
	// Immediate is the direct value (any type, known at analysis time).
	Immediate any `json:"immediate,omitempty" yaml:"immediate,omitempty"`

	// NodeRef is the ID of the node that produces this value (promise).
	NodeRef string `json:"node_ref,omitempty" yaml:"node_ref,omitempty"`

	// Slot is which output slot of the referenced node (empty = default output).
	Slot string `json:"slot,omitempty" yaml:"slot,omitempty"`
}

// IsPromise returns true if this slot value is a promise (reference to another node).
func (s SlotValue) IsPromise() bool {
	return s.NodeRef != ""
}

// IsImmediate returns true if this slot value is an immediate value.
func (s SlotValue) IsImmediate() bool {
	return s.NodeRef == "" && s.Immediate != nil
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
	if s.BackedUp > 0 {
		result += fmt.Sprintf(", %d backed up", s.BackedUp)
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
// Format: "<tool>-<timestamp>.yaml"
func (g *Graph) Filename() string {
	return fmt.Sprintf("%s-%s.yaml", g.Tool, g.Timestamp.Format("2006-01-02T15-04-05"))
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

// ApplyResults updates node states from execution results.
func (g *Graph) ApplyResults(results []*NodeResult) {
	resultMap := make(map[string]*NodeResult)
	for _, r := range results {
		resultMap[r.NodeID] = r
	}

	for _, n := range g.Nodes {
		if r, ok := resultMap[n.ID]; ok {
			switch r.Status {
			case ResultCompleted:
				n.Status = StatusCompleted
			case ResultSkipped:
				n.Status = StatusSkipped
			case ResultFailed:
				n.Status = StatusFailed
				if r.Error != nil {
					n.Error = r.Error.Error()
				}
			}
			n.Timestamp = time.Now().Format(time.RFC3339)
			n.SourceChecksum = r.SourceChecksum
			n.TargetChecksum = r.TargetChecksum
		}
	}
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

		// Check for backup annotation
		if n.Annotations != nil && n.Annotations["backup"] != "" {
			g.Summary.BackedUp++
		}
	}
}

// Hydrate replaces stubActions on graph nodes with real actions from the registry.
// This enables loaded/deserialized graphs to be executed. Nodes with no action name
// (e.g., nodes that were never serialized with an action) are skipped.
func (g *Graph) Hydrate(reg *ActionRegistry) error {
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

// PhaseByID returns the phase with the given ID, or nil if not found.
func (g *Graph) PhaseByID(id string) *Phase {
	for _, p := range g.Phases {
		if p.ID == id {
			return p
		}
	}
	return nil
}
