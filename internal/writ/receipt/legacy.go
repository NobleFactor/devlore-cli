// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package receipt

import (
	"time"
)

// LegacyReceipt is the v2/v3 receipt format (flat entries array).
// Retained for backward-compatible reads.
type LegacyReceipt struct {
	Version    string            `yaml:"version"`
	Timestamp  time.Time         `yaml:"timestamp"`
	SourceRoot string            `yaml:"source_root"`
	TargetRoot string            `yaml:"target_root"`
	Projects   []string          `yaml:"projects"`
	Segments   map[string]string `yaml:"segments"`
	Entries    []LegacyEntry     `yaml:"entries"`
	Backups    []Backup          `yaml:"backups,omitempty"`
	Skipped    []string          `yaml:"skipped,omitempty"`
	Delegated  []string          `yaml:"delegated,omitempty"`
	Summary    Summary           `yaml:"summary"`
	Signature  *Signature        `yaml:"signature,omitempty"`
}

// LegacyEntry records a single deployed file in v2/v3 format.
type LegacyEntry struct {
	Source          string   `yaml:"source"`
	Target          string   `yaml:"target"`
	RelTarget       string   `yaml:"rel_target"`
	Operations      []string `yaml:"operations"`
	Project         string   `yaml:"project"`
	AlreadyDeployed bool     `yaml:"already_deployed,omitempty"`
	SourceChecksum  string   `yaml:"source_checksum,omitempty"`
	TargetChecksum  string   `yaml:"target_checksum,omitempty"`
}

// ToGraph converts a legacy v2/v3 receipt to the v4 graph format.
func (lr *LegacyReceipt) ToGraph() *Receipt {
	r := &Receipt{
		Version:   CurrentVersion,
		Format:    "graph",
		Timestamp: lr.Timestamp,
		Tool:      "writ",
		Platform:  detectPlatform(),
		Context: WritContext{
			SourceRoot: lr.SourceRoot,
			TargetRoot: lr.TargetRoot,
			Projects:   lr.Projects,
			Segments:   lr.Segments,
		},
		Roots:     lr.Projects,
		Nodes:     make([]Node, 0, len(lr.Entries)+len(lr.Delegated)+len(lr.Skipped)),
		Edges:     nil,
		Signature: lr.Signature,
	}

	// Convert entries to nodes
	for _, e := range lr.Entries {
		op := "link"
		if len(e.Operations) > 0 {
			// Use the primary operation (first non-copy operation, or first)
			op = primaryOperation(e.Operations)
		}

		status := "completed"
		if e.AlreadyDeployed {
			status = "completed"
		}

		node := Node{
			ID:             e.RelTarget,
			Operation:      op,
			Status:         status,
			Source:         e.Source,
			Target:         e.Target,
			Project:        e.Project,
			SourceChecksum: e.SourceChecksum,
			TargetChecksum: e.TargetChecksum,
		}

		if e.AlreadyDeployed {
			if node.Annotations == nil {
				node.Annotations = make(map[string]string)
			}
			node.Annotations["already_deployed"] = "true"
		}

		r.Nodes = append(r.Nodes, node)
	}

	// Convert delegated entries to delegate nodes
	for _, source := range lr.Delegated {
		node := Node{
			ID:         delegatedNodeID(source, lr.SourceRoot),
			Operation:  "delegate",
			Status:     "completed",
			Source:     source,
			DelegateTo: "lore",
		}
		r.Nodes = append(r.Nodes, node)
	}

	// Convert skipped entries to skipped nodes
	for _, relTarget := range lr.Skipped {
		node := Node{
			ID:     relTarget,
			Status: "skipped",
		}
		r.Nodes = append(r.Nodes, node)
	}

	// Compute summary from converted data
	r.ComputeSummary()

	return r
}

// primaryOperation returns the meaningful operation from a pipeline.
// In v2/v3, operations are a pipeline like ["expand", "copy"] or ["decrypt", "copy"].
// We want the first non-"copy" operation, as "copy" is an implementation detail.
func primaryOperation(ops []string) string {
	for _, op := range ops {
		if op != "copy" {
			return op
		}
	}
	if len(ops) > 0 {
		return ops[0]
	}
	return "link"
}

// delegatedNodeID computes a node ID for a delegated file.
func delegatedNodeID(source, sourceRoot string) string {
	if len(source) > len(sourceRoot)+1 {
		return source[len(sourceRoot)+1:]
	}
	return source
}
