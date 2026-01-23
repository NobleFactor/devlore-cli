// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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

			for _, key := range args {
				value, exists := getNestedValue(config, key)
				if !exists {
					return fmt.Errorf("key not found: %s", key)
				}
				fmt.Println(formatValue(value))
			}
			return nil
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
					return fmt.Errorf("invalid argument %q: expected key=value", arg)
				}
				key := arg[:idx]
				value := arg[idx+1:]
				setNestedValue(config, key, value)
			}

			return saveConfig(cfgPath, config)
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
					return fmt.Errorf("key not found: %s", key)
				}
			}

			return saveConfig(cfgPath, config)
		},
	}

	cmd.ValidArgsFunction = configKeyCompletion(info)

	return cmd
}

func newConfigListCmd(info ConfigInfo) *cobra.Command {
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
				return nil
			}

			printFlattened("", config)
			return nil
		},
	}
}

func newConfigEditCmd(info ConfigInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Open configuration file in $EDITOR",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath := SharedConfigPath()
			return configEdit(cfgPath, info.DefaultConfig)
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
			return configValidate(cfgPath, info.Schema)
		},
	}
}

func newConfigSchemaCmd(info ConfigInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "schema",
		Short: "Output the embedded JSON schema",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return configSchema(info.Schema)
		},
	}
}

func newConfigPathCmd(info ConfigInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Show configuration file location",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return configPath(SharedConfigPath())
		},
	}
}

// loadConfig loads the config file as a map.
func loadConfig(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]interface{}), nil
		}
		return nil, err
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}

	if config == nil {
		config = make(map[string]interface{})
	}

	return config, nil
}

// saveConfig saves the config map to file.
func saveConfig(path string, config map[string]interface{}) error {
	// Create directory if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// configGet retrieves a configuration value.
func configGet(path, key string) error {
	config, err := loadConfig(path)
	if err != nil {
		return err
	}

	value, exists := getNestedValue(config, key)
	if !exists {
		return fmt.Errorf("key not found: %s", key)
	}

	fmt.Println(formatValue(value))
	return nil
}

// configSet sets a configuration value.
func configSet(path, key, value string) error {
	config, err := loadConfig(path)
	if err != nil {
		return err
	}

	setNestedValue(config, key, value)

	return saveConfig(path, config)
}

// configList displays all configuration settings.
func configList(path string) error {
	config, err := loadConfig(path)
	if err != nil {
		return err
	}

	if len(config) == 0 {
		fmt.Println("# No configuration set")
		return nil
	}

	printFlattened("", config)
	return nil
}

// configEdit opens the config file in the user's editor.
func configEdit(path string, defaultConfig []byte) error {
	// Create config with defaults if it doesn't exist
	if _, err := os.Stat(path); os.IsNotExist(err) {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
		if err := os.WriteFile(path, defaultConfig, 0644); err != nil {
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

	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// configUnset removes a configuration key.
func configUnset(path, key string) error {
	config, err := loadConfig(path)
	if err != nil {
		return err
	}

	if !deleteNestedValue(config, key) {
		return fmt.Errorf("key not found: %s", key)
	}

	return saveConfig(path, config)
}

// configValidate validates the config against the schema.
func configValidate(path string, schemaBytes []byte) error {
	config, err := loadConfig(path)
	if err != nil {
		return err
	}

	if len(config) == 0 {
		fmt.Println("No config file (using defaults)")
		return nil
	}

	// Parse schema
	var schema map[string]interface{}
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		return fmt.Errorf("failed to parse schema: %w", err)
	}

	// Basic validation: check for unknown keys
	properties, _ := schema["properties"].(map[string]interface{})
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
		fmt.Println("Validation warnings:")
		for _, w := range warnings {
			fmt.Printf("  %s\n", w)
		}
		return nil
	}

	fmt.Printf("Config %s is valid\n", path)
	return nil
}

// configSchema outputs the embedded JSON schema.
func configSchema(schemaBytes []byte) error {
	var schema interface{}
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		return fmt.Errorf("failed to parse schema: %w", err)
	}

	output, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format schema: %w", err)
	}

	fmt.Println(string(output))
	return nil
}

// configPath shows the config file location.
func configPath(path string) error {
	fmt.Println(path)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Println("# (file does not exist)")
	}
	return nil
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
func setNestedValue(m map[string]interface{}, key, value string) {
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

// printFlattened prints a nested map in key=value format.
func printFlattened(prefix string, m map[string]interface{}) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}

		if nested, ok := v.(map[string]interface{}); ok {
			printFlattened(key, nested)
		} else {
			fmt.Printf("%s=%s\n", key, formatValue(v))
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

// =============================================================================
// Shared Config: Repository Registration
// =============================================================================
//
// All configuration writes go through these functions to maintain a single
// source of truth. The shared config lives at $XDG_CONFIG_HOME/devlore/config.yaml
// and is shared across writ and lore.
//
// Repos are stored as:
//
//	writ:
//	  repos:
//	    - layer: personal
//	      path: ~/dotfiles
//	    - layer: team
//	      path: ~/work/configs
//	      url: git@github.com:company/configs.git

// RepoEntry represents a repository configuration entry.
type RepoEntry struct {
	Layer string // "personal", "team", or "base"
	Path  string // Local filesystem path
	URL   string // Optional remote URL
	Name  string // Optional display name
}

// RegisterRepo registers a repository in the shared config file.
// If a repo with the same layer already exists, it is replaced.
func RegisterRepo(tool string, entry RepoEntry) error {
	cfgPath := SharedConfigPath()

	config, err := loadConfig(cfgPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	// Ensure tool section exists
	toolSection, ok := config[tool].(map[string]interface{})
	if !ok {
		toolSection = make(map[string]interface{})
		config[tool] = toolSection
	}

	// Get or create repos array, removing existing entry for this layer
	var repos []interface{}
	if existing, ok := toolSection["repos"].([]interface{}); ok {
		for _, r := range existing {
			if repoMap, ok := r.(map[string]interface{}); ok {
				if repoMap["layer"] != entry.Layer {
					repos = append(repos, r)
				}
			}
		}
	}

	// Build new entry
	newRepo := map[string]interface{}{
		"layer": entry.Layer,
		"path":  entry.Path,
	}
	if entry.URL != "" {
		newRepo["url"] = entry.URL
	}
	if entry.Name != "" {
		newRepo["name"] = entry.Name
	}
	repos = append(repos, newRepo)

	toolSection["repos"] = repos

	return saveConfig(cfgPath, config)
}

// UnregisterRepo removes a repository for the given layer from the shared config.
func UnregisterRepo(tool, layer string) error {
	cfgPath := SharedConfigPath()

	config, err := loadConfig(cfgPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	toolSection, ok := config[tool].(map[string]interface{})
	if !ok {
		return fmt.Errorf("no %s configuration found", tool)
	}

	existing, ok := toolSection["repos"].([]interface{})
	if !ok || len(existing) == 0 {
		return fmt.Errorf("no repos configured for %s", tool)
	}

	var repos []interface{}
	found := false
	for _, r := range existing {
		if repoMap, ok := r.(map[string]interface{}); ok {
			if repoMap["layer"] == layer {
				found = true
				continue
			}
		}
		repos = append(repos, r)
	}

	if !found {
		return fmt.Errorf("no %s repo configured for layer %q", tool, layer)
	}

	toolSection["repos"] = repos

	return saveConfig(cfgPath, config)
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
