// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package config provides centralized configuration for the devlore ecosystem.
// Both lore and writ consume configuration from this package.
//
// Configuration is loaded from ~/.config/devlore/config.yaml with support for:
//   - Environment variable overrides (DEVLORE_*, LORE_*, WRIT_*)
//   - CLI flag overrides (applied by callers)
//   - Native keystore for API keys
package config

import (
	"os"
	"path/filepath"

	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/internal/credentials"
	"github.com/NobleFactor/devlore-cli/internal/document"
)

// Verbosity levels for output control.
const (
	VerbosityQuiet   = "quiet"   // Errors only
	VerbosityNormal  = "normal"  // Default output (empty string treated as normal)
	VerbosityVerbose = "verbose" // Extra output
)

// Config is the root configuration for the devlore ecosystem.
// Shared options and resources are at the top level.
// Tool-specific settings are nested under lore and writ.
type Config struct {
	// Shared runtime options
	Verbosity string `yaml:"verbosity,omitempty" json:"verbosity,omitempty"` // quiet, normal, verbose
	DryRun    bool   `yaml:"dry_run,omitempty" json:"dry_run,omitempty"`

	// Shared resources
	Model    ModelConfig    `yaml:"model,omitempty" json:"model,omitempty"`
	Registry RegistryConfig `yaml:"registry,omitempty" json:"registry,omitempty"`

	// Tool-specific
	Lore LoreConfig `yaml:"lore,omitempty" json:"lore,omitempty"`
	Writ WritConfig `yaml:"writ,omitempty" json:"writ,omitempty"`
}

// Path returns the path to the shared devlore config file.
// ~/.config/devlore/config.yaml
func Path() string {
	return filepath.Join(cli.DevloreConfigHome(), "config.yaml")
}

// Load reads configuration from the config file and applies environment overrides.
// API keys are loaded from the native keystore if not in config/env.
//
// Precedence (lowest to highest):
//  1. Config file
//  2. Environment variables
//  3. Native keystore (for API key only)
//  4. CLI flags (applied by caller via ApplyCLIFlags methods)
func Load() (*Config, error) {
	cfg := &Config{}

	// Read config file
	if _, err := document.ReadIfExists(Path(), cfg); err != nil {
		return nil, err
	}

	// Apply environment variable overrides
	applyEnvOverrides(cfg)

	// Load API key from keystore if not already set
	if cfg.Model.APIKey == "" && cfg.Model.Provider != "" {
		cfg.Model.APIKey, _ = credentials.Get(cfg.Model.Provider) //nolint:errcheck // fallback: continue without credential
	}

	return cfg, nil
}

// Save writes configuration to the config file.
// API keys are stored in the native keystore, not the config file.
func Save(cfg *Config) error {
	// Store API key in keystore (not config file)
	if cfg.Model.APIKey != "" && cfg.Model.Provider != "" && cfg.Model.Provider != "ollama" {
		if err := credentials.Set(cfg.Model.Provider, cfg.Model.APIKey); err != nil {
			cli.Warn("could not store API key in keystore: %v", err)
		}
	}

	// Clone config without API key for file storage
	fileCfg := *cfg
	fileCfg.Model.APIKey = ""

	return document.Write(Path(), &fileCfg)
}

// applyEnvOverrides applies environment variable overrides to the config.
// Naming convention: DEVLORE_ + config path with underscores, uppercase.
// Example: model.provider -> DEVLORE_MODEL_PROVIDER
func applyEnvOverrides(cfg *Config) {
	// Shared runtime options
	if v := os.Getenv("DEVLORE_VERBOSITY"); v != "" {
		cfg.Verbosity = v
	}
	if v := os.Getenv("DEVLORE_DRY_RUN"); v != "" {
		cfg.DryRun = v == "true" || v == "1"
	}

	// Model config
	if v := os.Getenv("DEVLORE_MODEL_PROVIDER"); v != "" {
		cfg.Model.Provider = v
	}
	if v := os.Getenv("DEVLORE_MODEL_NAME"); v != "" {
		cfg.Model.Name = v
	}
	if v := os.Getenv("DEVLORE_MODEL_ENDPOINT"); v != "" {
		cfg.Model.Endpoint = v
	}
	if v := os.Getenv("DEVLORE_MODEL_API_KEY"); v != "" {
		cfg.Model.APIKey = v
	}

	// Registry config
	if v := os.Getenv("DEVLORE_REGISTRY_URL"); v != "" {
		cfg.Registry.URL = v
	}
	if v := os.Getenv("DEVLORE_REGISTRY_BRANCH"); v != "" {
		cfg.Registry.Branch = v
	}
}
