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
// Do this in PersistentPreRunE of the root command.
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
// Do this after defining flags and before Execute().
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

// SharedConfigPath returns the path to the shared devlore config file.
func SharedConfigPath() string {
	return filepath.Join(ConfigHome(), "devlore", "config.yaml")
}
