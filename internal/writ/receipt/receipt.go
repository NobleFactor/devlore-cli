// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

// Package receipt provides deployment receipt tracking for writ.
// Receipts are v4 graph-format: nodes represent file operations,
// edges represent relationships (delegation, dependency).
package receipt

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/internal/writ/tree"
)

// CurrentVersion is the receipt format version.
// v1: Initial format (flat entries)
// v2: Added SourceChecksum, TargetChecksum for copied files
// v3: Added age-encrypted signature
// v4: Graph format (nodes + edges)
const CurrentVersion = "4"

// Receipt records a writ deployment operation as an execution graph.
type Receipt struct {
	// Version is the receipt format version.
	Version string `json:"version" yaml:"version"`

	// Format identifies the serialization structure (always "graph" for v4).
	Format string `json:"format" yaml:"format"`

	// Timestamp is when the deployment completed.
	Timestamp time.Time `json:"timestamp" yaml:"timestamp"`

	// Tool identifies which tool produced this receipt.
	Tool string `json:"tool" yaml:"tool"`

	// Platform records the OS and architecture.
	Platform Platform `json:"platform" yaml:"platform"`

	// Context contains writ-specific deployment metadata.
	Context WritContext `json:"context" yaml:"context"`

	// Roots are the entry points into the graph (project names).
	Roots []string `json:"roots" yaml:"roots"`

	// Nodes are the operations performed.
	Nodes []Node `json:"nodes" yaml:"nodes"`

	// Edges are the relationships between nodes.
	Edges []Edge `json:"edges,omitempty" yaml:"edges,omitempty"`

	// Summary contains deployment statistics.
	Summary Summary `json:"summary,omitempty" yaml:"summary,omitempty"`

	// Signature contains the cryptographic signature.
	Signature *Signature `json:"signature,omitempty" yaml:"signature,omitempty"`
}

// Platform records the OS and architecture.
type Platform struct {
	OS   string `json:"os" yaml:"os"`
	Arch string `json:"arch" yaml:"arch"`
}

// WritContext contains writ-specific deployment metadata.
type WritContext struct {
	SourceRoot string            `json:"source_root" yaml:"source_root"`
	TargetRoot string            `json:"target_root" yaml:"target_root"`
	Projects   []string          `json:"projects" yaml:"projects"`
	Segments   map[string]string `json:"segments" yaml:"segments"`
}

// Node represents a single operation in the execution graph.
type Node struct {
	// ID is the unique identifier (relative target path for writ).
	ID string `json:"id" yaml:"id"`

	// Operation performed: link, expand, decrypt, copy, delegate.
	Operation string `json:"operation" yaml:"operation"`

	// Status of the operation: completed, skipped.
	Status string `json:"status" yaml:"status"`

	// Timestamp is when this operation completed.
	Timestamp string `json:"timestamp,omitempty" yaml:"timestamp,omitempty"`

	// Source is the absolute path to the source file.
	Source string `json:"source,omitempty" yaml:"source,omitempty"`

	// Target is the absolute path to the target file.
	Target string `json:"target,omitempty" yaml:"target,omitempty"`

	// Project this file belongs to.
	Project string `json:"project,omitempty" yaml:"project,omitempty"`

	// SourceChecksum is the SHA256 of the source file at deploy time.
	SourceChecksum string `json:"source_checksum,omitempty" yaml:"source_checksum,omitempty"`

	// TargetChecksum is the SHA256 of the target file after deployment.
	TargetChecksum string `json:"target_checksum,omitempty" yaml:"target_checksum,omitempty"`

	// DelegateTo names the tool to delegate to (e.g., "lore").
	DelegateTo string `json:"delegate_to,omitempty" yaml:"delegate_to,omitempty"`

	// Annotations holds extensible metadata (backup paths, etc.).
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// Edge represents a relationship between two nodes.
type Edge struct {
	From     string `json:"from" yaml:"from"`
	To       string `json:"to" yaml:"to"`
	Relation string `json:"relation" yaml:"relation"`
	Artifact string `json:"artifact,omitempty" yaml:"artifact,omitempty"`
}

// Backup records a backed-up file.
type Backup struct {
	Original   string `json:"original" yaml:"original"`
	BackupPath string `json:"backup_path" yaml:"backup_path"`
}

// Summary contains deployment statistics.
type Summary struct {
	TotalFiles int `json:"total_files" yaml:"total_files"`
	Links      int `json:"links" yaml:"links"`
	Copies     int `json:"copies,omitempty" yaml:"copies,omitempty"`
	Templates  int `json:"templates" yaml:"templates"`
	Secrets    int `json:"secrets,omitempty" yaml:"secrets,omitempty"`
	Skipped    int `json:"skipped,omitempty" yaml:"skipped,omitempty"`
	BackedUp   int `json:"backed_up,omitempty" yaml:"backed_up,omitempty"`
	Delegated  int `json:"delegated,omitempty" yaml:"delegated,omitempty"`
}

// New creates a new v4 graph-format receipt.
func New(sourceRoot, targetRoot string, projects []string, segments map[string]string) *Receipt {
	return &Receipt{
		Version:   CurrentVersion,
		Format:    "graph",
		Timestamp: time.Now(),
		Tool:      "writ",
		Platform:  detectPlatform(),
		Context: WritContext{
			SourceRoot: sourceRoot,
			TargetRoot: targetRoot,
			Projects:   projects,
			Segments:   segments,
		},
		Roots: projects,
		Nodes: make([]Node, 0),
	}
}

// AddNode adds a deployed file node to the receipt.
func (r *Receipt) AddNode(node *tree.Node, alreadyDeployed bool) {
	n := Node{
		ID:        node.RelTarget,
		Operation: primaryOperation(node.Operations.Strings()),
		Status:    "completed",
		Timestamp: time.Now().Format(time.RFC3339),
		Source:    node.Source,
		Target:    node.Target,
		Project:   node.Project,
	}

	if alreadyDeployed {
		if n.Annotations == nil {
			n.Annotations = make(map[string]string)
		}
		n.Annotations["already_deployed"] = "true"
	}

	r.Nodes = append(r.Nodes, n)
}

// AddNodeWithChecksums adds a deployed file node with content checksums.
func (r *Receipt) AddNodeWithChecksums(node *tree.Node, alreadyDeployed bool, sourceChecksum, targetChecksum string) {
	n := Node{
		ID:             node.RelTarget,
		Operation:      primaryOperation(node.Operations.Strings()),
		Status:         "completed",
		Timestamp:      time.Now().Format(time.RFC3339),
		Source:         node.Source,
		Target:         node.Target,
		Project:        node.Project,
		SourceChecksum: sourceChecksum,
		TargetChecksum: targetChecksum,
	}

	if alreadyDeployed {
		if n.Annotations == nil {
			n.Annotations = make(map[string]string)
		}
		n.Annotations["already_deployed"] = "true"
	}

	r.Nodes = append(r.Nodes, n)
}

// AddDelegated records a delegated manifest as a delegate node.
func (r *Receipt) AddDelegated(node *tree.Node) {
	n := Node{
		ID:         node.RelTarget,
		Operation:  "delegate",
		Status:     "completed",
		Timestamp:  time.Now().Format(time.RFC3339),
		Source:     node.Source,
		Target:     node.Target,
		Project:    node.Project,
		DelegateTo: "lore",
	}
	r.Nodes = append(r.Nodes, n)
}

// AddBackup records a backup as an annotation on the affected node.
func (r *Receipt) AddBackup(original, backupPath string) {
	// Find the node for this file and annotate it
	for i := range r.Nodes {
		if r.Nodes[i].Target == original {
			if r.Nodes[i].Annotations == nil {
				r.Nodes[i].Annotations = make(map[string]string)
			}
			r.Nodes[i].Annotations["backup"] = backupPath
			return
		}
	}
	// If no matching node found, create a standalone annotation node
	r.Nodes = append(r.Nodes, Node{
		ID:        original,
		Operation: "backup",
		Status:    "completed",
		Target:    original,
		Annotations: map[string]string{
			"backup_path": backupPath,
		},
	})
}

// AddSkipped records a skipped file as a node with status "skipped".
func (r *Receipt) AddSkipped(relTarget string) {
	r.Nodes = append(r.Nodes, Node{
		ID:     relTarget,
		Status: "skipped",
	})
}

// AddEdge adds a relationship edge between two nodes.
func (r *Receipt) AddEdge(from, to, relation string) {
	r.Edges = append(r.Edges, Edge{
		From:     from,
		To:       to,
		Relation: relation,
	})
}

// ComputeSummary calculates summary statistics from nodes.
func (r *Receipt) ComputeSummary() {
	r.Summary = Summary{}

	for _, n := range r.Nodes {
		if n.Status == "skipped" {
			r.Summary.Skipped++
			continue
		}
		if n.Operation == "delegate" {
			r.Summary.Delegated++
			continue
		}
		if n.Operation == "backup" {
			r.Summary.BackedUp++
			continue
		}

		r.Summary.TotalFiles++

		switch n.Operation {
		case "link":
			r.Summary.Links++
		case "expand":
			r.Summary.Templates++
		case "decrypt":
			r.Summary.Secrets++
		case "copy":
			r.Summary.Copies++
		}
	}
}

// Checksum computes a SHA256 checksum of the given content.
// Returns format "sha256:<hex>".
func Checksum(content []byte) string {
	hash := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(hash[:])
}

// ChecksumFile computes a SHA256 checksum of a file's contents.
// Returns format "sha256:<hex>" or empty string on error.
func ChecksumFile(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return Checksum(content)
}

// StateDir returns the writ state directory.
// Default: ~/.local/state/writ
func StateDir() string {
	return filepath.Join(cli.StateHome(), "writ")
}

// ReceiptsDir returns the receipts directory.
func ReceiptsDir() string {
	return filepath.Join(StateDir(), "receipts")
}

// LatestReceiptPath returns the path to the "latest" symlink.
func LatestReceiptPath() string {
	return filepath.Join(ReceiptsDir(), "latest.yaml")
}

// Write saves the receipt to the state directory.
// Returns the path where it was written.
func (r *Receipt) Write() (string, error) {
	r.ComputeSummary()

	dir := ReceiptsDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create receipts dir: %w", err)
	}

	// Filename: YYYY-MM-DDTHH-MM-SS.yaml
	filename := r.Timestamp.Format("2006-01-02T15-04-05") + ".yaml"
	path := filepath.Join(dir, filename)

	data, err := yaml.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("marshal receipt: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("write receipt: %w", err)
	}

	// Update "latest" symlink
	latestPath := LatestReceiptPath()
	_ = os.Remove(latestPath) // Ignore error if doesn't exist
	_ = os.Symlink(filename, latestPath)

	return path, nil
}

// LoadLatest loads the most recent receipt.
func LoadLatest() (*Receipt, error) {
	return Load(LatestReceiptPath())
}

// Load reads a receipt from a file, detecting version and converting
// legacy formats to the v4 graph representation.
func Load(path string) (*Receipt, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Peek at version field to determine format
	var peek struct {
		Version string `yaml:"version"`
	}
	if err := yaml.Unmarshal(data, &peek); err != nil {
		return nil, fmt.Errorf("parse receipt version: %w", err)
	}

	switch peek.Version {
	case "4":
		var r Receipt
		if err := yaml.Unmarshal(data, &r); err != nil {
			return nil, fmt.Errorf("parse v4 receipt: %w", err)
		}
		return &r, nil
	default: // "1", "2", "3" — legacy formats
		var lr LegacyReceipt
		if err := yaml.Unmarshal(data, &lr); err != nil {
			return nil, fmt.Errorf("parse legacy receipt: %w", err)
		}
		return lr.ToGraph(), nil
	}
}

// JSON returns the receipt as JSON.
func (r *Receipt) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// YAML returns the receipt as YAML.
func (r *Receipt) YAML() ([]byte, error) {
	return yaml.Marshal(r)
}

// String returns a human-readable summary of the receipt.
func (r *Receipt) String() string {
	r.ComputeSummary()
	return fmt.Sprintf("%d files (%d links, %d templates, %d secrets), %d skipped, %d delegated",
		r.Summary.TotalFiles,
		r.Summary.Links,
		r.Summary.Templates,
		r.Summary.Secrets,
		r.Summary.Skipped,
		r.Summary.Delegated,
	)
}

// detectPlatform returns the current OS and architecture.
func detectPlatform() Platform {
	return Platform{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}
}

// DependencyGraph represents the coarse-grained dependency view
// projected from the execution graph.
type DependencyGraph struct {
	Nodes []DependencyNode `json:"nodes" yaml:"nodes"`
	Edges []DependencyEdge `json:"edges" yaml:"edges"`
}

// DependencyNode represents a project (writ) or package (lore).
type DependencyNode struct {
	ID   string `json:"id" yaml:"id"`
	Tool string `json:"tool" yaml:"tool"`
}

// DependencyEdge represents a structural relationship between projects/packages.
type DependencyEdge struct {
	From     string `json:"from" yaml:"from"`
	To       string `json:"to" yaml:"to"`
	Relation string `json:"relation" yaml:"relation"`
	Artifact string `json:"artifact,omitempty" yaml:"artifact,omitempty"`
}

// ToDependencyGraph projects the execution graph to a dependency graph.
// For writ: collapses file nodes into project nodes, preserves delegate edges.
func (r *Receipt) ToDependencyGraph() *DependencyGraph {
	dg := &DependencyGraph{}

	// Collect unique projects as dependency nodes
	projectSeen := make(map[string]bool)
	for _, n := range r.Nodes {
		if n.Project != "" && !projectSeen[n.Project] {
			projectSeen[n.Project] = true
			dg.Nodes = append(dg.Nodes, DependencyNode{
				ID:   n.Project,
				Tool: r.Tool,
			})
		}
		// Delegate nodes create cross-tool dependency nodes
		if n.Operation == "delegate" && n.DelegateTo != "" {
			delegateID := n.DelegateTo + ":" + n.ID
			if !projectSeen[delegateID] {
				projectSeen[delegateID] = true
				dg.Nodes = append(dg.Nodes, DependencyNode{
					ID:   delegateID,
					Tool: n.DelegateTo,
				})
			}
		}
	}

	// Promote edges: keep only inter-project and cross-tool edges
	for _, e := range r.Edges {
		dg.Edges = append(dg.Edges, DependencyEdge{
			From:     e.From,
			To:       e.To,
			Relation: e.Relation,
			Artifact: e.Artifact,
		})
	}

	// Add implicit delegate edges from project to delegate target
	for _, n := range r.Nodes {
		if n.Operation == "delegate" && n.DelegateTo != "" {
			dg.Edges = append(dg.Edges, DependencyEdge{
				From:     n.Project,
				To:       n.DelegateTo + ":" + n.ID,
				Relation: "delegates",
			})
		}
	}

	return dg
}
