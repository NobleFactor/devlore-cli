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

	"github.com/NobleFactor/devlore-cli/cmd/lore/lore/onboard"
	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/internal/config"
	"github.com/NobleFactor/devlore-cli/internal/lorepackage"
	"github.com/NobleFactor/devlore-cli/internal/manifest"
	"github.com/NobleFactor/devlore-cli/internal/model"
	"github.com/NobleFactor/devlore-cli/internal/registry"
	"github.com/NobleFactor/devlore-cli/pkg/op"
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

// resolvedPackage holds a package with its resolution confidence and features.
type resolvedPackage struct {
	pkg        *lorepackage.Release
	confidence lorepackage.Confidence
	features   []string
}

func runDeploy(cmd *cobra.Command, args []string) error {
	// 1. Parse config
	cfg, err := parseLoreDeployConfig(cmd, args)
	if err != nil {
		return err
	}

	// 2. Resolve packages and check confidence
	resolved, err := resolvePackages(cfg)
	if err != nil {
		return err
	}

	// 3. Handle low confidence packages
	resolved, err = filterLowConfidence(resolved, cfg)
	if err != nil {
		return err
	}

	if len(resolved) == 0 {
		return fmt.Errorf("no packages to deploy")
	}

	// 4. Execute deployments
	return executeDeployments(cmd.Context(), resolved, cfg)
}

// parseLoreDeployConfig parses flags and arguments into a deploy config.
func parseLoreDeployConfig(cmd *cobra.Command, args []string) (*loreDeployConfig, error) { //nolint:unparam // error return reserved for future use
	features, _ := cmd.Flags().GetStringArray("with") //nolint:errcheck // flag registered by AddCommand
	knownOnly, _ := cmd.Flags().GetBool("known-only") //nolint:errcheck // flag registered by AddCommand
	force, _ := cmd.Flags().GetBool("force")          //nolint:errcheck // flag registered by AddCommand
	parallel, _ := cmd.Flags().GetInt("parallel")     //nolint:errcheck // flag registered by AddCommand

	cfg := &loreDeployConfig{
		GlobalFeatures: features,
		DryRun:         viper.GetBool("lore.dry-run"),
		Verbose:        viper.GetBool("lore.verbose"),
		KnownOnly:      knownOnly,
		Force:          force,
		Parallel:       parallel,
	}

	// Parse args into package requests
	for _, arg := range args {
		if arg != "" && arg[0] == '@' {
			// Manifest file - use shared manifest package
			m, err := manifest.Load(arg[1:])
			if err != nil {
				cli.Error("Error reading manifest %q: %v", arg[1:], err)
				continue
			}
			cli.Note("Loaded %d packages from %s", len(m.Packages), arg[1:])
			for _, entry := range m.Packages {
				cfg.Packages = append(cfg.Packages, packageRequest{
					Name:     entry.Name,
					Features: entry.With,
				})
			}
		} else {
			// Direct package name
			cfg.Packages = append(cfg.Packages, packageRequest{Name: arg})
		}
	}

	return cfg, nil
}

// loreDeployConfig holds parsed configuration for lore deploy.
type loreDeployConfig struct {
	Packages       []packageRequest
	GlobalFeatures []string
	DryRun         bool
	Verbose        bool
	KnownOnly      bool
	Force          bool
	Parallel       int
}

// packageRequest represents a package to deploy with its features.
type packageRequest struct {
	Name     string
	Features []string
}

// resolvePackages resolves all packages and reports their confidence.
func resolvePackages(cfg *loreDeployConfig) ([]resolvedPackage, error) {
	regClient, err := lorepackage.NewRegistry()
	if err != nil {
		return nil, fmt.Errorf("creating registry client: %w", err)
	}

	targetPlatform := detectPlatform()

	fmt.Println("\nResolving packages...")
	fmt.Printf("%-30s %-10s %-8s %s\n", "PACKAGE", "SOURCE", "CONF", "STATUS")
	fmt.Printf("%s\n", strings.Repeat("-", 70))

	var resolved []resolvedPackage
	for _, req := range cfg.Packages {
		pkg, confidence, err := regClient.ResolveWithConfidence(req.Name, targetPlatform)
		if err != nil {
			cli.Error("Error resolving package %q: %v", req.Name, err)
			continue
		}

		status := "ready"
		if confidence == lorepackage.ConfidenceLow {
			status = "unverified"
		}

		featStr := ""
		if len(req.Features) > 0 {
			featStr = " [" + strings.Join(req.Features, ", ") + "]"
		}

		fmt.Printf("%-30s %-10s %-8s %s%s\n", pkg.Name, pkg.Source, confidence, status, featStr)

		resolved = append(resolved, resolvedPackage{
			pkg:        pkg,
			confidence: confidence,
			features:   req.Features,
		})
	}

	return resolved, nil
}

// filterLowConfidence handles low confidence packages based on config.
func filterLowConfidence(resolved []resolvedPackage, cfg *loreDeployConfig) ([]resolvedPackage, error) {
	var hasLow bool
	for _, rp := range resolved {
		if rp.confidence == lorepackage.ConfidenceLow {
			hasLow = true
			break
		}
	}

	if !hasLow || cfg.Force {
		return resolved, nil
	}

	if cfg.KnownOnly {
		var filtered []resolvedPackage
		for _, rp := range resolved {
			if rp.confidence != lorepackage.ConfidenceLow {
				filtered = append(filtered, rp)
			} else {
				cli.Warn("Skipping %s (LOW confidence, --known-only specified)", rp.pkg.Name)
			}
		}
		if len(filtered) == 0 {
			return nil, fmt.Errorf("no packages remaining after filtering low confidence")
		}
		return filtered, nil
	}

	// Prompt user
	fmt.Printf("\n⚠️  Some packages have LOW confidence (not verified to exist).\n")
	fmt.Printf("Use --force to proceed or --known-only to skip them.\n")
	fmt.Printf("\nProceed anyway? [y/N]: ")

	var response string
	_, _ = fmt.Scanln(&response) //nolint:errcheck
	if !strings.EqualFold(response, "y") {
		return nil, fmt.Errorf("deployment canceled by user")
	}
	return resolved, nil
}

// executeDeployments builds and runs the execution graph for each resolved package.
func executeDeployments(ctx context.Context, resolved []resolvedPackage, cfg *loreDeployConfig) error {
	if ctx == nil {
		ctx = context.Background()
	}

	fmt.Println("\nDeploying packages...")

	// Create action registry and executor.
	// op.NewReceiverRegistry() returns a registry populated with all announced providers.
	actionReg := op.NewReceiverRegistry()

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	root, err := op.NewConfinedRoot(wd)
	if err != nil {
		return fmt.Errorf("open root %s: %w", wd, err)
	}

	spec := op.NewRuntimeEnvironmentSpec("lore", actionReg).
		WithRoot(root).
		WithDryRun(cfg.DryRun)

	executor := op.NewGraphExecutor(spec)

	var lastErr error
	for _, rp := range resolved {
		// Merge global and package-specific features
		features := mergeFeatures(rp.features, cfg.GlobalFeatures)

		// Build the execution graph for this package
		buildResult, err := Build(BuildConfig{
			Packages:       []string{rp.pkg.Name},
			Platform:       detectPlatform(),
			Features:       features,
			DryRun:         cfg.DryRun,
			ActionRegistry: actionReg,
		})
		if err != nil {
			cli.Error("Error building graph for %q: %v", rp.pkg.Name, err)
			lastErr = err
			continue
		}

		if len(buildResult.Graph.Nodes()) == 0 {
			if cfg.Verbose {
				cli.Note("No actions for %q", rp.pkg.Name)
			}
			continue
		}

		if _, err := executor.Run(buildResult.Graph); err != nil {
			cli.Error("Error deploying %q: %v", rp.pkg.Name, err)
			lastErr = err
			continue
		}

		// Check for failures via graph state
		if buildResult.Graph.State == op.StateFailed {
			cli.Error("Deployment failed for %q", rp.pkg.Name)
			lastErr = fmt.Errorf("deployment failed for %s", rp.pkg.Name)
		}
	}

	return lastErr
}

// mergeFeatures combines per-package features with global features, deduplicating.
func mergeFeatures(pkg, global []string) []string {
	seen := make(map[string]bool)
	var merged []string

	for _, f := range pkg {
		if !seen[f] {
			seen[f] = true
			merged = append(merged, f)
		}
	}
	for _, f := range global {
		if !seen[f] {
			seen[f] = true
			merged = append(merged, f)
		}
	}

	return merged
}

func newUpgradeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade [@<receipt>]",
		Short: "Upgrade previously deployed packages to newer versions",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("upgrade: not yet implemented")
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
			return fmt.Errorf("decommission: not yet implemented")
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
			return fmt.Errorf("reconcile: not yet implemented")
		},
	}
	return cmd
}

func newBundleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bundle @<manifest> -o <output>",
		Short: "Create self-extracting deployment bundles for air-gapped environments",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("bundle: not yet implemented")
		},
	}

	cmd.Flags().StringP("output", "o", "", "Promise bundle path")
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
			return fmt.Errorf("manifest create: not yet implemented")
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
			return fmt.Errorf("manifest validate: not yet implemented")
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
			return fmt.Errorf("manifest test: not yet implemented")
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
			return fmt.Errorf("manifest show: not yet implemented")
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
			return fmt.Errorf("manifest update: not yet implemented")
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
  HIGH   - PkgPath is in the lore registry with full lifecycle support
  MEDIUM - PkgPath found in native PM and verified to exist
  LOW    - PkgPath synthesized but not verified

Use --lore-only to search only the lore lorepackage.
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

	loreOnly, _ := cmd.Flags().GetBool("lore-only")     //nolint:errcheck // flag registered by AddCommand
	nativeOnly, _ := cmd.Flags().GetBool("native-only") //nolint:errcheck // flag registered by AddCommand
	limit, _ := cmd.Flags().GetInt("limit")             //nolint:errcheck // flag registered by AddCommand

	// Create registry client
	regClient, err := lorepackage.NewRegistry()
	if err != nil {
		return fmt.Errorf("creating registry client: %w", err)
	}

	// Build search options
	opts := lorepackage.SearchOptions{
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
			name += " *"
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
			return fmt.Errorf("list: not yet implemented")
		},
	}

	cmd.Flags().String("format", "table", "Promise format (table, manifest, json)")

	return cmd
}

func newResolveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolve <package>",
		Short: "Show how a package would be installed on this platform",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("resolve: not yet implemented")
		},
	}
	return cmd
}

func newUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Synchronize the local registry cache from the central registry",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("update: not yet implemented")
		},
	}
	return cmd
}

func newOnboardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "onboard --from <source>",
		Short: "Parse wiki or script and generate packages-manifest.yaml and config",
		Long: `Parse an onboarding wiki page or setup script and generate both a
packages-manifest.yaml file and a config/ directory with configuration files.

Lore uses AI to extract installation steps, map them to known registry packages,
and flag org-specific items for human review.

After onboarding, use 'lore deploy @packages-manifest.yaml' to install software,
then 'writ adopt --from-receipt' to bring the generated config into your
environment repository.`,
		Example: `  lore onboard --from https://wiki.acme.com/backend-setup
  lore onboard --from ~/scripts/setup.sh

  # Full workflow:
  lore onboard --from https://wiki.acme.com/setup
  lore deploy @packages-manifest.yaml
  writ adopt --from-receipt`,
		RunE: runOnboard,
	}

	cmd.Flags().String("from", "", "Source URL or file path")
	cmd.Flags().String("output", "", "Promise directory path (default: current directory)")
	cmd.Flags().String("format", "plain", "Manifest format (plain, yaml)")
	cmd.Flags().Bool("verbose", false, "Show AI reasoning")
	cmd.Flags().Bool("explain", false, "Show detailed reasoning for each confidence decision")
	cmd.Flags().Int("max-fetches", 5, "Maximum additional URLs to fetch")
	_ = cmd.MarkFlagRequired("from") //nolint:errcheck

	return cmd
}

func runOnboard(cmd *cobra.Command, args []string) error { //nolint:gocognit,gocyclo
	ctx := cmd.Context()

	source, _ := cmd.Flags().GetString("from")         //nolint:errcheck // flag registered by AddCommand
	outputDir, _ := cmd.Flags().GetString("output")    //nolint:errcheck // flag registered by AddCommand
	format, _ := cmd.Flags().GetString("format")       //nolint:errcheck // flag registered by AddCommand
	verbose, _ := cmd.Flags().GetBool("verbose")       //nolint:errcheck // flag registered by AddCommand
	explain, _ := cmd.Flags().GetBool("explain")       //nolint:errcheck // flag registered by AddCommand
	maxFetches, _ := cmd.Flags().GetInt("max-fetches") //nolint:errcheck // flag registered by AddCommand

	if outputDir == "" {
		outputDir = "."
	}

	// Get AI provider
	providerName := viper.GetString("lore.model.provider")
	if providerName == "" {
		return fmt.Errorf("AI provider required; configure with 'lore config model'")
	}

	aiProvider, err := model.NewProvider(config.ModelConfig{
		Provider: providerName,
		Name:     viper.GetString("lore.model.model"),
		Endpoint: viper.GetString("lore.model.endpoint"),
		APIKey:   viper.GetString("lore.model.api_key"),
	})
	if err != nil {
		return fmt.Errorf("creating AI provider: %w", err)
	}

	// Get registry
	reg, err := lorepackage.NewRegistry()
	if err != nil {
		return fmt.Errorf("creating registry: %w", err)
	}

	// Ensure registry is synced
	if !reg.Exists() {
		cli.Note("Syncing registry...")
		if _, err := reg.Sync(ctx, registry.SyncOptions{}); err != nil {
			return fmt.Errorf("syncing registry: %w", err)
		}
	}

	cli.Note("Analyzing %s...", source)

	opts := onboard.Options{
		Source:     source,
		OutputDir:  outputDir,
		Format:     format,
		Verbose:    verbose,
		Explain:    explain,
		Provider:   aiProvider,
		RegClient:  reg,
		MaxFetches: maxFetches,
	}

	result, err := onboard.Run(ctx, opts)
	if err != nil {
		return err
	}

	// Display results
	if result.Product != nil {
		cli.Success("Discovered: %s (%s)", result.Product.Name, result.Product.Category)
		if result.Product.Vendor != "" {
			cli.Note("  Vendor: %s", result.Product.Vendor)
		}
		if result.Product.Version != "" {
			cli.Note("  Version: %s", result.Product.Version)
		}
	}

	if result.Complexity != nil {
		switch result.Complexity.Rating {
		case "simple":
			cli.Note("Complexity: simple")
		case "moderate":
			cli.Note("Complexity: moderate")
		case "complex":
			cli.Warn("Complexity: complex")
			for _, concern := range result.Complexity.Concerns {
				cli.Warn("  - %s", concern)
			}
		}
	}

	if len(result.Slots) > 0 {
		cli.Note("Extracted %d configuration slots", len(result.Slots))
	}

	// Write manifest
	manifestPath := outputDir + "/packages-manifest.yaml"
	if err := os.WriteFile(manifestPath, []byte(result.Manifest), 0o600); err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}

	cli.Success("Wrote %s", manifestPath)
	cli.Note("")
	cli.Note("Next steps:")
	cli.Note("  1. Review the generated manifest")
	cli.Note("  2. lore deploy @packages-manifest.yaml")

	return nil
}

func newInspectCmd() *cobra.Command {
	var opts cli.SinkOptions

	cmd := &cobra.Command{
		Use:   "inspect <package>",
		Short: "Show detailed information about a package",
		Long: `Show detailed information about a package.

Displays the resolved lifecycle manifest, platform support, features,
dependencies, and deployment history for a package.

Promise is JSON by default for scripting. Use --format for alternatives.`,
		Example: `  lore inspect docker
  lore inspect kubectl --format yaml
  lore inspect docker --format '{{.ReceiverName}}\t{{.Version}}'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("inspect: not yet implemented")
		},
	}

	cli.AddOutputFlags(cmd, &opts)

	return cmd
}

func newPublishCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "publish <name>",
		Short: "Submit a package manifest to the registry",
		Long: `Submit a validated package manifest to the lorepackage.

Runs final validation, creates a pull request for community review,
and triggers automated testing on macOS, Linux, and Windows.`,
		Example: `  lore publish mypackage`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("publish: not yet implemented")
		},
	}

	return cmd
}

func newAuditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "View security audit log entries",
		Long: `View security audit log entries.

The audit log records security-sensitive actions:
  - pmm.fetch: PkgPath fetch with signature status
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
			return fmt.Errorf("audit: not yet implemented")
		},
	}

	cmd.Flags().String("since", "", "Show entries since duration (e.g., 7d, 24h)")
	cmd.Flags().String("package", "", "Filter by package name")
	cmd.Flags().String("event", "", "Filter by event type (pmm.fetch, pmm.verify, privilege, binary, phase)")

	return cmd
}
