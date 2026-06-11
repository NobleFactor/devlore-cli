// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConfigSpec describes the configuration schema for an extension.
// It is used by Worker 3 (extension package) to register extension configs.
type ConfigSpec struct {
	Type     string                 // Go type name (informational)
	Fields   map[string]string      // field name → type (bool, string, int, []string, map[K]V)
	Nested   map[string]ConfigSpec  // nested type definitions (e.g., Pattern)
	Defaults map[string]interface{} // default values
}

// extensionsConfig is the fsroot of the extension configuration hierarchy.
// It embeds ConfigElement with path = "" and manages extension configs.
// This is private - consumers should use the unified Config type.
type extensionsConfig struct {
	ConfigElement                       // path = "", children = top-level sections
	source        string                // filename, e.g., "star/config.yaml"
	dirty         bool                  // modified since load
	specs         map[string]ConfigSpec // registered extension specs by path
}

// newExtensionsConfig creates a new empty extension configuration fsroot.
func newExtensionsConfig(source string) *extensionsConfig {
	return &extensionsConfig{
		ConfigElement: ConfigElement{path: ""},
		source:        source,
		specs:         make(map[string]ConfigSpec),
	}
}

// Source returns the configuration source path.
func (c *extensionsConfig) Source() string {
	return c.source
}

// IsDirty returns true if config has been modified since load.
func (c *extensionsConfig) IsDirty() bool {
	return c.dirty
}

// SetDirty marks the config as modified.
func (c *extensionsConfig) SetDirty(dirty bool) {
	c.dirty = dirty
}

// RegisterExtension registers an extension's config at a dotted path.
// Creates intermediate ConfigElements as needed and generates a typed instance.
//
// Example:
//
//	c.RegisterExtension("lint.copyright", ConfigSpec{
//	    Fields: map[string]string{
//	        "enabled": "bool",
//	        "license": "string",
//	    },
//	    Defaults: map[string]interface{}{
//	        "enabled": false,
//	        "license": "auto",
//	    },
//	})
func (c *extensionsConfig) registerExtension(path string, spec ConfigSpec) error {
	if path == "" {
		return fmt.Errorf("extension path cannot be empty")
	}

	// Store spec for later reference
	c.specs[path] = spec

	// Split path into parts
	parts := strings.Split(path, ".")
	current := &c.ConfigElement

	// Navigate/create intermediate elements
	for _, part := range parts[:len(parts)-1] {
		child := current.Get(part)
		if child == nil {
			// Create intermediate ConfigElement
			intermediate := &ConfigElement{}
			current.Register(part, intermediate)
			child = intermediate
		}

		// Get ConfigElement from child
		switch ch := child.(type) {
		case *ConfigElement:
			current = ch
		default:
			// Child embeds ConfigElement - extract it
			if elem := extractConfigElement(child); elem != nil {
				current = elem
			} else {
				return fmt.Errorf("cannot navigate through non-ConfigElement at %s", part)
			}
		}
	}

	// Create and register the leaf element
	typ := getOrCreateType(path, spec)
	instance := newConfigInstance(typ, spec.Defaults)
	current.Register(parts[len(parts)-1], instance)

	return nil
}

// getSpec returns the ConfigSpec for an extension path.
func (c *extensionsConfig) getSpec(path string) (ConfigSpec, bool) {
	spec, ok := c.specs[path]
	return spec, ok
}

// accessor returns a typed accessor for a section at the given path.
func (c *extensionsConfig) accessor(path string) *ConfigAccessor {
	elem := c.Navigate(path)
	if elem == nil {
		return &ConfigAccessor{}
	}
	return NewAccessor(elem)
}

// loadExtensions reads configuration from a file and merges it into the hierarchy.
// Extensions must be registered before calling Load.
func loadExtensions(source string) (*extensionsConfig, error) {
	cfg := newExtensionsConfig(source)

	// Read source file
	data, err := os.ReadFile(source)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist - return empty config
			return cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	// Parse YAML
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Merge raw values into hierarchy
	cfg.mergeRaw(raw)

	return cfg, nil
}

// loadExtensionsWithSpecs registers extensions and loads configuration.
// This is a convenience function that combines registerExtension and loadExtensions.
func loadExtensionsWithSpecs(source string, specs map[string]ConfigSpec) (*extensionsConfig, error) {
	cfg := newExtensionsConfig(source)

	// register all extensions
	for path, spec := range specs {
		if err := cfg.registerExtension(path, spec); err != nil {
			return nil, fmt.Errorf("register %s: %w", path, err)
		}
	}

	// Read and merge source file
	data, err := os.ReadFile(source)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.mergeRaw(raw)
	return cfg, nil
}

// save writes configuration back to the source file.
func (c *extensionsConfig) save() error {
	data, err := yaml.Marshal(c.toMap())
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(c.source, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	c.dirty = false
	return nil
}

// mergeRaw merges raw YAML values into the configuration hierarchy.
func (c *extensionsConfig) mergeRaw(raw map[string]interface{}) {
	c.mergeInto(&c.ConfigElement, raw, "")
}

// mergeInto recursively merges values into a config element.
func (c *extensionsConfig) mergeInto(elem *ConfigElement, values map[string]interface{}, pathPrefix string) {
	for key, val := range values {
		childPath := key
		if pathPrefix != "" {
			childPath = pathPrefix + "." + key
		}

		child := elem.Get(key)
		if child == nil {
			// No registered element at this path - skip
			continue
		}

		switch ch := child.(type) {
		case *ConfigElement:
			// Intermediate element - recurse
			if childMap, ok := val.(map[string]interface{}); ok {
				c.mergeInto(ch, childMap, childPath)
			}
		default:
			// Leaf element (generated config struct) - merge values
			if childMap, ok := val.(map[string]interface{}); ok {
				if elem := extractConfigElement(child); elem != nil {
					// Has children - recurse
					c.mergeInto(elem, childMap, childPath)
				}
				// Also merge into the struct fields
				mergeIntoStruct(child, childMap)
			}
		}
	}
}

// mergeIntoStruct merges map values into a struct's fields.
func mergeIntoStruct(obj interface{}, values map[string]interface{}) {
	acc := NewAccessor(obj)
	if !acc.IsValid() {
		return
	}

	rv := acc.Raw()
	for name, val := range values {
		fieldName := toPascalCase(name)
		field := rv.FieldByName(fieldName)
		if field.IsValid() && field.CanSet() {
			setFieldValue(field, val)
		}
	}
}

// toMap converts the configuration hierarchy to a map for YAML serialization.
func (c *extensionsConfig) toMap() map[string]interface{} {
	return c.elementToMap(&c.ConfigElement)
}

// elementToMap recursively converts a ConfigElement to a map.
func (c *extensionsConfig) elementToMap(elem *ConfigElement) map[string]interface{} {
	result := make(map[string]interface{})

	for name, child := range elem.Children() {
		switch ch := child.(type) {
		case *ConfigElement:
			result[name] = c.elementToMap(ch)
		default:
			// Struct - convert to map
			result[name] = structToMap(child)
		}
	}

	return result
}

// structToMap converts a struct to a map for serialization.
func structToMap(obj interface{}) map[string]interface{} {
	acc := NewAccessor(obj)
	if !acc.IsValid() {
		return nil
	}

	result := make(map[string]interface{})
	for _, name := range acc.Fields() {
		result[name] = acc.Get(name)
	}
	return result
}

// wrapAsStarlark wraps the config for Starlark access.
// Uses the ConfigValue type for reflection-based access.
func (c *extensionsConfig) wrapAsStarlark() interface{} {
	return WrapAsStarlarkValue(c)
}
