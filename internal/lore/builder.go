// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package lore

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/NobleFactor/devlore-cli/internal/engine"
)

// Builder implements engine.GraphBuilder for lore package manifests.
// It loads a packages.manifest file and builds an execution graph
// that the common engine can process.
type Builder struct{}

// BuildGraph loads a packages.manifest file and returns an execution graph.
// The manifest is a line-delimited list of package names, optionally with
// features specified after the package name.
func (b *Builder) BuildGraph(ctx context.Context, manifestPath string, opts engine.BuildOptions) (*engine.Graph, error) {
	packages, err := loadManifest(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("load manifest %s: %w", manifestPath, err)
	}

	graph := &engine.Graph{}

	for _, pkg := range packages {
		// Merge per-package features with global features
		features := mergeFeatures(pkg.Features, opts.Features)

		node := &engine.Node{
			ID:         pkg.Name,
			Operations: []string{"install"},
			Project:    pkg.Name,
			Metadata: map[string]string{
				"tool":     "lore",
				"manifest": manifestPath,
			},
		}
		if len(features) > 0 {
			node.Metadata["features"] = strings.Join(features, ",")
		}

		graph.Nodes = append(graph.Nodes, node)
	}

	return graph, nil
}

// manifestEntry represents a single package entry in a manifest file.
type manifestEntry struct {
	Name     string
	Features []string
}

// loadManifest parses a packages.manifest file into a list of entries.
// Format: one package per line; features follow the package name.
//
//	docker --with rootless --with compose
//	kubectl
//	gh
//	# comments and blank lines are ignored
func loadManifest(path string) ([]manifestEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []manifestEntry
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		entry := parseLine(line)
		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

// parseLine extracts a package name and optional features from a manifest line.
func parseLine(line string) manifestEntry {
	fields := strings.Fields(line)
	entry := manifestEntry{Name: fields[0]}

	// Parse --with flags
	for i := 1; i < len(fields); i++ {
		if fields[i] == "--with" && i+1 < len(fields) {
			entry.Features = append(entry.Features, fields[i+1])
			i++ // skip the feature value
		}
	}

	return entry
}

// mergeFeatures combines per-package features with global features,
// deduplicating.
func mergeFeatures(pkg, global []string) []string {
	seen := make(map[string]bool)
	var merged []string

	for _, f := range pkg {
		if !seen[f] {
			seen[f] = true
			merged = append(merged, f)
		}
	}
	for _, f := range global {
		if !seen[f] {
			seen[f] = true
			merged = append(merged, f)
		}
	}

	return merged
}
