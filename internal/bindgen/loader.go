// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bindgen

import (
	"os"

	"gopkg.in/yaml.v3"
)

// LoadYAML loads a binding definition from a YAML file.
func LoadYAML(path string) (*BindingDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var def BindingDef
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, err
	}

	// Populate command names from map keys
	for name, cmd := range def.Commands {
		cmd.Name = name
	}

	return &def, nil
}

// SaveYAML saves a binding definition to a YAML file.
func SaveYAML(def *BindingDef, path string) error {
	data, err := yaml.Marshal(def)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o600)
}

// Merge combines two binding definitions, with override taking precedence.
func Merge(base, override *BindingDef) *BindingDef {
	result := &BindingDef{
		Name:        base.Name,
		Description: base.Description,
		Commands:    make(map[string]*Command),
	}

	if override.Name != "" {
		result.Name = override.Name
	}
	if override.Description != "" {
		result.Description = override.Description
	}

	// Copy base commands
	for name, cmd := range base.Commands {
		result.Commands[name] = cmd
	}

	// Override/add from override
	for name, cmd := range override.Commands {
		if existing, ok := result.Commands[name]; ok {
			result.Commands[name] = mergeCommand(existing, cmd)
		} else {
			result.Commands[name] = cmd
		}
	}

	return result
}

// mergeCommand combines two command definitions.
func mergeCommand(base, override *Command) *Command {
	result := &Command{
		Name:        base.Name,
		Description: base.Description,
		Args:        base.Args,
		Flags:       base.Flags,
		Returns:     base.Returns,
	}

	if override.Description != "" {
		result.Description = override.Description
	}
	if len(override.Args) > 0 {
		result.Args = override.Args
	}
	if len(override.Flags) > 0 {
		result.Flags = mergeFlags(base.Flags, override.Flags)
	}
	if override.Returns != nil {
		result.Returns = override.Returns
	}

	return result
}

// mergeFlags combines flag lists, with override taking precedence.
func mergeFlags(base, override []*Flag) []*Flag {
	flagMap := make(map[string]*Flag)

	for _, f := range base {
		flagMap[f.Name] = f
	}
	for _, f := range override {
		flagMap[f.Name] = f
	}

	result := make([]*Flag, 0, len(flagMap))
	for _, f := range flagMap {
		result = append(result, f)
	}
	return result
}
