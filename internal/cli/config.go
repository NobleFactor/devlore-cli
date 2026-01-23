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

// NewConfigCmd creates the config command with git-style interface.
// Usage:
//
//	tool config <key>              # Get value
//	tool config <key> <value>      # Set value
//	tool config --list             # List all settings
//	tool config --edit             # Open in $EDITOR
//	tool config --unset <key>      # Remove a key
//	tool config --validate         # Validate against schema
//	tool config --schema           # Output JSON schema
//	tool config --path             # Show config file location
func NewConfigCmd(info ConfigInfo) *cobra.Command {
	var (
		list     bool
		edit     bool
		unset    string
		validate bool
		schema   bool
		path     bool
		mutation MutationFlags
	)

	cmd := &cobra.Command{
		Use:   "config [<key> [<value>]]",
		Short: "Get and set configuration options",
		Long: `Get and set ` + info.Name + ` configuration options.

The configuration file is stored at $XDG_CONFIG_HOME/` + info.Name + `/config.yaml
(default: ~/.config/` + info.Name + `/config.yaml).

Examples:
  ` + info.Name + ` config repo                    # Get repo path
  ` + info.Name + ` config repo ~/dotfiles         # Set repo path
  ` + info.Name + ` config --list                  # List all settings
  ` + info.Name + ` config --edit                  # Open config in editor
  ` + info.Name + ` config --unset repo            # Remove repo setting
`,
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath := configFilePath(info.Name)

			// Handle flags first (mutually exclusive operations)
			switch {
			case list:
				return configList(cfgPath)
			case edit:
				return configEdit(cfgPath, info.DefaultConfig)
			case unset != "":
				if err := configUnset(cfgPath, unset); err != nil {
					return err
				}
				return RenderMutationTo(map[string]string{"unset": unset}, mutation)
			case validate:
				return configValidate(cfgPath, info.Schema)
			case schema:
				return configSchema(info.Schema)
			case path:
				return configPath(cfgPath)
			}

			// Positional arguments: get or set
			switch len(args) {
			case 0:
				// No args and no flags: show help
				return cmd.Help()
			case 1:
				// Get value
				return configGet(cfgPath, args[0])
			case 2:
				// Set value
				if err := configSet(cfgPath, args[0], args[1]); err != nil {
					return err
				}
				// Output with --passthru
				return RenderMutationTo(map[string]string{args[0]: args[1]}, mutation)
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&list, "list", "l", false, "List all configuration settings")
	cmd.Flags().BoolVarP(&edit, "edit", "e", false, "Open config file in editor")
	cmd.Flags().StringVar(&unset, "unset", "", "Remove a configuration key")
	cmd.Flags().BoolVar(&validate, "validate", false, "Validate config against schema")
	cmd.Flags().BoolVar(&schema, "schema", false, "Output embedded JSON schema")
	cmd.Flags().BoolVar(&path, "path", false, "Show config file location")
	AddMutationFlags(cmd, &mutation)

	// Add completion for config keys from schema
	cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// Only complete first argument (the key)
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return getSchemaKeys(info.Schema, toComplete), cobra.ShellCompDirectiveNoFileComp
	}

	// Also add completion for --unset flag
	cmd.RegisterFlagCompletionFunc("unset", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return getSchemaKeys(info.Schema, toComplete), cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

// configFilePath returns the path to the config file.
func configFilePath(toolName string) string {
	return filepath.Join(ConfigHome(), toolName, "config.yaml")
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
