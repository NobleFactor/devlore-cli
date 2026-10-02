// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package writ

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/spf13/viper"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/tree"
	"github.com/NobleFactor/devlore-cli/pkg/xdg"
)

// scopeFlagUsage is the usage of `--scope`, which `deploy`, `upgrade`, `reconcile` and `decommission` each register:
// a choice for one run, not a setting (#926).
const scopeFlagUsage = "Scope to operate on, repeatable: Home, System, a Windows scope, or one writ.scopes names " +
	"(default: every scope defined on this platform)"

// LayerOrder defines the processing order for repository layers.
// Layers are processed in this order, with later layers overriding earlier ones.
var LayerOrder = []string{"base", "team", "personal"}

// builtinScopes are the scopes writ defines itself, in scope order: System before Home, as deploys have always run
// them, then the Windows scopes. Each name is reserved on every platform, defined there or not, so that one
// repository means one thing on every machine (#926).
var builtinScopes = []builtinScope{
	{name: "System", root: systemRoot},
	{name: "Home", root: homeRoot},
	{name: "ProgramData", root: windowsFolder("ProgramData")},
	{name: "ProgramFiles", root: windowsFolder("ProgramFiles")},
	{name: "ProgramFilesX86", root: windowsFolder("ProgramFiles(x86)")},
}

// ScopeSpec defines a scope: the directory a layer repository holds it in, and the root its files deploy beneath.
type ScopeSpec struct {
	SourceDir  string // the scope's name and its directory in a layer repository: "System", "Home", or a custom name
	TargetRoot string // a path: the root the scope's files deploy beneath
}

// builtinScope is one row of [builtinScopes]: a reserved name, and how a platform resolves the scope's root.
type builtinScope struct {
	name string                     // the scope's name, which is also its directory in a layer repository
	root func(scopePlatform) string // the scope's root on a platform; "" where the platform does not define it
}

// scopePlatform is what resolving the scopes reads from the machine: its operating system, its environment, and the
// user's home directory. A test builds one for a platform other than its own.
type scopePlatform struct {
	goos   string              // the operating system, as `runtime.GOOS` names it
	getenv func(string) string // the environment, which names the Windows folders
	home   string              // the user's home directory
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
	return scopeRoot(currentScopePlatform(), configuredScopeRoots(), "Home")
}

// ScopeSystem returns the root the System scope deploys beneath.
//
// The default is `/` on Unix and the system drive's root, `%SystemDrive%\`, on Windows, where a leading separator
// with no volume is drive-relative and would resolve against whatever drive the process stands on (#392).
//
// Returns:
//   - `string`: `writ.scopes.System` when set, else the platform's root.
func ScopeSystem() string {
	return scopeRoot(currentScopePlatform(), configuredScopeRoots(), "System")
}

// ScopeOrder returns every scope defined on this platform, in scope order (#926).
//
// The builtins this platform defines come first, in [builtinScopes]' order; then the custom scopes `writ.scopes`
// names, alphabetically. A builtin's key in `writ.scopes` relocates it rather than adding a scope.
//
// Returns:
//   - `[]ScopeSpec`: the defined scopes, each beneath its root.
func ScopeOrder() []ScopeSpec {
	return definedScopes(currentScopePlatform(), configuredScopeRoots())
}

// SelectScopes returns the scopes a run covers, as `--scope` names them (#926).
//
// Names match without case. With none, a run covers every scope defined on this platform; with some, it covers
// those, in scope order and each once.
//
// Parameters:
//   - `names`: the scopes `--scope` named; none for every defined scope.
//
// Returns:
//   - `[]ScopeSpec`: the selected scopes, in [ScopeOrder].
//   - `error`: an [cli.ExitUsage]-coded refusal naming the first name that is not a scope defined here, and the
//     scopes that are.
func SelectScopes(names []string) ([]ScopeSpec, error) {
	return selectScopes(currentScopePlatform(), ScopeOrder(), names)
}

// CollectLayerSources gathers all configured repository layers and expands them into source/target pairs: one per
// layer and scope whose directory exists, layers in [LayerOrder] and scopes in [ScopeOrder] within each.
//
// A layer directory for a scope this platform does not define is skipped in silence (#926): the repository is shared,
// and the directory is there for the machines that define the scope.
//
// Returns:
//   - `[]tree.LayerSource`: one source per configured layer and defined scope whose directory exists; nil when none.
//   - `error`: always nil.
func CollectLayerSources() ([]tree.LayerSource, error) {

	var sources []tree.LayerSource

	scopes := ScopeOrder()
	for i, layer := range LayerOrder {
		path := getConfiguredRepo(layer)
		if path == "" {
			continue
		}
		path = expandPath(path)

		for _, spec := range scopes {
			sourceDir, found := scopeDirectory(path, spec.SourceDir)
			if !found {
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

// region HELPER FUNCTIONS

// builtinScopeNamed returns the builtin scope a name denotes, matched without case.
//
// Parameters:
//   - `name`: the name to look up.
//
// Returns:
//   - `builtinScope`: the builtin; the zero value when `name` names none.
//   - `bool`: whether `name` names a builtin.
func builtinScopeNamed(name string) (builtinScope, bool) {

	index := slices.IndexFunc(builtinScopes, func(builtin builtinScope) bool {
		return strings.EqualFold(builtin.name, name)
	})
	if index < 0 {
		return builtinScope{}, false
	}
	return builtinScopes[index], true
}

// configuredScopeRoots returns `writ.scopes`: scope names, which viper folds to lower case, mapped to their roots.
//
// Returns:
//   - `map[string]string`: the configured roots; empty when `writ.scopes` is unset.
func configuredScopeRoots() map[string]string {
	return viper.GetStringMapString("writ.scopes")
}

// currentScopePlatform returns the platform writ is running on.
//
// Returns:
//   - `scopePlatform`: this operating system, this environment, and the user's home directory.
func currentScopePlatform() scopePlatform {
	return scopePlatform{goos: runtime.GOOS, getenv: os.Getenv, home: xdg.UserHomeDir()}
}

// definedScopes returns the scopes a platform defines, given the configured roots, in scope order.
//
// Parameters:
//   - `platform`: the platform the scopes resolve on.
//   - `configured`: `writ.scopes`, scope names to roots; its keys match without case.
//
// Returns:
//   - `[]ScopeSpec`: the builtins `platform` defines, each relocated when `configured` names it; then the custom
//     scopes, alphabetically, named in lower case as configuration keys are.
func definedScopes(platform scopePlatform, configured map[string]string) []ScopeSpec {

	configured = lowerCaseKeys(configured)

	var scopes []ScopeSpec
	for _, builtin := range builtinScopes {
		root := builtin.root(platform)
		if root == "" {
			continue
		}
		if relocated := configured[strings.ToLower(builtin.name)]; relocated != "" {
			root = expandPath(relocated)
		}
		scopes = append(scopes, ScopeSpec{SourceDir: builtin.name, TargetRoot: root})
	}
	for _, name := range slices.Sorted(maps.Keys(configured)) {
		if _, builtin := builtinScopeNamed(name); !builtin {
			scopes = append(scopes, ScopeSpec{SourceDir: name, TargetRoot: expandPath(configured[name])})
		}
	}
	return scopes
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

// homeRoot returns the Home scope's root on a platform: the user's home directory, which every platform defines.
//
// Parameters:
//   - `platform`: the platform the scope resolves on.
//
// Returns:
//   - `string`: the user's home directory.
func homeRoot(platform scopePlatform) string {
	return platform.home
}

// lowerCaseKeys returns a copy of a map with its keys in lower case, as viper folds configuration keys.
//
// Parameters:
//   - `configured`: the map to fold.
//
// Returns:
//   - `map[string]string`: the folded copy.
func lowerCaseKeys(configured map[string]string) map[string]string {

	folded := make(map[string]string, len(configured))
	for key, value := range configured {
		folded[strings.ToLower(key)] = value
	}
	return folded
}

// refuseScopeConfiguration refuses a `writ.scopes` that this platform cannot honor (#926).
//
// A key naming a builtin this platform does not define introduces nothing: `ProgramFiles` is Windows' name, and
// honoring it on Unix would make one repository mean two things. A scope with no root has nowhere to deploy.
//
// Parameters:
//   - `platform`: the platform the scopes resolve on.
//   - `configured`: `writ.scopes`, scope names to roots; its keys match without case.
//
// Returns:
//   - `error`: an [cli.ExitConfig]-coded refusal naming the first such key in name order; nil when there is none.
func refuseScopeConfiguration(platform scopePlatform, configured map[string]string) error {

	configured = lowerCaseKeys(configured)
	for _, key := range slices.Sorted(maps.Keys(configured)) {
		if builtin, ok := builtinScopeNamed(key); ok && builtin.root(platform) == "" {
			return cli.ExitWith(cli.ExitConfig, fmt.Errorf("writ.scopes.%s names %s, which %s does not define: a "+
				"builtin scope's name is reserved on every platform, and it relocates the scope only where it exists",
				key, builtin.name, platform.goos))
		}
		if configured[key] == "" {
			return cli.ExitWith(cli.ExitConfig, fmt.Errorf("writ.scopes.%s names no root", key))
		}
	}
	return nil
}

// scopeDirectory finds a scope's directory in a layer repository.
//
// A builtin's directory carries the builtin's own name. A custom scope's is found without case, since its name comes
// from a configuration key and viper folds keys to lower case: `writ.scopes.staging` finds `Staging/`.
//
// Parameters:
//   - `layerPath`: the layer repository's root.
//   - `name`: the scope's name.
//
// Returns:
//   - `string`: the directory's path; "" when the layer holds none.
//   - `bool`: whether the layer holds the scope's directory.
func scopeDirectory(layerPath, name string) (string, bool) {

	if _, builtin := builtinScopeNamed(name); builtin {
		path := filepath.Join(layerPath, name)
		return path, dirExists(path)
	}

	entries, err := os.ReadDir(layerPath)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if path := filepath.Join(layerPath, entry.Name()); strings.EqualFold(entry.Name(), name) && dirExists(path) {
			return path, true
		}
	}
	return "", false
}

// scopeRoot returns the root of one scope a platform defines.
//
// Parameters:
//   - `platform`: the platform the scope resolves on.
//   - `configured`: `writ.scopes`, scope names to roots.
//   - `name`: the scope's name, as [builtinScopes] or a configuration key spells it.
//
// Returns:
//   - `string`: the scope's root; "" when `platform` does not define the scope.
func scopeRoot(platform scopePlatform, configured map[string]string, name string) string {

	for _, scope := range definedScopes(platform, configured) {
		if strings.EqualFold(scope.SourceDir, name) {
			return scope.TargetRoot
		}
	}
	return ""
}

// selectScopes returns the defined scopes a list of names selects.
//
// Parameters:
//   - `platform`: the platform the scopes resolve on, named in a refusal.
//   - `defined`: the scopes `platform` defines, in scope order.
//   - `names`: the scopes to select, matched without case; none for every defined scope.
//
// Returns:
//   - `[]ScopeSpec`: the selected scopes, in `defined`'s order and each once.
//   - `error`: an [cli.ExitUsage]-coded refusal naming the first name that selects nothing, and the scopes defined.
func selectScopes(platform scopePlatform, defined []ScopeSpec, names []string) ([]ScopeSpec, error) {

	if len(names) == 0 {
		return defined, nil
	}

	selected := make(map[string]bool, len(names))
	for _, name := range names {
		index := slices.IndexFunc(defined, func(scope ScopeSpec) bool {
			return strings.EqualFold(scope.SourceDir, name)
		})
		if index < 0 {
			return nil, cli.ExitWith(cli.ExitUsage, undefinedScopeError(platform, defined, name))
		}
		selected[defined[index].SourceDir] = true
	}

	var scopes []ScopeSpec
	for _, scope := range defined {
		if selected[scope.SourceDir] {
			scopes = append(scopes, scope)
		}
	}
	return scopes, nil
}

// systemRoot returns the System scope's root on a platform, which every platform defines.
//
// Parameters:
//   - `platform`: the platform the scope resolves on.
//
// Returns:
//   - `string`: `/` on Unix; the system drive's root on Windows, `%SystemDrive%\`; "" when Windows names no drive.
func systemRoot(platform scopePlatform) string {

	if platform.goos != "windows" {
		return "/"
	}
	if drive := platform.getenv("SystemDrive"); drive != "" {
		return drive + `\`
	}
	return ""
}

// undefinedScopeError explains why a name selects no scope on a platform.
//
// Parameters:
//   - `platform`: the platform the scopes resolve on.
//   - `defined`: the scopes `platform` defines.
//   - `name`: the name that selected nothing.
//
// Returns:
//   - `error`: the explanation, naming the scopes `platform` defines.
func undefinedScopeError(platform scopePlatform, defined []ScopeSpec, name string) error {

	names := make([]string, len(defined))
	for i, scope := range defined {
		names[i] = scope.SourceDir
	}
	here := strings.Join(names, ", ")

	if builtin, ok := builtinScopeNamed(name); ok {
		return fmt.Errorf("scope %s is not defined on %s; the scopes defined here are %s", builtin.name, platform.goos,
			here)
	}
	return fmt.Errorf("no scope is named %s; the scopes defined here are %s", name, here)
}

// windowsFolder returns the root of a scope Windows defines by a known folder, which its environment names.
//
// Parameters:
//   - `variable`: the environment variable naming the folder, such as `ProgramFiles`.
//
// Returns:
//   - `func(scopePlatform) string`: the folder on Windows, when the environment names it; "" anywhere else.
func windowsFolder(variable string) func(scopePlatform) string {

	return func(platform scopePlatform) string {
		if platform.goos != "windows" {
			return ""
		}
		return platform.getenv(variable)
	}
}

// endregion
