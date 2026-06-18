// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package writ

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/identity"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"
	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// parseDeployConfig resolves all settings for a deploy operation.
// Settings are resolved from (in priority order):
// 1. Command-line flags
// 2. Environment variables (WRIT_*)
// 3. Config file (~/.config/devlore/config.yaml)
// 4. Defaults
func parseDeployConfig(cmd *cobra.Command, args []string) (*DeployConfig, error) {
	cfg := &DeployConfig{}
	cfg.Tool = "writ"
	cfg.Projects = args

	// Behavior flags
	cfg.DryRun = viper.GetBool("writ.dry-run")
	cfg.Verbose = viper.GetBool("writ.verbose")
	cfg.AllowDirty, _ = cmd.Flags().GetBool("allow-dirty") //nolint:errcheck // flag registered by AddCommand

	// Conflict policy
	conflictFlag, _ := cmd.Flags().GetString("conflict") //nolint:errcheck // flag registered by AddCommand
	policy, err := parseConflictPolicy(conflictFlag)
	if err != nil {
		return nil, err
	}
	cfg.ConflictPolicy = policy

	// Collect sources
	layerSources, err := CollectLayerSources()
	if err != nil {
		return nil, fmt.Errorf("collect layer sources: %w", err)
	}
	cfg.LayerSources = layerSources

	// Single-repo mode (when no layers configured)
	if len(layerSources) == 0 {
		sourceRoot := viper.GetString("writ.repo")
		if sourceRoot == "" {
			return nil, fmt.Errorf("no layer configured; use 'writ migrate <source>' to migrate your environment to a writ layer")
		}
		cfg.SourceRoot = expandPath(sourceRoot)
	}

	// Target root
	cfg.TargetRoot = os.Getenv("HOME")
	if cfg.TargetRoot == "" {
		return nil, fmt.Errorf("HOME environment variable not set")
	}

	// Segments
	cfg.Segments = segment.DetectSegments().LoadFromEnv()
	segmentFlags, _ := cmd.Flags().GetStringArray("segment") //nolint:errcheck // flag registered by AddCommand
	for _, sf := range segmentFlags {
		parts := strings.SplitN(sf, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid segment flag %q: expected KEY=value", sf)
		}
		cfg.Segments = cfg.Segments.Set(parts[0], parts[1])
	}

	// Template variables
	cfg.TemplateData = make(map[string]any)
	if varsMap := viper.GetStringMapString("writ.vars"); varsMap != nil {
		for k, v := range varsMap {
			cfg.TemplateData[k] = v
		}
	}

	// Identities for decryption and signing
	identities, err := identity.LoadIdentities()
	if err == nil {
		cfg.Identities = identities
		cfg.SigningKey = findSigningKey(identities)
	}

	return cfg, nil
}

// parseUpgradeConfig resolves all settings for an upgrade operation.
func parseUpgradeConfig(cmd *cobra.Command, args []string) (*UpgradeConfig, error) {
	cfg := &UpgradeConfig{}
	cfg.Tool = "writ"
	cfg.Projects = args

	// Behavior flags
	cfg.DryRun = viper.GetBool("writ.dry-run")
	cfg.Verbose = viper.GetBool("writ.verbose")
	cfg.Force, _ = cmd.Flags().GetBool("force") //nolint:errcheck // flag registered by AddCommand

	// Source root
	sourceRoot := viper.GetString("writ.repo")
	if sourceRoot != "" {
		cfg.SourceRoot = expandPath(sourceRoot)
	}

	// Target root
	cfg.TargetRoot = os.Getenv("HOME")
	if cfg.TargetRoot == "" {
		return nil, fmt.Errorf("HOME environment variable not set")
	}

	// Segments
	cfg.Segments = segment.DetectSegments().LoadFromEnv()

	// Template variables
	cfg.TemplateData = make(map[string]any)
	if varsMap := viper.GetStringMapString("writ.vars"); varsMap != nil {
		for k, v := range varsMap {
			cfg.TemplateData[k] = v
		}
	}

	// Identities
	identities, err := identity.LoadIdentities()
	if err == nil {
		cfg.Identities = identities
		cfg.SigningKey = findSigningKey(identities)
	}

	return cfg, nil
}

// parseReconcileConfig resolves all settings for a reconcile operation.
func parseReconcileConfig(cmd *cobra.Command, args []string) (*ReconcileConfig, error) {
	cfg := &ReconcileConfig{}
	cfg.Tool = "writ"
	cfg.Projects = args

	// Behavior flags
	cfg.Verbose = viper.GetBool("writ.verbose")
	cfg.CheckDrift, _ = cmd.Flags().GetBool("drift") //nolint:errcheck // flag registered by AddCommand
	cfg.JSONOutput, _ = cmd.Flags().GetBool("json")  //nolint:errcheck // flag registered by AddCommand

	// Source root
	sourceRoot := viper.GetString("writ.repo")
	if sourceRoot == "" {
		return nil, fmt.Errorf("no repo configured; set writ.repo in config or use WRIT_REPO env var")
	}
	cfg.SourceRoot = expandPath(sourceRoot)

	// Target root
	cfg.TargetRoot = os.Getenv("HOME")
	if cfg.TargetRoot == "" {
		return nil, fmt.Errorf("HOME environment variable not set")
	}

	// Segments
	cfg.Segments = segment.DetectSegments().LoadFromEnv()

	return cfg, nil
}

// parseDecommissionConfig resolves all settings for a decommission operation.
func parseDecommissionConfig(cmd *cobra.Command, args []string) (*DecommissionConfig, error) {
	cfg := &DecommissionConfig{}
	cfg.Tool = "writ"
	cfg.Projects = args

	// Behavior flags
	cfg.DryRun = viper.GetBool("writ.dry-run")
	cfg.Verbose = viper.GetBool("writ.verbose")
	cfg.Force, _ = cmd.Flags().GetBool("force") //nolint:errcheck // flag registered by AddCommand
	cfg.Prune, _ = cmd.Flags().GetBool("prune") //nolint:errcheck // flag registered by AddCommand

	// Target root
	cfg.TargetRoot = os.Getenv("HOME")
	if cfg.TargetRoot == "" {
		return nil, fmt.Errorf("HOME environment variable not set")
	}

	// Initialize template data (prune settings added in runDecommission if --prune)
	cfg.TemplateData = make(map[string]any)

	return cfg, nil
}

// parseAdoptConfig resolves all settings for an adopt operation.
func parseAdoptConfig(cmd *cobra.Command, args []string) (*AdoptConfig, error) {
	cfg := &AdoptConfig{}
	cfg.Tool = "writ"
	cfg.Files = args

	// Behavior flags
	cfg.DryRun = viper.GetBool("writ.dry-run")
	cfg.Verbose = viper.GetBool("writ.verbose")

	// Adopt-specific flags
	cfg.Layer, _ = cmd.Flags().GetString("layer")            //nolint:errcheck // flag registered by AddCommand
	cfg.Project, _ = cmd.Flags().GetString("project")        //nolint:errcheck // flag registered by AddCommand
	cfg.FromReceipt, _ = cmd.Flags().GetBool("from-receipt") //nolint:errcheck // flag registered by AddCommand

	// Skip validation for --from-receipt mode
	if cfg.FromReceipt {
		return cfg, nil
	}

	// Validate required flags
	if cfg.Project == "" {
		return nil, fmt.Errorf("--project is required")
	}
	if len(cfg.Files) < 1 {
		return nil, fmt.Errorf("requires at least 1 item to adopt")
	}

	// Validate layer
	if cfg.Layer != "personal" && cfg.Layer != "team" && cfg.Layer != "base" {
		return nil, fmt.Errorf("invalid --layer %q: must be personal, team, or base", cfg.Layer)
	}

	// Resolve layer path
	cfg.LayerPath = filepath.Join(cli.WritLayersDir(), cfg.Layer)
	if _, err := os.Stat(cfg.LayerPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("layer %q does not exist at %s\nRun 'writ self-install' to create layers", cfg.Layer, cfg.LayerPath)
	}

	// Target root (HOME)
	cfg.TargetRoot = os.Getenv("HOME")
	if cfg.TargetRoot == "" {
		return nil, fmt.Errorf("HOME environment variable not set")
	}

	return cfg, nil
}

// parseConflictPolicy parses the --conflict flag value.
func parseConflictPolicy(flag string) (op.ConflictPolicy, error) {
	switch flag {
	case "stop", "":
		return op.ConflictStop, nil
	case "backup":
		return op.ConflictBackup, nil
	case "overwrite":
		return op.ConflictOverwrite, nil
	case "skip":
		return op.ConflictSkip, nil
	default:
		return op.ConflictStop, fmt.Errorf("invalid --conflict value %q: must be stop, backup, overwrite, or skip", flag)
	}
}

// findSigningKey extracts the first X25519 identity for signing.
func findSigningKey(identities []age.Identity) *age.X25519Identity {
	for _, id := range identities {
		if x, ok := id.(*age.X25519Identity); ok {
			return x
		}
	}
	return nil
}
