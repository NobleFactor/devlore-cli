// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// ViperConfig holds configuration for Viper initialization.
type ViperConfig struct {
	// Name is the tool name (e.g., "lore", "writ")
	Name string

	// EnvPrefix is the environment variable prefix (e.g., "LORE", "WRIT")
	// If empty, defaults to uppercase Name
	EnvPrefix string

	// ConfigName is the config file name without extension (default: "config")
	ConfigName string

	// ConfigType is the config file type (default: "yaml")
	ConfigType string

	// UseSharedConfig uses ~/.config/devlore/config.yaml with tool-specific section
	// When true, config is read from the tool's section (e.g., config.writ.repo)
	UseSharedConfig bool
}

// InitViper initializes Viper with standard devlore conventions.
// Call this in PersistentPreRunE of the root command.
//
// Precedence (lowest to highest):
//  1. Config file defaults
//  2. Config file values
//  3. Environment variables (TOOL_KEY_NAME)
//  4. Command-line flags
//
// Environment variable mapping:
//   - WRIT_REPO → writ.repo (with UseSharedConfig)
//   - WRIT_VARS_USER_NAME → writ.vars.user_name
//   - Dots become underscores, keys are case-insensitive
func InitViper(cfg ViperConfig) error {
	if cfg.Name == "" {
		return fmt.Errorf("ViperConfig.ReceiverName is required")
	}

	if cfg.EnvPrefix == "" {
		cfg.EnvPrefix = strings.ToUpper(cfg.Name)
	}
	if cfg.ConfigName == "" {
		cfg.ConfigName = "config"
	}
	if cfg.ConfigType == "" {
		cfg.ConfigType = "yaml"
	}

	// Set config file details
	viper.SetConfigName(cfg.ConfigName)
	viper.SetConfigType(cfg.ConfigType)

	// Add config paths
	if cfg.UseSharedConfig {
		// Shared devlore config: ~/.config/devlore/config.yaml
		viper.AddConfigPath(filepath.Join(ConfigHome(), "devlore"))
	} else {
		// Tool-specific config: ~/.config/<tool>/config.yaml
		viper.AddConfigPath(filepath.Join(ConfigHome(), cfg.Name))
	}

	// Environment variable binding
	viper.SetEnvPrefix(cfg.EnvPrefix)
	viper.AutomaticEnv()
	// Replace dots with underscores for nested keys: writ.vars.user_name → WRIT_VARS_USER_NAME
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Read config file (ignore if not found)
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := errors.AsType[viper.ConfigFileNotFoundError](err); !ok {
			return fmt.Errorf("error reading config: %w", err)
		}
		// Config file not found is OK - use defaults and env vars
	}

	return nil
}

// BindFlags binds all persistent flags from a command to Viper.
// Call this after defining flags and before Execute().
//
// Flags are bound with the tool's section prefix when using shared config:
//   - --repo flag → viper key "writ.repo" (with UseSharedConfig)
//   - --repo flag → viper key "repo" (without UseSharedConfig)
func BindFlags(cmd *cobra.Command, toolName string, useSharedConfig bool) error {
	var bindErr error

	cmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		if bindErr != nil {
			return
		}

		key := f.Name
		if useSharedConfig {
			key = toolName + "." + f.Name
		}

		if err := viper.BindPFlag(key, f); err != nil {
			bindErr = fmt.Errorf("failed to bind flag %s: %w", f.Name, err)
		}
	})

	return bindErr
}

// BindFlagsWithPrefix binds flags with a custom prefix.
// Useful for subcommands that need their own config section.
//
// Example:
//
//	BindFlagsWithPrefix(addCmd.Flags(), "writ.add")
//	// --overwrite flag → viper key "writ.add.overwrite"
func BindFlagsWithPrefix(flags *pflag.FlagSet, prefix string) error {
	var bindErr error

	flags.VisitAll(func(f *pflag.Flag) {
		if bindErr != nil {
			return
		}

		key := prefix + "." + f.Name
		if err := viper.BindPFlag(key, f); err != nil {
			bindErr = fmt.Errorf("failed to bind flag %s: %w", f.Name, err)
		}
	})

	return bindErr
}

// Get retrieves a config value with the tool's section prefix.
// When using shared config, automatically prefixes with tool name.
//
// Example:
//
//	Get("writ", "repo", true)  → viper.Get("writ.repo")
//	Get("writ", "repo", false) → viper.Get("repo")
func Get(toolName, key string, useSharedConfig bool) interface{} {
	if useSharedConfig {
		return viper.Get(toolName + "." + key)
	}
	return viper.Get(key)
}

// GetString retrieves a string config value.
func GetString(toolName, key string, useSharedConfig bool) string {
	if useSharedConfig {
		return viper.GetString(toolName + "." + key)
	}
	return viper.GetString(key)
}

// GetBool retrieves a boolean config value.
func GetBool(toolName, key string, useSharedConfig bool) bool {
	if useSharedConfig {
		return viper.GetBool(toolName + "." + key)
	}
	return viper.GetBool(key)
}

// GetInt retrieves an integer config value.
func GetInt(toolName, key string, useSharedConfig bool) int {
	if useSharedConfig {
		return viper.GetInt(toolName + "." + key)
	}
	return viper.GetInt(key)
}

// GetStringSlice retrieves a string slice config value.
func GetStringSlice(toolName, key string, useSharedConfig bool) []string {
	if useSharedConfig {
		return viper.GetStringSlice(toolName + "." + key)
	}
	return viper.GetStringSlice(key)
}

// GetStringMap retrieves a string map config value.
func GetStringMap(toolName, key string, useSharedConfig bool) map[string]interface{} {
	if useSharedConfig {
		return viper.GetStringMap(toolName + "." + key)
	}
	return viper.GetStringMap(key)
}

// SharedConfigPath returns the path to the shared devlore config file.
func SharedConfigPath() string {
	return filepath.Join(ConfigHome(), "devlore", "config.yaml")
}

// ToolConfigPath returns the path to a tool-specific config file.
func ToolConfigPath(toolName string) string {
	return filepath.Join(ConfigHome(), toolName, "config.yaml")
}

// ConfigFileUsed returns the config file path that Viper loaded.
// Returns empty string if no config file was found.
func ConfigFileUsed() string {
	return viper.ConfigFileUsed()
}

// AllSettings returns all settings as a map.
func AllSettings() map[string]interface{} {
	return viper.AllSettings()
}

// Debug prints current Viper state for debugging.
func Debug() {
	Note("Config file: %s", viper.ConfigFileUsed())
	Note("Settings:")
	for k, v := range viper.AllSettings() {
		Note("  %s = %v", k, v)
	}
}
