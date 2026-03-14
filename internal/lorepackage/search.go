// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package lorepackage

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Confidence indicates how reliably a package can be installed.
type Confidence int

const (
	// ConfidenceHigh means the package is from the lore registry with full lifecycle support.
	ConfidenceHigh Confidence = iota
	// ConfidenceMedium means the package was found in a native PM and verified to exist.
	ConfidenceMedium
	// ConfidenceLow means the package was synthesized but not verified to exist.
	ConfidenceLow
)

func (c Confidence) String() string {
	switch c {
	case ConfidenceHigh:
		return "HIGH"
	case ConfidenceMedium:
		return "MEDIUM"
	case ConfidenceLow:
		return "LOW"
	default:
		return "UNKNOWN"
	}
}

// SearchResultItem represents a package found during federated search.
type SearchResultItem struct {
	Name        string        // Package name
	Version     string        // Available version (may be empty)
	Description string        // Package description
	Source      PackageSource // Where this package came from
	Confidence  Confidence    // How reliable is this result
	Installed   bool          // Is it currently installed
}

// SearchOptions controls search behavior.
type SearchOptions struct {
	IncludeLore   bool // Search lore registry
	IncludeNative bool // Search native package manager
	Limit         int  // Maximum results per source (0 = no limit)
}

// DefaultSearchOptions returns sensible defaults for search.
func DefaultSearchOptions() SearchOptions {
	return SearchOptions{
		IncludeLore:   true,
		IncludeNative: true,
		Limit:         25,
	}
}

// Search performs a federated search across the lore registry and native package managers.
func (r *Registry) Search(query string, opts SearchOptions) ([]SearchResultItem, error) {
	var results []SearchResultItem

	// Search lore registry first
	if opts.IncludeLore {
		loreResults, err := r.searchLore(query, opts.Limit)
		if err == nil {
			results = append(results, loreResults...)
		}
	}

	// Search native package manager
	if opts.IncludeNative {
		nativeResults := r.searchNative(query, opts.Limit)
		results = append(results, nativeResults...)
	}

	return results, nil
}

// searchLore searches the local lore registry cache.
func (r *Registry) searchLore(query string, limit int) ([]SearchResultItem, error) {
	packagesDir := filepath.Join(r.cacheDir, "packages")
	entries, err := os.ReadDir(packagesDir)
	if err != nil {
		return nil, err
	}

	query = strings.ToLower(query)
	var results []SearchResultItem

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.Contains(strings.ToLower(name), query) {
			continue
		}

		// Load lifecycle for description
		pkgDir := filepath.Join(packagesDir, name)
		lc, err := LoadLifecycle(pkgDir)
		if err != nil {
			continue
		}

		results = append(results, SearchResultItem{
			Name:        lc.Name,
			Version:     lc.Version,
			Description: lc.Description,
			Source:      SourceLore,
			Confidence:  ConfidenceHigh,
		})

		if limit > 0 && len(results) >= limit {
			break
		}
	}

	return results, nil
}

// searchNative searches the native package manager.
func (r *Registry) searchNative(query string, limit int) []SearchResultItem {
	p := op.NewPlatform()
	packageManager := p.PackageManager
	if packageManager == nil {
		return nil
	}

	// Map package manager name to source.
	sourceMap := map[string]PackageSource{
		"brew":   SourceBrew,
		"port":   SourcePort,
		"apt":    SourceApt,
		"dnf":    SourceDnf,
		"winget": SourceWinget,
	}

	source := sourceMap[packageManager.Name()]
	if source == "" {
		source = SourceApt // Default fallback
	}

	// Search the native package manager.
	searchResults := packageManager.Search(query, limit)
	if searchResults == nil {
		return nil
	}

	results := make([]SearchResultItem, 0, len(searchResults))
	for _, sr := range searchResults {
		// Check if it's installed.
		installed := packageManager.Installed(sr.Name)

		// Check if available (for confidence).
		available := packageManager.Available(sr.Name)
		confidence := ConfidenceLow
		if available {
			confidence = ConfidenceMedium
		}

		results = append(results, SearchResultItem{
			Name:        sr.Name,
			Version:     sr.Version,
			Description: sr.Description,
			Source:      source,
			Confidence:  confidence,
			Installed:   installed,
		})
	}

	return results
}

// ListPackages returns all packages in the lore registry.
func (r *Registry) ListPackages() ([]SearchResultItem, error) {
	packagesDir := filepath.Join(r.cacheDir, "packages")
	entries, err := os.ReadDir(packagesDir)
	if err != nil {
		return nil, err
	}

	var results []SearchResultItem
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pkgDir := filepath.Join(packagesDir, entry.Name())
		lc, err := LoadLifecycle(pkgDir)
		if err != nil {
			// Skip packages with invalid lifecycle
			continue
		}

		results = append(results, SearchResultItem{
			Name:        lc.Name,
			Version:     lc.Version,
			Description: lc.Description,
			Source:      SourceLore,
			Confidence:  ConfidenceHigh,
		})
	}

	return results, nil
}

// ResolveWithConfidence resolves a package and returns confidence information.
func (r *Registry) ResolveWithConfidence(name, targetPlatform string) (*Release, Confidence, error) {
	release, err := r.Resolve(name, targetPlatform)
	if err != nil {
		return nil, ConfidenceLow, err
	}

	// Lore packages have high confidence.
	if release.Source == SourceLore {
		return release, ConfidenceHigh, nil
	}

	// Native packages: check if available.
	detected := op.NewPlatform()
	if detected.PackageManager != nil && detected.PackageManager.Available(name) {
		return release, ConfidenceMedium, nil
	}

	return release, ConfidenceLow, nil
}
