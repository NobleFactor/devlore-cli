// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// ConflictType describes the kind of conflict at a target path.
type ConflictType int

const (
	// ConflictNone indicates no conflict exists.
	ConflictNone ConflictType = iota
	// ConflictRegularFile indicates a regular file exists at target.
	ConflictRegularFile
	// ConflictDirectory indicates a directory exists at target.
	ConflictDirectory
	// ConflictForeignSymlink indicates a symlink pointing elsewhere exists.
	ConflictForeignSymlink
	// ConflictOurSymlink indicates our symlink already exists (no action needed).
	ConflictOurSymlink
)

// Conflict represents a pre-flight detected conflict.
type Conflict struct {
	Node         *op.Node
	Type         ConflictType
	ExistingPath string // For symlinks, where it points
	Message      string
}

// PreflightResult contains the results of pre-flight conflict detection.
type PreflightResult struct {
	Conflicts   []Conflict
	AlreadyDone []Conflict // Symlinks that already point correctly
	Ready       []*op.Node // Nodes ready to deploy (no conflict)
}

// HasConflicts returns true if any conflicts were detected.
func (p *PreflightResult) HasConflicts() bool {
	return len(p.Conflicts) > 0
}

// Preflight performs pre-flight conflict detection without modifying anything.
// Only applies to nodes with file actions (link, copy).
func Preflight(graph *op.Graph) *PreflightResult {
	result := &PreflightResult{}

	for _, node := range graph.Nodes {
		// Skip nodes that don't write to target
		if !nodeWritesToTarget(node) {
			result.Ready = append(result.Ready, node)
			continue
		}

		conflict := detectConflict(node)
		switch conflict.Type {
		case ConflictNone:
			result.Ready = append(result.Ready, node)
		case ConflictOurSymlink:
			result.AlreadyDone = append(result.AlreadyDone, conflict)
		default:
			result.Conflicts = append(result.Conflicts, conflict)
		}
	}

	return result
}

// nodeWritesToTarget returns true if the node's action produces a file at
// node's "path" slot (link or copy).
func nodeWritesToTarget(node *op.Node) bool {
	path, _ := node.GetSlot("path").(string) //nolint:errcheck // zero value (empty) is acceptable
	if path == "" {
		return false
	}
	return node.ActionName() == "file.link" || node.ActionName() == "file.copy"
}

// detectConflict checks if a target path has a conflict.
func detectConflict(node *op.Node) Conflict {
	path, _ := node.GetSlot("path").(string) //nolint:errcheck // zero value (empty) is acceptable
	if path == "" {
		return Conflict{Node: node, Type: ConflictNone}
	}

	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return Conflict{Node: node, Type: ConflictNone}
	}
	if err != nil {
		return Conflict{
			Node:    node,
			Type:    ConflictRegularFile,
			Message: fmt.Sprintf("cannot stat: %v", err),
		}
	}

	if info.IsDir() {
		return Conflict{
			Node:    node,
			Type:    ConflictDirectory,
			Message: fmt.Sprintf("directory exists at %s", path),
		}
	}

	if info.Mode()&os.ModeSymlink != 0 {
		linkTarget, err := os.Readlink(path)
		if err != nil {
			return Conflict{
				Node:    node,
				Type:    ConflictForeignSymlink,
				Message: fmt.Sprintf("cannot read symlink: %v", err),
			}
		}

		source, _ := node.GetSlot("source").(string) //nolint:errcheck // zero value (empty) is acceptable
		if linkTarget == source {
			return Conflict{
				Node:         node,
				Type:         ConflictOurSymlink,
				ExistingPath: linkTarget,
				Message:      "already deployed",
			}
		}

		return Conflict{
			Node:         node,
			Type:         ConflictForeignSymlink,
			ExistingPath: linkTarget,
			Message:      fmt.Sprintf("symlink exists pointing to %s", linkTarget),
		}
	}

	return Conflict{
		Node:    node,
		Type:    ConflictRegularFile,
		Message: fmt.Sprintf("file exists at %s (%d bytes)", path, info.Size()),
	}
}

// ResolveResources verifies that all discovered resources in the catalog
// exist on the target machine. Discovery entries are created by
// catalog.Resolve during planning — they represent URIs referenced as
// Resource-typed parameters.
//
// Only file:// scheme URIs are checked (os.Stat). Other schemes (git,
// pkg, svc) are skipped — their resolution requires provider-specific
// logic that runs during execution.
//
// Returns an error listing all missing sources. Callers should invoke
// this explicitly when they know all discovery entries represent genuine
// sources (inputs that must already exist). The executor does NOT call
// this automatically because the planned bridge currently registers both
// source and destination parameters via catalog.Resolve — destination
// files may not exist yet (they will be created by the graph).
func ResolveResources(catalog *op.ResourceCatalog) error {
	if catalog == nil {
		return nil
	}

	uris := catalog.DiscoveryURIs()
	sort.Strings(uris) // deterministic error messages

	var missing []string
	for _, rawURI := range uris {
		u, err := url.Parse(rawURI)
		if err != nil {
			continue
		}
		if u.Scheme != op.SchemeFile {
			continue
		}
		if _, err := os.Stat(u.Path); errors.Is(err, os.ErrNotExist) {
			missing = append(missing, u.Path)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("pre-flight: %d missing source(s): %s",
			len(missing), strings.Join(missing, ", "))
	}
	return nil
}
