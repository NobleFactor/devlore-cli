// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package writ

import (
	"os"
	"path/filepath"

	"github.com/NobleFactor/devlore-cli/internal/writ/tree"
)

// LayerOrder defines the processing order for repository layers.
// Layers are processed in this order, with later layers overriding earlier ones.
var LayerOrder = []string{"base", "team", "personal"}

// TargetSpec defines a source directory within a repo and its deployment target.
type TargetSpec struct {
	SourceDir  string // "System" or "Home"
	TargetRoot string // "/" or "$HOME"
}

// TargetOrder defines the processing order for targets within each repo.
// System files are deployed before Home files.
func TargetOrder() []TargetSpec {
	home := os.Getenv("HOME")
	return []TargetSpec{
		{SourceDir: "System", TargetRoot: "/"},
		{SourceDir: "Home", TargetRoot: home},
	}
}

// CollectLayerSources gathers all configured repository layers and expands them
// into source/target pairs. Returns sources ordered: base/System, base/Home,
// team/System, team/Home, personal/System, personal/Home (if configured/exist).
func CollectLayerSources() ([]tree.LayerSource, error) {
	var sources []tree.LayerSource

	for i, layer := range LayerOrder {
		path := getConfiguredRepo(layer)
		if path == "" {
			continue
		}
		// Expand path
		path = expandPath(path)

		// Expand each target (System, Home) within this layer
		for _, spec := range TargetOrder() {
			sourceDir := filepath.Join(path, spec.SourceDir)
			if !dirExists(sourceDir) {
				continue
			}
			sources = append(sources, tree.LayerSource{
				Layer:      layer,
				Path:       path,
				Order:      i,
				SourceRoot: sourceDir,
				TargetRoot: spec.TargetRoot,
				TargetName: spec.SourceDir,
			})
		}
	}
	return sources, nil
}

// dirExists checks if a directory exists.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
