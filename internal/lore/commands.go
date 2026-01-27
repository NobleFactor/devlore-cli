// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package lore

import (
	"context"
	"fmt"
	"os"

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
	cmd.Flags().String("receipt", "", "Save receipt to specific path")
	cmd.Flags().Int("parallel", 1, "Install n packages concurrently")

	return cmd
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

	// Create registry client
	regClient, err := registry.NewDefault()
	if err != nil {
		return fmt.Errorf("creating registry client: %w", err)
	}

	// Create executor config
	cfg := pipeline.ExecutorConfig{
		Features: features,
		DryRun:   dryRun,
		Verbose:  verbose,
		Output:   os.Stdout,
	}

	executor := pipeline.NewExecutor(cfg)

	// Detect platform for package resolution
	platform := pipeline.DetectPlatform()

	// Process each package argument
	var lastErr error
	for _, arg := range args {
		// TODO: Handle @manifest syntax
		if len(arg) > 0 && arg[0] == '@' {
			fmt.Fprintf(os.Stderr, "Manifest files not yet supported: %s\n", arg)
			continue
		}

		// Resolve package from registry
		pkg, err := regClient.Resolve(arg, platform)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving package %q: %v\n", arg, err)
			lastErr = err
			continue
		}

		// Execute the deploy pipeline
		result, err := executor.ExecutePackage(ctx, pkg, pipeline.OpDeploy)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error deploying %q: %v\n", arg, err)
			lastErr = err
			continue
		}

		if !result.Success {
			fmt.Fprintf(os.Stderr, "Deployment failed for %q\n", arg)
			lastErr = fmt.Errorf("deployment failed for %s", arg)
		}
	}

	return lastErr
}

func newUpgradeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade [@<receipt>]",
		Short: "Upgrade previously deployed packages to newer versions",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("upgrade: not yet implemented")
			return nil
		},
	}
	return cmd
}

func newDecommissionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "decommission [@<receipt>] [<package>...]",
		Short: "Remove packages and clean up their resources",
		Long: `Remove packages and clean up their resources.

With --orphans-only, only removes packages that are no longer referenced
by any packages.manifest in the environment repository. This is used by
writ when removing a project to clean up software that's no longer needed.`,
		Example: `  lore decommission @workstation
  lore decommission docker kubectl
  lore decommission --orphans-only  # Remove unreferenced packages`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("decommission: not yet implemented")
			return nil
		},
	}

	cmd.Flags().Bool("orphans-only", false, "Only remove packages not referenced by any manifest")

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
			fmt.Println("reconcile: not yet implemented")
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
			fmt.Println("bundle: not yet implemented")
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
			fmt.Printf("manifest create %s: not yet implemented\n", args[0])
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
			fmt.Printf("manifest validate %s: not yet implemented\n", args[0])
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
			fmt.Printf("manifest test %s: not yet implemented\n", args[0])
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
			fmt.Printf("manifest show %s: not yet implemented\n", args[0])
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
			fmt.Printf("manifest update %s: not yet implemented\n", args[0])
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
		Short: "Search available packages in the registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("search %s: not yet implemented\n", args[0])
			return nil
		},
	}

	cmd.Flags().String("platform", "", "Filter by platform")

	return cmd
}

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List deployed packages from pipeline receipts",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("list: not yet implemented")
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
			fmt.Printf("resolve %s: not yet implemented\n", args[0])
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
			fmt.Println("update: not yet implemented")
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
			fmt.Println("onboard: not yet implemented")
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
			fmt.Printf("inspect %s: not yet implemented\n", args[0])
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
			fmt.Printf("publish %s: not yet implemented\n", args[0])
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
			fmt.Println("audit: not yet implemented")
			return nil
		},
	}

	cmd.Flags().String("since", "", "Show entries since duration (e.g., 7d, 24h)")
	cmd.Flags().String("package", "", "Filter by package name")
	cmd.Flags().String("event", "", "Filter by event type (pmm.fetch, pmm.verify, privilege, binary, phase)")

	return cmd
}
