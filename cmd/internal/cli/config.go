// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package cli provides shared CLI infrastructure for writ and lore commands.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/cmd/internal/devlore"
	"github.com/NobleFactor/devlore-cli/pkg/document"
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
)

// ConfigInfo contains configuration metadata for a tool.
type ConfigInfo struct {
	Name          string // Tool name (e.g., "lore", "writ")
	Schema        []byte // Embedded JSON schema
	DefaultConfig []byte // Default configuration content
}

// NewConfigCmd creates the config command with git-style subcommands.
//
// Dot-paths match the config file structure exactly. No implicit prefixing.
// "writ.repos.0.path" in the CLI reads writ.repos[0].path in the file.
// "secrets.mode" reads secrets.mode. WYSIWYG.
//
// Usage:
//
//	tool config get <key>...                    # Get values
//	tool config set <key>=<value>...            # Set values
//	tool config unset <key>...                  # Remove keys
//	tool config list                            # List all settings
//	tool config edit                            # Open in $EDITOR
//	tool config validate                        # Validate against schema
//	tool config schema                          # Output JSON schema
//	tool config path                            # Show config file location
func NewConfigCmd(info ConfigInfo) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config <command>",
		Short: "Get and set configuration options",
		Long: `Get and set ` + info.Name + ` configuration options.

Configuration is stored at $XDG_CONFIG_HOME/devlore/config.yaml
(default: ~/.config/devlore/config.yaml). This file is shared across
writ and lore, with tool-specific settings under their respective keys.

Dot-paths match the file structure exactly:
  "` + info.Name + `.repos.0.path"    → tool-specific setting
  "secrets.mode"             → shared setting
`,
	}

	cmd.AddCommand(newConfigGetCmd(info))
	cmd.AddCommand(newConfigSetCmd(info))
	cmd.AddCommand(newConfigUnsetCmd(info))
	cmd.AddCommand(newConfigListCmd(info))
	cmd.AddCommand(newConfigEditCmd(info))
	cmd.AddCommand(newConfigValidateCmd(info))
	cmd.AddCommand(newConfigSchemaCmd(info))
	cmd.AddCommand(newConfigPathCmd(info))

	return cmd
}

// configKeyCompletion returns a ValidArgsFunction for config key completion.
// Completions include full dot-paths matching the file structure.
func configKeyCompletion(info ConfigInfo) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		keys := getSchemaKeys(info.Schema, toComplete)
		return keys, cobra.ShellCompDirectiveNoFileComp
	}
}

func newConfigGetCmd(info ConfigInfo) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <key>...",
		Short: "Get configuration values",
		Long: `Get one or more configuration values by dot-path key.

Examples:
  ` + info.Name + ` config get ` + info.Name + `.repos.0.path
  ` + info.Name + ` config get secrets.identity_file
  ` + info.Name + ` config get ` + info.Name + `.vars.USER_NAME ` + info.Name + `.vars.USER_EMAIL`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath := SharedConfigPath()
			config, err := loadConfig(cfgPath)
			if err != nil {
				return err
			}

			// The result is the keys asked for and their values; `-o value` prints the values alone.
			values := make(map[string]any, len(args))
			for _, key := range args {
				value, exists := getNestedValue(config, key)
				if !exists {
					return ExitWith(ExitDataErr, fmt.Errorf("key not found: %s", key))
				}
				values[key] = value
			}
			return Emit(cmd, values)
		},
	}

	cmd.ValidArgsFunction = configKeyCompletion(info)

	return cmd
}

func newConfigSetCmd(info ConfigInfo) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <key>=<value>...",
		Short: "Set configuration values",
		Long: `Set one or more configuration values using key=value syntax.

Examples:
  ` + info.Name + ` config set ` + info.Name + `.vars.USER_NAME="David Noble"
  ` + info.Name + ` config set secrets.mode=0600
  ` + info.Name + ` config set ` + info.Name + `.vars.USER_NAME=david ` + info.Name + `.vars.USER_EMAIL=d@example.com`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath := SharedConfigPath()
			config, err := loadConfig(cfgPath)
			if err != nil {
				return err
			}

			for _, arg := range args {
				idx := strings.Index(arg, "=")
				if idx == -1 {
					return ExitWith(ExitUsage, fmt.Errorf("invalid argument %q: expected key=value", arg))
				}
				key := arg[:idx]
				value := arg[idx+1:]
				typed, err := coerceValue(info.Schema, key, value)
				if err != nil {
					return err
				}
				setNestedValue(config, key, typed)
			}

			configRoot, err := OpenTree(devlore.ConfigHome())
			if err != nil {
				return err
			}
			//nolint:errcheck // diagnose-ignored-error: the write's own error is what the command reports; a close failure after it has nothing left to protect
			defer configRoot.Close()

			return saveConfig(configRoot, configRoot.NewPath(cfgPath), config)
		},
	}

	cmd.ValidArgsFunction = configKeyCompletion(info)

	return cmd
}

func newConfigUnsetCmd(info ConfigInfo) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unset <key>...",
		Short: "Remove configuration keys",
		Long: `Remove one or more configuration keys.

Examples:
  ` + info.Name + ` config unset ` + info.Name + `.vars.USER_NAME
  ` + info.Name + ` config unset secrets.identity_command`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath := SharedConfigPath()
			config, err := loadConfig(cfgPath)
			if err != nil {
				return err
			}

			for _, key := range args {
				if !deleteNestedValue(config, key) {
					return ExitWith(ExitDataErr, fmt.Errorf("key not found: %s", key))
				}
			}

			configRoot, err := OpenTree(devlore.ConfigHome())
			if err != nil {
				return err
			}
			//nolint:errcheck // diagnose-ignored-error: the write's own error is what the command reports; a close failure after it has nothing left to protect
			defer configRoot.Close()

			return saveConfig(configRoot, configRoot.NewPath(cfgPath), config)
		},
	}

	cmd.ValidArgsFunction = configKeyCompletion(info)

	return cmd
}

func newConfigListCmd(_ ConfigInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all configuration settings",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath := SharedConfigPath()
			config, err := loadConfig(cfgPath)
			if err != nil {
				return err
			}

			if len(config) == 0 {
				Note("No configuration set")
			}

			// The result is every setting by its dotted key; `-o value` prints the values, `-o list` one per line.
			flat := map[string]any{}
			flatten("", config, flat)
			return Emit(cmd, flat)
		},
	}
}

func newConfigEditCmd(info ConfigInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Open configuration file in $EDITOR",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			// The command owns the root (#405, phase 2b). OpenTree because the config tree may not exist
			// on first run, and opening is a query: it creates the tree, then opens a confined root at it.
			configRoot, err := OpenTree(devlore.ConfigHome())
			if err != nil {
				return err
			}
			defer iox.Close(&err, configRoot)

			return configEdit(configRoot, configRoot.NewPath(SharedConfigPath()), info.DefaultConfig)
		},
	}
}

func newConfigValidateCmd(info ConfigInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration against schema",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath := SharedConfigPath()
			return configValidate(cmd, cfgPath, info.Schema)
		},
	}
}

func newConfigSchemaCmd(info ConfigInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "schema",
		Short: "Output the embedded JSON schema",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return configSchema(cmd, info.Schema)
		},
	}
}

func newConfigPathCmd(_ ConfigInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Show configuration file location",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return configPath(cmd, SharedConfigPath())
		},
	}
}

// loadConfig loads the config file as a map. Supports YAML and JSON formats, detected by file extension.
//
// Parameters:
//   - path: filesystem path to the config file
//
// Returns:
//   - map[string]interface{}: parsed config (empty map if file does not exist)
//   - error: read or parse error
func loadConfig(path string) (map[string]interface{}, error) {

	cfg, err := document.ReadFile[map[string]interface{}](path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return make(map[string]interface{}), nil
	}
	if *cfg == nil {
		return make(map[string]interface{}), nil
	}

	return *cfg, nil
}

// saveConfig saves the config map to file. Supports YAML and JSON formats, detected by file extension.
//
// The root is received, never constructed (#405, phase 2b): the command owns the config tree.
//
// Parameters:
//   - configRoot: the config tree, opened by the calling command
//   - path: the config file's path within configRoot
//   - config: config map to serialize
//
// Returns:
//   - error: marshal or write error
func saveConfig(configRoot fsroot.Dir, path fsroot.Path, config map[string]interface{}) error {

	return document.WriteFile(configRoot, path, config)
}

// configEdit opens the config file in the user's editor, seeding it with defaults when absent.
//
// The root is received, never constructed (#405, phase 2b): the command owns the config tree.
//
// Parameters:
//   - `configRoot`: the devlore config tree, opened by the caller.
//   - `path`: the config file within that root.
//   - `defaultConfig`: the contents to seed the file with when it does not exist.
//
// Returns:
//   - `error`: non-nil when the file cannot be created or the editor fails.
func configEdit(configRoot fsroot.Dir, path fsroot.Path, defaultConfig []byte) error {
	// Create config with defaults if it doesn't exist
	if _, err := configRoot.Stat(path); os.IsNotExist(err) {
		if err := configRoot.MkdirAll(configRoot.NewPath(filepath.Dir(path.Rel())), 0o750); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
		if err := configRoot.WriteFile(path, defaultConfig, 0o600); err != nil {
			return fmt.Errorf("failed to create config: %w", err)
		}
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = "vi"
	}

	cmd := exec.CommandContext(context.Background(), editor, path.Abs()) //nolint:gosec // G204: editor from EDITOR/VISUAL env var

	return RunInteractive(cmd, "run `config path` and open the file in your own editor")
}

// configReport is `config validate`'s result: the file, whether it passed, and what was warned about.
type configReport struct {
	Path     string   `json:"path"`
	Present  bool     `json:"present"`
	Valid    bool     `json:"valid"`
	Warnings []string `json:"warnings"`
}

// configValidate validates the config against the schema. The report is the result; the verdict narrates.
func configValidate(cmd *cobra.Command, path string, schemaBytes []byte) error {
	config, err := loadConfig(path)
	if err != nil {
		return err
	}

	if len(config) == 0 {
		Note("No config file (using defaults)")
		return Emit(cmd, configReport{Path: path, Present: false, Valid: true, Warnings: []string{}})
	}

	// Parse schema
	var schema map[string]interface{}
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		return fmt.Errorf("failed to parse schema: %w", err)
	}

	// Basic validation: check for unknown keys
	properties, _ := schema["properties"].(map[string]interface{}) //nolint:errcheck // zero value (nil) is acceptable
	if properties == nil {
		properties = make(map[string]interface{})
	}

	var warnings []string
	for key := range config {
		if _, exists := properties[key]; !exists {
			warnings = append(warnings, fmt.Sprintf("unknown key: %s", key))
		}
	}

	if len(warnings) > 0 {
		Warn("Validation warnings:")
		for _, w := range warnings {
			Warn("  %s", w)
		}
	} else {
		Success("Config %s is valid", path)
	}

	if warnings == nil {
		warnings = []string{}
	}
	return Emit(cmd, configReport{Path: path, Present: true, Valid: true, Warnings: warnings})
}

// configSchema emits the embedded JSON schema as the result; `-o yaml` reads it as YAML.
func configSchema(cmd *cobra.Command, schemaBytes []byte) error {
	var schema interface{}
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		return fmt.Errorf("failed to parse schema: %w", err)
	}

	return Emit(cmd, schema)
}

// configPath emits the config file location as the result; whether the file exists is narration.
func configPath(cmd *cobra.Command, path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		Note("%s does not exist yet", path)
	}
	return Emit(cmd, path)
}

// getNestedValue retrieves a value from a nested map using dot notation.
func getNestedValue(m map[string]interface{}, key string) (interface{}, bool) {
	parts := strings.Split(key, ".")
	current := interface{}(m)

	for _, part := range parts {
		if cm, ok := current.(map[string]interface{}); ok {
			current, ok = cm[part]
			if !ok {
				return nil, false
			}
		} else {
			return nil, false
		}
	}

	return current, true
}

// setNestedValue sets a value in a nested map using dot notation.
func setNestedValue(m map[string]interface{}, key string, value interface{}) {
	parts := strings.Split(key, ".")

	if len(parts) == 1 {
		m[key] = value
		return
	}

	// Navigate/create nested maps
	current := m
	for _, part := range parts[:len(parts)-1] {
		if next, ok := current[part].(map[string]interface{}); ok {
			current = next
		} else {
			next := make(map[string]interface{})
			current[part] = next
			current = next
		}
	}

	current[parts[len(parts)-1]] = value
}

// deleteNestedValue removes a value from a nested map using dot notation.
func deleteNestedValue(m map[string]interface{}, key string) bool {
	parts := strings.Split(key, ".")

	if len(parts) == 1 {
		if _, exists := m[key]; exists {
			delete(m, key)
			return true
		}
		return false
	}

	// Navigate to parent
	current := m
	for _, part := range parts[:len(parts)-1] {
		if next, ok := current[part].(map[string]interface{}); ok {
			current = next
		} else {
			return false
		}
	}

	lastKey := parts[len(parts)-1]
	if _, exists := current[lastKey]; exists {
		delete(current, lastKey)
		return true
	}
	return false
}

// flatten folds a nested config into dotted keys and their values, for `config list`'s result.
func flatten(prefix string, m map[string]interface{}, into map[string]any) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}

		if nested, ok := v.(map[string]interface{}); ok {
			flatten(key, nested, into)
		} else {
			into[key] = v
		}
	}
}

// formatValue formats a value for display.
func formatValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", val)
	}
}

// getSchemaKeys extracts all valid config keys from a JSON schema.
// Returns keys in dot notation (e.g., "vars.USER_NAME").
func getSchemaKeys(schemaBytes []byte, prefix string) []string {
	var schema map[string]interface{}
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		return nil
	}

	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		return nil
	}

	var keys []string
	extractKeys("", properties, &keys)

	// Filter by prefix if provided
	if prefix == "" {
		return keys
	}

	var filtered []string
	for _, k := range keys {
		if strings.HasPrefix(k, prefix) {
			filtered = append(filtered, k)
		}
	}
	return filtered
}

// extractKeys recursively extracts keys from a JSON schema properties object.
func extractKeys(prefix string, properties map[string]interface{}, keys *[]string) {
	for name, prop := range properties {
		key := name
		if prefix != "" {
			key = prefix + "." + name
		}

		*keys = append(*keys, key)

		// Check for nested properties (object type)
		if propMap, ok := prop.(map[string]interface{}); ok {
			if nested, ok := propMap["properties"].(map[string]interface{}); ok {
				extractKeys(key, nested, keys)
			}
		}
	}
}

// coerceValue converts a string value to the appropriate Go type based on
// the JSON schema type for the given key. Returns an error if the key is
// unknown (not declared in schema and parent has no additionalProperties)
// or if the value can't be parsed to the declared type.
func coerceValue(schemaBytes []byte, key, value string) (interface{}, error) {
	schemaType, found := schemaTypeForKey(schemaBytes, key)
	if !found {
		return nil, ExitWith(ExitDataErr, fmt.Errorf("unknown configuration key: %s", key))
	}

	switch schemaType {
	case "boolean":
		switch strings.ToLower(value) {
		case "true", "1", "yes", "on":
			return true, nil
		case "false", "0", "no", "off":
			return false, nil
		default:
			return nil, ExitWith(ExitDataErr, fmt.Errorf("invalid boolean for %s: %q (expected true/false)", key, value))
		}
	case "integer":
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, ExitWith(ExitDataErr, fmt.Errorf("invalid integer for %s: %q", key, value))
		}
		return n, nil
	case "number":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, ExitWith(ExitDataErr, fmt.Errorf("invalid number for %s: %q", key, value))
		}
		return f, nil
	}

	return value, nil
}

// schemaTypeForKey walks the JSON schema to find the type declaration for
// a dot-path key. Returns the type string and true if found, or "" and false
// if the key is not declared in the schema. Respects additionalProperties
// for keys under objects like writ.vars or writ.targets.
func schemaTypeForKey(schemaBytes []byte, key string) (string, bool) {
	var schema map[string]interface{}
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		return "", false
	}

	parts := strings.Split(key, ".")
	current := schema

	for i, part := range parts {
		properties, _ := current["properties"].(map[string]interface{}) //nolint:errcheck // zero value (nil) is acceptable

		prop, inProperties := properties[part].(map[string]interface{})
		if !inProperties {
			// Check additionalProperties on the current schema node
			addlProps, hasAddl := current["additionalProperties"].(map[string]interface{})
			if !hasAddl {
				return "", false
			}

			// For the last part, the type comes from additionalProperties
			if i == len(parts)-1 {
				if t, ok := addlProps["type"].(string); ok {
					return t, true
				}
				return "string", true
			}

			// For intermediate parts, descend into additionalProperties
			current = addlProps
			continue
		}

		// Found in declared properties
		if i == len(parts)-1 {
			if t, ok := prop["type"].(string); ok {
				return t, true
			}
			return "string", true
		}

		// Descend into this property for the next part
		current = prop
	}

	return "", false
}
