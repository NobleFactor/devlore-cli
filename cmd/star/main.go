// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// star is the Starlark-powered operations tool for NobleFactor projects.
// Commands are defined as extensions in the star/extensions/ directory.
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	starruntime "github.com/NobleFactor/devlore-cli/cmd/star/star"
	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/sink"
	"github.com/NobleFactor/devlore-cli/pkg/status"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"

	_ "github.com/NobleFactor/devlore-cli/cmd/star/inventory"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
)

var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

const starlarkDocs = `WRITING STARLARK OPERATIONS

Commands are defined as extensions in the star/extensions/ directory.
Each extension can register one or more commands via extension.yaml.

BASIC STRUCTURE

    # star/extensions/com.example.MyExt/commands/my-operation.star

    def run(ctx):
        """Main entry point for the operation."""
        path = ctx.args.get("path", ".")

        note("Processing: " + path)
        # fs.write is dry-run safe - automatically skips when --dry-run is set
        fs.write(fs.join(path, "output.txt"), "Hello!")
        success("Wrote output.txt")

    command(
        name = "mygroup.my-operation",
        help = "Description shown in help output",
        flags = [
            {"name": "path", "help": "Path to process", "default": "."},
        ],
        run = run,
    )

COMMAND NAMING

The command name uses dots to create subcommand hierarchy:
  - "foo"           -> star foo
  - "foo.bar"       -> star foo bar
  - "foo.bar.baz"   -> star foo bar baz

AVAILABLE MODULES

fs - File system operations:
  fs.read(path)              Read file contents as string
  fs.write(path, content)    Write string to file [dry-run safe]
  fs.exists(path)            Check if path exists
  fs.is_dir(path)            Check if path is directory
  fs.is_file(path)           Check if path is file
  fs.list_dir(path)          List directory entries
                             Returns list of {name, path, is_dir}
  fs.join(a, b, ...)         Join path components
  fs.basename(path)          Get filename from path
  fs.dirname(path)           Get directory from path
  fs.glob(pattern)           Find files matching pattern
  fs.mkdir(path)             Create directory (with parents) [dry-run safe]
  fs.remove(path)            Remove file [dry-run safe]
  fs.remove_all(path)        Remove file or directory recursively [dry-run safe]

  Functions marked [dry-run safe] log their intent and skip execution
  when --dry-run is set.

yaml - YAML encoding/decoding:
  yaml.encode(value)         Convert dict/list to YAML string
  yaml.decode(string)        Parse YAML string to dict/list

ui - User-facing terminal messaging:
  ui.note(msg)               Informational message (gray +)
  ui.warn(msg)               Warning message (yellow △)
  ui.error(msg)              Error message (red ✖)
  ui.success(msg)            Success message (green ✔)
  ui.fail(msg)               Error message + abort execution

Use print(msg) for raw stdout output (e.g., YAML content in dry-run mode).

CONTEXT OBJECT

The run function receives a context object with:
  ctx.args                   Dict of flag values (all strings)
  ctx.dry_run                Bool: true if --dry-run flag is set

FLAG DEFINITION

Each flag is a dict with:
  name      (required)       Flag name (becomes --name)
  help      (optional)       Help text
  default   (optional)       Default value (string)
  required  (optional)       If true, flag must be provided

EXAMPLES

List files in a directory:

    def run(ctx):
        for entry in fs.list_dir(ctx.args.get("path", ".")):
            if entry.is_dir:
                print("[DIR]  " + entry.name)
            else:
                print("[FILE] " + entry.name)

    command(name="list", help="List directory contents", run=run)

Generate YAML index:

    def run(ctx):
        items = []
        for entry in fs.list_dir(ctx.args.get("path", ".")):
            if entry.name.endswith(".md"):
                items.append({"name": entry.name, "path": entry.path})
        index = {"version": "1", "items": items}
        print(yaml.encode(index))

    command(name="index", help="Generate index", run=run)
`

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

// run builds the star command tree and executes it, returning the execution error.
//
// The extraction keeps main's single os.Exit above every defer: an os.Exit inside this body would
// skip the runtime's Close (gocritic exitAfterDefer).
//
// Returns:
//   - `error`: the command execution error, or nil on success.
func run() (err error) {

	var silent bool

	rootCmd := &cobra.Command{
		Use:   "star",
		Short: "Starlark-powered operations tool",
		// A failing command verdict (a ui.fail in a .star script, a lint gate saying no) is not a
		// usage mistake — suppress the usage block on RunE errors (ruling 2026-08-04).
		SilenceUsage: true,
		Long: `star is the Starlark-powered operations tool for NobleFactor projects.

Commands are defined as extensions in the star/extensions/ directory.
Run 'star docs starlark' for details on writing operations.

SHELL COMPLETION

Generate shell completions with:
  star completion bash > /etc/bash_completion.d/star
  star completion zsh > "${fpath[1]}/_star"
  star completion fish > ~/.config/fish/completions/star.fish`,
	}

	// Global flags — declared BEFORE building the Application so that rootCmd has the persistent flag surface in place
	// when application.NewApplication walks cmd.Flags(). (Cobra hasn't parsed argv yet; user-supplied values land via
	// Refresh in PersistentPreRunE below.)

	rootCmd.PersistentFlags().BoolVar(&starruntime.DryRun, "dry-run", false, "Preview changes without executing side effects")
	rootCmd.PersistentFlags().BoolVar(&silent, "silent", false, "Suppress all status messages")

	// Build the session: star.NewApplication owns the underlying op.RuntimeEnvironment via its starlarkbridge.Runtime.
	// Defer Close once.

	runtime := starruntime.NewApplication(rootCmd)
	defer iox.Close(&err, runtime)

	// Refresh Application.Flags from cobra's parsed argv at command-dispatch time.

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		runtime.Refresh(cmd)
		return nil
	}

	cobra.OnInitialize(func() {

		// Construct the canonical status.UI from the parsed --silent flag and install the single instance on
		// both narration seams: the shared cli package-global backing cmd/star/cli's Note/Warn/etc. forwarding
		// wrappers (output.go), and the runtime environment's Status backing the starlark ui.note() / ui.warn()
		// paths through pkg/op/provider/ui.Provider's passthrough. One instance, one silent gate, every
		// emission consistent on stderr.
		narratorSink := sink.Stderr()
		if silent {
			narratorSink = sink.Discard()
		}
		narrator := status.NewNarrator("star", narratorSink)
		cli.SetUI(narrator)
		runtime.Environment().Status = narrator
	})

	// Version command

	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("star %s (%s) built %s\n", version, commit, buildDate)
		},
	})

	// Key management commands.

	keyCmd := &cobra.Command{
		Use:   "key",
		Short: "Key management operations",
	}

	keyCmd.AddCommand(&cobra.Command{
		Use:   "generate",
		Short: "Generate a new signing key",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Key generation not yet implemented")
			fmt.Println("See ADR-040 for the key ceremony protocol")
		},
	})

	keyCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List managed signing keys",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Key listing not yet implemented")
		},
	})

	keyCmd.AddCommand(&cobra.Command{
		Use:   "rotate",
		Short: "Rotate a signing key with ceremony",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Key rotation not yet implemented")
			fmt.Println("This operation requires hardware key presence")
		},
	})

	rootCmd.AddCommand(keyCmd)

	// Documentation commands

	docsCmd := &cobra.Command{
		Use:   "docs",
		Short: "Generate documentation",
	}

	docsCmd.AddCommand(&cobra.Command{
		Use:   "man <output-dir>",
		Short: "Generate man pages",
		Long: `Generate man pages for star and all subcommands.

The man pages are written to the specified output directory.
Install them to your man path (e.g., /usr/local/share/man/man1/).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			outDir := args[0]
			//nolint:gosec // G301: extension directories are shared content (0o755 by design).
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return fmt.Errorf("creating output directory: %w", err)
			}
			header := &doc.GenManHeader{
				Title:   "STAR",
				Section: "1",
				Source:  "Noble Factor",
				Manual:  "Star Operations Manual",
			}
			if err := doc.GenManTree(rootCmd, header, outDir); err != nil {
				return fmt.Errorf("generating man pages: %w", err)
			}
			fmt.Printf("Man pages written to %s\n", outDir)
			return nil
		},
	})

	docsCmd.AddCommand(&cobra.Command{
		Use:   "markdown <output-dir>",
		Short: "Generate markdown documentation",
		Long:  `Generate markdown documentation for star and all subcommands.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			outDir := args[0]
			//nolint:gosec // G301: extension directories are shared content (0o755 by design).
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return fmt.Errorf("creating output directory: %w", err)
			}
			if err := doc.GenMarkdownTree(rootCmd, outDir); err != nil {
				return fmt.Errorf("generating markdown: %w", err)
			}
			fmt.Printf("Markdown docs written to %s\n", outDir)
			return nil
		},
	})

	docsCmd.AddCommand(&cobra.Command{
		Use:   "starlark",
		Short: "Show how to write Starlark operations",
		Long:  `Show documentation for writing Starlark operations.`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Print(starlarkDocs)
		},
	})

	rootCmd.AddCommand(docsCmd)

	// CLI status output is wired in cobra.OnInitialize above via cli.SetUI(status.NewNarrator(...)). cmd/star/cli's
	// local Note/Warn/Error/Success/Failure functions forward to that shared UI.

	// Self commands (install, upgrade, etc.)

	rootCmd.AddCommand(cli.NewSelfCmd(rootCmd, cli.SelfInstallInfo{
		Name:    "star",
		Version: version,
		ManHeader: cli.ManHeader{
			Title:   "STAR",
			Section: "1",
			Source:  "Noble Factor",
			Manual:  "Star Operations Manual",
		},
		PostInstallHooks:   []func(string) []string{installStarExtensions},
		PostUninstallHooks: []func(string) error{uninstallStarExtensions},
	}))

	// Load Starlark commands from extensions.

	if err = loadStarlarkCommands(rootCmd, runtime); err != nil {
		_, err = fmt.Fprintf(os.Stderr, "Warning: failed to load Starlark commands: %v\n", err)
		if err != nil {
			return
		}
	}

	return rootCmd.Execute()
}

// =============================================================================
// Self-Install Hooks
// =============================================================================

// installStarExtensions copies the star/extensions/ directory to <prefix>/share/star/extensions/.
// Returns the list of installed file paths relative to prefix.
func installStarExtensions(prefix string) []string {
	srcExtDir := findExtensionsDir()
	if srcExtDir == "" {
		return nil
	}

	targetExtDir := filepath.Join(prefix, "share", "star", "extensions")
	if err := cli.CopyDir(srcExtDir, targetExtDir); err != nil {
		cli.Warn("Failed to install extensions: %v", err)
		return nil
	}

	return cli.CollectFiles(prefix, targetExtDir)
}

// uninstallStarExtensions removes the star extensions directory.
func uninstallStarExtensions(prefix string) error {
	targetExtDir := filepath.Join(prefix, "share", "star", "extensions")
	if err := os.RemoveAll(targetExtDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove extensions: %w", err)
	}
	return nil
}

// findExtensionsDir looks for the star/extensions/ directory.
func findExtensionsDir() string {
	// Check relative to cwd (project-local).
	if info, err := os.Stat(filepath.Join("star", "extensions")); err == nil && info.IsDir() {
		return filepath.Join("star", "extensions")
	}

	// Check relative to executable.
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		shareExt := filepath.Join(filepath.Dir(exeDir), "share", "star", "extensions")
		if info, err := os.Stat(shareExt); err == nil && info.IsDir() {
			return shareExt
		}
	}

	return ""
}

// =============================================================================
// Starlark Commands
// =============================================================================

// loadStarlarkCommands discovers, deduplicates, and loads all extensions, then
// registers their commands with cobra.
func loadStarlarkCommands(rootCmd *cobra.Command, runtime *starruntime.Application) error {
	// Extract the embedded extensions filesystem.
	extFS, err := fs.Sub(bundledExtensions, "extensions")
	if err != nil {
		return fmt.Errorf("embedded extensions: %w", err)
	}

	// Create loader and discover/register/activate all extensions.
	loader := starruntime.NewExtensionLoader(extFS)
	if err := runtime.DiscoverAndLoad(loader); err != nil {
		return err
	}

	// register each Starlark command with cobra.
	for _, cmd := range runtime.Commands() {
		registerStarlarkCommand(rootCmd, cmd)
	}

	return nil
}

// registerStarlarkCommand creates a cobra command from a Starlark command.
func registerStarlarkCommand(rootCmd *cobra.Command, cmd *starruntime.Command) {
	// Parse command name (e.g., "registry.index-knowledge" -> registry subcommand with index-knowledge)
	parts := strings.Split(cmd.Name, ".")
	parent := findOrCreateParent(rootCmd, parts)

	// Create the leaf command
	cobraCmd := &cobra.Command{
		Use:   useLineFor(parts[len(parts)-1], cmd.Args),
		Short: cmd.Help,
		RunE: func(c *cobra.Command, args []string) error {
			return cmd.Run(collectFlagValues(c, cmd.Flags), args...)
		},
	}

	// Set positional arg validation.
	if len(cmd.Args) > 0 {
		cobraCmd.Args = cobra.ArbitraryArgs
	} else {
		cobraCmd.Args = cobra.NoArgs
	}

	defineFlags(cobraCmd, cmd.Flags)

	parent.AddCommand(cobraCmd)
}

// findOrCreateParent walks the dotted command path, creating intermediate cobra commands as needed.
//
// Parameters:
//   - `rootCmd`: the root command the path hangs from.
//   - `parts`: the dotted-name segments; the last is the leaf and is not walked.
//
// Returns:
//   - `*cobra.Command`: the leaf's parent command.
func findOrCreateParent(rootCmd *cobra.Command, parts []string) *cobra.Command {

	parent := rootCmd
	for i := 0; i < len(parts)-1; i++ {
		found := false
		for _, child := range parent.Commands() {
			if child.Use == parts[i] || strings.HasPrefix(child.Use, parts[i]+" ") {
				parent = child
				found = true
				break
			}
		}
		if !found {
			newCmd := &cobra.Command{
				Use:   parts[i],
				Short: fmt.Sprintf("%s commands", parts[i]),
			}
			parent.AddCommand(newCmd)
			parent = newCmd
		}
	}

	return parent
}

// useLineFor builds the leaf's Use string with arg placeholders (e.g., "go-style [path ...]").
//
// Parameters:
//   - `leafName`: the leaf command name.
//   - `args`: the command's positional arg specs.
//
// Returns:
//   - `string`: the cobra Use line.
func useLineFor(leafName string, args []starruntime.Arg) string {

	useLine := leafName
	for _, arg := range args {
		if arg.Variadic {
			useLine += fmt.Sprintf(" [%s ...]", arg.Name)
		} else {
			useLine += fmt.Sprintf(" [%s]", arg.Name)
		}
	}

	return useLine
}

// collectFlagValues reads the parsed flag values as strings (Command.Run converts to native
// starlark types).
//
// Parameters:
//   - `c`: the invoked cobra command carrying the parsed flags.
//   - `flags`: the command's flag specs.
//
// Returns:
//   - `map[string]string`: flag name to string value; unreadable flags are omitted.
func collectFlagValues(c *cobra.Command, flags []starruntime.Flag) map[string]string {

	flagValues := make(map[string]string)
	for _, flag := range flags {
		switch flag.Type {
		case "bool":
			val, err := c.Flags().GetBool(flag.Name)
			if err == nil {
				flagValues[flag.Name] = strconv.FormatBool(val)
			}
		case "int":
			val, err := c.Flags().GetInt(flag.Name)
			if err == nil {
				flagValues[flag.Name] = strconv.Itoa(val)
			}
		default:
			val, err := c.Flags().GetString(flag.Name)
			if err == nil {
				flagValues[flag.Name] = val
			}
		}
	}

	return flagValues
}

// defineFlags registers the command's flags on the cobra command with proper cobra types.
//
// Parameters:
//   - `cobraCmd`: the leaf cobra command.
//   - `flags`: the command's flag specs.
func defineFlags(cobraCmd *cobra.Command, flags []starruntime.Flag) {

	for _, flag := range flags {
		switch flag.Type {
		case "bool":
			cobraCmd.Flags().Bool(flag.Name, flag.Default == "true", flag.Help)
		case "int":
			//nolint:errcheck // diagnose-ignored-error: falls back to 0; see docs/architecture/2.8-eventing-infrastructure.md
			n, _ := strconv.Atoi(flag.Default)
			cobraCmd.Flags().Int(flag.Name, n, flag.Help)
		default:
			cobraCmd.Flags().String(flag.Name, flag.Default, flag.Help)
		}
		if flag.Required {
			err := cobraCmd.MarkFlagRequired(flag.Name)
			assert.NoError(fmt.Sprintf("MarkFlagRequired(%q)", flag.Name), err)
		}
	}
}
