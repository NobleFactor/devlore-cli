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

// ScopeSpec defines a scope: the directory a layer repository holds it in, and the root its files deploy beneath.
type ScopeSpec struct {
	SourceDir  string // the scope's name and its directory in a layer repository: "System" or "Home"
	TargetRoot string // a path: the root the scope's files deploy beneath, "/" or the home directory
}

// ScopeHome returns the root the Home scope deploys beneath.
//
// The configured value wins when set, which is what makes a deployment addressable somewhere other than the
// operator's own home — a staging tree, or a test sandbox. Home itself is resolved, never injected: the
// account database outranks the environment (see [xdg]), so `HOME` cannot move a deployment and this key is
// the only thing that can.
//
// Returns:
//   - `string`: `writ.scopes.Home` when set, else the user's home directory.
func ScopeHome() string {

	if configured := viper.GetString("writ.scopes.home"); configured != "" {
		return expandPath(configured)
	}

	return xdg.UserHomeDir()
}

// ScopeSystem returns the root the System scope deploys beneath.
//
// The default is `/`, which is correct on Unix and **wrong on Windows**, where a leading separator with no
// volume is drive-relative and therefore resolves against whatever drive the process is standing on. Fixing
// that default is [step 58]; this accessor exists so the fix lands in one place, and so a caller that cannot
// wait — a test, or a staging deployment — can name the root explicitly today.
//
// [step 58]: ../../../docs/plans/extract-starlark-from-op/phase-8/steps/58-windows-system-target-root.md
//
// Returns:
//   - `string`: `writ.scopes.System` when set, else `/`.
func ScopeSystem() string {

	if configured := viper.GetString("writ.scopes.system"); configured != "" {
		return expandPath(configured)
	}

	return "/"
}

// ScopeOrder defines the order scopes are processed in within each layer repository.
// System files are deployed before Home files.
//
// Returns:
//   - `[]ScopeSpec`: System beneath [ScopeSystem], then Home beneath [ScopeHome].
func ScopeOrder() []ScopeSpec {
	return []ScopeSpec{
		{SourceDir: "System", TargetRoot: ScopeSystem()},
		{SourceDir: "Home", TargetRoot: ScopeHome()},
	}
}

// CollectLayerSources gathers all configured repository layers and expands them
// into source/target pairs. Returns sources ordered: base/System, base/Home,
// team/System, team/Home, personal/System, personal/Home (if configured/exist).
//
// Returns:
//   - `[]tree.LayerSource`: one source per configured layer and scope whose directory exists, in [LayerOrder]
//     then [ScopeOrder]; nil when none.
//   - `error`: always nil.
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
		for _, spec := range ScopeOrder() {
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
				ScopeName:  spec.SourceDir,
			})
		}
	}
	return sources, nil
}

// PartitionByScope groups layer sources by their scope ("System", "Home").
//
// Sources within each partition keep their original order.
//
// Parameters:
//   - `sources`: the flat list of layer sources from [CollectLayerSources].
//
// Returns:
//   - `map[string][]tree.LayerSource`: the sources keyed by scope name; empty when `sources` is empty.
func PartitionByScope(sources []tree.LayerSource) map[string][]tree.LayerSource {

	partitions := make(map[string][]tree.LayerSource)
	for _, s := range sources {
		partitions[s.ScopeName] = append(partitions[s.ScopeName], s)
	}
	return partitions
}

// dirExists checks if a directory exists.
//
// Parameters:
//   - `path`: the path to stat, following symlinks.
//
// Returns:
//   - `bool`: true when `path` exists and is a directory; false when it is not, or cannot be stat'ed.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
