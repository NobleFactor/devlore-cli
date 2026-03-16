// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package registry provides the transport abstraction for devlore registry access.
// The Transport interface decouples registry consumers from the underlying protocol
// (git, OCI, HTTP). Today only git is implemented; the design accommodates future
// transports without changing callers.
package registry

import (
	"context"
	"time"
)

// Interface Guard: gitTransport implements Transport.
var _ Transport = (*gitTransport)(nil)

// Transport abstracts the protocol used to synchronize a local registry cache
// with a remote registry.
type Transport interface {
	// Sync updates the local cache from the remote registry.
	//
	// Parameters:
	//   - ctx: context for cancellation
	//   - cacheDir: local cache directory path
	//   - opts: sync behavior options
	//
	// Returns:
	//   - *SyncResult: information about what changed
	//   - error: sync failure
	Sync(ctx context.Context, cacheDir string, opts SyncOptions) (*SyncResult, error)

	// Name returns the transport type identifier ("git", "oci", etc.).
	//
	// Returns:
	//   - string: transport type name
	Name() string

	// ListVersions returns all available version tags.
	// Tags are returned in descending semver order (newest first).
	//
	// Parameters:
	//   - ctx: context for cancellation
	//   - cacheDir: local cache directory path
	//
	// Returns:
	//   - []string: version tags sorted newest-first
	//   - error: listing failure
	ListVersions(ctx context.Context, cacheDir string) ([]string, error)

	// ResolveVersion resolves a version string to a ref.
	//
	// Parameters:
	//   - ctx: context for cancellation
	//   - cacheDir: local cache directory path
	//   - version: version string ("latest", "v1.0.0", or "")
	//
	// Returns:
	//   - string: resolved ref (commit SHA or "HEAD")
	//   - error: resolution failure
	ResolveVersion(ctx context.Context, cacheDir, version string) (string, error)

	// CheckoutVersion checks out a specific version in the cache.
	//
	// Parameters:
	//   - ctx: context for cancellation
	//   - cacheDir: local cache directory path
	//   - version: version to checkout
	//
	// Returns:
	//   - error: checkout failure
	CheckoutVersion(ctx context.Context, cacheDir, version string) error

	// HasTag checks whether a specific tag exists.
	//
	// Parameters:
	//   - ctx: context for cancellation
	//   - cacheDir: local cache directory path
	//   - tag: tag name to check
	//
	// Returns:
	//   - bool: true if the tag exists
	//   - error: lookup failure
	HasTag(ctx context.Context, cacheDir, tag string) (bool, error)

	// CurrentRef returns the current HEAD ref in the cache.
	//
	// Parameters:
	//   - ctx: context for cancellation
	//   - cacheDir: local cache directory path
	//
	// Returns:
	//   - string: HEAD commit SHA
	//   - error: ref lookup failure
	CurrentRef(ctx context.Context, cacheDir string) (string, error)

	// CurrentVersion returns the version tag at HEAD, or "" if HEAD is not tagged.
	//
	// Parameters:
	//   - ctx: context for cancellation
	//   - cacheDir: local cache directory path
	//
	// Returns:
	//   - string: version tag at HEAD, or empty string
	//   - error: lookup failure
	CurrentVersion(ctx context.Context, cacheDir string) (string, error)
}

// Config holds the configuration needed to construct a Transport.
type Config struct {
	URL       string // Remote registry URL
	Branch    string // Branch to track
	ForceTags bool   // Force tag resolution even on non-main branches
}

// NewTransport creates a Transport from configuration.
// Today this always returns a git transport; future transports (OCI, HTTP) will
// be selected by inspecting cfg.URL scheme or adding a Type field.
//
// Parameters:
//   - cfg: transport configuration
//
// Returns:
//   - Transport: configured transport implementation
func NewTransport(cfg Config) Transport {

	return newGitTransport(cfg)
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
