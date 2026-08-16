// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package writ

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/tree"
	"github.com/NobleFactor/devlore-cli/pkg/xdg"
)

// LayerOrder defines the processing order for repository layers.
// Layers are processed in this order, with later layers overriding earlier ones.
var LayerOrder = []string{"base", "team", "personal"}

// TargetSpec defines a source directory within a repo and its deployment target.
type TargetSpec struct {
	SourceDir  string // "System" or "Home"
	TargetRoot string // "/" or "$HOME"
}

// TargetHome returns the deployment target for Home-scoped sources.
//
// The configured value wins when set, which is what makes a deployment addressable somewhere other than the
// operator's own home — a staging tree, or a test sandbox. Home itself is resolved, never injected: the
// account database outranks the environment (see [xdg]), so `HOME` cannot move a deployment and this key is
// the only thing that can.
//
// Returns:
//   - `string`: `writ.targets.home` when set, else the user's home directory.
func TargetHome() string {

	if configured := viper.GetString("writ.targets.home"); configured != "" {
		return expandPath(configured)
	}

	return xdg.UserHomeDir()
}

// TargetSystem returns the deployment target for System-scoped sources.
//
// The default is `/`, which is correct on Unix and **wrong on Windows**, where a leading separator with no
// volume is drive-relative and therefore resolves against whatever drive the process is standing on. Fixing
// that default is [step 58]; this accessor exists so the fix lands in one place, and so a caller that cannot
// wait — a test, or a staging deployment — can name the root explicitly today.
//
// [step 58]: ../../../docs/plans/extract-starlark-from-op/phase-8/steps/58-windows-system-target-root.md
//
// Returns:
//   - `string`: `writ.targets.system` when set, else `/`.
func TargetSystem() string {

	if configured := viper.GetString("writ.targets.system"); configured != "" {
		return expandPath(configured)
	}

	return "/"
}

// TargetOrder defines the processing order for targets within each repo.
// System files are deployed before Home files.
func TargetOrder() []TargetSpec {
	return []TargetSpec{
		{SourceDir: "System", TargetRoot: TargetSystem()},
		{SourceDir: "Home", TargetRoot: TargetHome()},
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
				OriginRoot: sourceDir,
				TargetRoot: spec.TargetRoot,
				TargetName: spec.SourceDir,
			})
		}
	}
	return sources, nil
}

// PartitionByScope groups layer sources by their TargetName ("System", "Home").
// Sources within each partition retain their original ordering.
// Returns an empty map when sources is empty.
//
// Parameters:
//   - sources: flat list of layer sources from CollectLayerSources
//
// Returns:
//   - map[string][]tree.LayerSource: sources keyed by TargetName
func PartitionByScope(sources []tree.LayerSource) map[string][]tree.LayerSource {

	partitions := make(map[string][]tree.LayerSource)
	for _, s := range sources {
		partitions[s.TargetName] = append(partitions[s.TargetName], s)
	}
	return partitions
}

// dirExists checks if a directory exists.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
