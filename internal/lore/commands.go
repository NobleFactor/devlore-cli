// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package lore

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/internal/lore/pipeline"
	"github.com/NobleFactor/devlore-cli/internal/registry"
)

func newDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy [flags] <package>... | @<manifest>",
		Short: "Deploy packages to the local system",
		Long: `Deploy packages to the local system.

This is the primary command for installing software. Lore resolves how to
install each package on your platform, then executes the four-phase pipeline:
prepare → install → provision → verify.

Packages can be specified directly or via manifest files (prefixed with @).`,
		Example: `  lore deploy kubectl gh docker
  lore deploy docker --with rootless
  lore deploy @team.manifest
  lore deploy @base.manifest @team.manifest neovim --with lsp`,
		Args: cobra.MinimumNArgs(1),
		RunE: runDeploy,
	}

	cmd.Flags().Bool("known-only", false, "Skip LOW CONFIDENCE items")
	cmd.Flags().Bool("force", false, "Proceed with LOW CONFIDENCE items without prompting")
	cmd.Flags().StringArray("with", nil, "Enable feature (can be repeated)")
	cmd.Flags().Int("parallel", 1, "Install n packages concurrently")

	return cmd
}

// resolvedPackage holds a package with its resolution confidence.
type resolvedPackage struct {
	pkg        *registry.LorePackage
	confidence registry.Confidence
}

func runDeploy(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// Get flags
	features, _ := cmd.Flags().GetStringArray("with")
	dryRun := viper.GetBool("lore.dry-run")
	verbose := viper.GetBool("lore.verbose")
	knownOnly, _ := cmd.Flags().GetBool("known-only")
	force, _ := cmd.Flags().GetBool("force")

	// Create registry client
	regClient, err := registry.NewDefault()
	if err != nil {
		return fmt.Errorf("creating registry client: %w", err)
	}

	// Detect platform for package resolution
	platform := pipeline.DetectPlatform()

	// Phase 1: Resolve all packages and show confidence
	var resolved []resolvedPackage
	var hasLowConfidence bool

	fmt.Println("\nResolving packages...")
	fmt.Printf("%-30s %-10s %-8s %s\n", "PACKAGE", "SOURCE", "CONF", "STATUS")
	fmt.Printf("%s\n", strings.Repeat("-", 70))

	for _, arg := range args {
		// TODO: Handle @manifest syntax
		if len(arg) > 0 && arg[0] == '@' {
			cli.Warn("Manifest files not yet supported: %s", arg)
			continue
		}

		// Resolve package with confidence
		pkg, confidence, err := regClient.ResolveWithConfidence(arg, platform)
		if err != nil {
			cli.Error("Error resolving package %q: %v", arg, err)
			continue
		}

		// Show resolution result
		confStr := confidence.String()
		sourceStr := string(pkg.Source)
		status := "ready"

		if confidence == registry.ConfidenceLow {
			hasLowConfidence = true
			status = "unverified"
		}

		fmt.Printf("%-30s %-10s %-8s %s\n", pkg.Name, sourceStr, confStr, status)

		resolved = append(resolved, resolvedPackage{pkg: pkg, confidence: confidence})
	}

	if len(resolved) == 0 {
		return fmt.Errorf("no packages resolved")
	}

	// Phase 2: Handle low confidence packages
	if hasLowConfidence && !force {
		if knownOnly {
			// Filter out low confidence packages
			var filtered []resolvedPackage
			for _, rp := range resolved {
				if rp.confidence != registry.ConfidenceLow {
					filtered = append(filtered, rp)
				} else {
					cli.Warn("Skipping %s (LOW confidence, --known-only specified)", rp.pkg.Name)
				}
			}
			resolved = filtered

			if len(resolved) == 0 {
				return fmt.Errorf("no packages remaining after filtering low confidence")
			}
		} else {
			// Prompt user
			fmt.Printf("\n⚠️  Some packages have LOW confidence (not verified to exist).\n")
			fmt.Printf("Use --force to proceed or --known-only to skip them.\n")
			fmt.Printf("\nProceed anyway? [y/N]: ")

			var response string
			fmt.Scanln(&response)
			if strings.ToLower(response) != "y" {
				return fmt.Errorf("deployment cancelled by user")
			}
		}
	}

	// Phase 3: Execute deployment
	fmt.Println("\nDeploying packages...")

	// Create executor config
	cfg := pipeline.ExecutorConfig{
		Features: features,
		DryRun:   dryRun,
		Verbose:  verbose,
		Output:   os.Stdout,
	}

	executor := pipeline.NewExecutor(cfg)

	var lastErr error
	for _, rp := range resolved {
		// Execute the deploy pipeline
		result, err := executor.ExecutePackage(ctx, rp.pkg, pipeline.OpDeploy)
		if err != nil {
			cli.Error("Error deploying %q: %v", rp.pkg.Name, err)
			lastErr = err
			continue
		}

		if !result.Success {
			cli.Error("Deployment failed for %q", rp.pkg.Name)
			lastErr = fmt.Errorf("deployment failed for %s", rp.pkg.Name)
		}
	}

	return lastErr
}

func newUpgradeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade [@<receipt>]",
		Short: "Upgrade previously deployed packages to newer versions",
		RunE: func(cmd *cobra.Command, args []string) error {
			cli.Note("upgrade: not yet implemented")
			return nil
		},
	}
	return cmd
}

func newDecommissionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "decommission [@<receipt>] [<package>...]",
		Short: "Remove packages and clean up their resources",
		Long:  `Remove packages and clean up their resources.`,
		Example: `  lore decommission @workstation
  lore decommission docker kubectl`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cli.Note("decommission: not yet implemented")
			return nil
		},
	}

	return cmd
}

func newReconcileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reconcile @<receipt>",
		Short: "Compare deployment receipts against actual system state",
		Long: `Compare expected state (from receipt) against actual system state and report drift.

Use this to verify that your system matches what was deployed, detect
configuration changes, or audit compliance.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cli.Note("reconcile: not yet implemented")
			return nil
		},
	}
	return cmd
}

func newBundleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bundle @<manifest> -o <output>",
		Short: "Create self-extracting deployment bundles for air-gapped environments",
		RunE: func(cmd *cobra.Command, args []string) error {
			cli.Note("bundle: not yet implemented")
			return nil
		},
	}

	cmd.Flags().StringP("output", "o", "", "Output bundle path")
	cmd.Flags().String("platform", "", "Target platform (e.g., linux/fedora)")
	cmd.Flags().StringArray("include-repo", nil, "Include repository in bundle")

	return cmd
}

func newManifestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "manifest <subcommand>",
		Short: "Create and manage package lifecycle manifests",
	}

	createCmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Scaffold a new package manifest",
		Long: `Scaffold a new package manifest in the staging directory.

Use --ai for AI-assisted creation from documentation.
Use --from to import from existing scripts or directories.
Use --from-url to import from upstream documentation.`,
		Example: `  lore manifest create mypackage
  lore manifest create postgresql --ai --from-url https://postgresql.org/download/
  lore manifest create pandoc --from ~/scripts/install-pandoc/`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli.Note("manifest create %s: not yet implemented", args[0])
			return nil
		},
	}
	createCmd.Flags().Bool("ai", false, "Enable AI-assisted creation")
	createCmd.Flags().String("from", "", "Import from existing scripts or directory")
	createCmd.Flags().String("from-url", "", "Import from upstream documentation URL")
	createCmd.Flags().Bool("resume", false, "Resume interrupted AI session")
	cmd.AddCommand(createCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "validate <name>",
		Short: "Validate a package manifest",
		Long: `Validate a package manifest against the schema.

Checks:
  - Schema validity (lifecycle.yaml conforms to schema)
  - Phase files exist (referenced .star files present)
  - Starlark syntax (files parse without errors)
  - Contract compliance (each phase has main() function)
  - Feature consistency (features match phase conditionals)
  - Platform coverage (conditionals cover declared platforms)`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli.Note("manifest validate %s: not yet implemented", args[0])
			return nil
		},
	})

	testCmd := &cobra.Command{
		Use:   "test <name>",
		Short: "Dry-run a package manifest on current system",
		Long: `Test a package manifest without modifying the system.

Shows what each phase would do. Use --with to test specific features.
Use --set to test with specific settings. Use --debug for verbose output.`,
		Example: `  lore manifest test mypackage
  lore manifest test mypackage --with completions --with debug-symbols
  lore manifest test mypackage --set shell=zsh`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli.Note("manifest test %s: not yet implemented", args[0])
			return nil
		},
	}
	testCmd.Flags().StringArray("with", nil, "Enable feature for testing")
	testCmd.Flags().StringArray("set", nil, "Set setting value (key=value)")
	testCmd.Flags().Bool("debug", false, "Show debug-level messages")
	testCmd.Flags().String("break", "", "Break at specific phase (prepare, install, provision, verify)")
	cmd.AddCommand(testCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "show <name>",
		Short: "Display package manifest details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli.Note("manifest show %s: not yet implemented", args[0])
			return nil
		},
	})

	updateCmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Modify an existing package manifest",
		Long: `Update an existing package manifest.

Add new features, platform support, or import updates from documentation.`,
		Example: `  lore manifest update docker --add-feature gpu-support
  lore manifest update docker --add-platform windows
  lore manifest update docker --from-url https://docs.docker.com/engine/install/`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli.Note("manifest update %s: not yet implemented", args[0])
			return nil
		},
	}
	updateCmd.Flags().String("add-feature", "", "Add a new feature to the package")
	updateCmd.Flags().String("add-platform", "", "Add platform support")
	updateCmd.Flags().String("from-url", "", "Import updates from documentation URL")
	cmd.AddCommand(updateCmd)

	return cmd
}

func newSearchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search available packages in the registry and native package managers",
		Long: `Search for packages across the lore registry and native package managers.

Results show the package source and confidence level:
  HIGH   - Package is in the lore registry with full lifecycle support
  MEDIUM - Package found in native PM and verified to exist
  LOW    - Package synthesized but not verified

Use --lore-only to search only the lore registry.
Use --native-only to search only the native package manager.`,
		Example: `  lore search docker
  lore search kubectl --lore-only
  lore search ripgrep --limit 10`,
		Args: cobra.ExactArgs(1),
		RunE: runSearch,
	}

	cmd.Flags().String("platform", "", "Filter by platform")
	cmd.Flags().Bool("lore-only", false, "Search only the lore registry")
	cmd.Flags().Bool("native-only", false, "Search only the native package manager")
	cmd.Flags().Int("limit", 25, "Maximum results per source")

	return cmd
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := args[0]

	loreOnly, _ := cmd.Flags().GetBool("lore-only")
	nativeOnly, _ := cmd.Flags().GetBool("native-only")
	limit, _ := cmd.Flags().GetInt("limit")

	// Create registry client
	regClient, err := registry.NewDefault()
	if err != nil {
		return fmt.Errorf("creating registry client: %w", err)
	}

	// Build search options
	opts := registry.SearchOptions{
		IncludeLore:   !nativeOnly,
		IncludeNative: !loreOnly,
		Limit:         limit,
	}

	// Perform search
	results, err := regClient.Search(query, opts)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		cli.Note("No packages found matching %q", query)
		return nil
	}

	// Print results
	fmt.Printf("\n%-30s %-10s %-8s %-10s %s\n", "PACKAGE", "SOURCE", "CONF", "VERSION", "DESCRIPTION")
	fmt.Printf("%-30s %-10s %-8s %-10s %s\n", strings.Repeat("-", 30), strings.Repeat("-", 10), strings.Repeat("-", 8), strings.Repeat("-", 10), strings.Repeat("-", 30))

	for _, r := range results {
		// Format confidence with color indicator
		confStr := r.Confidence.String()
		sourceStr := string(r.Source)

		// Truncate description if too long
		desc := r.Description
		if len(desc) > 50 {
			desc = desc[:47] + "..."
		}

		version := r.Version
		if version == "" {
			version = "-"
		}

		// Add installed indicator
		name := r.Name
		if r.Installed {
			name = name + " *"
		}

		fmt.Printf("%-30s %-10s %-8s %-10s %s\n", name, sourceStr, confStr, version, desc)
	}

	fmt.Printf("\n* = installed\n")

	return nil
}

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List deployed packages from pipeline receipts",
		RunE: func(cmd *cobra.Command, args []string) error {
			cli.Note("list: not yet implemented")
			return nil
		},
	}

	cmd.Flags().String("format", "table", "Output format (table, manifest, json)")

	return cmd
}

func newResolveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolve <package>",
		Short: "Show how a package would be installed on this platform",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli.Note("resolve %s: not yet implemented", args[0])
			return nil
		},
	}
	return cmd
}

func newUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Synchronize the local registry cache from the central registry",
		RunE: func(cmd *cobra.Command, args []string) error {
			cli.Note("update: not yet implemented")
			return nil
		},
	}
	return cmd
}

func newOnboardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "onboard --from <source>",
		Short: "Parse wiki or script and generate packages.manifest and config",
		Long: `Parse an onboarding wiki page or setup script and generate both a
packages.manifest file and a config/ directory with configuration files.

Lore uses AI to extract installation steps, map them to known registry packages,
and flag org-specific items for human review.

After onboarding, use 'lore deploy @packages.manifest' to install software,
then 'writ adopt --from-receipt' to bring the generated config into your
environment repository.`,
		Example: `  lore onboard --from https://wiki.acme.com/backend-setup
  lore onboard --from ~/scripts/setup.sh

  # Full workflow:
  lore onboard --from https://wiki.acme.com/setup
  lore deploy @packages.manifest
  writ adopt --from-receipt`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cli.Note("onboard: not yet implemented")
			return nil
		},
	}

	cmd.Flags().String("from", "", "Source URL or file path")
	cmd.Flags().String("output", "", "Output directory path (default: current directory)")
	cmd.Flags().String("format", "plain", "Manifest format (plain, yaml)")
	cmd.Flags().String("registry", "", "Registry to match against")
	cmd.Flags().Bool("verbose", false, "Show AI reasoning")
	cmd.Flags().Bool("explain", false, "Show detailed reasoning for each confidence decision")
	_ = cmd.MarkFlagRequired("from")

	return cmd
}

func newInspectCmd() *cobra.Command {
	var output cli.OutputFlags

	cmd := &cobra.Command{
		Use:   "inspect <package>",
		Short: "Show detailed information about a package",
		Long: `Show detailed information about a package.

Displays the resolved lifecycle manifest, platform support, features,
dependencies, and deployment history for a package.

Output is JSON by default for scripting. Use --format for alternatives.`,
		Example: `  lore inspect docker
  lore inspect kubectl --format yaml
  lore inspect docker --format '{{.Name}}\t{{.Version}}'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli.Note("inspect %s: not yet implemented", args[0])
			return nil
		},
	}

	cli.AddOutputFlags(cmd, &output)

	return cmd
}

func newPublishCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "publish <name>",
		Short: "Submit a package manifest to the registry",
		Long: `Submit a validated package manifest to the registry.

Runs final validation, creates a pull request for community review,
and triggers automated testing on macOS, Linux, and Windows.`,
		Example: `  lore publish mypackage`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli.Note("publish %s: not yet implemented", args[0])
			return nil
		},
	}

	return cmd
}

func newAuditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "View security audit log entries",
		Long: `View security audit log entries.

The audit log records security-sensitive operations:
  - pmm.fetch: Package fetch with signature status
  - pmm.verify: Signature verification results
  - privilege.request: Sudo/elevation requests
  - binary.download: Upstream binary downloads with hash verification
  - phase.execute: Pipeline phase execution

Log location: ~/.local/share/lore/audit.log`,
		Example: `  lore audit
  lore audit --since 7d
  lore audit --package kubectl
  lore audit --event privilege`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cli.Note("audit: not yet implemented")
			return nil
		},
	}

	cmd.Flags().String("since", "", "Show entries since duration (e.g., 7d, 24h)")
	cmd.Flags().String("package", "", "Filter by package name")
	cmd.Flags().String("event", "", "Filter by event type (pmm.fetch, pmm.verify, privilege, binary, phase)")

	return cmd
}
