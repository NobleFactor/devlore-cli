// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package tree

import (
	"os"
	"path/filepath"
	"strings"
)

// DelegateFiles are filenames that should be delegated to lore.
var DelegateFiles = []string{
	"packages.manifest",
}

// Node represents a file operation in the deployment tree.
type Node struct {
	// Source is the absolute path to the source file in the dotfiles repo.
	Source string `json:"source"`

	// Target is the absolute path to the target file (e.g., in $HOME).
	Target string `json:"target"`

	// RelSource is the relative path from the project directory.
	RelSource string `json:"rel_source"`

	// RelTarget is the relative path from the target root.
	RelTarget string `json:"rel_target"`

	// Operations to perform (outside-in order for chained extensions).
	Operations Operations `json:"operations"`

	// Mode is the file mode for the target (0600 for secrets, 0 to preserve).
	Mode os.FileMode `json:"mode,omitempty"`

	// Project is the project this file belongs to.
	Project string `json:"project"`

	// Suffixes are the segment suffixes for the source directory.
	Suffixes []string `json:"suffixes,omitempty"`
}

// ProcessingPipeline determines operations from a filename.
// Extensions are processed outside-in (like .tar.gz).
//
// Examples:
//
//	"foo"                → "foo",              [link]
//	"foo.template"       → "foo",              [expand, copy]
//	"foo.age"            → "foo",              [decrypt, copy]
//	"foo.template.age"   → "foo",              [decrypt, expand, copy]
//	"packages.manifest"  → "packages.manifest" [delegate]
func ProcessingPipeline(filename string) (targetName string, ops Operations) {
	name := filename
	baseName := filepath.Base(name)

	// Check for delegate files (e.g., packages.manifest → lore)
	for _, df := range DelegateFiles {
		if baseName == df {
			return name, Operations{OpDelegate}
		}
	}

	var pipeline Operations

	// Process extensions outside-in
	// .age is outermost (decrypt first)
	if strings.HasSuffix(name, ".age") {
		name = strings.TrimSuffix(name, ".age")
		pipeline = append(pipeline, OpDecrypt)
	}

	// .template is inner (expand after decrypt)
	if strings.HasSuffix(name, ".template") {
		name = strings.TrimSuffix(name, ".template")
		pipeline = append(pipeline, OpExpand)
	}

	// Determine final operation
	if len(pipeline) > 0 {
		// After decrypt or expand, we copy the result
		pipeline = append(pipeline, OpCopy)
	} else {
		// Plain file: just link
		pipeline = append(pipeline, OpLink)
	}

	return name, pipeline
}

// IsSecret returns true if this node involves decryption.
func (n *Node) IsSecret() bool {
	for _, op := range n.Operations {
		if op == OpDecrypt {
			return true
		}
	}
	return false
}

// IsTemplate returns true if this node involves template expansion.
func (n *Node) IsTemplate() bool {
	for _, op := range n.Operations {
		if op == OpExpand {
			return true
		}
	}
	return false
}

// IsLink returns true if this node is a simple symlink.
func (n *Node) IsLink() bool {
	return len(n.Operations) == 1 && n.Operations[0] == OpLink
}

// IsDelegate returns true if this node should be delegated to another tool.
func (n *Node) IsDelegate() bool {
	for _, op := range n.Operations {
		if op == OpDelegate {
			return true
		}
	}
	return false
}
