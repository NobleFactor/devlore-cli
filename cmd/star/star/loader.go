// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package star

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/NobleFactor/devlore-cli/cmd/star/config"
	"github.com/NobleFactor/devlore-cli/internal/document"
)

// ExtensionLoader discovers, parses, and deduplicates extensions from the
// filesystem and embedded sources. It holds the search paths and embedded FS
// as state.
type ExtensionLoader struct {
	searchPaths []string
	embeddedFS  fs.FS
}

// NewExtensionLoader creates a loader with the given embedded FS and default
// search paths.
func NewExtensionLoader(embeddedFS fs.FS) *ExtensionLoader {
	return &ExtensionLoader{
		searchPaths: defaultSearchPaths(),
		embeddedFS:  embeddedFS,
	}
}

// NewExtensionLoaderWithPaths creates a loader with explicit search paths and
// the given embedded FS. Used by tests that need to control the search order.
func NewExtensionLoaderWithPaths(searchPaths []string, embeddedFS fs.FS) *ExtensionLoader {
	return &ExtensionLoader{
		searchPaths: searchPaths,
		embeddedFS:  embeddedFS,
	}
}

// DefaultSearchPaths returns the search paths this loader will use.
func (l *ExtensionLoader) DefaultSearchPaths() []string {
	return l.searchPaths
}

// FindExtensionDir locates the directory containing an extension by name.
// Searches the loader's search paths and returns the path to the extension
// directory. Extension directories use the extension name directly (reverse
// domain format).
func (l *ExtensionLoader) FindExtensionDir(name string) (string, error) {
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
func (l *ExtensionLoader) DiscoverAll() ([]*Extension, error) {
	seen := make(map[string]bool)
	var result []*Extension

	// Walk filesystem search paths in priority order.
	sources := []Source{SourceProjectLocal, SourceUser, SourceSystem}
	for i, dir := range l.searchPaths {
		source := SourceSystem
		if i < len(sources) {
			source = sources[i]
		}

		exts, err := l.discoverDir(dir, source)
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
func (l *ExtensionLoader) discoverDir(dir string, source Source) ([]*Extension, error) {
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
func (l *ExtensionLoader) discoverEmbedded() ([]*Extension, error) {
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

// defaultSearchPaths returns the standard directories to search for extensions.
func defaultSearchPaths() []string {
	var paths []string

	root := config.GitWorkspaceRoot()
	if root != "" {
		paths = append(paths, filepath.Join(root, "star", "extensions"))
	}

	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		if home, err := os.UserHomeDir(); err == nil {
			dataHome = filepath.Join(home, ".local", "share")
		}
	}
	if dataHome != "" {
		paths = append(paths, filepath.Join(dataHome, "star", "extensions"))
	}

	paths = append(paths, "/usr/local/share/star/extensions")

	return paths
}
