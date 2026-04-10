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

	commandsprov "github.com/NobleFactor/devlore-cli/cmd/star/provider/commands"
	"github.com/NobleFactor/devlore-cli/pkg/op/bind"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/ui"

	"github.com/NobleFactor/devlore-cli/cmd/star/config"
)

// DryRun is set by the --dry-run global flag. When true, side-effect
// bindings (fs.write, fs.mkdir, fs.remove, etc.) log what they would
// do instead of executing.
var DryRun bool

// Application manages Starlark script execution.
type Application struct {
	registry *ExtensionRegistry
	commands map[string]*Command
	config   *config.Config // Unified config for builtin and extension config
	star     *bind.StarlarkRuntime
	data     map[string]any // Shared context data (dry_run, config, etc.)

	// UIProvider is the canonical UI output provider.
	// Wire --silent to UIProvider.Silent in main.
	UIProvider *ui.Provider
}

// NewApplication creates a new star Application with a fully initialized Starlark runtime.
func NewApplication() *Application {
	wd, _ := os.Getwd()
	data := map[string]any{"dry_run": DryRun}

	registry := op.NewReceiverRegistry()
	cfg := op.NewRuntimeEnvironmentSpec("star", registry).
		WithModules(registry.Modules()...).
		WithRoot(op.NewRootReaderWriter(wd)).
		WithData(data).
		WithColor()
	star := bind.NewStarlarkRuntime(cfg)

	// UIProvider is exposed for --silent flag wiring in main.
	uip := &ui.Provider{
		Writer:      os.Stderr,
		ProgramName: "star",
		Color:       true,
	}

	rt := &Application{
		registry:   NewExtensionRegistry(),
		commands:   make(map[string]*Command),
		star:       star,
		data:       data,
		UIProvider: uip,
	}

	// Wire command tree into context data (shared map — providers read it lazily).
	data["command_tree"] = rt

	return rt
}

// Config returns the unified config, initializing if needed.
func (r *Application) Config() *config.Config {
	if r.config == nil {
		r.config = config.New()
	}
	return r.config
}

// Registry returns the extension registry.
func (r *Application) Registry() *ExtensionRegistry {
	return r.registry
}

// DiscoverAndLoad uses the given loader to discover extensions, then registers
// and activates them. This is the single entry point for extension loading.
func (r *Application) DiscoverAndLoad(loader *ExtensionLoader) error {
	// Step 1: Discover — parse and deduplicate.
	exts, err := loader.DiscoverAll()
	if err != nil {
		return err
	}

	// Step 2: register — add to registry, register config schemas, load config.
	for _, ext := range exts {
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

	// Step 3: Activate — parse .star files, set RunFunc, build cobra tree.
	for _, ext := range exts {
		ext.SetConfig(r.Config())

		if ext.HasCommands() {
			if err := r.loadExtensionCommands(ext); err != nil {
				return fmt.Errorf("load extension %s: %w", ext.Name, err)
			}
		}
	}

	return nil
}

// LoadExtensionsFrom loads extensions from a specific directory. Used by tests
// that need to load from a known path without the full discovery flow.
func (r *Application) LoadExtensionsFrom(dir string) error {
	loader := &ExtensionLoader{
		searchPaths: []string{dir},
	}

	exts, err := loader.DiscoverAll()
	if err != nil {
		return err
	}

	for _, ext := range exts {
		if err := r.registry.Register(ext); err != nil {
			// Skip duplicates in tests.
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

	for _, ext := range exts {
		ext.SetConfig(r.Config())

		if ext.HasCommands() {
			if err := r.loadExtensionCommands(ext); err != nil {
				return fmt.Errorf("load extension %s: %w", ext.Name, err)
			}
		}
	}

	return nil
}

// loadExtensionCommands loads all starlark commands from an extension.
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

// readExtensionFile reads a file from the extension's filesystem. If the extension has an
// embedded FS, reads from there; otherwise reads from the OS filesystem.
func readExtensionFile(ext *Extension, path string) ([]byte, error) {
	if ext.FS != nil {
		return fs.ReadFile(ext.FS, path)
	}
	return os.ReadFile(path)
}

// loadCommand loads a single starlark command.
func (r *Application) loadCommand(ext *Extension, cmd *Command) error {
	// Build path to implementation file.
	var implPath string
	if ext.FS != nil {
		implPath = cmd.Implementation
	} else {
		implPath = filepath.Join(ext.Dir, cmd.Implementation)
	}

	// Read the implementation source.
	src, err := readExtensionFile(ext, implPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", implPath, err)
	}

	// Build predeclared environment.
	predeclared := r.star.Predeclared()

	// Dialect options shared by the main script and any load() targets.
	fileOpts := syntax.FileOptions{
		Set:             true,
		While:           true,
		TopLevelControl: true,
		GlobalReassign:  true,
		Recursion:       true,
	}

	// Module cache for load() — prevents re-executing the same file.
	moduleCache := map[string]starlark.StringDict{}

	// Create thread with print and load functions.
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

	// Execute the command script.
	globals, err := starlark.ExecFileOptions(&fileOpts, thread, implPath, src, predeclared)
	if err != nil {
		return fmt.Errorf("exec %s: %w", implPath, err)
	}

	// Look for the run function.
	runVal, ok := globals["run"]
	if !ok {
		return fmt.Errorf("%s: missing 'run' function", implPath)
	}
	runFunc, ok := runVal.(starlark.Callable)
	if !ok {
		return fmt.Errorf("%s: 'run' is not callable", implPath)
	}

	// Set runtime fields on the command.
	cmd.RunFunc = runFunc
	cmd.globals = globals
	cmd.predeclared = predeclared
	cmd.runtime = r

	// register with space-separated name (e.g., "lint.go" -> "lint go").
	cmdName := strings.ReplaceAll(cmd.Name, ".", " ")
	r.commands[cmdName] = cmd

	return nil
}

// Commands returns all registered commands.
func (r *Application) Commands() map[string]*Command {
	return r.commands
}

// CommandNames implements commands.CommandTree.
func (r *Application) CommandNames() []string {
	names := make([]string, 0, len(r.commands))
	for name := range r.commands {
		names = append(names, name)
	}
	return names
}

// RunCommand implements commands.CommandTree.
func (r *Application) RunCommand(name string, flags map[string]string, positional ...string) error {
	cmd, ok := r.commands[name]
	if !ok {
		return fmt.Errorf("command %q not found", name)
	}
	return cmd.Run(flags, positional...)
}

// CommandHelp implements commands.CommandTree.
func (r *Application) CommandHelp(name string) string {
	spaceName := strings.ReplaceAll(name, ".", " ")
	if cmd, ok := r.commands[spaceName]; ok {
		return cmd.Help
	}
	return ""
}

// CommandFlags implements commands.CommandTree.
func (r *Application) CommandFlags(name string) []commandsprov.CommandFlag {
	spaceName := strings.ReplaceAll(name, ".", " ")
	cmd, ok := r.commands[spaceName]
	if !ok {
		return nil
	}
	flags := make([]commandsprov.CommandFlag, len(cmd.Flags))
	for i, f := range cmd.Flags {
		flags[i] = commandsprov.CommandFlag{Name: f.Name, Help: f.Help, Default: f.Default}
	}
	return flags
}

// Close releases all resources held by the application.
func (r *Application) Close() error {
	return nil
}
