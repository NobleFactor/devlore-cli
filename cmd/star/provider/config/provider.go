// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

// Package config provides configuration management operations for the star runtime.
package config

import (
	cfg "github.com/NobleFactor/devlore-cli/cmd/star/config"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

var _ op.Provider = (*Provider)(nil)

// Provider provides configuration operations: get (merged config), show (config with sources),
// and sync (write tool-specific config files).
//
// +devlore:access=immediate
type Provider struct {
	op.ProviderBase
}

// NewProvider creates a config provider bound to the given context.
func NewProvider(ctx *op.RuntimeEnvironment) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(ctx)}
}

func (p *Provider) loadConfig() (*cfg.Config, error) {
	if c, ok := p.RuntimeEnvironment().Data["config"].(*cfg.Config); ok && c != nil {
		return c, nil
	}
	return cfg.Load()
}

// Get returns the merged configuration from the hierarchy as a Starlark struct
// with attribute access (e.g., cfg.lint.copyright.enabled).
//
// Returns:
//   - any: the config as a Starlark value
//   - error: if config loading fails
func (p *Provider) Get() (any, error) {
	c, err := p.loadConfig()
	if err != nil {
		return nil, err
	}
	return c.ToStarlark(), nil
}

// Show returns the config with source file information.
//
// Returns:
//   - ShowResult: config value and list of source files
//   - error: if config loading fails
func (p *Provider) Show() (ShowResult, error) {
	c, err := p.loadConfig()
	if err != nil {
		return ShowResult{}, err
	}

	_, sources, err := cfg.LoadWithSources()
	if err != nil {
		return ShowResult{}, err
	}

	result := ShowResult{Config: c.ToStarlark()}
	for _, s := range sources {
		result.Sources = append(result.Sources, SourceInfo{
			Path: s.Path, Exists: s.Exists,
		})
	}

	return result, nil
}

// Sync writes tool-specific config files from star/config.yaml.
//
// Returns:
//   - SyncResult: paths of generated files and count
//   - error: if config loading or sync fails
func (p *Provider) Sync() (SyncResult, error) {
	c, err := p.loadConfig()
	if err != nil {
		return SyncResult{}, err
	}

	synced, err := c.Sync()
	if err != nil {
		return SyncResult{}, err
	}

	return SyncResult{
		GolangciLint:   synced.GolangciLint,
		MarkdownLint:   synced.MarkdownLint,
		FilesGenerated: synced.FilesGenerated,
	}, nil
}
