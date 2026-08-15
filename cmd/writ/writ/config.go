// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package writ

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"filippo.io/age"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/NobleFactor/devlore-cli/cmd/internal/devlore"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/identity"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"
	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/xdg"
)

// parseDeployConfig resolves all settings for a deploy operation.
// Settings are resolved from (in priority order):
// 1. Command-line flags
// 2. Environment variables (WRIT_*)
// 3. Config file (~/.config/devlore/config.yaml)
// 4. Defaults
// withCommonProject returns the selection with the reserved `common` project included — common holds
// configuration that applies everywhere and is always matched (the platform-awareness guide's spec;
// Ansible's `all` group is the pattern, renamed to kill the every-project misreading). An empty
// selection returns unchanged: where emptiness is permitted it already means "every project", and
// decommission never receives the injection — destruction stays explicit.
func withCommonProject(projects []string) []string {

	if len(projects) == 0 || slices.Contains(projects, "common") {
		return projects
	}
	return append([]string{"common"}, projects...)
}

func parseDeployConfig(cmd *cobra.Command, args []string) (*DeployConfig, error) {
	cfg := &DeployConfig{}
	cfg.Tool = "writ"
	cfg.Projects = withCommonProject(args)

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
	cfg.TargetRoot = xdg.UserHomeDir()

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
func parseUpgradeConfig(cmd *cobra.Command, args []string) *UpgradeConfig {
	cfg := &UpgradeConfig{}
	cfg.Tool = "writ"
	cfg.Projects = withCommonProject(args)

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
	cfg.TargetRoot = xdg.UserHomeDir()

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

	return cfg
}

// parseReconcileConfig resolves all settings for a reconcile operation.
func parseStatusConfig(cmd *cobra.Command, args []string) *StatusConfig {
	cfg := &StatusConfig{}
	cfg.Tool = "writ"
	cfg.Projects = args

	// Behavior flags
	cfg.Verbose = viper.GetBool("writ.verbose")
	cfg.JSONOutput = assert.Must(cmd.Flags().GetBool("json"))

	// Segments and template variables feed the freshness comparison; status needs no repo — the deployed
	// inventory (sources included) comes from the store readback.
	cfg.Segments = segment.DetectSegments().LoadFromEnv()
	cfg.TemplateData = make(map[string]any)
	if varsMap := viper.GetStringMapString("writ.vars"); varsMap != nil {
		for k, v := range varsMap {
			cfg.TemplateData[k] = v
		}
	}

	return cfg
}

// parseDecommissionConfig resolves all settings for a decommission operation.
func parseDecommissionConfig(cmd *cobra.Command, args []string) *DecommissionConfig {
	cfg := &DecommissionConfig{}
	cfg.Tool = "writ"
	cfg.Projects = args

	// Behavior flags
	cfg.DryRun = viper.GetBool("writ.dry-run")
	cfg.Verbose = viper.GetBool("writ.verbose")
	cfg.Prune = assert.Must(cmd.Flags().GetBool("prune"))

	// Target root
	cfg.TargetRoot = xdg.UserHomeDir()

	// Initialize template data (prune settings added in runDecommission if --prune)
	cfg.TemplateData = make(map[string]any)

	return cfg
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
	cfg.LayerPath = filepath.Join(devlore.WritLayersDir(), cfg.Layer)
	if _, err := os.Stat(cfg.LayerPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("layer %q does not exist at %s\nRun 'writ self install' to create layers", cfg.Layer, cfg.LayerPath)
	}

	// Target root (HOME)
	cfg.TargetRoot = xdg.UserHomeDir()

	return cfg, nil
}

// parseConflictPolicy parses the --conflict flag value ({stop, skip, replace} — phase-8 step 49).
func parseConflictPolicy(flag string) (op.ConflictPolicy, error) {
	policy, err := op.ParseConflictPolicy(flag)
	if err != nil {
		return op.ConflictStop, fmt.Errorf("invalid --conflict value %q: must be stop, skip, or replace", flag)
	}
	return policy, nil
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
