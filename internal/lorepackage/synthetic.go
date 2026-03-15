// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package lorepackage

import (
	"os"
	"path/filepath"
	"time"

	"github.com/NobleFactor/devlore-cli/internal/document"
)

// SyntheticCache manages cached synthetic package definitions.
// Synthetic packages are created for native PM packages that aren't in the lore registry.
// The cache reduces repeated package manager queries and provides persistence.
type SyntheticCache struct {
	cacheDir string // ProviderBase cache directory (e.g., ~/.cache/devlore/registry)
}

// SyntheticPackageInfo is the cached metadata for a synthetic package.
type SyntheticPackageInfo struct {
	Name        string        `yaml:"name"`
	Source      PackageSource `yaml:"source"`
	NativeName  string        `yaml:"native_name"`
	Version     string        `yaml:"version,omitempty"`
	Description string        `yaml:"description,omitempty"`
	Verified    bool          `yaml:"verified"`    // Was availability verified?
	CachedAt    time.Time     `yaml:"cached_at"`   // When was this cached?
	VerifiedAt  time.Time     `yaml:"verified_at"` // When was availability last verified?
}

// NewSyntheticCache creates a new synthetic package cache.
func NewSyntheticCache(cacheDir string) *SyntheticCache {
	return &SyntheticCache{cacheDir: cacheDir}
}

// cachePathForSource returns the cache directory for a package source.
func (c *SyntheticCache) cachePathForSource(source PackageSource) string {
	return filepath.Join(c.cacheDir, "synthetic", string(source))
}

// cachePathForPackage returns the cache file path for a specific package.
func (c *SyntheticCache) cachePathForPackage(source PackageSource, name string) string {
	return filepath.Join(c.cachePathForSource(source), name+".yaml")
}

// Get retrieves a cached synthetic package, if it exists and is not expired. Returns nil if not cached or expired.
//
// Parameters:
//   - source: package source (e.g., brew, apt)
//   - name: package name
//
// Returns:
//   - *SyntheticPackageInfo: cached info, or nil if missing/expired
func (c *SyntheticCache) Get(source PackageSource, name string) *SyntheticPackageInfo {

	path := c.cachePathForPackage(source, name)

	var info SyntheticPackageInfo
	found, _ := document.ReadIfExists(path, &info)
	if !found {
		return nil
	}

	// Check if cache is expired (7 days for unverified, 30 days for verified)
	maxAge := 7 * 24 * time.Hour
	if info.Verified {
		maxAge = 30 * 24 * time.Hour
	}

	if time.Since(info.CachedAt) > maxAge {
		return nil // Expired
	}

	return &info
}

// Put stores a synthetic package in the cache.
//
// Parameters:
//   - info: synthetic package metadata to cache
//
// Returns:
//   - error: marshal or write error
func (c *SyntheticCache) Put(info *SyntheticPackageInfo) error {

	info.CachedAt = time.Now()
	if info.Verified {
		info.VerifiedAt = time.Now()
	}

	return document.Write(c.cachePathForPackage(info.Source, info.Name), info)
}

// Delete removes a synthetic package from the cache.
func (c *SyntheticCache) Delete(source PackageSource, name string) error {
	path := c.cachePathForPackage(source, name)
	return os.Remove(path)
}

// Clear removes all cached synthetic packages.
func (c *SyntheticCache) Clear() error {
	syntheticDir := filepath.Join(c.cacheDir, "synthetic")
	return os.RemoveAll(syntheticDir)
}

// ClearSource removes all cached synthetic packages for a specific source.
func (c *SyntheticCache) ClearSource(source PackageSource) error {
	return os.RemoveAll(c.cachePathForSource(source))
}

// List returns all cached synthetic packages.
//
// Returns:
//   - []SyntheticPackageInfo: all cached entries across all sources
//   - error: directory read error
func (c *SyntheticCache) List() ([]SyntheticPackageInfo, error) {

	var packages []SyntheticPackageInfo

	syntheticDir := filepath.Join(c.cacheDir, "synthetic")
	sources, err := os.ReadDir(syntheticDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No cache yet
		}
		return nil, err
	}

	for _, sourceEntry := range sources {
		if !sourceEntry.IsDir() {
			continue
		}

		sourceDir := filepath.Join(syntheticDir, sourceEntry.Name())
		files, err := os.ReadDir(sourceDir)
		if err != nil {
			continue
		}

		for _, file := range files {
			if filepath.Ext(file.Name()) != ".yaml" {
				continue
			}

			path := filepath.Join(sourceDir, file.Name())

			var info SyntheticPackageInfo
			if found, _ := document.ReadIfExists(path, &info); found {
				packages = append(packages, info)
			}
		}
	}

	return packages, nil
}

// CacheStats holds cache statistics.
type CacheStats struct {
	TotalPackages    int
	VerifiedPackages int
	ExpiredPackages  int
	BySource         map[PackageSource]int
}

// Stats returns statistics about the synthetic cache.
func (c *SyntheticCache) Stats() (*CacheStats, error) {
	packages, err := c.List()
	if err != nil {
		return nil, err
	}

	stats := &CacheStats{
		BySource: make(map[PackageSource]int),
	}

	now := time.Now()
	for i := range packages {
		pkg := &packages[i]
		stats.TotalPackages++
		stats.BySource[pkg.Source]++

		if pkg.Verified {
			stats.VerifiedPackages++
		}

		maxAge := 7 * 24 * time.Hour
		if pkg.Verified {
			maxAge = 30 * 24 * time.Hour
		}
		if now.Sub(pkg.CachedAt) > maxAge {
			stats.ExpiredPackages++
		}
	}

	return stats, nil
}
