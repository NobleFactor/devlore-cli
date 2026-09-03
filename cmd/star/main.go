// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// star is the Starlark-powered operations tool for NobleFactor projects.
// Commands are defined as extensions in the star/extensions/ directory.
package main

import (
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	starruntime "github.com/NobleFactor/devlore-cli/cmd/star/star"
	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/schema"
	"github.com/spf13/cobra"

	_ "github.com/NobleFactor/devlore-cli/cmd/star/inventory"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
)

// Version information, stamped once for every command in [application].
var (
	version   = application.Version
	commit    = application.Commit
	buildDate = application.BuildDate
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

	rootCmd, runtime := newRootCmd()
	defer iox.Close(&err, runtime)

	return rootCmd.Execute()
}

// newRootCmd builds the star command tree on the shared root.
//
// [cli.NewRootCmd] supplies the persistent flags every program carries, `--dry-run` and `--silent` among
// them, and the `config`, `man`, `self` and `version` commands. star's own steps at dispatch time wrap the
// shared pre-run rather than replace it: the shared one builds the narrator and the configuration; then
// star copies `--dry-run` into [starruntime.DryRun], refreshes the application's flag map from parsed
// argv, and points the runtime environment's status at the one narrator, so `--silent` gates cli.Note and
// the starlark ui.note() path alike.
//
// The Application is built after every persistent flag is registered, because it walks the root's flag
// surface once; the values arrive at dispatch through Refresh. Extension commands load last, so an
// extension whose command group shares a name with a shared command -- `config` -- attaches beneath it.
//
// Returns:
//   - `*cobra.Command`: the root, with every command attached.
//   - `*starruntime.Application`: the session the commands run in; the caller closes it.
func newRootCmd() (*cobra.Command, *starruntime.Application) {

	rootCmd := cli.NewRootCmd(cli.RootConfig{
		Name:  "star",
		Short: "Starlark-powered operations tool",
		Long: `star is the Starlark-powered operations tool for NobleFactor projects.

Commands are defined as extensions in the star/extensions/ directory.
Run 'star docs starlark' for details on writing operations.

SHELL COMPLETION

Generate shell completions with:
  star completion bash > /etc/bash_completion.d/star
  star completion zsh > "${fpath[1]}/_star"
  star completion fish > ~/.config/fish/completions/star.fish`,
		DefaultConfig:      schema.StarDefaultConfig,
		Version:            version,
		Commit:             commit,
		BuildDate:          buildDate,
		PostInstallHooks:   []func(string) []string{installStarExtensions},
		PostUninstallHooks: []func(string) error{uninstallStarExtensions},
	})

	// The common set, on the root: every command of star accepts every flag, and a fix in
	// cmd/internal/cli reaches all of them at once (10-command-line-interface.md §4, §15).
	cli.AddOutputFlags(rootCmd, &outputOptions)

	runtime := starruntime.NewApplication(rootCmd)

	sharedPreRun := rootCmd.PersistentPreRunE
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if err := sharedPreRun(cmd, args); err != nil {
			return err
		}
		starruntime.DryRun = assert.Must(cmd.Flags().GetBool("dry-run"))
		runtime.Refresh(cmd)
		runtime.Environment().Status = cli.UI()
		return nil
	}

	rootCmd.AddCommand(newKeyCmd())
	rootCmd.AddCommand(newDocsCmd())

	// An extension that fails to load costs its commands, not the program: the rest of the tree still
	// runs. The narrator does not exist yet -- the shared pre-run builds it at dispatch -- so this goes to
	// stderr directly.
	if err := loadStarlarkCommands(rootCmd, runtime); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load Starlark commands: %v\n", err) //nolint:errcheck // diagnose-ignored-error: a warning that cannot be written has nowhere else to go
	}

	return rootCmd, runtime
}

// newKeyCmd builds the `key` group: signing-key management, every leaf unimplemented until the key
// ceremony (ADR-040) is built. An unimplemented leaf fails and says so; it does not print a note and exit 0,
// because a stub that reports success is a lie a script cannot detect.
//
// Returns:
//   - `*cobra.Command`: the `key` command with `generate`, `list` and `rotate`.
func newKeyCmd() *cobra.Command {

	keyCmd := &cobra.Command{
		Use:   "key",
		Short: "Key management operations",
	}

	keyCmd.AddCommand(&cobra.Command{
		Use:   "generate",
		Short: "Generate a new signing key",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return errNotImplemented("key generation", "the key ceremony protocol is ADR-040")
		},
	})

	keyCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List managed signing keys",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return errNotImplemented("key listing", "the key ceremony protocol is ADR-040")
		},
	})

	keyCmd.AddCommand(&cobra.Command{
		Use:   "rotate",
		Short: "Rotate a signing key with ceremony",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return errNotImplemented("key rotation", "it requires hardware key presence, and the ceremony is ADR-040")
		},
	})

	return keyCmd
}

// newDocsCmd builds the `docs` group. It carries star's own documentation only: the shared `man` command
// is the one route to man pages on every program, and `make docs` generates the markdown reference for
// the whole suite (10-command-line-interface.md §12, ruled 2026-09-02).
//
// Returns:
//   - `*cobra.Command`: the `docs` command with `starlark`.
func newDocsCmd() *cobra.Command {

	docsCmd := &cobra.Command{
		Use:   "docs",
		Short: "star's own documentation",
	}

	// The guide is the command's result and goes through the pipeline like any other: under the json
	// default it is one quoted string, and `-o value` reads it as prose. Whether a prose result should
	// default to value is #740's open question, logged there; this site holds the invariant either way.
	docsCmd.AddCommand(&cobra.Command{
		Use:   "starlark",
		Short: "Show how to write Starlark operations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return emitResult(cmd, starlarkDocs)
		},
	})

	return docsCmd
}

// errNotImplemented is the error an unimplemented command returns: it names the operation and points at
// where the design lives, so the failure is a fact on stderr and exit 1, never a note and exit 0.
//
// Parameters:
//   - `operation`: what was asked for, in words.
//   - `pointer`: where the reader goes next.
//
// Returns:
//   - `error`: the failure.
func errNotImplemented(operation, pointer string) error {
	return fmt.Errorf("%s is not implemented; %s", operation, pointer)
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

	// The hook owns the root for the length of its work: it receives a prefix string, because the hook
	// contract is a path (#405, phase 2b — the contract itself is the campaign's LAST item).
	prefixRoot, err := cli.OpenTree(prefix)
	if err != nil {
		cli.Warn("Failed to open the install prefix: %v", err)
		return nil
	}

	//nolint:errcheck // diagnose-ignored-error: best-effort hook, and the extensions are already copied; see docs/architecture/2.8-eventing-infrastructure.md
	defer prefixRoot.Close()

	targetExtDir := prefixRoot.NewPath("share", "star", "extensions")
	if err := cli.CopyDir(prefixRoot, srcExtDir, targetExtDir); err != nil {
		cli.Warn("Failed to install extensions: %v", err)
		return nil
	}

	return cli.CollectFiles(prefix, targetExtDir.Abs())
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

	// Register each Starlark command with cobra, in name order: a group's leaf (`setup`) must exist before
	// its children (`setup.check`) attach beneath it, and a map walk would decide that by chance.
	commands := runtime.Commands()
	for _, name := range slices.Sorted(maps.Keys(commands)) {
		if err := registerStarlarkCommand(rootCmd, commands[name]); err != nil {
			return fmt.Errorf("extension %s, command %s: %w", commands[name].Extension.Name, name, err)
		}
	}

	return nil
}

// registerStarlarkCommand creates a cobra command from a Starlark command.
//
// The script's return value is the command's result and goes to stdout through the shared pipeline;
// a script that returns None emits nothing.
//
// Parameters:
//   - `rootCmd`: the root the command's dotted path hangs from.
//   - `cmd`: the Starlark command.
//
// Returns:
//   - `error`: non-nil when the command declares a flag the common set owns.
func registerStarlarkCommand(rootCmd *cobra.Command, cmd *starruntime.Command) error {
	// Parse command name (e.g., "registry.index-knowledge" -> registry subcommand with index-knowledge)
	parts := strings.Split(cmd.Name, ".")
	parent := findOrCreateParent(rootCmd, parts)

	// Create the leaf command
	cobraCmd := &cobra.Command{
		Use:   useLineFor(parts[len(parts)-1], cmd.Args),
		Short: cmd.Help,
		RunE: func(c *cobra.Command, args []string) error {
			result, err := cmd.Run(collectFlagValues(c, cmd.Flags), args...)
			if err != nil {
				return err
			}
			if result == nil {
				return nil
			}
			return emitResult(c, result)
		},
	}

	cobraCmd.Args = argsValidatorFor(cmd.Args)

	if err := defineFlags(cobraCmd, cmd.Flags); err != nil {
		return err
	}

	parent.AddCommand(cobraCmd)

	return nil
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
		switch {
		case arg.Variadic:
			useLine += fmt.Sprintf(" [%s ...]", arg.Name)
		case arg.Default != "":
			useLine += fmt.Sprintf(" [%s]", arg.Name)
		default:
			useLine += fmt.Sprintf(" <%s>", arg.Name)
		}
	}

	return useLine
}

// argsValidatorFor derives cobra's positional validation from the arg specs: an arg with no default is
// required, an arg with one is optional, and a variadic arg absorbs the rest. A missing required operand
// is then a usage error at parse time, not a script failure after the runtime has been built.
//
// Parameters:
//   - `args`: the command's positional arg specs, in declaration order.
//
// Returns:
//   - `cobra.PositionalArgs`: the validator.
func argsValidatorFor(args []starruntime.Arg) cobra.PositionalArgs {

	if len(args) == 0 {
		return cobra.NoArgs
	}

	required := 0
	variadic := false
	for _, arg := range args {
		if arg.Variadic {
			variadic = true
			continue
		}
		if arg.Default == "" {
			required++
		}
	}
	if variadic {
		return cobra.MinimumNArgs(required)
	}

	return cobra.RangeArgs(required, len(args))
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
func defineFlags(cobraCmd *cobra.Command, flags []starruntime.Flag) error {

	for _, flag := range flags {
		if slices.Contains(cli.ReservedOutputFlagNames, flag.Name) {
			return fmt.Errorf("flag --%s is the common set's; a destination is a positional operand and a rendering is --output (10-command-line-interface.md §4)", flag.Name)
		}
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

	return nil
}
