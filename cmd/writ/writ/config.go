// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package writ

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"filippo.io/age"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	devconfig "github.com/NobleFactor/devlore-cli/cmd/internal/config"
	"github.com/NobleFactor/devlore-cli/cmd/internal/devlore"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/adopt"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/identity"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"
	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/selector"
)

// withCommonProject returns the selection with the reserved `common` project included — common holds
// configuration that applies everywhere and is always matched (the platform-awareness guide's spec;
// Ansible's `all` group is the pattern, renamed to kill the every-project misreading). An empty
// selection is the implicit set, `common` alone (#843; #850 widens it to one project per configured
// layer repository). Decommission never receives the injection — destruction stays explicit.
//
// Parameters:
//   - `projects`: the projects selected, possibly none.
//
// Returns:
//   - `[]string`: `common` alone when `projects` is empty; `projects` unchanged when it already names `common`;
//     otherwise `common` followed by `projects`.
func withCommonProject(projects []string) []string {

	if len(projects) == 0 {
		return []string{"common"}
	}
	if slices.Contains(projects, "common") {
		return projects
	}
	return append([]string{"common"}, projects...)
}

// parseDeployConfig resolves all settings for a deploy operation.
//
// Settings are resolved from (in priority order):
// 1. Command-line flags
// 2. Environment variables (WRIT_*)
// 3. Config file (~/.config/devlore/config.yaml)
// 4. Defaults
//
// Parameters:
//   - `cmd`: the deploy command, whose context drives the selection and whose `--allow-dirty`, `--conflict` and
//     `--segment` flags are read.
//   - `args`: the projects the command line named, possibly none.
//
// Returns:
//   - `*DeployConfig`: the selection, behavior flags, conflict policy, layer sources (or the single-repo source
//     root when no layer is configured), the Home target root, segments, template variables, and the identities
//     and signing key when identities load.
//   - `error`: the selection's or the segments' refusal, an invalid `--conflict` value, a failure to collect layer
//     sources, or a refusal when no layer is configured and `writ.repo` is unset.
func parseDeployConfig(cmd *cobra.Command, args []string) (*DeployConfig, error) {
	cfg := &DeployConfig{}
	cfg.Tool = "writ"

	// The selection: the implicit set, what the record holds, and what was named (#843, #850).
	selection, err := resolveSelection(cmd.Context(), args)
	if err != nil {
		return nil, err
	}
	cfg.Selection = selection
	cfg.Projects = selection.Projects()

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
	cfg.TargetRoot = ScopeHome()

	// Segments
	if cfg.Segments, err = resolveSegments(cmd); err != nil {
		return nil, err
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
//
// Upgrade selects the way deploy does (#850): the implicit set, what the record holds, and what was named.
//
// Parameters:
//   - `cmd`: the upgrade command, whose context drives the selection and whose `--force` and `--segment` flags
//     are read.
//   - `args`: the projects the command line named, possibly none.
//
// Returns:
//   - `*UpgradeConfig`: the selected projects, behavior flags, the `writ.repo` source root when set, the Home
//     target root, segments, template variables, and the identities and signing key when identities load.
//   - `error`: the selection's or the segments' refusal.
func parseUpgradeConfig(cmd *cobra.Command, args []string) (*UpgradeConfig, error) {
	cfg := &UpgradeConfig{}
	cfg.Tool = "writ"

	selection, err := resolveSelection(cmd.Context(), args)
	if err != nil {
		return nil, err
	}
	cfg.Projects = selection.Projects()

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
	cfg.TargetRoot = ScopeHome()

	// Segments
	if cfg.Segments, err = resolveSegments(cmd); err != nil {
		return nil, err
	}

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
//
// Reconcile reads the deployed inventory from the store, not the layer trees, so it selects no directory; it resolves
// the segments the way every other command does, so `--segment` and `WRIT_SEGMENT_<NAME>` mean one thing everywhere
// and a bad value is refused here as there (#944).
//
// Parameters:
//   - `cmd`: the reconcile command, whose `--segment` flags are read.
//   - `args`: the projects the command line named, taken as given.
//
// Returns:
//   - `*ReconcileConfig`: the projects, the verbose flag, segments, and template variables.
//   - `error`: the segments' refusal.
func parseReconcileConfig(cmd *cobra.Command, args []string) (*ReconcileConfig, error) {
	cfg := &ReconcileConfig{}
	cfg.Tool = "writ"
	cfg.Projects = args

	// Behavior flags
	cfg.Verbose = viper.GetBool("writ.verbose")

	var err error
	if cfg.Segments, err = resolveSegments(cmd); err != nil {
		return nil, err
	}
	cfg.TemplateData = make(map[string]any)
	if varsMap := viper.GetStringMapString("writ.vars"); varsMap != nil {
		for k, v := range varsMap {
			cfg.TemplateData[k] = v
		}
	}

	return cfg, nil
}

// parseDecommissionConfig resolves all settings for a decommission operation.
//
// Parameters:
//   - `cmd`: the decommission command, whose `--prune` flag is read; reading it panics when it is not registered.
//   - `args`: the projects the command line named, taken as given and never widened with `common`.
//
// Returns:
//   - `*DecommissionConfig`: the projects, behavior flags, the prune flag, the Home target root, and empty
//     template data.
func parseDecommissionConfig(cmd *cobra.Command, args []string) *DecommissionConfig {
	cfg := &DecommissionConfig{}
	cfg.Tool = "writ"
	cfg.Projects = args

	// Behavior flags
	cfg.DryRun = viper.GetBool("writ.dry-run")
	cfg.Verbose = viper.GetBool("writ.verbose")
	cfg.Prune = assert.Must(cmd.Flags().GetBool("prune"))

	// Target root
	cfg.TargetRoot = ScopeHome()

	// Initialize template data (prune settings added in runDecommission if --prune)
	cfg.TemplateData = make(map[string]any)

	return cfg
}

// parseAdoptConfig resolves all settings for an adopt operation.
//
// Parameters:
//   - `cmd`: the adopt command, whose `--layer`, `--project`, `--platform`, `--from-receipt` and `--segment` flags
//     are read.
//   - `args`: the files to adopt.
//
// Returns:
//   - `*AdoptConfig`: the files, behavior and adopt flags, and — unless `--from-receipt` is set, which skips
//     validation — the layer path resolved through its symlink and the Home target root.
//   - `error`: a refusal of a missing `--project`, no files, a layer other than personal, team or base, the
//     segments' refusal, an [cli.ExitUsage]-coded invalid platform, or a layer that does not exist or does not
//     resolve.
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
	cfg.Platform, _ = cmd.Flags().GetString("platform")      //nolint:errcheck // flag registered by AddCommand
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
	segs, err := resolveSegments(cmd)
	if err != nil {
		return nil, err
	}
	if err := adopt.ValidatePlatform(cfg.Project, cfg.Platform, segs); err != nil {
		return nil, cli.ExitWith(cli.ExitUsage, err)
	}

	// Resolve layer path
	cfg.LayerPath = filepath.Join(devlore.WritLayersDir(), cfg.Layer)
	if _, err := os.Stat(cfg.LayerPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("layer %q does not exist at %s\nRun 'writ self install' to create layers", cfg.Layer, cfg.LayerPath)
	}
	// A registered layer is a symlink into its repository, and the confined run root refuses to write through an
	// absolute symlink ("path escapes from parent"), so adopt plans against the repository itself -- which is also
	// what the record must name, since links target the origin (#931; found on both VMs 2026-09-23).
	resolved, err := filepath.EvalSymlinks(cfg.LayerPath)
	if err != nil {
		return nil, fmt.Errorf("layer %q at %s does not resolve: %w", cfg.Layer, cfg.LayerPath, err)
	}
	cfg.LayerPath = resolved

	// Target root (HOME)
	cfg.TargetRoot = ScopeHome()

	return cfg, nil
}

// resolveSegments resolves this machine's segments for a command: the built-ins detection supplies, then the extras
// writ.segments declares, each value from `--segment`, else WRIT_SEGMENT_<NAME>, else configuration (#944).
//
// Parameters:
//   - `cmd`: the command, whose `--segment` flags are read when it has them.
//
// Returns:
//   - `segment.Segments`: the segments.
//   - `error`: an [cli.ExitUsage]-coded refusal of a malformed declaration, or of a flag or variable naming an
//     undeclared segment or value.
func resolveSegments(cmd *cobra.Command) (segment.Segments, error) {

	var declared []devconfig.SegmentDeclaration
	if err := viper.UnmarshalKey("writ.segments", &declared); err != nil {
		return nil, cli.ExitWith(cli.ExitUsage, fmt.Errorf("writ.segments: %w", err))
	}

	extras := make([]selector.Segment, 0, len(declared))
	for _, d := range declared {
		extras = append(extras, selector.Segment{Name: d.Name, Values: d.Values, Value: d.Value})
	}

	var flags []string
	if cmd != nil && cmd.Flags().Lookup("segment") != nil {
		flags = assert.Must(cmd.Flags().GetStringArray("segment"))
	}

	segs, err := segment.Resolve(extras, flags)
	if err != nil {
		return nil, cli.ExitWith(cli.ExitUsage, err)
	}
	return segs, nil
}

// parseConflictPolicy parses the --conflict flag value ({stop, skip, replace} — phase-8 step 49).
//
// Parameters:
//   - `flag`: the `--conflict` value as given.
//
// Returns:
//   - `op.ConflictPolicy`: the policy named; [op.ConflictStop] when the value is invalid.
//   - `error`: a refusal naming the invalid value and the three accepted ones.
func parseConflictPolicy(flag string) (op.ConflictPolicy, error) {
	policy, err := op.ParseConflictPolicy(flag)
	if err != nil {
		return op.ConflictStop, fmt.Errorf("invalid --conflict value %q: must be stop, skip, or replace", flag)
	}
	return policy, nil
}

// findSigningKey extracts the first X25519 identity for signing.
//
// Parameters:
//   - `identities`: the loaded age identities, searched in order.
//
// Returns:
//   - `*age.X25519Identity`: the first X25519 identity; nil when there is none.
func findSigningKey(identities []age.Identity) *age.X25519Identity {
	for _, id := range identities {
		if x, ok := id.(*age.X25519Identity); ok {
			return x
		}
	}
	return nil
}
