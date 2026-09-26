// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package writ

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/readback"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/tree"
)

// Selection is a deploy's project selection, by how each project got in (#843, #850, ruled 2026-09-06 on #463):
// the implicit set, what the record already holds, and what the command line named. The bare form is the
// implicit set alone; naming a project adds it; and the record is the selection between deploys, so a project
// named once stays selected wherever it put a file.
type Selection struct {

	// Implicit is `common`, then one project per configured layer repository, named for the repository, in
	// layer order.
	Implicit []string

	// Recorded is every project the current record holds beyond the implicit ones, sorted.
	Recorded []string

	// Named is every project the command line named beyond those, in command-line order.
	Named []string
}

// resolveSelection resolves a deploy's or upgrade's selection from the registrations, the record and the command
// line.
//
// Parameters:
//   - `ctx`: for the record fold.
//   - `named`: the projects the command line named, possibly none.
//
// Returns:
//   - `Selection`: the three kinds.
//   - `error`: an [cli.ExitUsage]-coded refusal naming a project no registered layer carries; the fold's error
//     otherwise.
func resolveSelection(ctx context.Context, named []string) (Selection, error) {

	selection := Selection{Implicit: implicitProjects()}

	recorded, err := recordedProjects(ctx)
	if err != nil {
		return Selection{}, err
	}
	for _, project := range recorded {
		if !slices.Contains(selection.Implicit, project) {
			selection.Recorded = append(selection.Recorded, project)
		}
	}

	sources, err := CollectLayerSources()
	if err != nil {
		return Selection{}, fmt.Errorf("collect layer sources: %w", err)
	}
	for _, project := range named {
		if slices.Contains(selection.Projects(), project) {
			continue
		}
		if err := knownProject(project, sources); err != nil {
			return Selection{}, err
		}
		selection.Named = append(selection.Named, project)
	}

	return selection, nil
}

// Projects is the selection as the tree builds it: implicit, recorded, named, in that order.
//
// Returns:
//   - `[]string`: the project names, each once.
func (s Selection) Projects() []string {

	projects := make([]string, 0, len(s.Implicit)+len(s.Recorded)+len(s.Named))
	projects = append(projects, s.Implicit...)
	projects = append(projects, s.Recorded...)
	projects = append(projects, s.Named...)
	return projects
}

// Narration says which projects were selected and how, one kind after another, empty kinds omitted:
// `common, noblefactor-ops (implicit); thenobles (recorded); noblefamily (named)`.
//
// Returns:
//   - `string`: the line.
func (s Selection) Narration() string {

	var parts []string
	for _, kind := range []struct {
		projects []string
		label    string
	}{{s.Implicit, "implicit"}, {s.Recorded, "recorded"}, {s.Named, "named"}} {
		if len(kind.projects) > 0 {
			parts = append(parts, strings.Join(kind.projects, ", ")+" ("+kind.label+")")
		}
	}
	return strings.Join(parts, "; ")
}

// region HELPER FUNCTIONS

// implicitProjects is `common` plus one project per configured layer repository, named for the repository: the
// registration's root directory, which is the clone's name for a URL registration (#793) and the directory's for
// a path one.
//
// Returns:
//   - `[]string`: `common` first, then the repository names in layer order, each once.
func implicitProjects() []string {

	var names []string
	for _, layer := range LayerOrder {
		path := getConfiguredRepo(layer)
		if path == "" {
			continue
		}
		name := filepath.Base(path)
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return withCommonProject(names)
}

// recordedProjects is every project the current record holds, sorted; none when nothing is deployed.
//
// Parameters:
//   - `ctx`: for the record fold.
//
// Returns:
//   - `[]string`: the project names, sorted, each once.
//   - `error`: the fold's error, other than not-found.
func recordedProjects(ctx context.Context) ([]string, error) {

	inventory, err := readback.Fold(ctx)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var projects []string
	for target := range inventory.Entries {
		project := inventory.Entries[target].Project
		if project != "" && !slices.Contains(projects, project) {
			projects = append(projects, project)
		}
	}
	sort.Strings(projects)
	return projects, nil
}

// knownProject refuses a named project no registered layer carries, in any suffix form. With no layer sources
// (single-repo mode) there is nothing to check against, and every name passes to the tree.
//
// Parameters:
//   - `project`: the name the command line gave.
//   - `sources`: the registered layers' source directories.
//
// Returns:
//   - `error`: an [cli.ExitUsage]-coded refusal naming the project, or nil.
func knownProject(project string, sources []tree.LayerSource) error {

	if len(sources) == 0 {
		return nil
	}
	for _, source := range sources {
		entries, err := os.ReadDir(source.SourceRoot)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() && (entry.Name() == project || strings.HasPrefix(entry.Name(), project+".")) {
				return nil
			}
		}
	}
	return cli.ExitWith(cli.ExitUsage,
		fmt.Errorf("unknown project %q: no registered layer has Home/%s or Home/%s.<suffix>", project, project, project))
}

// endregion
