// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package config

import (
	"fmt"
	"os"

	"go.starlark.net/starlark"
	"gopkg.in/yaml.v3"
)

// Config provides a unified view of all configuration.
// Extensions register their config specs, then LoadFromFiles() merges
// values from star/config.yaml files into the registered hierarchy.
type Config struct {
	extensions *extensionsConfig
}

// New creates a Config with an empty extension hierarchy.
// Call RegisterExtension() to add config specs, then LoadFromFiles() to populate.
func New() *Config {
	return &Config{
		extensions: newExtensionsConfig("star/config.yaml"),
	}
}

// Load creates a Config, registers no extensions, and loads from files.
// This is a convenience wrapper for tests and standalone use.
func Load() (*Config, error) {
	c := New()
	if err := c.LoadFromFiles(); err != nil {
		return nil, err
	}
	return c, nil
}

// LoadWithSources loads configuration and returns the source of each file.
func LoadWithSources() (*Config, []ConfigSource, error) {
	c := New()

	var sources []ConfigSource
	sources = append(sources, ConfigSource{
		Path:   "<defaults>",
		Exists: true,
	})

	userPath := userConfigPath()
	if userPath != "" {
		_, err := os.Stat(userPath)
		sources = append(sources, ConfigSource{
			Path:   userPath,
			Exists: err == nil,
		})
	}

	projectPath := projectConfigPath()
	if projectPath != "" {
		_, err := os.Stat(projectPath)
		sources = append(sources, ConfigSource{
			Path:   projectPath,
			Exists: err == nil,
		})
	}

	if err := c.LoadFromFiles(); err != nil {
		return nil, nil, err
	}

	return c, sources, nil
}

// LoadFromFiles reads user and project star/config.yaml files and merges
// values into the registered extension hierarchy. Extensions must be
// registered before calling this.
func (c *Config) LoadFromFiles() error {
	// Load user config from XDG_CONFIG_HOME
	userPath := userConfigPath()
	if userPath != "" {
		if err := c.loadFile(userPath); err != nil {
			return fmt.Errorf("load user config: %w", err)
		}
	}

	// Load project config from git workspace root
	projectPath := projectConfigPath()
	if projectPath != "" {
		if err := c.loadFile(projectPath); err != nil {
			return fmt.Errorf("load project config: %w", err)
		}
	}

	return nil
}

// loadFile reads a YAML file and merges values into the extension hierarchy.
func (c *Config) loadFile(path string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	c.extensions.mergeRaw(raw)
	return nil
}

// MergeYAML parses YAML bytes and merges values into the extension hierarchy.
func (c *Config) MergeYAML(data []byte) error {
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parse yaml: %w", err)
	}
	c.extensions.mergeRaw(raw)
	return nil
}

// Navigate traverses the config hierarchy by dotted path.
// Returns the element or field value at the given path, or nil if not found.
func (c *Config) Navigate(path string) interface{} {
	return c.extensions.Navigate(path)
}

// RegisterExtension registers an extension's config at a dotted path.
// Creates intermediate elements as needed.
func (c *Config) RegisterExtension(path string, spec ConfigSpec) error {
	return c.extensions.registerExtension(path, spec)
}

// GetSpec returns the ConfigSpec for an extension path.
func (c *Config) GetSpec(path string) (ConfigSpec, bool) {
	return c.extensions.getSpec(path)
}

// Accessor returns a typed accessor for a section at the given path.
func (c *Config) Accessor(path string) *ConfigAccessor {
	return c.extensions.accessor(path)
}

// Sync generates tool-specific config files from star/config.yaml.
func (c *Config) Sync() (*SyncResult, error) {
	return syncFromConfig(c)
}

// ToStarlark returns the config wrapped for Starlark access.
func (c *Config) ToStarlark() starlark.Value {
	return WrapAsStarlarkValue(c.extensions)
}
