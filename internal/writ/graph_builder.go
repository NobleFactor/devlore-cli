// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package writ

import (
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/lore"
	"github.com/NobleFactor/devlore-cli/internal/writ/secrets"
	"github.com/NobleFactor/devlore-cli/internal/writ/tree"
)

// CurrentVersion is the graph format version.
const CurrentVersion = "6"

// GraphBuilder is the interface for all graph builders.
type GraphBuilder interface {
	Build() (*execution.Graph, error)
}

// NewGraph creates an execution.Graph with common fields populated for writ.
func NewGraph(cfg *Config) *execution.Graph {
	return &execution.Graph{
		Version:   CurrentVersion,
		Tool:      cfg.Tool,
		Timestamp: time.Now(),
		State:     execution.StatePending,
		Platform: execution.Platform{
			OS:   runtime.GOOS,
			Arch: runtime.GOARCH,
		},
		Context: execution.GraphContext{
			SourceRoot: cfg.SourceRoot,
			TargetRoot: cfg.TargetRoot,
			Projects:   cfg.Projects,
			Segments:   cfg.SegmentMap(),
		},
		Nodes: make([]*execution.Node, 0),
	}
}

// BuildTree walks the source directories and populates the graph with file nodes.
// This processes layers in order (base → team → personal) with collision detection.
// Returns discovered manifest source paths instead of creating nodes for them.
func BuildTree(g *execution.Graph, cfg *Config, reg *execution.ActionRegistry) (manifests []string, err error) {
	result, err := tree.Build(tree.BuildConfig{
		SourceRoot: cfg.SourceRoot,
		TargetRoot: cfg.TargetRoot,
		Sources:    cfg.LayerSources,
		Projects:   cfg.Projects,
		Segments:   cfg.Segments,
	})
	if err != nil {
		return nil, fmt.Errorf("build tree: %w", err)
	}

	// Convert file entries to graph nodes.
	// Multi-op pipelines from tree (e.g., ["render", "copy"]) become node chains.
	// Manifest files are collected separately for planner resolution.
	for _, f := range result.Files {
		ops := f.Operations

		// Collect manifest files instead of creating nodes
		if len(ops) == 1 && ops[0] == "manifest.resolve" {
			manifests = append(manifests, f.Source)
			continue
		}

		if len(ops) == 1 {
			// Single operation — single node
			node := &execution.Node{
				ID:      f.ID,
				Action:  reg.MustGet(ops[0]),
				Status:  execution.StatusPending,
				Project: f.Project,
				Layer:   f.Layer,
			}
			node.SetSlotImmediate("source", f.Source)
			node.SetSlotImmediate("path", f.Target)
			if f.Mode != 0 {
				node.SetSlotImmediate("mode", f.Mode)
			}
			g.Nodes = append(g.Nodes, node)
		} else {
			// Multi-op pipeline → node chain
			var prevNode *execution.Node
			for i, op := range ops {
				isLast := (i == len(ops) - 1)
				nodeID := f.ID
				if !isLast {
					nodeID = f.ID + ":" + op
				}

				node := &execution.Node{
					ID:      nodeID,
					Action:  reg.MustGet(op),
					Status:  execution.StatusPending,
					Project: f.Project,
					Layer:   f.Layer,
				}
				if i == 0 {
					node.SetSlotImmediate("source", f.Source)
				}
				if isLast {
					node.SetSlotImmediate("path", f.Target)
					if f.Mode != 0 {
						node.SetSlotImmediate("mode", f.Mode)
					}
				}
				g.Nodes = append(g.Nodes, node)

				if prevNode != nil {
					g.Edges = append(g.Edges, execution.Edge{
						From: prevNode.ID, To: node.ID,
					})
				}
				prevNode = node
			}
		}
	}

	// Record collisions
	for _, c := range result.Collisions {
		g.Collisions = append(g.Collisions, execution.Collision{
			Target:            c.Target,
			Winner:            c.Winner,
			WinnerLayer:       c.WinnerLayer,
			WinnerSpecificity: c.WinnerSpecificity,
			Loser:             c.Loser,
			LoserLayer:        c.LoserLayer,
			LoserSpecificity:  c.LoserSpecificity,
		})
	}

	return manifests, nil
}

// ConfigureEngine creates and configures an execution engine for the graph.
func ConfigureEngine(cfg *Config) (*execution.GraphExecutor, error) {
	// Build engine data
	engineData := graphBuiltinTemplateData(cfg.SegmentMap())
	for k, v := range cfg.TemplateData {
		engineData[k] = v
	}

	// Set up SOPS decryptor
	secretsMgr, _ := secrets.NewManager(cfg.SourceRoot)
	if secretsMgr != nil {
		engineData["decryptor"] = secretsMgr.Decryptor()
	}

	// Create engine
	engine := execution.NewGraphExecutor(execution.ExecutorOptions{
		DryRun:             cfg.DryRun,
		Data:               engineData,
		ConflictResolution: cfg.ConflictResolution,
	})

	return engine, nil
}

// graphBuiltinTemplateData returns the built-in template variables for graph building.
func graphBuiltinTemplateData(segments map[string]string) map[string]any {
	data := make(map[string]any)

	// Add segments
	for k, v := range segments {
		data[k] = v
	}

	// Add common builtins
	data["os"] = runtime.GOOS
	data["arch"] = runtime.GOARCH

	return data
}

// ----------------------------------------------------------------------------
// DeployGraphBuilder
// ----------------------------------------------------------------------------

// DeployGraphBuilder builds execution graphs for deploy operations.
type DeployGraphBuilder struct {
	config  *Config
	reg     *execution.ActionRegistry
	Planner *lore.Planner // nil means skip manifest resolution
}

// NewDeployGraphBuilder creates a new deploy graph builder.
func NewDeployGraphBuilder(cfg *DeployConfig, reg *execution.ActionRegistry) *DeployGraphBuilder {
	return &DeployGraphBuilder{config: &cfg.Config, reg: reg}
}

// Build creates an execution graph for a deploy operation.
func (b *DeployGraphBuilder) Build() (*execution.Graph, error) {
	// Create the graph
	g := NewGraph(b.config)

	// Build the file tree and populate nodes
	manifests, err := BuildTree(g, b.config, b.reg)
	if err != nil {
		return nil, err
	}

	// Resolve manifest files through the planner
	if b.Planner != nil && len(manifests) > 0 {
		for _, m := range manifests {
			if _, err := b.Planner.PlanPackages(g, m); err != nil {
				return nil, fmt.Errorf("manifest %s: %w", m, err)
			}
		}
	}

	return g, nil
}

// ----------------------------------------------------------------------------
// DecommissionGraphBuilder
// ----------------------------------------------------------------------------

// DecommissionGraphBuilder builds execution graphs for decommission operations.
type DecommissionGraphBuilder struct {
	config *Config
	reg    *execution.ActionRegistry
	view   *execution.StateView
	force  bool
}

// NewDecommissionGraphBuilder creates a new decommission graph builder.
func NewDecommissionGraphBuilder(cfg *DecommissionConfig, view *execution.StateView, reg *execution.ActionRegistry) *DecommissionGraphBuilder {
	return &DecommissionGraphBuilder{
		config: &cfg.Config,
		reg:    reg,
		view:   view,
		force:  cfg.Force,
	}
}

// Build creates an execution graph for a decommission operation.
func (b *DecommissionGraphBuilder) Build() (*execution.Graph, error) {
	// Create the graph
	g := NewGraph(b.config)
	g.Context.TargetRoot = b.view.Files.Root

	// Build project set for filtering
	projects := projectSet(b.config.Projects)

	// Convert state entries to removal nodes
	for relTarget, entry := range b.view.Files.Entries {
		// Filter by project if specified
		if len(projects) > 0 && !projects[entry.Project] {
			continue
		}

		// Determine operation: unlink for symlinks, remove for copied files
		op := "file.unlink"
		if entry.IsCopied() {
			op = "file.remove"
		}

		// Check for local modifications on copied files
		targetChecksum := entry.TargetChecksum()
		if entry.IsCopied() && targetChecksum != "" {
			currentChecksum := execution.ChecksumFile(filepath.Join(b.view.Files.Root, relTarget))
			if currentChecksum != "" && currentChecksum != targetChecksum {
				if !b.force {
					// Skip modified files unless --force
					continue
				}
			}
		}

		target := filepath.Join(b.view.Files.Root, relTarget)
		node := &execution.Node{
			ID:      relTarget,
			Action:  b.reg.MustGet(op),
			Status:  execution.StatusPending,
			Project: entry.Project,
			Layer:   entry.Layer,
		}
		node.SetSlotImmediate("source", entry.Source)
		node.SetSlotImmediate("path", target)

		g.Nodes = append(g.Nodes, node)
	}

	return g, nil
}

// ----------------------------------------------------------------------------
// UpgradeGraphBuilder
// ----------------------------------------------------------------------------

// UpgradeGraphBuilder builds execution graphs for upgrade operations.
type UpgradeGraphBuilder struct {
	config *Config
	view   *execution.StateView
	force  bool
}

// NewUpgradeGraphBuilder creates a new upgrade graph builder.
func NewUpgradeGraphBuilder(cfg *UpgradeConfig, view *execution.StateView) *UpgradeGraphBuilder {
	return &UpgradeGraphBuilder{
		config: &cfg.Config,
		view:   view,
		force:  cfg.Force,
	}
}

// Build creates an execution graph for an upgrade operation.
func (b *UpgradeGraphBuilder) Build() (*execution.Graph, error) {
	// TODO: implement upgrade graph building
	return nil, fmt.Errorf("upgrade graph building not yet implemented")
}

// ----------------------------------------------------------------------------
// ReconcileGraphBuilder
// ----------------------------------------------------------------------------

// ReconcileGraphBuilder builds execution graphs for reconcile operations.
type ReconcileGraphBuilder struct {
	config *Config
}

// NewReconcileGraphBuilder creates a new reconcile graph builder.
func NewReconcileGraphBuilder(cfg *ReconcileConfig) *ReconcileGraphBuilder {
	return &ReconcileGraphBuilder{config: &cfg.Config}
}

// Build creates an execution graph for a reconcile operation.
func (b *ReconcileGraphBuilder) Build() (*execution.Graph, error) {
	// TODO: implement reconcile graph building
	return nil, fmt.Errorf("reconcile graph building not yet implemented")
}

// ----------------------------------------------------------------------------
// AdoptGraphBuilder
// ----------------------------------------------------------------------------

// AdoptGraphBuilder builds execution graphs for adopt operations.
type AdoptGraphBuilder struct {
	config    *Config
	files     []string
	layer     string
	layerPath string
	project   string
}

// NewAdoptGraphBuilder creates a new adopt graph builder.
func NewAdoptGraphBuilder(cfg *AdoptConfig) *AdoptGraphBuilder {
	return &AdoptGraphBuilder{
		config:    &cfg.Config,
		files:     cfg.Files,
		layer:     cfg.Layer,
		layerPath: cfg.LayerPath,
		project:   cfg.Project,
	}
}

// Build creates an execution graph for an adopt operation.
func (b *AdoptGraphBuilder) Build() (*execution.Graph, error) {
	// TODO: implement adopt graph building
	return nil, fmt.Errorf("adopt graph building not yet implemented")
}

// ----------------------------------------------------------------------------
// MigrateGraphBuilder
// ----------------------------------------------------------------------------

// MigrateGraphBuilder builds execution graphs for migrate operations.
type MigrateGraphBuilder struct {
	config     *Config
	sourcePath string
}

// NewMigrateGraphBuilder creates a new migrate graph builder.
func NewMigrateGraphBuilder(cfg *Config, sourcePath string) *MigrateGraphBuilder {
	return &MigrateGraphBuilder{
		config:     cfg,
		sourcePath: sourcePath,
	}
}

// Build creates an execution graph for a migrate operation.
func (b *MigrateGraphBuilder) Build() (*execution.Graph, error) {
	// TODO: implement migrate graph building
	return nil, fmt.Errorf("migrate graph building not yet implemented")
}
