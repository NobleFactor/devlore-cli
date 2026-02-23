// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"gopkg.in/yaml.v3"
)

// EntryType distinguishes between package and file entries.
type EntryType string

const (
	// EntryPackage represents a lore package lifecycle entry.
	EntryPackage EntryType = "package"
	// EntryFile represents a project file entry (link/copy/expand/decrypt).
	EntryFile EntryType = "file"
)

// HistoryRecord represents a single action on an entry from a receipt.
type HistoryRecord struct {
	// Timestamp is when this action occurred.
	Timestamp time.Time `json:"timestamp" yaml:"timestamp"`

	// Receipt is the filename of the receipt that recorded this.
	Receipt string `json:"receipt" yaml:"receipt"`

	// Tool is which tool created this record ("lore" or "writ").
	Tool string `json:"tool" yaml:"tool"`

	// Action performed: link, copy, render, decrypt, install, etc.
	Action string `json:"action" yaml:"action"`

	// Status of this action: completed, skipped, failed.
	Status op.NodeStatus `json:"status" yaml:"status"`
}

// PackageEntry represents a lore package's lifecycle history.
type PackageEntry struct {
	// Name is the package name (e.g., "docker").
	Name string `json:"name" yaml:"name"`

	// History of lifecycle actions, ordered by time.
	History []HistoryRecord `json:"history" yaml:"history"`
}

// LastAction returns the most recent action, or nil if no history.
func (e *PackageEntry) LastAction() *HistoryRecord {
	if len(e.History) == 0 {
		return nil
	}
	return &e.History[len(e.History)-1]
}

// FileEntry represents a project file's deployment history.
type FileEntry struct {
	// Target is the relative target path (e.g., ".bashrc").
	Target string `json:"target" yaml:"target"`

	// Source is the absolute source path.
	Source string `json:"source" yaml:"source"`

	// Project this file belongs to.
	Project string `json:"project" yaml:"project"`

	// Layer is the repository layer (base, team, personal).
	Layer string `json:"layer,omitempty" yaml:"layer,omitempty"`

	// History of deployment actions, ordered by time.
	History []HistoryRecord `json:"history" yaml:"history"`
}

// LastAction returns the most recent action, or nil if no history.
func (e *FileEntry) LastAction() *HistoryRecord {
	if len(e.History) == 0 {
		return nil
	}
	return &e.History[len(e.History)-1]
}

// IsCopied returns true if the latest action was a copy (not symlink).
func (e *FileEntry) IsCopied() bool {
	last := e.LastAction()
	if last == nil {
		return false
	}
	return last.Action == "file.copy"
}

// IsLinked returns true if the latest action was a symlink.
func (e *FileEntry) IsLinked() bool {
	last := e.LastAction()
	if last == nil {
		return false
	}
	return last.Action == "file.link"
}

// LastActionName returns the action name from the latest deployment.
func (e *FileEntry) LastActionName() string {
	last := e.LastAction()
	if last == nil {
		return ""
	}
	return last.Action
}

// FileTreeNode represents a node in the target filesystem tree.
type FileTreeNode struct {
	// Name is the filename or directory name.
	Name string `json:"name" yaml:"name"`

	// IsDir is true if this is a directory.
	IsDir bool `json:"is_dir" yaml:"is_dir"`

	// Entry is the file entry (nil for directories).
	Entry *FileEntry `json:"entry,omitempty" yaml:"entry,omitempty"`

	// Children are the child nodes (nil for files).
	Children map[string]*FileTreeNode `json:"children,omitempty" yaml:"children,omitempty"`
}

// FileTree provides both flat and hierarchical access to files.
type FileTree struct {
	// Root is the target root path (e.g., $HOME).
	Root string `json:"root" yaml:"root"`

	// Entries provides flat lookup by relative target path.
	Entries map[string]*FileEntry `json:"entries" yaml:"entries"`

	// Tree is the hierarchical view.
	Tree *FileTreeNode `json:"tree" yaml:"tree"`
}

// ForProject returns all file entries for a specific project.
func (t *FileTree) ForProject(project string) map[string]*FileEntry {
	result := make(map[string]*FileEntry)
	for path, entry := range t.Entries {
		if entry.Project == project {
			result[path] = entry
		}
	}
	return result
}

// CopiedFiles returns all entries that were copied (not symlinked).
func (t *FileTree) CopiedFiles() map[string]*FileEntry {
	result := make(map[string]*FileEntry)
	for path, entry := range t.Entries {
		if entry.IsCopied() {
			result[path] = entry
		}
	}
	return result
}

// LinkedFiles returns all entries that are symlinks.
func (t *FileTree) LinkedFiles() map[string]*FileEntry {
	result := make(map[string]*FileEntry)
	for path, entry := range t.Entries {
		if entry.IsLinked() {
			result[path] = entry
		}
	}
	return result
}

// Projects returns a list of all projects with files in the tree.
func (t *FileTree) Projects() []string {
	seen := make(map[string]bool)
	for _, entry := range t.Entries {
		if entry.Project != "" {
			seen[entry.Project] = true
		}
	}

	projects := make([]string, 0, len(seen))
	for p := range seen {
		projects = append(projects, p)
	}
	sort.Strings(projects)
	return projects
}

// buildTree constructs the hierarchical tree from flat entries.
func (t *FileTree) buildTree() {
	t.Tree = &FileTreeNode{
		Name:     filepath.Base(t.Root),
		IsDir:    true,
		Children: make(map[string]*FileTreeNode),
	}

	for relPath, entry := range t.Entries {
		t.insertPath(relPath, entry)
	}
}

// insertPath inserts a file entry into the tree at the given path.
func (t *FileTree) insertPath(relPath string, entry *FileEntry) {
	parts := strings.Split(relPath, string(filepath.Separator))
	current := t.Tree

	// Navigate/create directories
	for i := 0; i < len(parts)-1; i++ {
		name := parts[i]
		if current.Children == nil {
			current.Children = make(map[string]*FileTreeNode)
		}
		if _, ok := current.Children[name]; !ok {
			current.Children[name] = &FileTreeNode{
				Name:     name,
				IsDir:    true,
				Children: make(map[string]*FileTreeNode),
			}
		}
		current = current.Children[name]
	}

	// Insert file
	fileName := parts[len(parts)-1]
	if current.Children == nil {
		current.Children = make(map[string]*FileTreeNode)
	}
	current.Children[fileName] = &FileTreeNode{
		Name:  fileName,
		IsDir: false,
		Entry: entry,
	}
}

// StateView is a read-only view over multiple execution graphs.
// It represents "what we believe happened" over a time interval.
type StateView struct {
	// Since is the start of the time window (inclusive).
	Since time.Time `json:"since" yaml:"since"`

	// Until is the end of the time window (inclusive).
	Until time.Time `json:"until" yaml:"until"`

	// ReceiptCount is the number of receipts included in this view.
	ReceiptCount int `json:"receipt_count" yaml:"receipt_count"`

	// Packages maps package names to their lifecycle history.
	Packages map[string]*PackageEntry `json:"packages" yaml:"packages"`

	// Files provides file entry access (flat and tree).
	Files *FileTree `json:"files" yaml:"files"`
}

// ViewOptions configures how the view is built.
type ViewOptions struct {
	// Since filters to receipts after this time (zero = no lower bound).
	Since time.Time

	// Until filters to receipts before this time (zero = no upper bound).
	Until time.Time

	// Tools filters to specific tools (empty = all tools).
	Tools []string
}

// StateViewBuilder creates StateViews from receipts.
type StateViewBuilder struct {
	opts ViewOptions
}

// NewStateViewBuilder creates a new builder with the given options.
func NewStateViewBuilder(opts ViewOptions) *StateViewBuilder {
	return &StateViewBuilder{opts: opts}
}

// Build loads all receipts from the directory and builds a StateView.
func (b *StateViewBuilder) Build(receiptsDir string) (*StateView, error) {
	graphs, err := b.loadReceipts(receiptsDir)
	if err != nil {
		return nil, err
	}
	return b.BuildFrom(graphs), nil
}

// BuildFrom creates a StateView from the given graphs.
func (b *StateViewBuilder) BuildFrom(graphs []*op.Graph) *StateView {
	// Filter graphs according to options
	var filtered []*op.Graph
	for _, g := range graphs {
		if b.includeGraph(g) {
			filtered = append(filtered, g)
		}
	}

	view := &StateView{
		Since:        b.opts.Since,
		Until:        b.opts.Until,
		ReceiptCount: len(filtered),
		Packages:     make(map[string]*PackageEntry),
		Files: &FileTree{
			Entries: make(map[string]*FileEntry),
		},
	}

	// Sort graphs by timestamp for consistent history ordering
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp.Before(filtered[j].Timestamp)
	})

	for _, g := range filtered {
		b.processGraph(view, g)
	}

	// Set target root from first graph that has it
	for _, g := range filtered {
		if g.Context.TargetRoot != "" && view.Files.Root == "" {
			view.Files.Root = g.Context.TargetRoot
			break
		}
	}

	// Build the file tree hierarchy
	view.Files.buildTree()

	return view
}

// loadReceipts loads all receipt files from the directory.
func (b *StateViewBuilder) loadReceipts(dir string) ([]*op.Graph, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No receipts directory is OK
		}
		return nil, err
	}

	var graphs []*op.Graph
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Skip symlinks (like "writ-latest.yaml")
		if strings.HasSuffix(name, "-latest.yaml") {
			continue
		}
		if !strings.HasSuffix(name, ".yaml") {
			continue
		}

		path := filepath.Join(dir, name)
		g, err := b.loadReceipt(path)
		if err != nil {
			continue // Skip unreadable receipts
		}

		if !b.includeGraph(g) {
			continue
		}

		graphs = append(graphs, g)
	}

	return graphs, nil
}

// loadReceipt loads a single receipt file.
func (b *StateViewBuilder) loadReceipt(path string) (*op.Graph, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var g op.Graph
	if err := yaml.Unmarshal(data, &g); err != nil {
		return nil, err
	}

	return &g, nil
}

// includeGraph checks if a graph should be included based on options.
func (b *StateViewBuilder) includeGraph(g *op.Graph) bool {
	// Filter by time
	if !b.opts.Since.IsZero() && g.Timestamp.Before(b.opts.Since) {
		return false
	}
	if !b.opts.Until.IsZero() && g.Timestamp.After(b.opts.Until) {
		return false
	}

	// Filter by tool
	if len(b.opts.Tools) > 0 {
		found := false
		for _, t := range b.opts.Tools {
			if g.Tool == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// isTransformOnlyNode returns true if the node is an intermediate transform.
func isTransformOnlyNode(node *op.Node) bool {
	switch node.ActionName() {
	case "template.render", "encryption.decrypt":
		return true
	}
	return false
}

// processGraph adds nodes from a graph to the view.
func (b *StateViewBuilder) processGraph(view *StateView, g *op.Graph) {
	receiptName := g.Filename()

	for _, node := range g.Nodes {
		// Skip skipped nodes and intermediate transform nodes
		if node.Status == op.StatusSkipped || isTransformOnlyNode(node) {
			continue
		}

		record := HistoryRecord{
			Timestamp: g.Timestamp,
			Receipt:   receiptName,
			Tool:      g.Tool,
			Action:    node.ActionName(),
			Status:    node.Status,
		}

		// Determine if this is a package or file node
		if b.isPackageNode(node) {
			b.addPackageRecord(view, node, record)
		} else {
			b.addFileRecord(view, node, record)
		}
	}
}

// isPackageNode determines if a node represents a package lifecycle action.
func (b *StateViewBuilder) isPackageNode(node *op.Node) bool {
	switch node.ActionName() {
	case "pkg.prepare", "pkg.install", "pkg.verify", "pkg.upgrade", "pkg.uninstall", "pkg.cleanup",
		"pkg.remove":
		return true
	}
	return false
}

// addPackageRecord adds a package lifecycle record to the view.
func (b *StateViewBuilder) addPackageRecord(view *StateView, node *op.Node, record HistoryRecord) {
	name := node.ID // Package name is the node ID

	entry, ok := view.Packages[name]
	if !ok {
		entry = &PackageEntry{
			Name:    name,
			History: make([]HistoryRecord, 0),
		}
		view.Packages[name] = entry
	}

	entry.History = append(entry.History, record)
}

// addFileRecord adds a file deployment record to the view.
func (b *StateViewBuilder) addFileRecord(view *StateView, node *op.Node, record HistoryRecord) {
	relTarget := node.ID                         // Relative target path is the node ID
	source, _ := node.GetSlot("source").(string) //nolint:errcheck // zero value (empty) is acceptable

	entry, ok := view.Files.Entries[relTarget]
	if !ok {
		entry = &FileEntry{
			Target:  relTarget,
			Source:  source,
			Project: node.Project,
			Layer:   node.Layer,
			History: make([]HistoryRecord, 0),
		}
		view.Files.Entries[relTarget] = entry
	} else {
		// Update source/project/layer from latest record
		if source != "" {
			entry.Source = source
		}
		if node.Project != "" {
			entry.Project = node.Project
		}
		if node.Layer != "" {
			entry.Layer = node.Layer
		}
	}

	entry.History = append(entry.History, record)
}

// Summary returns counts of packages, linked files, and copied files.
func (v *StateView) Summary() (packages, links, copied int) {
	packages = len(v.Packages)
	for _, entry := range v.Files.Entries {
		if entry.IsCopied() {
			copied++
		} else {
			links++
		}
	}
	return
}
