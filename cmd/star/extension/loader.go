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

// Loader discovers, parses, and deduplicates extensions from the
// filesystem and embedded sources. It holds the search paths and embedded FS
// as state.
type Loader struct {
	searchPaths []string
	embeddedFS  fs.FS
}

// NewLoader creates a loader with the given embedded FS and default
// search paths.
func NewLoader(embeddedFS fs.FS) *Loader {
	return &Loader{
		searchPaths: defaultSearchPaths(),
		embeddedFS:  embeddedFS,
	}
}

// NewLoaderWithPaths creates a loader with explicit search paths and
// the given embedded FS. Used by tests that need to control the search order.
func NewLoaderWithPaths(searchPaths []string, embeddedFS fs.FS) *Loader {
	return &Loader{
		searchPaths: searchPaths,
		embeddedFS:  embeddedFS,
	}
}

// DefaultSearchPaths returns the search paths this loader will use.
func (l *Loader) DefaultSearchPaths() []string {
	return l.searchPaths
}

// FindExtensionDir locates the directory containing an extension by name.
// Searches the loader's search paths and returns the path to the extension
// directory. Extension directories use the extension name directly (reverse
// domain format).
func (l *Loader) FindExtensionDir(name string) (string, error) {
	for _, searchPath := range l.searchPaths {
		dir := filepath.Join(searchPath, name)
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
	for _, dir := range l.searchPaths {

		exts, err := l.discoverDir(dir, sourceOf(dir))
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
// # The prefix problem, and why an exe-relative path is here
//
// `self install` writes to `$PREFIX/share/devlore/star/extensions`, and this list was written independently
// of it. It matched exactly two prefixes by coincidence -- `~/.local`, via [devlore.DataPath] and only while
// `XDG_DATA_HOME` is unset, and `/usr/local`, via a string literal. Every other prefix installed somewhere
// nothing searched, with `make install` reporting success (#989).
//
// The exe-relative entry fixes that for every prefix at once: the binary lands at `$PREFIX/bin/star`, so
// `<dir of star>/../share/devlore/star/extensions` IS the install target, whatever the prefix was. This is
// what git does for `git-core`, Python for `sys.prefix`, Go for `GOROOT` and clang for its resource
// directory, and the installer already had the same computation at `findExtensionsDir`.
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

	// Project-local: the repository this star was invoked inside, which outranks anything installed.
	if root := config.GitWorkspaceRoot(); root != "" {
		paths = append(paths, filepath.Join(root, "star", "extensions"))
	}

	// Relocatable: wherever this binary was installed to, derived from the binary itself.
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		paths = append(paths, filepath.Join(filepath.Dir(filepath.Dir(exe)), "share", "devlore", "star", "extensions"))
	}

	// star is one of devlore's products, so its data lives under devlore/ like writ's layers and repos (#918). Its
	// config and cache already did. The paths without it are what this release replaces, probed after their
	// replacements so a machine carrying both prefers the new one, and nothing breaks before writ redeploys or star is
	// reinstalled. They go in #920.
	paths = append(paths, devlore.DataPath("star", "extensions"))
	paths = append(paths, xdg.DataPath("star", "extensions")) // deprecated, removed by #920

	for _, dir := range xdg.DataDirs() {
		paths = append(paths, filepath.Join(dir, "devlore", "star", "extensions"))
		paths = append(paths, filepath.Join(dir, "star", "extensions")) // deprecated, removed by #920
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
// The lists overlap by construction: an exe-relative path for a `~/.local` install is the same directory
// [devlore.DataPath] names, and `XDG_DATA_DIRS` may repeat a default. A duplicate search path is not wrong,
// but it makes the same extension look like two, and [Loader.DiscoverAll] labels a [Source] by position --
// so a repeat shifts every later path's label.
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
