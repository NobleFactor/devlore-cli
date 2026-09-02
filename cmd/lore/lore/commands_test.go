// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package lore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/lore/lore/onboard"
	"github.com/NobleFactor/devlore-cli/internal/manifest"
	"github.com/NobleFactor/devlore-cli/pkg/sink"
	"github.com/NobleFactor/devlore-cli/pkg/status"
)

func TestManifestLoad_YAML(t *testing.T) {

	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "packages.yaml")

	content := `packages:
  - name: gh
  - name: jq
  - name: ripgrep
  - name: neovim
    with: [lsp, treesitter]
  - name: docker
    with: [rootless, compose]
`
	if err := os.WriteFile(manifestPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := manifest.Load(manifestPath)
	if err != nil {
		t.Fatalf("manifest.Load: %v", err)
	}

	packages := m.Packages
	if len(packages) != 5 {
		t.Fatalf("expected 5 packages, got %d", len(packages))
	}

	if packages[0].Name != "gh" {
		t.Errorf("expected package 0 to be 'gh', got %q", packages[0].Name)
	}
	if len(packages[0].With) != 0 {
		t.Errorf("expected package 0 to have no features, got %v", packages[0].With)
	}

	if packages[3].Name != "neovim" {
		t.Errorf("expected package 3 to be 'neovim', got %q", packages[3].Name)
	}
	if len(packages[3].With) != 2 {
		t.Errorf("expected neovim to have 2 features, got %d", len(packages[3].With))
	}
	if packages[3].With[0] != "lsp" || packages[3].With[1] != "treesitter" {
		t.Errorf("expected neovim features [lsp, treesitter], got %v", packages[3].With)
	}

	if packages[4].Name != "docker" {
		t.Errorf("expected package 4 to be 'docker', got %q", packages[4].Name)
	}
	if len(packages[4].With) != 2 {
		t.Errorf("expected docker to have 2 features, got %d", len(packages[4].With))
	}
}

func TestManifestLoad_JSON(t *testing.T) {

	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "packages.json")

	content := `{
  "packages": [
    {"name": "gh"},
    {"name": "jq"},
    {"name": "neovim", "with": ["lsp", "treesitter"]}
  ]
}`
	if err := os.WriteFile(manifestPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := manifest.Load(manifestPath)
	if err != nil {
		t.Fatalf("manifest.Load: %v", err)
	}

	packages := m.Packages
	if len(packages) != 3 {
		t.Fatalf("expected 3 packages, got %d", len(packages))
	}

	if packages[0].Name != "gh" {
		t.Errorf("expected package 0 to be 'gh', got %q", packages[0].Name)
	}
	if packages[2].Name != "neovim" {
		t.Errorf("expected package 2 to be 'neovim', got %q", packages[2].Name)
	}
	if len(packages[2].With) != 2 {
		t.Errorf("expected neovim to have 2 features, got %d", len(packages[2].With))
	}
}

func TestManifestLoad_ManifestExtension(t *testing.T) {

	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "team.manifest")

	content := `packages:
  - name: kubectl
  - name: helm
  - name: terraform
    with: [aws, gcp]
`
	if err := os.WriteFile(manifestPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := manifest.Load(manifestPath)
	if err != nil {
		t.Fatalf("manifest.Load: %v", err)
	}

	packages := m.Packages
	if len(packages) != 3 {
		t.Fatalf("expected 3 packages, got %d", len(packages))
	}

	if packages[2].Name != "terraform" {
		t.Errorf("expected package 2 to be 'terraform', got %q", packages[2].Name)
	}
	if len(packages[2].With) != 2 {
		t.Errorf("expected terraform to have 2 features, got %d", len(packages[2].With))
	}
}

func TestManifestLoad_Empty(t *testing.T) {

	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "empty.yaml")

	if err := os.WriteFile(manifestPath, []byte("packages: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := manifest.Load(manifestPath)
	if err != nil {
		t.Fatalf("manifest.Load: %v", err)
	}

	if len(m.Packages) != 0 {
		t.Fatalf("expected 0 packages, got %d", len(m.Packages))
	}
}

func TestManifestLoad_NotFound(t *testing.T) {

	_, err := manifest.Load("/nonexistent/path/manifest.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

// The onboard helper tests below pin the behavior runOnboard delegates to. Their expectations are
// hand-derived from the pre-decomposition runOnboard, so a decomposition that changed behavior
// fails them rather than being absorbed by them.

// newOnboardTestCommand builds a command carrying the same flags newOnboardCmd registers, so
// parseLoreOnboardConfig can be exercised without constructing the real command tree.
//
// Parameters:
//   - `t`: the running test.
//   - `args`: the command line to parse, excluding the command name.
//
// Returns:
//   - `*cobra.Command`: the command with its flags parsed.
func newOnboardTestCommand(t *testing.T, args ...string) *cobra.Command {

	t.Helper()

	cmd := &cobra.Command{Use: "onboard"}
	cmd.Flags().String("from", "", "")
	cmd.Flags().Bool("verbose", false, "")
	cmd.Flags().Bool("explain", false, "")
	cmd.Flags().Int("max-fetches", 5, "")

	if err := cmd.Flags().Parse(args); err != nil {
		t.Fatalf("parsing flags: %v", err)
	}

	return cmd
}

func TestParseLoreOnboardConfig_DefaultsOutputDirToWorkingDirectory(t *testing.T) {

	cmd := newOnboardTestCommand(t, "--from", "https://example.test/wiki")
	cfg := parseLoreOnboardConfig(cmd, cmd.Flags().Args())

	if cfg.OutputDir != "." {
		t.Errorf("OutputDir = %q, want %q", cfg.OutputDir, ".")
	}
	if cfg.Source != "https://example.test/wiki" {
		t.Errorf("Source = %q, want %q", cfg.Source, "https://example.test/wiki")
	}
}

func TestParseLoreOnboardConfig_CarriesEveryFlag(t *testing.T) {

	cmd := newOnboardTestCommand(t,
		"--from", "setup.sh",
		"--verbose",
		"--explain",
		"--max-fetches", "9",
		"/tmp/out", // the destination is positional, never a flag
	)

	cfg := parseLoreOnboardConfig(cmd, cmd.Flags().Args())

	if cfg.Source != "setup.sh" {
		t.Errorf("Source = %q, want %q", cfg.Source, "setup.sh")
	}
	if cfg.OutputDir != "/tmp/out" {
		t.Errorf("OutputDir = %q, want %q", cfg.OutputDir, "/tmp/out")
	}
	if !cfg.Verbose {
		t.Error("Verbose = false, want true")
	}
	if !cfg.Explain {
		t.Error("Explain = false, want true")
	}
	if cfg.MaxFetches != 9 {
		t.Errorf("MaxFetches = %d, want 9", cfg.MaxFetches)
	}
}

func TestNewOnboardProvider_RefusesWhenNoProviderConfigured(t *testing.T) {

	viper.Reset()
	t.Cleanup(viper.Reset)

	_, err := newOnboardProvider()

	if err == nil {
		t.Fatal("expected an error when lore.model.provider is unset")
	}
	if !strings.Contains(err.Error(), "lore config model") {
		t.Errorf("error %q does not name the remedy 'lore config model'", err)
	}
}

func TestNewOnboardProvider_ConstructsTheConfiguredProvider(t *testing.T) {

	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("lore.model.provider", "ollama")
	viper.Set("lore.model.model", "llama3")
	viper.Set("lore.model.endpoint", "http://localhost:11434")

	provider, err := newOnboardProvider()

	if err != nil {
		t.Fatalf("newOnboardProvider: %v", err)
	}
	if provider == nil {
		t.Fatal("provider is nil")
	}
}

func TestNewOnboardProvider_WrapsAConstructionFailure(t *testing.T) {

	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("lore.model.provider", "anthropic") // requires an api_key, which is absent

	_, err := newOnboardProvider()

	if err == nil {
		t.Fatal("expected an error constructing a provider with no api_key")
	}
	if !strings.Contains(err.Error(), "creating AI provider") {
		t.Errorf("error %q is not wrapped with 'creating AI provider'", err)
	}
}

// captureNarration installs a capturing narrator for the duration of the test and returns a
// function reading what was written.
//
// Parameters:
//   - `t`: the running test.
//
// Returns:
//   - `func() string`: reads the narration captured so far.
func captureNarration(t *testing.T) func() string {

	t.Helper()

	previous := cli.UI()
	captured, buffer := sink.Capture()
	cli.SetUI(status.NewNarrator("", captured))
	t.Cleanup(func() { cli.SetUI(previous) })

	return buffer.String
}

func TestReportOnboardResult_ReportsProductVendorAndVersion(t *testing.T) {

	narration := captureNarration(t)

	reportOnboardResult(&onboard.Result{
		Product: &onboard.ProductInfo{
			Name:     "ripgrep",
			Category: "cli",
			Vendor:   "BurntSushi",
			Version:  "14.1.0",
		},
	})

	for _, want := range []string{"ripgrep", "cli", "BurntSushi", "14.1.0"} {
		if !strings.Contains(narration(), want) {
			t.Errorf("narration does not mention %q\nnarration: %s", want, narration())
		}
	}
}

func TestReportOnboardResult_OmitsAnEmptyVendorAndVersion(t *testing.T) {

	narration := captureNarration(t)

	reportOnboardResult(&onboard.Result{
		Product: &onboard.ProductInfo{Name: "fd", Category: "cli"},
	})

	for _, unwanted := range []string{"Vendor:", "Version:"} {
		if strings.Contains(narration(), unwanted) {
			t.Errorf("narration mentions %q for an absent field\nnarration: %s", unwanted, narration())
		}
	}
}

func TestReportOnboardResult_ReportsEveryConcernOfAComplexInstallation(t *testing.T) {

	narration := captureNarration(t)

	reportOnboardResult(&onboard.Result{
		Complexity: &onboard.ComplexityInfo{
			Rating:   "complex",
			Concerns: []string{"requires sudo", "builds a kernel module"},
		},
	})

	for _, want := range []string{"complex", "requires sudo", "builds a kernel module"} {
		if !strings.Contains(narration(), want) {
			t.Errorf("narration does not mention %q\nnarration: %s", want, narration())
		}
	}
}

func TestReportOnboardResult_ReportsASimpleRatingWithoutConcerns(t *testing.T) {

	narration := captureNarration(t)

	reportOnboardResult(&onboard.Result{
		Complexity: &onboard.ComplexityInfo{Rating: "simple", Concerns: []string{"unreported"}},
	})

	if !strings.Contains(narration(), "simple") {
		t.Errorf("narration does not mention the simple rating\nnarration: %s", narration())
	}
	if strings.Contains(narration(), "unreported") {
		t.Errorf("concerns are reported for a non-complex rating\nnarration: %s", narration())
	}
}

func TestReportOnboardResult_ReportsAModerateRatingWithoutConcerns(t *testing.T) {

	narration := captureNarration(t)

	reportOnboardResult(&onboard.Result{
		Complexity: &onboard.ComplexityInfo{Rating: "moderate", Concerns: []string{"unreported"}},
	})

	if !strings.Contains(narration(), "moderate") {
		t.Errorf("narration does not mention the moderate rating\nnarration: %s", narration())
	}
	if strings.Contains(narration(), "unreported") {
		t.Errorf("concerns are reported for a non-complex rating\nnarration: %s", narration())
	}
}

func TestReportOnboardResult_ReportsTheSlotCountOnlyWhenSlotsExist(t *testing.T) {

	withSlots := captureNarration(t)
	reportOnboardResult(&onboard.Result{
		Slots: []onboard.ExtractedSlot{{Name: "install_command"}, {Name: "package_manager"}},
	})
	if !strings.Contains(withSlots(), "2") {
		t.Errorf("narration does not report the slot count\nnarration: %s", withSlots())
	}

	withoutSlots := captureNarration(t)
	reportOnboardResult(&onboard.Result{})
	if strings.Contains(withoutSlots(), "configuration slots") {
		t.Errorf("slot count reported for an empty slot list\nnarration: %s", withoutSlots())
	}
}

func TestReportOnboardResult_ToleratesAnEmptyResult(t *testing.T) {

	captureNarration(t)

	reportOnboardResult(&onboard.Result{})
}

func TestWriteOnboardManifest_WritesTheManifestAndNamesThePath(t *testing.T) {

	narration := captureNarration(t)
	outputDir := t.TempDir()

	err := writeOnboardManifest(outputDir, &onboard.Result{Manifest: "# PkgPath: ripgrep\n"})

	if err != nil {
		t.Fatalf("writeOnboardManifest: %v", err)
	}

	manifestPath := filepath.Join(outputDir, "packages-manifest.yaml")
	written, readErr := os.ReadFile(manifestPath) //nolint:gosec // G704: path built from t.TempDir
	if readErr != nil {
		t.Fatalf("reading the written manifest: %v", readErr)
	}
	if string(written) != "# PkgPath: ripgrep\n" {
		t.Errorf("manifest contents = %q, want %q", string(written), "# PkgPath: ripgrep\n")
	}

	if !strings.Contains(narration(), manifestPath) {
		t.Errorf("narration does not name the written path\nnarration: %s", narration())
	}
	if !strings.Contains(narration(), "lore deploy @packages-manifest.yaml") {
		t.Errorf("narration does not print the next steps\nnarration: %s", narration())
	}
}

func TestWriteOnboardManifest_FailsWhenTheDirectoryDoesNotExist(t *testing.T) {

	captureNarration(t)

	err := writeOnboardManifest(filepath.Join(t.TempDir(), "absent"), &onboard.Result{Manifest: "x"})

	if err == nil {
		t.Fatal("expected an error writing into a nonexistent directory")
	}
	if !strings.Contains(err.Error(), "writing manifest") {
		t.Errorf("error %q is not wrapped with 'writing manifest'", err)
	}
}
