// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package writ

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/NobleFactor/devlore-cli/cmd/lore/lore"
	"github.com/NobleFactor/devlore-cli/pkg/xdg"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/decommission"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/deploy"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/reconcile"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/upgrade"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/verify"
	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"

	"github.com/NobleFactor/devlore-cli/cmd/internal/devlore"
	"github.com/NobleFactor/devlore-cli/pkg/signing"
)

func newDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy [flags] <project>...",
		Short: "Deploy projects by creating symlinks in the target location",
		Long: `Deploy projects by creating symlinks in the target location.

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
		Example: `  writ deploy noblefactor
  writ deploy all noblefactor thenobles
  writ deploy --conflict=backup noblefactor
  writ deploy --conflict=overwrite noblefactor
  writ deploy -s ROLE=desktop noblefactor`,
		Args: cobra.MinimumNArgs(1),
		RunE: runDeployV2,
	}

	cmd.Flags().StringP("conflict", "c", "stop", "Occupied-target policy: stop, skip, replace")
	cmd.Flags().StringArrayP("segment", "s", nil, "Set custom segment value (KEY=value, repeatable)")
	cmd.Flags().Bool("allow-dirty", false, "Allow planning against layers with uncommitted changes")

	return cmd
}

// runDeployV2 implements the deploy command on the deploy package (phase-8 step 47 slice 1).
//
// Parsing stays here (cobra/viper are command-layer); planning and execution live in
// [deploy.Execute] — tree walk, layer pinning, per-scope graphs, store persistence, and reporting.
func runDeployV2(cmd *cobra.Command, args []string) error {

	cfg, err := parseDeployConfig(cmd, args)
	if err != nil {
		return err
	}

	// The manifest planner defaults its platform token (via platform.DetectToken) and its registry client;
	// writ supplies only the flags that are writ's to know.
	return deploy.Execute(cmd.Context(), &deploy.Config{
		SourceRoot:      cfg.SourceRoot,
		TargetRoot:      cfg.TargetRoot,
		LayerSources:    cfg.LayerSources,
		Projects:        cfg.Projects,
		Segments:        cfg.Segments,
		Vars:            cfg.TemplateData,
		Conflict:        cfg.ConflictPolicy,
		ManifestPlanner: &lore.Planner{DryRun: cfg.DryRun},
		AllowDirty:      cfg.AllowDirty,
		DryRun:          cfg.DryRun,
		Verbose:         cfg.Verbose,
	})
}

// expandPath expands ~ to $HOME in paths.
func expandPath(path string) string {

	if strings.HasPrefix(path, "~/") {
		return xdg.UserHomeDir() + path[1:]
	}
	if path == "~" {
		return xdg.UserHomeDir()
	}

	return path
}

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
  writ decommission --prune noblefactor      # Also remove empty parent directories
  writ decommission --force noblefactor      # Skip confirmation prompts`,
		Args: cobra.MinimumNArgs(1),
		RunE: runDecommission,
	}

	cmd.Flags().Bool("prune", false, "Remove empty parent directories after file removal")

	return cmd
}

// runDecommission implements the decommission command on the decommission package (phase-8 step 47 slice 2).
func runDecommission(cmd *cobra.Command, args []string) error {

	cfg := parseDecommissionConfig(cmd, args)

	return decommission.Execute(cmd.Context(), &decommission.Config{
		Projects: cfg.Projects,
		Prune:    cfg.Prune,
		DryRun:   cfg.DryRun,
		Verbose:  cfg.Verbose,
	})
}

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

	return cmd
}

// runUpgrade implements the upgrade command on the upgrade package (phase-8 step 47 slice 2).
func runUpgrade(cmd *cobra.Command, args []string) error {

	cfg := parseUpgradeConfig(cmd, args)

	return upgrade.Execute(cmd.Context(), &upgrade.Config{
		Projects: cfg.Projects,
		Force:    cfg.Force,
		Segments: cfg.Segments,
		Vars:     cfg.TemplateData,
		DryRun:   cfg.DryRun,
		Verbose:  cfg.Verbose,
	})
}

func newReconcileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reconcile [<project>...]",
		Short: "Report deployed state: what should be present, from where, and what's missing or different",
		Long: `Report deployed state: what should be present, where it should have come from, and
what's missing or different.

The report is derived from the store (the run index plus the persisted graphs and
traces) — never from a directory scan — and has four sections: the registered layer
tree, the deployed inventory classified against the live filesystem, the package
operations writ's runs performed, and store health. Reconcile produces a report; each
finding names the lifecycle command that repairs it. The report is one JSON document,
rendered by --output like every other result; --jq '.entries' selects the delta alone.

Entry states:
  linked             Symlink present and resolving to its source
  copied             Copied file present (encrypted content is not compared)
  missing            Deployed target is gone                     → writ deploy
  conflict           Something else occupies the target
  orphan             Target's source no longer exists            → writ decommission
  stale              Source changed since the run                → writ upgrade
  modified           Target edited out-of-band                   → writ upgrade --force
  modified-or-stale  Differs, but the run predates recorded content
                     identity: attribution indeterminate         → writ upgrade`,
		Example: `  writ reconcile                 # Report everything writ has deployed
  writ reconcile noblefactor     # Report one project
  writ reconcile -o json         # Machine-readable report
  writ reconcile -o table        # Aligned columns`,
		RunE: runReconcile,
	}

	return cmd
}

// runReconcile implements the reconcile command on the reconcile package.
func runReconcile(cmd *cobra.Command, args []string) error {

	cfg := parseReconcileConfig(args)

	report, err := reconcile.BuildReport(cmd.Context(), &reconcile.Config{
		Projects: cfg.Projects,
		Verbose:  cfg.Verbose,
		Segments: cfg.Segments,
		Vars:     cfg.TemplateData,
	})
	if err != nil {
		return err
	}

	// The report is the result. Rendering is the pipeline's, so every --output value, --jq and --filter
	// apply to it exactly as they do to any other command's result.
	return emitResult(cmd, report)
}

func newVerifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify <document>...",
		Short: "Verify the publisher signatures of graph and trace documents",
		Long: `Verify the publisher signatures of graph and trace documents.

Each document is re-canonicalized and its raw ssh-ed25519 signature checked over
the namespace-prefixed canonical bytes; the publisher key resolves against the
verifier's allowed_signers trust list. What each outcome does to the exit status
is the signing policy ladder:

  ignore           No verification at all
  report           (default) Report every outcome; never fail
  reject_external  Reject unsigned/invalid/untrusted documents from OUTSIDE this
                   machine's own store; own-store documents only report
  reject           Reject anything that is not valid`,
		Example: `  writ verify ~/.local/state/devlore/graphs/*.yaml
  writ verify --signing-policy=reject_external ~/Downloads/shared-plan.yaml
  writ verify -o table --signing-policy=reject trace.yaml`,
		Args: cobra.MinimumNArgs(1),
		RunE: runVerify,
	}

	cmd.Flags().String("signing-policy", "report", "Verification policy: ignore, report, reject_external, reject")
	cmd.Flags().String("allowed-signers", "", "Trust-list path (default: <config>/devlore/allowed_signers)")

	return cmd
}

// runVerify implements the verify command on the verify package (phase-8 step 46).
func runVerify(cmd *cobra.Command, args []string) error {

	policyValue, _ := cmd.Flags().GetString("signing-policy") //nolint:errcheck // flag registered above
	policy, err := signing.ParsePolicy(policyValue)
	if err != nil {
		return err
	}

	allowedSigners, _ := cmd.Flags().GetString("allowed-signers") //nolint:errcheck // flag registered above

	reports, err := verify.Execute(cmd.Context(), &verify.Config{
		Paths:          args,
		Policy:         policy,
		AllowedSigners: allowedSigners,
	})

	// The reports are the result and are emitted whether or not the policy rejected: a rejection is the
	// answer to the question, not a reason to withhold it.
	if emitErr := emitResult(cmd, reports); emitErr != nil {
		return emitErr
	}

	return err
}

// outputOptions holds the common set's values for every writ command.
//
// Bound once on the root by [NewRootCmd]; read by [emitResult] wherever a command has a result to render.
var outputOptions cli.SinkOptions

// emitResult renders a command's result to stdout through the shared pipeline.
//
// Parameters:
//   - `cmd`: the running command, for its output writer.
//   - `value`: the result to render.
//
// Returns:
//   - `error`: non-nil when the pipeline cannot be built or the value cannot be rendered.
func emitResult(cmd *cobra.Command, value any) error {

	pipeline, err := cli.BuildPipeline(outputOptions, cmd.OutOrStdout())
	if err != nil {
		return err
	}

	return pipeline.Emit(value)
}

// getConfiguredRepo returns the path for a layer, or empty string if it doesn't exist.
// Layers are directories (or symlinks) at ~/.local/share/devlore/writ/layers/{layer}/
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
