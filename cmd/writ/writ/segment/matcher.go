// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package segment

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/selector"
)

// region EXPORTED FUNCTIONS

// MatchDirectories selects a layer's directories for the given projects: the ones this machine includes, in the order
// to apply them, and every directory name that breaks the selector grammar.
//
// Every directory in the layer is judged, not only the requested projects', because a layer with a malformed name is
// malformed for every deploy. Hidden directories are not projects and are skipped.
//
// Parameters:
//   - `sourceRoot`: the layer's Home or System tree.
//   - `projects`: the projects, in the order they're applied.
//   - `segs`: this machine's segments.
//
// Returns:
//   - `[]MatchResult`: the directories included, in the order to apply them; the last applied wins.
//   - `[]*selector.GrammarError`: every directory name that breaks the grammar.
//   - `error`: non-nil when the tree cannot be read.
func MatchDirectories(sourceRoot string, projects []string, segs Segments) ([]MatchResult, []*selector.GrammarError,
	error) {

	entries, err := os.ReadDir(sourceRoot)
	if err != nil {
		return nil, nil, err
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			names = append(names, entry.Name())
		}
	}

	selected, grammarErrors := segs.Selector(projects).Select(names)

	results := make([]MatchResult, 0, len(selected))
	for _, s := range selected {
		results = append(results, MatchResult{
			Path:    filepath.Join(sourceRoot, s.Name),
			Project: s.Project,
			Rank:    s.Rank,
		})
	}

	return results, grammarErrors, nil
}

// endregion

// region SUPPORTING TYPES

// MatchResult is a directory a selection includes.
type MatchResult struct {

	// Path is the directory's full path.
	Path string

	// Project is the project it belongs to: noblefactor.
	Project string

	// Rank is its place in the order of application.
	Rank selector.Rank
}

// Refusal is every directory name that breaks the selector grammar, in every layer a run reads. writ refuses the run
// with it before anything changes (ruled 2026-09-30).
type Refusal struct {

	// Entries are the names, each with the tree it was found in.
	Entries []RefusalEntry
}

// Error lists every name and the rule it breaks.
//
// Returns:
//   - `string`: the list, one name to a line.
func (r *Refusal) Error() string {

	var b strings.Builder
	fmt.Fprintf(&b, "%d directory name(s) break the selector grammar (docs/guides/selectors.md); nothing was changed:",
		len(r.Entries))
	for _, entry := range r.Entries {
		fmt.Fprintf(&b, "\n  %s: %v", entry.Root, entry.Err)
	}
	return b.String()
}

// RefusalEntry is one directory name that breaks the grammar.
type RefusalEntry struct {

	// Root is the tree the name was found in.
	Root string

	// Err is the name and the rule it breaks.
	Err *selector.GrammarError
}

// endregion
