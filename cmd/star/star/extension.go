// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package star

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/cmd/star/config"
)

// Source identifies where an extension was discovered.
type Source int

const (
	SourceProjectLocal Source = iota // ${GIT_WORKSPACE_ROOT}/star/extensions/
	SourceUser                       // ${XDG_DATA_HOME}/star/extensions/
	SourceSystem                     // /usr/local/share/star/extensions/
	SourceEmbedded                   // compiled into binary via //go:embed
)

// String returns the human-readable name of the source.
func (s Source) String() string {
	switch s {
	case SourceProjectLocal:
		return "project-local"
	case SourceUser:
		return "user"
	case SourceSystem:
		return "system"
	case SourceEmbedded:
		return "embedded"
	default:
		return fmt.Sprintf("Source(%d)", int(s))
	}
}

// Extension is the immutable identity and context for a loaded extension.
// YAML fields are deserialized directly via UnmarshalYAML. Runtime fields are
// set by the discovery and loading code after unmarshaling.
type Extension struct {
	// YAML fields — populated by UnmarshalYAML.
	Name        string        `yaml:"extension"`
	Description string        `yaml:"description"`
	Commands    []*Command    `yaml:"commands"`
	Config      *ConfigSchema `yaml:"config"`

	// Runtime fields — set after unmarshaling.
	Source Source         `yaml:"-"`
	Dir    string         `yaml:"-"`
	FS     fs.FS          `yaml:"-"`
	config *config.Config `yaml:"-"` // reference to unified config (not owned)
}

// ConfigSchema describes the configuration schema for an extension.
type ConfigSchema struct {
	Path     string                  `yaml:"path"`
	Type     string                  `yaml:"type"`
	Fields   map[string]string       `yaml:"fields"`
	Nested   map[string]ConfigNested `yaml:"nested"`
	Defaults map[string]interface{}  `yaml:"defaults"`
}

// ConfigNested describes a struct type used within an extension's config fields.
type ConfigNested struct {
	Fields map[string]string       `yaml:"fields"`
	Nested map[string]ConfigNested `yaml:"nested,omitempty"`
}

// Validate checks that the extension is well-formed.
func (e *Extension) Validate() error {
	if e.Name == "" {
		return fmt.Errorf("extension name is required")
	}
	if err := validateReverseDomainName(e.Name); err != nil {
		return fmt.Errorf("extension name %q: %w", e.Name, err)
	}
	if len(e.Commands) == 0 {
		return fmt.Errorf("extension must define at least one command")
	}

	for i, c := range e.Commands {
		if c.Name == "" {
			return fmt.Errorf("command[%d] name is required", i)
		}
		if c.Implementation == "" {
			return fmt.Errorf("command %q: implementation is required", c.Name)
		}
		if !strings.HasPrefix(c.Implementation, "commands/") {
			return fmt.Errorf("command %q: implementation must be in commands/ subdirectory", c.Name)
		}
		for j, a := range c.Args {
			if a.Name == "" {
				return fmt.Errorf("command %q arg[%d]: name is required", c.Name, j)
			}
			if a.Variadic && j != len(c.Args)-1 {
				return fmt.Errorf("command %q arg %q: variadic arg must be last", c.Name, a.Name)
			}
		}
		for j, f := range c.Flags {
			if f.Name == "" {
				return fmt.Errorf("command %q flag[%d]: name is required", c.Name, j)
			}
			if f.Type == "" {
				return fmt.Errorf("command %q flag %q: type is required", c.Name, f.Name)
			}
			switch f.Type {
			case "bool", "string", "int", "glob":
				// valid
			default:
				return fmt.Errorf("command %q flag %q: unknown type %q", c.Name, f.Name, f.Type)
			}
		}
	}

	return nil
}

// ConfigPath returns the dotted path where this extension's config is registered.
func (e *Extension) ConfigPath() string {
	if e.Config != nil && e.Config.Path != "" {
		return e.Config.Path
	}
	if len(e.Commands) == 1 {
		return e.Commands[0].Name
	}
	return e.Name
}

// ToConfigSpec converts the extension's ConfigSchema to config.ConfigSpec.
func (e *Extension) ToConfigSpec() config.ConfigSpec {
	if e.Config == nil {
		return config.ConfigSpec{}
	}
	return config.ConfigSpec{
		Type:     e.Config.Type,
		Fields:   copyStringMap(e.Config.Fields),
		Nested:   convertNested(e.Config.Nested),
		Defaults: copyDefaults(e.Config.Defaults),
	}
}

// HasCommands returns true if this extension provides CLI commands.
func (e *Extension) HasCommands() bool {
	return len(e.Commands) > 0
}

// HasConfig returns true if this extension has a configuration schema.
func (e *Extension) HasConfig() bool {
	return e.Config != nil
}

// GetCommand returns the Command for the given name, or nil if not found.
func (e *Extension) GetCommand(name string) *Command {
	for _, cmd := range e.Commands {
		if cmd.Name == name {
			return cmd
		}
	}
	return nil
}

// SetConfig sets the reference to the unified config tree.
func (e *Extension) SetConfig(cfg *config.Config) {
	e.config = cfg
}

// ResolveConfig returns the resolved config accessor for this extension on demand.
func (e *Extension) ResolveConfig() *config.ConfigAccessor {
	if e.config == nil || e.Config == nil {
		return nil
	}
	return e.config.Accessor(e.ConfigPath())
}

// region starlark.Value interface

// String implements starlark.Value.
func (e *Extension) String() string {
	return fmt.Sprintf("<extension %s>", e.Name)
}

// Type implements starlark.Value.
func (e *Extension) Type() string { return "extension" }

// Freeze implements starlark.Value.
func (e *Extension) Freeze() {} // immutable

// Truth implements starlark.Value.
func (e *Extension) Truth() starlark.Bool { return starlark.True }

// Hash implements starlark.Value.
func (e *Extension) Hash() (uint32, error) {
	return starlark.String(e.Name).Hash()
}

// Attr implements starlark.HasAttrs.
func (e *Extension) Attr(name string) (starlark.Value, error) {
	switch name {
	case "name":
		return starlark.String(e.Name), nil
	case "description":
		return starlark.String(e.Description), nil
	case "dir":
		return starlark.String(e.Dir), nil
	case "source":
		return starlark.String(e.Source.String()), nil
	case "config":
		return starlark.NewBuiltin("extension.config", e.starlarkConfig), nil
	default:
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("extension has no .%s attribute", name))
	}
}

// AttrNames implements starlark.HasAttrs.
func (e *Extension) AttrNames() []string {
	return []string{"config", "description", "dir", "name", "source"}
}

// starlarkConfig is the starlark builtin for extension.config().
func (e *Extension) starlarkConfig(
	_ *starlark.Thread,
	_ *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	if err := starlark.UnpackArgs("extension.config", args, kwargs); err != nil {
		return nil, err
	}
	acc := e.ResolveConfig()
	if acc == nil {
		return starlark.None, nil
	}
	return config.WrapAsStarlarkValue(acc.Raw().Interface()), nil
}

// endregion

// region helpers

func validateReverseDomainName(name string) error {
	parts := strings.Split(name, ".")
	if len(parts) < 3 {
		return fmt.Errorf("must have at least 3 segments (e.g., com.example.Name)")
	}
	tld := parts[0]
	validTLDs := map[string]bool{"com": true, "org": true, "io": true, "net": true, "dev": true, "app": true}
	if !validTLDs[tld] {
		return fmt.Errorf("first segment %q should be a TLD (com, org, io, net, dev, app)", tld)
	}
	for i, part := range parts {
		if part == "" {
			return fmt.Errorf("segment %d is empty", i)
		}
	}
	return nil
}

func convertNested(defs map[string]ConfigNested) map[string]config.ConfigSpec {
	if len(defs) == 0 {
		return nil
	}
	result := make(map[string]config.ConfigSpec, len(defs))
	for name, def := range defs {
		result[name] = config.ConfigSpec{
			Fields: copyStringMap(def.Fields),
			Nested: convertNested(def.Nested),
		}
	}
	return result
}

func copyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	result := make(map[string]string, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

func copyDefaults(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		switch val := v.(type) {
		case map[string]interface{}:
			result[k] = copyDefaults(val)
		case []interface{}:
			result[k] = copySlice(val)
		default:
			result[k] = v
		}
	}
	return result
}

func copySlice(s []interface{}) []interface{} {
	result := make([]interface{}, len(s))
	for i, v := range s {
		switch val := v.(type) {
		case map[string]interface{}:
			result[i] = copyDefaults(val)
		case []interface{}:
			result[i] = copySlice(val)
		default:
			result[i] = v
		}
	}
	return result
}

// Ensure interfaces are satisfied.
var (
	_ starlark.Value    = (*Extension)(nil)
	_ starlark.HasAttrs = (*Extension)(nil)
)

// endregion

// Sort extensions by name for consistent output.
func init() {
	_ = sort.StringSlice{}
}
