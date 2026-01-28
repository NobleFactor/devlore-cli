// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

// Package registry provides the client interface for accessing the devlore registry.
// It abstracts the underlying transport (git for demo, OCI for scale) and provides
// a sync-based API where the registry is cloned/pulled locally and accessed from cache.
package registry

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Client provides access to a devlore registry.
type Client struct {
	name     string   // Registry name (e.g., "central")
	provider Provider // Transport provider (git, oci, etc.)
	cacheDir string   // Local cache directory
}

// Provider abstracts the transport mechanism for registry access.
type Provider interface {
	// Sync updates the local cache from the remote registry.
	// Returns information about what changed.
	Sync(ctx context.Context, cacheDir string, opts SyncOptions) (*SyncResult, error)

	// Name returns the provider type ("git", "oci", etc.)
	Name() string
}

// SyncOptions controls sync behavior.
type SyncOptions struct {
	Force bool // Force sync even if cache is fresh
}

// SyncResult contains information about a sync operation.
type SyncResult struct {
	Updated   bool      // Whether the cache was updated
	FromRef   string    // Previous reference (commit SHA, digest, etc.)
	ToRef     string    // New reference
	SyncedAt  time.Time // When sync completed
	FromClone bool      // True if this was a fresh clone
}

// SyncInfo tracks the last sync state.
type SyncInfo struct {
	LastSync time.Time `yaml:"last_sync"`
	Ref      string    `yaml:"ref"`
	Provider string    `yaml:"provider"`
	Endpoint string    `yaml:"endpoint"`
}

// New creates a new registry client.
func New(name string, provider Provider, cacheDir string) *Client {
	return &Client{
		name:     name,
		provider: provider,
		cacheDir: cacheDir,
	}
}

// NewDefault creates a registry client with default settings for the central registry.
// Uses the develop branch during demo phase (AI assets and latest packages).
func NewDefault() (*Client, error) {
	cacheDir, err := defaultCacheDir()
	if err != nil {
		return nil, err
	}

	provider := NewGitProvider(
		"https://github.com/NobleFactor/devlore-registry.git",
		"develop", // develop branch has AI assets; main is release-only
	)

	return New("central", provider, filepath.Join(cacheDir, "central")), nil
}

// defaultCacheDir returns the default cache directory.
func defaultCacheDir() (string, error) {
	// XDG_CACHE_HOME or ~/.cache
	cacheHome := os.Getenv("XDG_CACHE_HOME")
	if cacheHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		cacheHome = filepath.Join(home, ".cache")
	}
	return filepath.Join(cacheHome, "devlore", "registry"), nil
}

// Sync updates the local cache from the remote registry.
func (c *Client) Sync(ctx context.Context, opts SyncOptions) (*SyncResult, error) {
	return c.provider.Sync(ctx, c.cacheDir, opts)
}

// CacheDir returns the local cache directory path.
func (c *Client) CacheDir() string {
	return c.cacheDir
}

// Exists returns true if the cache exists.
func (c *Client) Exists() bool {
	_, err := os.Stat(c.cacheDir)
	return err == nil
}

// SyncInfo returns information about the last sync, if available.
func (c *Client) SyncInfo() (*SyncInfo, error) {
	infoPath := filepath.Join(c.cacheDir, ".sync-info.yaml")
	data, err := os.ReadFile(infoPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var info SyncInfo
	if err := yaml.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// ReadFile reads a file from the cache.
func (c *Client) ReadFile(relPath string) ([]byte, error) {
	return os.ReadFile(filepath.Join(c.cacheDir, relPath))
}

// Open opens a file from the cache for reading.
func (c *Client) Open(relPath string) (io.ReadCloser, error) {
	return os.Open(filepath.Join(c.cacheDir, relPath))
}

// FileExists returns true if the file exists in the cache.
func (c *Client) FileExists(relPath string) bool {
	_, err := os.Stat(filepath.Join(c.cacheDir, relPath))
	return err == nil
}

// ReadDir reads a directory from the cache.
func (c *Client) ReadDir(relPath string) ([]os.DirEntry, error) {
	return os.ReadDir(filepath.Join(c.cacheDir, relPath))
}

// FilePath returns the absolute path to a file in the cache.
func (c *Client) FilePath(relPath string) string {
	return filepath.Join(c.cacheDir, relPath)
}

// AIPrompt reads an AI prompt file from the cache.
func (c *Client) AIPrompt(name string) (string, error) {
	data, err := c.ReadFile(filepath.Join("ai", "prompts", name))
	if err != nil {
		return "", fmt.Errorf("reading AI prompt %s: %w", name, err)
	}
	return string(data), nil
}

// AISchema reads an AI JSON schema file from the cache.
func (c *Client) AISchema(name string) ([]byte, error) {
	data, err := c.ReadFile(filepath.Join("ai", "schemas", name))
	if err != nil {
		return nil, fmt.Errorf("reading AI schema %s: %w", name, err)
	}
	return data, nil
}

// AIExamples reads an AI examples file from the cache.
func (c *Client) AIExamples(name string) ([]byte, error) {
	data, err := c.ReadFile(filepath.Join("ai", "examples", name))
	if err != nil {
		return nil, fmt.Errorf("reading AI examples %s: %w", name, err)
	}
	return data, nil
}

// MigrationGuide reads a migration guide from the cache.
func (c *Client) MigrationGuide(system string) ([]byte, error) {
	filename := fmt.Sprintf("from-%s.yaml", system)
	data, err := c.ReadFile(filepath.Join("ai", "migrations", filename))
	if err != nil {
		return nil, fmt.Errorf("reading migration guide for %s: %w", system, err)
	}
	return data, nil
}
