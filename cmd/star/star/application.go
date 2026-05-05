// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

// Package star provides the runtime types and execution engine for the star CLI.
package star

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/NobleFactor/devlore-cli/cmd/star/config"
	"github.com/NobleFactor/devlore-cli/cmd/star/provider/commands"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/starlarkbridge"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// DryRun is set by the --dry-run global flag.
//
// When true, bindings with side effects (fs.write, fs.mkdir, fs.remove, etc.) log what they would do instead of
// executing.
var DryRun bool

// Application manages Starlark script execution for the star CLI.
//
// Holds the extension registry, the loaded-command map keyed by space-separated name, the unified config (lazily
// initialized), the [starlarkbridge.Runtime], and the shared context-data map that providers read at execution time.
type Application struct {
	registry *ExtensionRegistry
	commands map[string]*Command
	config   *config.Config // Unified config for builtin and extension config.
	star     *starlarkbridge.Runtime
	data     map[string]any // Shared context data (dry_run, config, command_tree, etc.).
}

// NewApplication creates a new star Application with a fully initialized Starlark runtime.
//
// Constructs the receiver registry, builds a [op.RuntimeEnvironmentSpec] with every registered module exposed and the
// current working directory wired as the read+write filesystem root, hands the spec to [starlarkbridge.NewRuntime],
// and inserts the resulting Application into the shared context-data map under the "command_tree" key so providers
// can read it lazily.
//
// Returns:
//   - *Application: the initialized application.
func NewApplication() *Application {

	wd, _ := os.Getwd()
	data := map[string]any{"dry_run": DryRun}

	registry := op.NewReceiverRegistry()

	cfg := op.NewRuntimeEnvironmentSpec("star", registry).
		WithModules(registry.Modules()...).
		WithRoot(op.NewRootReaderWriter(wd)).
		WithData(data)

	star := starlarkbridge.NewRuntime(cfg)

	rt := &Application{
		registry: NewExtensionRegistry(),
		commands: make(map[string]*Command),
		star:     star,
		data:     data,
	}

	data["command_tree"] = rt

	return rt
}

// region EXPORTED METHODS

// region State management

// Commands returns the map of registered commands keyed by space-separated command name.
//
// Returns:
//   - map[string]*Command: the registered commands.
func (r *Application) Commands() map[string]*Command {
	return r.commands
}

// Config returns the unified config, lazily initializing it on first access.
//
// Returns:
//   - *config.Config: the unified config.
func (r *Application) Config() *config.Config {
	if r.config == nil {
		r.config = config.New()
	}
	return r.config
}

// Registry returns the extension registry.
//
// Returns:
//   - *ExtensionRegistry: the extension registry.
func (r *Application) Registry() *ExtensionRegistry {
	return r.registry
}

// endregion

// region Behaviors

// Fallible actions

// Close releases all resources held by the application.
//
// Returns:
//   - error: non-nil if releasing resources fails.
func (r *Application) Close() error {
	return nil
}

// DiscoverAndLoad uses the given loader to discover extensions, then registers and activates them.
//
// Single entry point for extension loading. Discovery parses and deduplicates the candidate set; registration adds each
// extension to the registry, registers its config schema (when present), and reads config files; activation binds
// config to extensions and parses each extension's starlark commands into the application's command map.
//
// Parameters:
//   - loader: the extension loader configured with the search paths to scan.
//
// Returns:
//   - error: non-nil if discovery, registration, config loading, or activation fails.
func (r *Application) DiscoverAndLoad(loader *ExtensionLoader) error {

	extensions, err := loader.DiscoverAll()
	if err != nil {
		return err
	}

	for _, ext := range extensions {
		if err := r.registry.Register(ext); err != nil {
			return fmt.Errorf("register extension %s: %w", ext.Name, err)
		}

		if ext.HasConfig() {
			if err := r.Config().RegisterExtension(ext.ConfigPath(), ext.ToConfigSpec()); err != nil {
				return fmt.Errorf("register config for %s: %w", ext.Name, err)
			}
		}
	}

	if err := r.Config().LoadFromFiles(); err != nil {
		return fmt.Errorf("load config files: %w", err)
	}
	r.data["config"] = r.Config()

	for _, ext := range extensions {
		ext.SetConfig(r.Config())

		if ext.HasCommands() {
			if err := r.loadExtensionCommands(ext); err != nil {
				return fmt.Errorf("load extension %s: %w", ext.Name, err)
			}
		}
	}

	return nil
}

// LoadExtensionsFrom loads extensions from a specific directory.
//
// Used by tests that need to load from a known path without the full discovery flow. Duplicate registrations are
// silently skipped to keep test isolation simple.
//
// Parameters:
//   - dir: the directory to scan for extensions.
//
// Returns:
//   - error: non-nil if discovery, config registration, config loading, or activation fails.
func (r *Application) LoadExtensionsFrom(dir string) error {

	loader := &ExtensionLoader{
		searchPaths: []string{dir},
	}

	extensions, err := loader.DiscoverAll()
	if err != nil {
		return err
	}

	for _, ext := range extensions {
		if err := r.registry.Register(ext); err != nil {
			continue
		}

		if ext.HasConfig() {
			if err := r.Config().RegisterExtension(ext.ConfigPath(), ext.ToConfigSpec()); err != nil {
				return fmt.Errorf("register config for %s: %w", ext.Name, err)
			}
		}
	}

	if err := r.Config().LoadFromFiles(); err != nil {
		return fmt.Errorf("load config files: %w", err)
	}
	r.data["config"] = r.Config()

	for _, ext := range extensions {
		ext.SetConfig(r.Config())

		if ext.HasCommands() {
			if err := r.loadExtensionCommands(ext); err != nil {
				return fmt.Errorf("load extension %s: %w", ext.Name, err)
			}
		}
	}

	return nil
}

// RunCommand executes the registered command identified by name with the given flags and positional args.
//
// Implements the [commands.CommandTree] contract. The name is matched against the space-separated form stored in the
// command map.
//
// Parameters:
//   - name: the space-separated command name (e.g., "lint go").
//   - flags: the parsed flag values.
//   - positional: the positional arguments.
//
// Returns:
//   - error: non-nil if no command matches name or if command execution fails.
func (r *Application) RunCommand(name string, flags map[string]string, positional ...string) error {
	cmd, ok := r.commands[name]
	if !ok {
		return fmt.Errorf("command %q not found", name)
	}
	return cmd.Run(flags, positional...)
}

// Actions

// CommandFlags returns the flag descriptors for the registered command identified by name.
//
// Implements the [commands.CommandTree] contract. Accepts dotted or space-separated names by normalizing dots to
// spaces. Returns nil when no command matches.
//
// Parameters:
//   - name: the dotted or space-separated command name.
//
// Returns:
//   - []commands.CommandFlag: the flag descriptors, or nil if name does not match.
func (r *Application) CommandFlags(name string) []commands.CommandFlag {
	spaceName := strings.ReplaceAll(name, ".", " ")
	cmd, ok := r.commands[spaceName]
	if !ok {
		return nil
	}
	flags := make([]commands.CommandFlag, len(cmd.Flags))
	for i, f := range cmd.Flags {
		flags[i] = commands.CommandFlag{Name: f.Name, Help: f.Help, Default: f.Default}
	}
	return flags
}

// CommandHelp returns the help text for the registered command identified by name.
//
// Implements the [commands.CommandTree] contract. Accepts dotted or space-separated names by normalizing dots to
// spaces. Returns the empty string when no command matches.
//
// Parameters:
//   - name: the dotted or space-separated command name.
//
// Returns:
//   - string: the help text, or "" if name does not match.
func (r *Application) CommandHelp(name string) string {
	spaceName := strings.ReplaceAll(name, ".", " ")
	if cmd, ok := r.commands[spaceName]; ok {
		return cmd.Help
	}
	return ""
}

// CommandNames returns the names of every registered command in space-separated form.
//
// Implements the [commands.CommandTree] contract. Order is not guaranteed.
//
// Returns:
//   - []string: the registered command names.
func (r *Application) CommandNames() []string {
	names := make([]string, 0, len(r.commands))
	for name := range r.commands {
		names = append(names, name)
	}
	return names
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// loadCommand loads a single starlark command from an extension.
//
// Reads the implementation source from the extension's embedded FS or the OS filesystem, executes the .star file in a
// fresh starlark thread whose load() consults a per-load module cache, extracts the required `run` callable from the
// resulting globals, and registers the command on the application keyed by the space-separated form of its dotted
// name (e.g., "lint.go" → "lint go").
//
// Parameters:
//   - ext: the owning extension.
//   - cmd: the command to load.
//
// Returns:
//   - error: non-nil if reading, executing, or extracting the run function fails.
func (r *Application) loadCommand(ext *Extension, cmd *Command) error {

	var implPath string
	if ext.FS != nil {
		implPath = cmd.Implementation
	} else {
		implPath = filepath.Join(ext.Dir, cmd.Implementation)
	}

	src, err := readExtensionFile(ext, implPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", implPath, err)
	}

	predeclared := r.star.Predeclared()

	fileOpts := syntax.FileOptions{
		Set:             true,
		While:           true,
		TopLevelControl: true,
		GlobalReassign:  true,
		Recursion:       true,
	}

	moduleCache := map[string]starlark.StringDict{}

	thread := &starlark.Thread{
		Name:  cmd.Name,
		Print: func(_ *starlark.Thread, msg string) { fmt.Println(msg) },
		Load: func(thread *starlark.Thread, module string) (starlark.StringDict, error) {
			var modulePath string
			if ext.FS != nil {
				modulePath = module
			} else {
				modulePath = filepath.Join(ext.Dir, module)
			}
			if cached, ok := moduleCache[modulePath]; ok {
				return cached, nil
			}

			moduleSrc, readErr := readExtensionFile(ext, modulePath)
			if readErr != nil {
				return nil, fmt.Errorf("load %s: %w", module, readErr)
			}

			globals, execErr := starlark.ExecFileOptions(&fileOpts, thread, modulePath, moduleSrc, predeclared)
			if execErr != nil {
				return nil, fmt.Errorf("load %s: %w", module, execErr)
			}
			moduleCache[modulePath] = globals

			return globals, nil
		},
	}

	globals, err := starlark.ExecFileOptions(&fileOpts, thread, implPath, src, predeclared)
	if err != nil {
		return fmt.Errorf("exec %s: %w", implPath, err)
	}

	runVal, ok := globals["run"]
	if !ok {
		return fmt.Errorf("%s: missing 'run' function", implPath)
	}
	runFunc, ok := runVal.(starlark.Callable)
	if !ok {
		return fmt.Errorf("%s: 'run' is not callable", implPath)
	}

	cmd.RunFunc = runFunc
	cmd.globals = globals
	cmd.predeclared = predeclared
	cmd.runtime = r

	cmdName := strings.ReplaceAll(cmd.Name, ".", " ")
	r.commands[cmdName] = cmd

	return nil
}

// loadExtensionCommands loads every starlark command in an extension.
//
// Skips commands without an Implementation file. Returns the first failure with the offending command name in the error
// wrap.
//
// Parameters:
//   - ext: the extension whose commands to load.
//
// Returns:
//   - error: non-nil if any command fails to load.
func (r *Application) loadExtensionCommands(ext *Extension) error {
	for _, cmd := range ext.Commands {
		if cmd.Implementation == "" {
			continue
		}
		if err := r.loadCommand(ext, cmd); err != nil {
			return fmt.Errorf("load command %s: %w", cmd.Name, err)
		}
	}
	return nil
}

// endregion

// endregion

// readExtensionFile reads a file from the extension's filesystem.
//
// Reads from the extension's embedded FS when present; otherwise reads from the OS filesystem.
//
// Parameters:
//   - ext: the owning extension.
//   - path: the file path (embedded-FS-relative or OS-relative).
//
// Returns:
//   - []byte: the file contents.
//   - error: non-nil on read failure.
func readExtensionFile(ext *Extension, path string) ([]byte, error) {
	if ext.FS != nil {
		return fs.ReadFile(ext.FS, path)
	}
	return os.ReadFile(path)
}
