// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package execution provides the core execution graph primitives and executor
// shared by writ (configuration deployment) and lore (package management).
//
// # Core Types
//
//   - Graph: A directed graph of nodes and edges representing work to be done
//   - Node: A single unit of work with operations to execute
//   - Edge: A dependency relationship between nodes
//
// # Execution Model
//
//   - GraphBuilder: Interface for building graphs (implementations in tools)
//   - GraphExecutor: Runs graphs by executing operations on nodes
//   - OperationRegistry: Maps operation names to implementations
//
// # Operation Categories
//
//   - Transform: Read content, produce transformed content (decrypt, expand)
//   - Writer: Read content, write to filesystem (copy)
//   - Direct: Manage own I/O, no content flow (link, mkdir, install)
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
	"fmt"
	"os"
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

	// Operations to perform: link, copy, expand, decrypt, install, etc.
	Operations []string `json:"operations" yaml:"operations"`

	// Status of this node: pending, completed, skipped, failed.
	Status NodeStatus `json:"status" yaml:"status"`

	// Timestamp is when this operation completed.
	Timestamp string `json:"timestamp,omitempty" yaml:"timestamp,omitempty"`

	// Source is the absolute path to the source file.
	Source string `json:"source,omitempty" yaml:"source,omitempty"`

	// Target is the absolute path to the target file.
	Target string `json:"target,omitempty" yaml:"target,omitempty"`

	// Project this node belongs to.
	Project string `json:"project,omitempty" yaml:"project,omitempty"`

	// Layer is the repository layer (base, team, personal).
	Layer string `json:"layer,omitempty" yaml:"layer,omitempty"`

	// Mode is the file permissions to set.
	Mode os.FileMode `json:"-" yaml:"-"`

	// SourceChecksum is the SHA256 of the source file at deploy time.
	SourceChecksum string `json:"source_checksum,omitempty" yaml:"source_checksum,omitempty"`

	// TargetChecksum is the SHA256 of the target file after deployment.
	TargetChecksum string `json:"target_checksum,omitempty" yaml:"target_checksum,omitempty"`

	// Error message if status is failed.
	Error string `json:"error,omitempty" yaml:"error,omitempty"`

	// Annotations holds extensible metadata (serialized to receipts).
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`

	// Metadata holds operation-specific data (not serialized).
	Metadata map[string]string `json:"-" yaml:"-"`
}

// GetID returns the node's unique identifier.
func (n *Node) GetID() string { return n.ID }

// GetOperations returns the list of operations to perform.
func (n *Node) GetOperations() []string { return n.Operations }

// GetSource returns the source path.
func (n *Node) GetSource() string { return n.Source }

// GetTarget returns the target path.
func (n *Node) GetTarget() string { return n.Target }

// GetProject returns the project name.
func (n *Node) GetProject() string { return n.Project }

// GetMode returns the file mode.
func (n *Node) GetMode() os.FileMode { return n.Mode }

// GetMetadata returns operation-specific metadata.
func (n *Node) GetMetadata() map[string]string { return n.Metadata }

// Edge represents a dependency relationship between two nodes.
type Edge struct {
	From     string `json:"from" yaml:"from"`
	To       string `json:"to" yaml:"to"`
	Relation string `json:"relation" yaml:"relation"`
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

	// Nodes are the operations to perform/performed.
	Nodes []*Node `json:"nodes" yaml:"nodes"`

	// Edges are the dependencies between nodes.
	Edges []Edge `json:"edges,omitempty" yaml:"edges,omitempty"`

	// Collisions records source conflicts resolved during tree building (writ-specific).
	Collisions []Collision `json:"collisions,omitempty" yaml:"collisions,omitempty"`

	// Summary contains execution statistics (populated after Run).
	Summary Summary `json:"summary,omitempty" yaml:"summary,omitempty"`

	// Checksum is the git-style integrity hash.
	Checksum string `json:"checksum,omitempty" yaml:"checksum,omitempty"`

	// Signature contains the cryptographic signature (optional).
	Signature *Signature `json:"signature,omitempty" yaml:"signature,omitempty"`
}

// Executable represents a unit of work that can be executed.
// This interface is implemented by Node.
type Executable interface {
	GetID() string
	GetOperations() []string
	GetSource() string
	GetTarget() string
	GetProject() string
	GetMode() os.FileMode
	GetMetadata() map[string]string
}

// Ensure Node implements Executable.
var _ Executable = (*Node)(nil)

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
		Nodes:      g.Nodes,
		Edges:      g.Edges,
		Collisions: g.Collisions,
	}

	return yaml.Marshal(canonical)
}

// ApplyResults updates node states from execution results.
func (g *Graph) ApplyResults(results []*Result) {
	resultMap := make(map[string]*Result)
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
			// Count by operation type below
		default:
			continue
		}

		// Determine primary operation
		if len(n.Operations) > 0 {
			switch n.Operations[0] {
			case "link":
				g.Summary.TotalFiles++
				g.Summary.Links++
			case "expand":
				g.Summary.TotalFiles++
				g.Summary.Templates++
			case "decrypt":
				g.Summary.TotalFiles++
				g.Summary.Secrets++
			case "copy":
				g.Summary.TotalFiles++
				g.Summary.Copies++
			case "package-install", "package-upgrade", "package-remove":
				g.Summary.Packages++
			}
		}

		// Check for backup annotation
		if n.Annotations != nil && n.Annotations["backup"] != "" {
			g.Summary.BackedUp++
		}
	}
}
