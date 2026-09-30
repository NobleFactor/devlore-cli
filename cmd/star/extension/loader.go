// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package extension

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/NobleFactor/devlore-cli/cmd/internal/devlore"
	"github.com/NobleFactor/devlore-cli/cmd/star/config"
	"github.com/NobleFactor/devlore-cli/pkg/document"
	"github.com/NobleFactor/devlore-cli/pkg/xdg"
)

// searchPath is a directory to search together with the [Source] its extensions are labeled with.
//
// The two travel as one field rather than as parallel slices so they cannot fall out of step, and so a
// [Loader] built as a struct literal -- which [Application.LoadExtensionsFrom] and the package's own tests
// do -- cannot hold a path whose scope is missing.
type searchPath struct {
	dir    string
	source Source
}

// Loader discovers, parses, and deduplicates extensions from the filesystem and embedded sources. It
// holds the search paths and embedded FS as state.
//
// A path's [Source] is decided where the path is added, not re-derived from the path later: a caller
// supplying paths explicitly is declaring their scopes, and a path's shape does not always read back --
// a temporary directory standing in for a checkout is under neither a repository nor the user's home.
type Loader struct {
	searchPaths []searchPath
	embeddedFS  fs.FS
}

// NewLoader creates a loader with the given embedded FS and default search paths.
//
// Parameters:
//   - `embeddedFS`: the extensions that travel in the binary, searched after every directory.
//
// Returns:
//   - `*Loader`: a loader over [defaultSearchPaths], each path classified by [sourceOf].
func NewLoader(embeddedFS fs.FS) *Loader {

	dirs := defaultSearchPaths()
	paths := make([]searchPath, len(dirs))

	for i, dir := range dirs {
		paths[i] = searchPath{dir: dir, source: sourceOf(dir)}
	}

	return &Loader{
		searchPaths: paths,
		embeddedFS:  embeddedFS,
	}
}

// NewLoaderWithPaths creates a loader with explicit search paths and the given embedded FS. Used by
// tests that need to control the search order, and by [Application.LoadExtensionsFrom].
//
// The paths are taken in scope order -- project, then user, then system -- because supplying them is how
// the caller declares that order. [sourceOf] is deliberately not used here: it reads a path's shape, and
// the first path a caller gives stands in for a checkout without having to look like one.
//
// Parameters:
//   - `searchPaths`: the directories to search, highest precedence first.
//   - `embeddedFS`: the extensions that travel in the binary, searched after every directory.
//
// Returns:
//   - `*Loader`: a loader over exactly those directories, labeled in scope order.
func NewLoaderWithPaths(searchPaths []string, embeddedFS fs.FS) *Loader {

	paths := make([]searchPath, len(searchPaths))

	for i, dir := range searchPaths {

		source := SourceSystem
		switch i {
		case 0:
			source = SourceProjectLocal
		case 1:
			source = SourceUser
		}

		paths[i] = searchPath{dir: dir, source: source}
	}

	return &Loader{
		searchPaths: paths,
		embeddedFS:  embeddedFS,
	}
}

// DefaultSearchPaths returns the directories this loader will search, highest precedence first.
//
// Returns:
//   - `[]string`: the directories, without the [Source] each is labeled with.
func (l *Loader) DefaultSearchPaths() []string {

	dirs := make([]string, len(l.searchPaths))

	for i, path := range l.searchPaths {
		dirs[i] = path.dir
	}

	return dirs
}

// FindExtensionDir locates the directory containing an extension by name.
// Searches the loader's search paths and returns the path to the extension
// directory. Extension directories use the extension name directly (reverse
// domain format).
func (l *Loader) FindExtensionDir(name string) (string, error) {
	for _, path := range l.searchPaths {
		dir := filepath.Join(path.dir, name)
		yamlPath := filepath.Join(dir, "extension.yaml")

		if _, err := os.Stat(yamlPath); err == nil {
			return dir, nil
		}

		ymlPath := filepath.Join(dir, "extension.yml")
		if _, err := os.Stat(ymlPath); err == nil {
			return dir, nil
		}
	}

	return "", fmt.Errorf("extension %q not found in search paths", name)
}

// DiscoverAll walks all search paths and embedded sources in priority order,
// parses each extension.yaml into *Extension, and deduplicates by name (first
// seen wins). Returns an ordered slice of the winners.
func (l *Loader) DiscoverAll() ([]*Extension, error) {
	seen := make(map[string]bool)
	var result []*Extension

	// Walk filesystem search paths in priority order.
	for _, path := range l.searchPaths {

		exts, err := l.discoverDir(path.dir, path.source)
		if err != nil {
			return nil, err
		}
		for _, ext := range exts {
			if !seen[ext.Name] {
				seen[ext.Name] = true
				result = append(result, ext)
			}
		}
	}

	// Walk embedded extensions (lowest priority).
	if l.embeddedFS != nil {
		exts, err := l.discoverEmbedded()
		if err != nil {
			return nil, err
		}
		for _, ext := range exts {
			if !seen[ext.Name] {
				seen[ext.Name] = true
				result = append(result, ext)
			}
		}
	}

	return result, nil
}

// discoverDir scans a filesystem directory for extension.yaml files and parses
// each into an *Extension. Nonexistent directories are silently skipped.
func (l *Loader) discoverDir(dir string, source Source) ([]*Extension, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil
	}

	var exts []*Extension

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		name := d.Name()
		if name != "extension.yaml" && name != "extension.yml" {
			return nil
		}

		ext, err := document.ReadFile[Extension](path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", path, err)
			return nil
		}

		ext.Source = source
		ext.Dir = filepath.Dir(path)

		// Set back-pointers from commands to parent extension.
		for _, cmd := range ext.Commands {
			cmd.Extension = ext
		}

		if err := ext.Validate(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", path, err)
			return nil
		}

		exts = append(exts, ext)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("discover extensions in %s: %w", dir, err)
	}

	return exts, nil
}

// discoverEmbedded scans the embedded FS for extension.yaml files and parses
// each into an *Extension.
func (l *Loader) discoverEmbedded() ([]*Extension, error) {
	var exts []*Extension

	err := fs.WalkDir(l.embeddedFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		name := d.Name()
		if name != "extension.yaml" && name != "extension.yml" {
			return nil
		}

		f, openErr := l.embeddedFS.Open(path)
		if openErr != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", path, openErr)
			return nil
		}

		ext, readErr := document.Read[Extension](f)
		//nolint:errcheck // diagnose-ignored-error: read-only close; see docs/architecture/2.8-eventing-infrastructure.md
		_ = f.Close()
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", path, readErr)
			return nil
		}

		ext.Source = SourceEmbedded
		ext.Dir = filepath.Dir(path)

		// Build a sub-FS scoped to the extension directory.
		extDir := filepath.Dir(path)
		sub, subErr := fs.Sub(l.embeddedFS, extDir)
		if subErr != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", path, subErr)
			return nil
		}
		ext.FS = sub

		// Set back-pointers from commands to parent extension.
		for _, cmd := range ext.Commands {
			cmd.Extension = ext
		}

		if err := ext.Validate(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", path, err)
			return nil
		}

		exts = append(exts, ext)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("discover embedded extensions: %w", err)
	}

	return exts, nil
}

// defaultSearchPaths returns the standard directories to search for extensions, in precedence order.
//
// # The scopes, and why there is no path derived from the binary
//
// The scopes are the design's, in `docs/architecture/9-star-extensions.md`: project for what a repository
// builds itself with, user for what writ deploys under `${XDG_DATA_HOME}`, system for a privileged
// machine-wide install, and embedded for packaging. Every row below serves one of them.
//
// A path derived from the running binary -- `<dir of star>/../share/devlore/star/extensions` -- was added
// here and removed again. It served no scope. It existed to find what `star self install` wrote to
// `$PREFIX/share/devlore/star/extensions`, and `self install` writes no extensions at all: builtin
// extensions are embedded and travel in the binary, and everything else is placed deliberately, by writ at
// user scope or by the repository at project scope (#990). Searching a prefix accommodated a breach rather
// than reading the design that forbids it.
//
// The system entries come from [xdg.DataDirs], which is the specification's own answer and defaults to
// `/usr/local/share` and `/usr/share`. The literals it replaces covered the first and never the second, and
// ignored `XDG_DATA_DIRS` entirely -- the user half of the specification was honored here and the system
// half was not.
//
// Returns:
//   - `[]string`: the directories to search, highest precedence first.
func defaultSearchPaths() []string {

	var paths []string

	// Project-local: the repository this star was invoked inside, which outranks anything deployed.
	if root := config.GitWorkspaceRoot(); root != "" {
		paths = append(paths, filepath.Join(root, "star", "extensions"))
	}

	// star is one of devlore's products, so its data lives under devlore/ like writ's layers and repos (#918). Its
	// config and cache already did. The paths without it are what this release replaces, probed after their
	// replacements so a machine carrying both prefers the new one, and nothing breaks before writ redeploys or star is
	// reinstalled. They go in #920.
	paths = append(paths,
		devlore.DataPath("star", "extensions"),
		xdg.DataPath("star", "extensions"), // deprecated, removed by #920
	)

	for _, dir := range xdg.DataDirs() {
		paths = append(paths,
			filepath.Join(dir, "devlore", "star", "extensions"),
			filepath.Join(dir, "star", "extensions"), // deprecated, removed by #920
		)
	}

	return deduplicate(paths)
}

// sourceOf classifies a search path by what it is, not by where it sits in the list.
//
// It was assigned by index against a three-entry array, defaulting to [SourceSystem] beyond it. The list was
// already five long, so `${XDG_DATA_HOME}/star/extensions` -- a USER path -- was reported as `system`; adding
// the exe-relative entry and [xdg.DataDirs] would have shifted every label after it (#989). The label is what
// someone reads when an extension resolved from somewhere they did not expect, so a wrong one sends them to
// the wrong directory.
//
// Parameters:
//   - `dir`: a search path.
//
// Returns:
//   - `Source`: what kind of location it is.
func sourceOf(dir string) Source {

	clean := filepath.Clean(dir)

	if root := config.GitWorkspaceRoot(); root != "" {
		if clean == filepath.Join(root, "star", "extensions") {
			return SourceProjectLocal
		}
	}

	// Anything under the user's home is the user's, however it was spelled -- XDG_DATA_HOME, the pre-#918
	// path, or an exe-relative path for a binary installed into a prefix there.
	if home := xdg.UserHomeDir(); home != "" {
		if relative, err := filepath.Rel(home, clean); err == nil && !strings.HasPrefix(relative, "..") {
			return SourceUser
		}
	}

	return SourceSystem
}

// deduplicate returns the paths with later repeats removed, preserving order.
//
// The lists overlap by construction: `XDG_DATA_DIRS` may repeat one of its own defaults, and a machine
// carrying both the pre-#918 and post-#918 layouts names one directory twice. A repeated search path is
// not wrong in itself, but it walks the same tree twice and makes one extension look like two.
//
// Parameters:
//   - `paths`: the candidate directories, in precedence order.
//
// Returns:
//   - `[]string`: the same order, first occurrence of each kept.
func deduplicate(paths []string) []string {

	seen := make(map[string]bool, len(paths))
	unique := make([]string, 0, len(paths))

	for _, path := range paths {

		clean := filepath.Clean(path)
		if seen[clean] {
			continue
		}

		seen[clean] = true
		unique = append(unique, clean)
	}

	return unique
}
