// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package registry provides the client interface for accessing the devlore registry.
// It abstracts the underlying transport (git for demo, OCI for scale) and provides
// a sync-based API where the registry is cloned/pulled locally and accessed from cache.
package lorepackage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// Registry provides access to a devlore registry.
type Registry struct {
	name      string   // Registry name (e.g., "central")
	provider  Provider // Transport provider (git, oci, etc.)
	cacheDir  string   // Local cache directory
	forceTags bool     // Force tag resolution even on non-main branches
}

// RegistryConfig holds optional configuration for registry access.
// These values can be set in ~/.config/devlore/config.yaml under lore.registry.
//
// Example config:
//
//	lore:
//	  registry:
//	    url: https://github.com/MyOrg/my-registry.git
//	    branch: main
//	    force_tags: true
type RegistryConfig struct {
	// URL overrides the default registry URL.
	// Default: https://github.com/NobleFactor/devlore-registry.git
	URL string `yaml:"url" mapstructure:"url"`

	// Branch overrides the default branch.
	// Default: develop (for demo phase; main for releases)
	Branch string `yaml:"branch" mapstructure:"branch"`

	// ForceTags forces tag resolution even on non-main branches.
	// When true, "latest" always resolves to the "latest" tag.
	// Default: false
	ForceTags bool `yaml:"force_tags" mapstructure:"force_tags"`
}

// LoadRegistryConfig loads registry configuration from viper.
// Reads from lore.registry.* keys in the config file.
func LoadRegistryConfig() RegistryConfig {
	return RegistryConfig{
		URL:       viper.GetString("lore.registry.url"),
		Branch:    viper.GetString("lore.registry.branch"),
		ForceTags: viper.GetBool("lore.registry.force_tags"),
	}
}

// NewWithConfig creates a registry using configuration from viper.
// This is the preferred way to create a registry in lore commands.
func NewWithConfig() (*Registry, error) {
	return NewFromConfig(LoadRegistryConfig())
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

// New creates a new registry.
func New(name string, provider Provider, cacheDir string) *Registry {
	return &Registry{
		name:     name,
		provider: provider,
		cacheDir: cacheDir,
	}
}

// Default registry settings.
const (
	DefaultRegistryURL    = "https://github.com/NobleFactor/devlore-registry.git"
	DefaultRegistryBranch = "develop" // develop branch has AI assets; main is release-only
)

// NewDefault creates a registry with default settings for the central registry.
// Uses the develop branch during demo phase (AI assets and latest packages).
//
// To customize registry settings, use NewFromConfig with values from
// ~/.config/devlore/config.yaml:
//
//	lore:
//	  registry:
//	    url: https://github.com/MyOrg/my-registry.git
//	    branch: main
//	    force_tags: true
func NewDefault() (*Registry, error) {
	return NewFromConfig(RegistryConfig{})
}

// NewFromConfig creates a registry with the given configuration.
// Empty config values use defaults.
func NewFromConfig(cfg RegistryConfig) (*Registry, error) {
	cacheDir, err := defaultCacheDir()
	if err != nil {
		return nil, err
	}

	// Apply defaults
	url := cfg.URL
	if url == "" {
		url = DefaultRegistryURL
	}

	branch := cfg.Branch
	if branch == "" {
		branch = DefaultRegistryBranch
	}

	provider := NewGitProvider(url, branch)

	reg := &Registry{
		name:      "central",
		provider:  provider,
		cacheDir:  filepath.Join(cacheDir, "central"),
		forceTags: cfg.ForceTags,
	}

	return reg, nil
}

// ForceTags returns whether tag resolution is forced (even on non-main branches).
func (r *Registry) ForceTags() bool {
	return r.forceTags
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
func (r *Registry) Sync(ctx context.Context, opts SyncOptions) (*SyncResult, error) {
	return r.provider.Sync(ctx, r.cacheDir, opts)
}

// CacheDir returns the local cache directory path.
func (r *Registry) CacheDir() string {
	return r.cacheDir
}

// Exists returns true if the cache exists.
func (r *Registry) Exists() bool {
	_, err := os.Stat(r.cacheDir)
	return err == nil
}

// SyncInfo returns information about the last sync, if available.
func (r *Registry) SyncInfo() (*SyncInfo, error) {
	infoPath := filepath.Join(r.cacheDir, ".sync-info.yaml")
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
func (r *Registry) ReadFile(relPath string) ([]byte, error) {
	return os.ReadFile(filepath.Join(r.cacheDir, relPath))
}

// Open opens a file from the cache for reading.
func (r *Registry) Open(relPath string) (io.ReadCloser, error) {
	return os.Open(filepath.Join(r.cacheDir, relPath))
}

// FileExists returns true if the file exists in the cache.
func (r *Registry) FileExists(relPath string) bool {
	_, err := os.Stat(filepath.Join(r.cacheDir, relPath))
	return err == nil
}

// ReadDir reads a directory from the cache.
func (r *Registry) ReadDir(relPath string) ([]os.DirEntry, error) {
	return os.ReadDir(filepath.Join(r.cacheDir, relPath))
}

// FilePath returns the absolute path to a file in the cache.
func (r *Registry) FilePath(relPath string) string {
	return filepath.Join(r.cacheDir, relPath)
}

// =============================================================================
// Version Operations
// =============================================================================

// ListVersions returns all available version tags.
// Tags are returned in descending semver order (newest first).
func (r *Registry) ListVersions(ctx context.Context) ([]string, error) {
	gp, ok := r.provider.(*GitProvider)
	if !ok {
		return nil, fmt.Errorf("version listing requires git provider")
	}
	return gp.ListVersions(ctx, r.cacheDir)
}

// ResolveVersion resolves a version string to a git ref.
// Uses ForceTags setting to determine behavior on non-main branches.
func (r *Registry) ResolveVersion(ctx context.Context, version string) (string, error) {
	gp, ok := r.provider.(*GitProvider)
	if !ok {
		return "", fmt.Errorf("version resolution requires git provider")
	}

	// If ForceTags is set, always resolve via tags (even on non-main)
	if r.forceTags && (version == "" || version == "latest") {
		return gp.resolveTag(ctx, r.cacheDir, "latest")
	}

	return gp.ResolveVersion(ctx, r.cacheDir, version)
}

// CheckoutVersion checks out a specific version in the cache.
func (r *Registry) CheckoutVersion(ctx context.Context, version string) error {
	gp, ok := r.provider.(*GitProvider)
	if !ok {
		return fmt.Errorf("version checkout requires git provider")
	}

	// If ForceTags is set and version is "latest", use tag even on non-main
	if r.forceTags && (version == "" || version == "latest") {
		if err := gp.fetchTags(ctx, r.cacheDir); err != nil {
			return err
		}
		return gp.runGit(ctx, r.cacheDir, "checkout", "latest")
	}

	return gp.CheckoutVersion(ctx, r.cacheDir, version)
}

// CurrentVersion returns the version tag at HEAD, or "" if HEAD is not tagged.
func (r *Registry) CurrentVersion(ctx context.Context) (string, error) {
	gp, ok := r.provider.(*GitProvider)
	if !ok {
		return "", fmt.Errorf("version query requires git provider")
	}
	return gp.CurrentVersion(ctx, r.cacheDir)
}

// Branch returns the configured branch name.
func (r *Registry) Branch() string {
	gp, ok := r.provider.(*GitProvider)
	if !ok {
		return ""
	}
	return gp.Branch()
}

// Knowledge returns a domain accessor for reading knowledge assets.
// The registry organizes knowledge by domain:
//
//	knowledge/migration/   - writ migrate prompts, transforms, signatures
//	knowledge/onboarding/  - environment initialization
//	knowledge/package-authoring/ - lore package creation
//	knowledge/shared/      - common assets inherited by all domains
//
// Usage:
//
//	registry.Knowledge("migration").Prompt("migrate-to-writ.txt")
//	registry.Knowledge("migration").Transform("from-stow.yaml")
func (r *Registry) Knowledge(domain string) *KnowledgeDomain {
	return &KnowledgeDomain{
		registry: r,
		domain:   domain,
	}
}

// SignatureIndex returns the package signature index from signatures.yaml.
// The index maps manager → native_name → lore_package for detecting
// native package installations and resolving them to lore packages.
// Returns an empty map if the file doesn't exist or is invalid.
func (r *Registry) SignatureIndex() map[string]map[string]string {
	data, err := r.ReadFile("signatures.yaml")
	if err != nil {
		return make(map[string]map[string]string)
	}

	var idx map[string]map[string]string
	if err := yaml.Unmarshal(data, &idx); err != nil {
		return make(map[string]map[string]string)
	}

	return idx
}

// KnowledgeDomain provides access to knowledge assets within a domain.
// Methods correspond to subdirectories in the knowledge/{domain}/ structure.
// Assets are resolved with fallback to the "shared" domain.
type KnowledgeDomain struct {
	registry *Registry
	domain   string
}

// read attempts to read from the domain, falling back to shared.
func (k *KnowledgeDomain) read(subdir, name string) ([]byte, error) {
	// Try domain-specific first
	path := filepath.Join("knowledge", k.domain, subdir, name)
	data, err := k.registry.ReadFile(path)
	if err == nil {
		return data, nil
	}

	// Fall back to shared (unless we're already in shared)
	if k.domain != "shared" {
		sharedPath := filepath.Join("knowledge", "shared", subdir, name)
		data, sharedErr := k.registry.ReadFile(sharedPath)
		if sharedErr == nil {
			return data, nil
		}
	}

	// Return original error (more specific)
	return nil, fmt.Errorf("reading %s/%s/%s: %w", k.domain, subdir, name, err)
}

// Prompt reads a prompt file from knowledge/{domain}/prompts/{name}.
// Falls back to knowledge/shared/prompts/{name} if not found in domain.
func (k *KnowledgeDomain) Prompt(name string) (string, error) {
	data, err := k.read("prompts", name)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Schema reads a JSON schema file from knowledge/{domain}/schemas/{name}.
// Falls back to knowledge/shared/schemas/{name} if not found in domain.
func (k *KnowledgeDomain) Schema(name string) ([]byte, error) {
	return k.read("schemas", name)
}

// Examples reads an examples file from knowledge/{domain}/examples/{name}.
// Falls back to knowledge/shared/examples/{name} if not found in domain.
func (k *KnowledgeDomain) Examples(name string) ([]byte, error) {
	return k.read("examples", name)
}

// Transform reads a transform file from knowledge/{domain}/transforms/{name}.
// Falls back to knowledge/shared/transforms/{name} if not found in domain.
func (k *KnowledgeDomain) Transform(name string) ([]byte, error) {
	return k.read("transforms", name)
}

// Signature reads a signature file from knowledge/{domain}/signatures/{name}.
// Falls back to knowledge/shared/signatures/{name} if not found in domain.
func (k *KnowledgeDomain) Signature(name string) ([]byte, error) {
	return k.read("signatures", name)
}

// Slots reads a slots definition file from knowledge/{domain}/slots/{name}.
// Falls back to knowledge/shared/slots/{name} if not found in domain.
func (k *KnowledgeDomain) Slots(name string) ([]byte, error) {
	return k.read("slots", name)
}

// Providers reads a providers reference file from knowledge/{domain}/providers/{name}.
// Falls back to knowledge/shared/providers/{name} if not found in domain.
// This is typically used for model context limits and configuration.
func (k *KnowledgeDomain) Providers(name string) ([]byte, error) {
	return k.read("providers", name)
}

// KnowledgeIndex represents the index.yaml manifest for a knowledge domain.
// It lists all available assets by type with metadata for discovery.
type KnowledgeIndex struct {
	Domain     string           `yaml:"domain"`
	Prompts    []PromptEntry    `yaml:"prompts,omitempty"`
	Schemas    []SchemaEntry    `yaml:"schemas,omitempty"`
	Examples   []ExampleEntry   `yaml:"examples,omitempty"`
	Transforms []TransformEntry `yaml:"transforms,omitempty"`
	Signatures []SignatureEntry `yaml:"signatures,omitempty"`
	Slots      []SlotEntry      `yaml:"slots,omitempty"`
}

// PromptEntry describes a prompt asset with discovery metadata.
type PromptEntry struct {
	Name        string `yaml:"name"`
	Purpose     string `yaml:"purpose,omitempty"`     // semantic key for discovery
	Description string `yaml:"description,omitempty"` // human-readable description
}

// SchemaEntry describes a JSON schema asset with discovery metadata.
type SchemaEntry struct {
	Name        string `yaml:"name"`
	Purpose     string `yaml:"purpose,omitempty"`
	Description string `yaml:"description,omitempty"`
}

// ExampleEntry describes an examples asset with discovery metadata.
type ExampleEntry struct {
	Name        string `yaml:"name"`
	Purpose     string `yaml:"purpose,omitempty"`
	Description string `yaml:"description,omitempty"`
}

// TransformEntry describes a transform asset with discovery metadata.
type TransformEntry struct {
	Name         string `yaml:"name"`
	SourceSystem string `yaml:"source_system,omitempty"` // source system this transform handles
	Description  string `yaml:"description,omitempty"`
}

// SignatureEntry describes a signature asset with discovery metadata.
type SignatureEntry struct {
	Name        string `yaml:"name"`
	System      string `yaml:"system,omitempty"` // system this signature detects
	Description string `yaml:"description,omitempty"`
}

// SlotEntry describes a slots asset with discovery metadata.
type SlotEntry struct {
	Name        string `yaml:"name"`
	Purpose     string `yaml:"purpose,omitempty"`
	Description string `yaml:"description,omitempty"`
}

// PromptByPurpose finds a prompt by its semantic purpose key.
// Returns empty string if not found.
func (idx *KnowledgeIndex) PromptByPurpose(purpose string) string {
	for _, p := range idx.Prompts {
		if p.Purpose == purpose {
			return p.Name
		}
	}
	return ""
}

// SchemaByPurpose finds a schema by its semantic purpose key.
func (idx *KnowledgeIndex) SchemaByPurpose(purpose string) string {
	for _, s := range idx.Schemas {
		if s.Purpose == purpose {
			return s.Name
		}
	}
	return ""
}

// TransformBySourceSystem finds a transform by the source system it handles.
func (idx *KnowledgeIndex) TransformBySourceSystem(system string) string {
	for _, t := range idx.Transforms {
		if t.SourceSystem == system {
			return t.Name
		}
	}
	return ""
}

// SignatureNames returns just the filenames for iteration.
func (idx *KnowledgeIndex) SignatureNames() []string {
	names := make([]string, len(idx.Signatures))
	for i, s := range idx.Signatures {
		names[i] = s.Name
	}
	return names
}

// Index loads the index.yaml manifest for this knowledge domain.
// Returns nil if the index doesn't exist (domain may have no indexed assets).
func (k *KnowledgeDomain) Index() (*KnowledgeIndex, error) {
	path := filepath.Join("knowledge", k.domain, "index.yaml")
	data, err := k.registry.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var index KnowledgeIndex
	if err := yaml.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("parsing index.yaml: %w", err)
	}

	return &index, nil
}
