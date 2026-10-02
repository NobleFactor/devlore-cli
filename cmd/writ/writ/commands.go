// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package writ

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/NobleFactor/devlore-cli/cmd/internal/lorepackage"
	"github.com/NobleFactor/devlore-cli/pkg/xdg"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/decommission"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/deploy"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/reconcile"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/upgrade"
	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"

	"github.com/NobleFactor/devlore-cli/cmd/internal/devlore"
)

// newDeployCmd creates `writ deploy`, which deploys the selected projects from every layer, one graph per scope.
//
// Returns:
//   - `*cobra.Command`: the `deploy` command, with its `--conflict`, `--segment`, `--allow-dirty` and `--scope` flags.
func newDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy [flags] [<project>...]",
		Short: "Deploy projects by creating symlinks in the target location",
		Long: `Deploy projects by creating symlinks in the target location.

A bare "writ deploy" deploys the implicit set: the reserved common project and one
project per configured layer repository, named for the repository (#843, #850), plus
whatever the current record already holds. Naming a project adds it, from every
layer that carries it; a name no registered layer carries is refused.

Files inside each project directory are symlinked to the target (default: ~).
Platform-specific variants (e.g., project.Darwin) are selected automatically.
If a project contains packages-manifest.yaml, the manifest is resolved through
the lore Planner, adding package installation nodes to the execution graph.

Conflict handling (--conflict) — occupied targets (phase-8 step 49):
  stop     (default) Refuse foreign or locally-modified occupants, listing them;
           writ's own unmodified outputs are recognized and replaced, so
           redeploys flow without the flag
  skip     Leave every occupied target untouched and continue
  replace  Archive each occupant to the recovery site and overwrite (restorable)`,
		Example: `  writ deploy                    # the implicit set: common and the layer repositories' projects
  writ deploy noblefactor
  writ deploy noblefactor thenobles
  writ deploy --conflict=replace noblefactor
  writ deploy --conflict=skip noblefactor
  writ deploy -s ROLE=desktop noblefactor`,
		RunE: runDeployV2,
	}

	cmd.Flags().StringP("conflict", "c", "stop", "Occupied-target policy: stop, skip, replace")
	cmd.Flags().StringArrayP("segment", "s", nil,
		"Set a segment's value, NAME=value (repeatable); an extra must be declared in writ.segments")
	cmd.Flags().Bool("allow-dirty", false, "Allow planning against layers with uncommitted changes")
	cmd.Flags().StringSlice("scope", nil, scopeFlagUsage)

	return cmd
}

// runDeployV2 implements the deploy command on the deploy package (phase-8 step 47 slice 1).
//
// Parsing stays here (cobra/viper are command-layer); planning and execution live in
// [deploy.Execute] — tree walk, layer pinning, per-scope graphs, store persistence, and reporting.
//
// Parameters:
//   - `cmd`: the `deploy` command, for its flags and context.
//   - `args`: the project names given on the command line.
//
// Returns:
//   - `error`: a refusal from parsing, the registry client's error, [deploy.Execute]'s, or the plan's rendering
//     error under `--dry-run`; nil when done.
func runDeployV2(cmd *cobra.Command, args []string) error {

	cfg, err := parseDeployConfig(cmd, args)
	if err != nil {
		return err
	}

	// The selection, and how each project got in: the bare form deploys the implicit set and what the record
	// holds, and naming a project adds it (#843, #850).
	cli.Note("Projects: %s", cfg.Selection.Narration())

	// The registry answers one question at plan time, whether a manifest claim names a registry package; the
	// packages themselves plan through the pkg provider (#814).
	registryClient, err := lorepackage.NewRegistry()
	if err != nil {
		return fmt.Errorf("registry client: %w", err)
	}

	graphs, err := deploy.Execute(cmd.Context(), &deploy.Config{
		SourceRoot:   cfg.SourceRoot,
		TargetRoot:   cfg.TargetRoot,
		LayerSources: cfg.LayerSources,
		ScopeOrder:   cfg.ScopeOrder,
		Scopes:       cfg.Scopes,
		Projects:     cfg.Projects,
		Segments:     cfg.Segments,
		Vars:         cfg.TemplateData,
		Conflict:     cfg.ConflictPolicy,
		Registry:     registryClient,
		AllowDirty:   cfg.AllowDirty,
		DryRun:       cfg.DryRun,
		Verbose:      cfg.Verbose,
	})
	if err != nil {
		return err
	}

	// Under --dry-run the plan is the result, and the pipeline renders it like any other.
	if graphs != nil {
		return cli.Emit(cmd, graphs)
	}
	return nil
}

// expandPath expands a leading `~` to the user's home directory.
//
// Parameters:
//   - `path`: the path, which may begin with `~` or `~/`.
//
// Returns:
//   - `string`: the path with its leading `~` replaced by the home directory [xdg] resolves; any other path as given.
func expandPath(path string) string {

	if strings.HasPrefix(path, "~/") {
		return xdg.UserHomeDir() + path[1:]
	}
	if path == "~" {
		return xdg.UserHomeDir()
	}

	return path
}

// newDecommissionCmd creates `writ decommission`, which removes what writ's runs deployed for the named projects.
//
// Returns:
//   - `*cobra.Command`: the `decommission` command, with its `--prune` and `--scope` flags.
func newDecommissionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "decommission [flags] <project>...",
		Short: "Remove deployed files and clean up resources for specified projects",
		Long: `Remove deployed files and clean up resources for specified projects.

The deployed inventory comes from the store readback (never a directory scan), so
decommission removes exactly what writ's runs put into effect. Symlinked entries are
unlinked — a target the user replaced with a real file is refused, not deleted.
Copied files are removed with recovery-site archival (restorable on unwind).

Signature-gated safety (refusing unsigned state) arrives with graph signing (step 46).

`,
		Example: `  writ decommission noblefactor              # Remove project files
  writ decommission all noblefactor          # Remove multiple projects
  writ decommission --prune noblefactor      # Also remove empty parent directories`,
		Args: cobra.MinimumNArgs(1),
		RunE: runDecommission,
	}

	cmd.Flags().Bool("prune", false, "Remove empty parent directories after file removal")
	cmd.Flags().StringSlice("scope", nil, scopeFlagUsage)

	return cmd
}

// runDecommission implements the decommission command on the decommission package (phase-8 step 47 slice 2).
//
// Parameters:
//   - `cmd`: the `decommission` command, for its flags and context.
//   - `args`: the project names given on the command line.
//
// Returns:
//   - `error`: a refusal from parsing, [decommission.Execute]'s error, or the plan's rendering error under
//     `--dry-run`; nil when done.
func runDecommission(cmd *cobra.Command, args []string) error {

	cfg, err := parseDecommissionConfig(cmd, args)
	if err != nil {
		return err
	}

	graphs, err := decommission.Execute(cmd.Context(), &decommission.Config{
		Projects:   cfg.Projects,
		Scopes:     cfg.Scopes,
		ScopeOrder: cfg.ScopeOrder,
		Prune:      cfg.Prune,
		DryRun:     cfg.DryRun,
		Verbose:    cfg.Verbose,
	})
	if err != nil {
		return err
	}

	// Under --dry-run the plan is the result, and the pipeline renders it like any other.
	if graphs != nil {
		return cli.Emit(cmd, graphs)
	}
	return nil
}

// newUpgradeCmd creates `writ upgrade`, which regenerates the selected projects' copied files from their sources.
//
// Returns:
//   - `*cobra.Command`: the `upgrade` command, with its `--force`, `--segment` and `--scope` flags.
func newUpgradeCmd() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "upgrade [<project>...]",
		Short: "Regenerate copied files (templates, secrets) from current sources",
		Long: `Regenerate copied files (templates, secrets) from current sources.

Symlinks are not affected. Only files that were copied during deployment
(templates expanded, secrets decrypted) are regenerated.

The copied inventory comes from the store readback; source templates/secrets are
re-processed through the same planned chains deploy uses.

Each copied file is classified first: a missing target regenerates freely; an
up-to-date target is left alone; a differing target skips with a warning and
regenerates only under --force — until recorded content identity lands (step 48),
source-changed cannot be distinguished from target-modified. Encrypted (sops)
entries cannot be compared without decrypting and follow the same --force rule.`,
		Example: `  writ upgrade                     # Regenerate all copied files
  writ upgrade noblefactor         # Regenerate for specific project
  writ upgrade --force             # Overwrite locally modified files`,
		RunE: runUpgrade,
	}

	cmd.Flags().Bool("force", false, "Overwrite locally modified files without prompting")
	cmd.Flags().StringArrayP("segment", "s", nil,
		"Set a segment's value, NAME=value (repeatable); an extra must be declared in writ.segments")
	cmd.Flags().StringSlice("scope", nil, scopeFlagUsage)

	return cmd
}

// runUpgrade implements the upgrade command on the upgrade package (phase-8 step 47 slice 2).
//
// Parameters:
//   - `cmd`: the `upgrade` command, for its flags and context.
//   - `args`: the project names given on the command line.
//
// Returns:
//   - `error`: a refusal from parsing, [upgrade.Execute]'s error, or the plan's rendering error under `--dry-run`;
//     nil when done.
func runUpgrade(cmd *cobra.Command, args []string) error {

	cfg, err := parseUpgradeConfig(cmd, args)
	if err != nil {
		return err
	}

	graphs, err := upgrade.Execute(cmd.Context(), &upgrade.Config{
		Projects:   cfg.Projects,
		Scopes:     cfg.Scopes,
		ScopeOrder: cfg.ScopeOrder,
		Force:      cfg.Force,
		Segments:   cfg.Segments,
		Vars:       cfg.TemplateData,
		DryRun:     cfg.DryRun,
		Verbose:    cfg.Verbose,
	})
	if err != nil {
		return err
	}

	// Under --dry-run the plan is the result, and the pipeline renders it like any other.
	if graphs != nil {
		return cli.Emit(cmd, graphs)
	}
	return nil
}

// newReconcileCmd creates `writ reconcile`, which reports the system against the record and exits with the answer.
//
// Returns:
//   - `*cobra.Command`: the `reconcile` command, with its `--segment` and `--scope` flags.
func newReconcileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reconcile [<project>...]",
		Short: "Report deployed state: what should be present, from where, and what's missing or different",
		Long: `Report deployed state: what should be present, where it should have come from, and
what's missing or different.

The report is derived from the store (the current deployment's receipts and their
graphs) — never from a directory scan — and has four sections: the registered layer
tree, the deployed inventory classified against the live filesystem, the package
operations writ's runs performed, and store health. Reconcile produces a report; each
finding names the lifecycle command that repairs it. The report is one JSON document,
rendered by --output like every other result; --jq '.entries' selects the delta alone.

Entry states -- the record is the reference:
  linked     The symlink is as recorded, its referent's content as recorded
  copied     The copied file is as recorded, its source's content as recorded
  absent     The record says a target is there and it is not      → writ deploy
  changed    The target is there but is not what the record says  → writ deploy (link), writ upgrade --force (copy)
  dangling   The source the record names does not resolve         → writ deploy
  stale      As recorded; the source resolves and its content moved → writ upgrade

Exit status -- the answer, gateable like git diff --exit-code (#756):
  0    deployed and clean: every entry is linked or copied
  1    deployed and drifted: any entry is absent, changed, dangling or stale
  66   never deployed: no current deployment to compare against`,
		Example: `  writ reconcile                 # Report everything writ has deployed
  writ reconcile noblefactor     # Report one project
  writ reconcile -o json         # Machine-readable report
  writ reconcile -o table        # Aligned columns`,
		RunE: runReconcile,
	}

	cmd.Flags().StringArrayP("segment", "s", nil,
		"Set a segment's value, NAME=value (repeatable); an extra must be declared in writ.segments")
	cmd.Flags().StringSlice("scope", nil, scopeFlagUsage)

	return cmd
}

// runReconcile implements the reconcile command on the reconcile package.
//
// Parameters:
//   - `cmd`: the `reconcile` command, for its flags and context.
//   - `args`: the project names given on the command line.
//
// Returns:
//   - `error`: a refusal from parsing, [reconcile.BuildReport]'s error, the rendering error, or an
//     [cli.ExitError]-coded error when an entry drifted; nil when the system matches the record.
func runReconcile(cmd *cobra.Command, args []string) error {

	cfg, err := parseReconcileConfig(cmd, args)
	if err != nil {
		return err
	}

	report, err := reconcile.BuildReport(cmd.Context(), &reconcile.Config{
		Projects: cfg.Projects,
		Scopes:   cfg.Scopes,
		Verbose:  cfg.Verbose,
		Segments: cfg.Segments,
		Vars:     cfg.TemplateData,
	})
	if err != nil {
		return err
	}

	// The report is the result. Rendering is the pipeline's, so every --output value, --jq and --filter
	// apply to it exactly as they do to any other command's result.
	if err := cli.Emit(cmd, report); err != nil {
		return err
	}

	// The exit status is the answer (#756): the report is rendered whatever it says, and drift exits 1 the way
	// `git diff --exit-code` does, so a script gates on the code and reads the report for the repair.
	if report.HasDrift() {
		return cli.ExitWith(cli.ExitError,
			fmt.Errorf("reconcile: %d of %d entries drifted; the report names the repair per entry",
				report.DriftCount(), len(report.Entries)))
	}

	return nil
}

// getConfiguredRepo returns the directory a layer is registered at.
//
// Layers are directories, or links to them, at `~/.local/share/devlore/writ/layers/<layer>/`.
//
// Parameters:
//   - `layer`: the layer's name: base, team or personal.
//
// Returns:
//   - `string`: the layer's directory, its link resolved; "" when the layer is not registered or its link dangles.
func getConfiguredRepo(layer string) string {
	layerPath := filepath.Join(devlore.WritLayersDir(), layer)

	// Check if layer exists (directory or symlink)
	info, err := os.Lstat(layerPath)
	if err != nil {
		return ""
	}

	// If it's a symlink, resolve it to get the actual path
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := filepath.EvalSymlinks(layerPath)
		if err != nil {
			return "" // Broken symlink
		}
		return target
	}

	// It's a directory
	if info.IsDir() {
		return layerPath
	}

	return ""
}
