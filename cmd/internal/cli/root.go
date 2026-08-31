// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"golang.org/x/term"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"github.com/NobleFactor/devlore-cli/pkg/sink"
	"github.com/NobleFactor/devlore-cli/pkg/status"
	"github.com/NobleFactor/devlore-cli/schema"
)

// RootConfig configures a root CLI command for lore or writ.
type RootConfig struct {
	Name          string // Command name ("lore" or "writ")
	Short         string // One-line description
	Long          string // Multi-line description
	DefaultConfig []byte // Schema default config (e.g., schema.LoreDefaultConfig)
	Version       string // Semantic version, set via ldflags
	Commit        string // Git commit hash, set via ldflags
	BuildDate     string // Build timestamp, set via ldflags
}

// NewRootCmd creates a root cobra command with all shared flags, metadata
// commands, and Viper configuration. The caller adds tool-specific flags
// and subcommands to the returned command.
//
// Parameters:
//   - cfg: root command configuration (name, descriptions, version info)
//
// Returns:
//   - *cobra.Command: configured root command with shared flags and metadata commands
func NewRootCmd(cfg RootConfig) *cobra.Command {

	rootCmd := &cobra.Command{
		Use:               cfg.Name,
		Short:             cfg.Short,
		Long:              cfg.Long,
		DisableAutoGenTag: true,
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {

			// Construct the package-global status.UI from parsed flags. The same instance flows
			// into RuntimeEnvironmentSpec.Status so --silent applies uniformly to cli.Note,
			// env.Status.Note (provider emissions), and starlark print() output. The choice
			// between Console and Discard is at the construction site — Console always emits;
			// Discard always drops.
			silent := assert.Must(cmd.Flags().GetBool("silent"))
			var s sink.Sink
			if silent {
				s = sink.Discard()
			} else {
				s = sink.Stderr()
			}
			SetUI(status.NewNarrator(cfg.Name, s))

			return initRootConfig(cmd, cfg.Name)
		},
	}

	wrapHelp(rootCmd)

	// Standard flags
	rootCmd.PersistentFlags().String("config", "", "Config file (default: ~/.config/devlore/config.yaml)")
	rootCmd.PersistentFlags().Bool("dry-run", false, "Show what would be done without making changes")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")
	AddSilentFlag(rootCmd)

	// Deployment mode flags (ADR-033)
	rootCmd.PersistentFlags().Bool("interactive", false, "Force interactive mode (prompts, rich output)")
	rootCmd.PersistentFlags().Bool("unattended", false, "Force unattended mode (no prompts, sensible defaults)")
	rootCmd.MarkFlagsMutuallyExclusive("interactive", "unattended")

	// Model configuration flags
	// Resolution order: CLI flags → Environment → Config file → Keystore (api-key only)
	rootCmd.PersistentFlags().String("model", "", "Model name (e.g., claude-sonnet-4-20250514, gpt-4o)")
	rootCmd.PersistentFlags().String("model-api-key", "", "Model provider API key")
	rootCmd.PersistentFlags().String("model-endpoint", "", "Model provider endpoint URL")
	rootCmd.PersistentFlags().String("model-provider", "", "Model provider: anthropic, openai, azure-openai, ollama, github")

	// Shared metadata commands
	capitalized := strings.ToUpper(cfg.Name[:1]) + cfg.Name[1:]

	manHeader := ManHeader{
		Title:   strings.ToUpper(cfg.Name),
		Section: "1",
		Source:  capitalized + " " + cfg.Version,
		Manual:  capitalized + " Manual",
	}
	configInfo := ConfigInfo{
		Name:          cfg.Name,
		Schema:        schema.DevloreSchema,
		DefaultConfig: cfg.DefaultConfig,
	}

	versionInfo := VersionInfo{
		Version:   cfg.Version,
		Commit:    cfg.Commit,
		BuildDate: cfg.BuildDate,
	}

	rootCmd.SetHelpCommand(NewHelpCmd(rootCmd, manHeader))
	AddVersionFlag(rootCmd, versionInfo)
	rootCmd.AddCommand(NewVersionCmd(versionInfo))
	rootCmd.AddCommand(NewManCmd(rootCmd, manHeader))
	rootCmd.AddCommand(NewConfigCmd(configInfo))
	rootCmd.AddCommand(NewSelfCmd(rootCmd, SelfInstallInfo{
		Name:       cfg.Name,
		Version:    cfg.Version,
		ManHeader:  manHeader,
		ConfigInfo: &configInfo,
	}))

	return rootCmd
}

// initRootConfig initializes Viper configuration for a root command.
// Precedence (lowest to highest): config file → environment variables → flags.
//
// Parameters:
//   - cmd: the cobra command triggering initialization
//   - name: tool name ("lore" or "writ"), used as Viper prefix and env prefix
//
// Returns:
//   - error: configuration, config file read, or flag binding failure
func initRootConfig(cmd *cobra.Command, name string) error {
	if err := InitViper(ViperConfig{
		Name:            name,
		EnvPrefix:       strings.ToUpper(name),
		UseSharedConfig: true,
	}); err != nil {
		return err
	}

	if cfgFile, _ := cmd.Flags().GetString("config"); cfgFile != "" { //nolint:errcheck // flag registered above
		viper.SetConfigFile(cfgFile)
		if err := viper.ReadInConfig(); err != nil {
			return fmt.Errorf("failed to read config %s: %w", cfgFile, err)
		}
	}

	if err := BindFlags(cmd.Root(), name, true); err != nil {
		return err
	}

	if viper.GetBool(name + ".verbose") {
		Note("Using config: %s", viper.ConfigFileUsed())
	}

	return nil
}

// region Help wrapping

// helpFallbackWidth is the width used when neither COLUMNS nor the terminal answers.
//
// Chosen over pflag's zero, which means "do not wrap at all": a pipe or a CI log has no width to report,
// and an unwrapped line there is a wall of text rather than a deliberate choice.
const helpFallbackWidth = 100

// helpMinimumTextWidth is the narrowest column of text worth hanging under.
//
// Below it, honoring a hanging indent leaves a sliver too narrow to read, so the line falls back to its
// leading indent and gives the text the whole width instead.
const helpMinimumTextWidth = 24

// wrapHelp makes flag usage wrap to the terminal, keeping any column structure the usage text has.
//
// Cobra's default template calls [pflag.FlagSet.FlagUsages], which is `FlagUsagesWrapped(0)`, and zero means
// no wrapping at all -- so without this every line's width is the author's to maintain by hand, correct only
// on a terminal at least as wide as the constant they guessed (#755).
//
// pflag's own wrapping is not the answer either. It indents every continuation to the flag's description
// column, having one indent level and no notion of structure, so a two-column usage -- `--output`'s eight
// renderings, each with a name and a sentence -- collapses the moment it wraps:
//
//	csv            quoted and parseable; when
//	a spreadsheet or a data tool reads it
//
// [wrapUsageLine] hangs continuations under the text they continue, which is what keeps that readable.
//
// Parameters:
//   - `cmd`: the root command whose usage template is rewritten.
func wrapHelp(cmd *cobra.Command) {

	cobra.AddTemplateFunc("wrappedFlagUsages", func(flags *pflag.FlagSet) string {
		return wrapUsage(flags.FlagUsages(), helpWidth())
	})

	template := cmd.UsageTemplate()
	template = strings.ReplaceAll(template, ".LocalFlags.FlagUsages", "wrappedFlagUsages .LocalFlags")
	template = strings.ReplaceAll(template, ".InheritedFlags.FlagUsages", "wrappedFlagUsages .InheritedFlags")
	cmd.SetUsageTemplate(template)
}

// wrapUsage wraps pflag's laid-out usage block to width, line by line.
//
// The input is pflag's own output with no wrapping applied, so the flag-name column and the description
// column are already aligned; only the over-long lines need breaking.
//
// Parameters:
//   - `usage`: pflag's rendered usage block.
//   - `width`: the column count to wrap to; zero or less leaves the block untouched.
//
// Returns:
//   - `string`: the wrapped block, newline-terminated as pflag's is.
func wrapUsage(usage string, width int) string {

	if width <= 0 {
		return usage
	}

	lines := strings.Split(strings.TrimRight(usage, "\n"), "\n")
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		wrapped = append(wrapped, wrapUsageLine(line, width))
	}

	return strings.Join(wrapped, "\n") + "\n"
}

// wrapUsageLine breaks one line at width, hanging continuations under the text they continue.
//
// The hanging indent is where the line's text begins, which is after its leading whitespace and after any
// name column the line carries -- see [usageTextColumn]. Widths are measured in runes rather than bytes, so
// a description containing a multi-byte character wraps where it looks like it should.
//
// Parameters:
//   - `line`: one line of pflag's usage block.
//   - `width`: the column count to wrap to.
//
// Returns:
//   - `string`: the line, broken across as many lines as it needs.
func wrapUsageLine(line string, width int) string {

	if len([]rune(line)) <= width {
		return line
	}

	hang := usageTextColumn(line)
	if width-hang < helpMinimumTextWidth {
		hang = len(line) - len(strings.TrimLeft(line, " "))
	}
	if width-hang < helpMinimumTextWidth {
		return line
	}

	indent := strings.Repeat(" ", hang)
	current := line[:hang]
	first := true

	var wrapped []string
	for _, word := range strings.Fields(line[hang:]) {
		switch {
		case first:
			current += word
			first = false
		case len([]rune(current))+1+len([]rune(word)) > width:
			wrapped = append(wrapped, strings.TrimRight(current, " "))
			current = indent + word
		default:
			current += " " + word
		}
	}

	return strings.Join(append(wrapped, strings.TrimRight(current, " ")), "\n")
}

// usageTextColumn returns the column at which a usage line's prose begins.
//
// pflag separates a name column from its description with a run of two or more spaces, and `--output`'s
// rendering list uses the same shape one level in. Taking the first such run after the line's first
// non-space character finds the description column on a flag line and the sentence column on a rendering
// line, which is where each one's continuations belong.
//
// Parameters:
//   - `line`: one line of pflag's usage block.
//
// Returns:
//   - `int`: the column, or the line's leading indent when it carries no name column.
func usageTextColumn(line string) int {

	leading := len(line) - len(strings.TrimLeft(line, " "))

	rest := line[leading:]
	for i := 0; i < len(rest)-1; i++ {
		if rest[i] == ' ' && rest[i+1] == ' ' {
			text := i
			for text < len(rest) && rest[text] == ' ' {
				text++
			}
			return leading + text
		}
	}

	return leading
}

// helpWidth reports the column count help text should wrap to.
//
// COLUMNS wins when it is set and sane: a user who exports it has said what they want, and it is the only
// answer available when stdout is a pipe. The terminal is asked next, and [helpFallbackWidth] answers when
// neither does.
//
// Returns:
//   - `int`: the wrap width in columns.
func helpWidth() int {

	if columns, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && columns > 0 {
		return columns
	}

	if width, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && width > 0 {
		return width
	}

	return helpFallbackWidth
}

// endregion
