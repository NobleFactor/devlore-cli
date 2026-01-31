// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package writ

import (
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/writ/secrets"
	"github.com/NobleFactor/devlore-cli/internal/writ/tree"
)

// CurrentVersion is the graph format version.
const CurrentVersion = "5"

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
func BuildTree(g *execution.Graph, cfg *Config) error {
	result, err := tree.Build(tree.BuildConfig{
		SourceRoot: cfg.SourceRoot,
		TargetRoot: cfg.TargetRoot,
		Sources:    cfg.LayerSources,
		Projects:   cfg.Projects,
		Segments:   cfg.Segments,
	})
	if err != nil {
		return fmt.Errorf("build tree: %w", err)
	}

	// Convert file entries to graph nodes
	for _, f := range result.Files {
		node := &execution.Node{
			ID:         f.ID,
			Operations: f.Operations,
			Status:     execution.StatusPending,
			Source:     f.Source,
			Target:     f.Target,
			Project:    f.Project,
			Layer:      f.Layer,
			Mode:       f.Mode,
		}
		g.Nodes = append(g.Nodes, node)
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

	return nil
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

	// Create registry and register all operations
	registry := execution.NewOperationRegistry()
	for _, op := range execution.AllOps() {
		registry.Register(op)
	}

	// Create engine
	engine := execution.NewGraphExecutor(registry, execution.ExecutorOptions{
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
	config *Config
}

// NewDeployGraphBuilder creates a new deploy graph builder.
func NewDeployGraphBuilder(cfg *DeployConfig) *DeployGraphBuilder {
	return &DeployGraphBuilder{config: &cfg.Config}
}

// Build creates an execution graph for a deploy operation.
func (b *DeployGraphBuilder) Build() (*execution.Graph, error) {
	// Create the graph
	g := NewGraph(b.config)

	// Build the file tree and populate nodes
	if err := BuildTree(g, b.config); err != nil {
		return nil, err
	}

	return g, nil
}

// ----------------------------------------------------------------------------
// DecommissionGraphBuilder
// ----------------------------------------------------------------------------

// DecommissionGraphBuilder builds execution graphs for decommission operations.
type DecommissionGraphBuilder struct {
	config *Config
	view   *execution.StateView
	force  bool
}

// NewDecommissionGraphBuilder creates a new decommission graph builder.
func NewDecommissionGraphBuilder(cfg *DecommissionConfig, view *execution.StateView) *DecommissionGraphBuilder {
	return &DecommissionGraphBuilder{
		config: &cfg.Config,
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
		op := "unlink"
		if entry.IsCopied() {
			op = "remove"
		}

		// Build metadata
		metadata := map[string]string{
			"prune_empty_dirs": "true",
		}

		// Check for local modifications on copied files
		targetChecksum := entry.TargetChecksum()
		if entry.IsCopied() && targetChecksum != "" {
			currentChecksum := execution.ChecksumFile(filepath.Join(b.view.Files.Root, relTarget))
			if currentChecksum != "" && currentChecksum != targetChecksum {
				metadata["locally_modified"] = "true"
				if !b.force {
					// Skip modified files unless --force
					continue
				}
			}
		}

		node := &execution.Node{
			ID:         relTarget,
			Operations: []string{op},
			Status:     execution.StatusPending,
			Source:     entry.Source,
			Target:     filepath.Join(b.view.Files.Root, relTarget),
			Project:    entry.Project,
			Layer:      entry.Layer,
			Metadata:   metadata,
		}

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
