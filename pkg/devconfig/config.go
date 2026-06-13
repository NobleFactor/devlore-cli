// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package devconfig is the domain-free foundation for devlore's unified configuration model.
//
// Configuration is a distributed-participation problem: independent participants — providers, subsystems, and star
// extensions — each own a slice of the configuration surface, announce their schema, and a registry assembles the
// announcements into one resolved [Config] per application process. This package holds the foundation types only; owner
// packages (pkg/signing, pkg/op, …) define their own concrete sections, and the loader builds a [Config] by rolling
// values up through a fixed precedence. See docs/architecture/configuration.md for the full design.
//
// The section family has two shapes. A Go-typed section is a plain struct embedding [SectionBase] (e.g. SigningSection
// in pkg/signing): its fields are the settings, read directly by Go consumers. A [DataSection] holds settings as a
// typed key/value bag and is the shape runtime-discovered star extensions take; it crosses into Starlark as a sealed
// mapping. Both satisfy the [Section] interface, so a [Config] holds them uniformly.
package devconfig

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"go.starlark.net/starlark"
	"gopkg.in/yaml.v3"
)

// Interface guards.
var (
	_ Section                  = SectionBase{}
	_ Section                  = (*DataSection)(nil)
	_ starlark.Value           = (*DataSection)(nil)
	_ starlark.Mapping         = (*DataSection)(nil)
	_ starlark.IterableMapping = (*DataSection)(nil)
	_ starlark.Iterator        = (*dataSectionIterator)(nil)
)

// Config is one application's resolved configuration: the family of sections, keyed by name, the build produced.
//
// It is constructed once per application process by the loader, snapshotting the schema registry, and is sealed
// thereafter — the sections it hands back are the registered instances, and mutating them after the build is a bug.
// Alongside the sections it carries a provenance sidecar recording which source won each setting, read through
// [Config.Provenance] for diagnostics.
type Config struct {
	sections   map[string]Section
	provenance map[string]map[string]SettingSourceKind
}

// NewConfig assembles a [Config] from resolved sections and their provenance sidecar.
//
// The loader calls this once, after the roll-up, with the fully resolved maps; the result is sealed. Both maps are
// retained by reference, not copied — the caller must not mutate them after construction.
//
// Parameters:
//   - `sections`: the resolved sections, keyed by section name.
//   - `provenance`: per-section maps of setting name to the source that won it.
//
// Returns:
//   - `*Config`: the assembled, sealed configuration.
func NewConfig(sections map[string]Section, provenance map[string]map[string]SettingSourceKind) *Config {
	return &Config{sections: sections, provenance: provenance}
}

// region EXPORTED METHODS

// region State management

// Provenance reports which source won a setting's resolved value.
//
// Provenance is recorded per setting, not per section: within one section, different settings may come from different
// layers. Every declared setting has a source — [SourceBuiltin] at the floor — so a false result means the section or
// setting is unknown, never that a real setting lacks provenance.
//
// Parameters:
//   - `section`: the section name.
//   - `setting`: the setting name within that section.
//
// Returns:
//   - `SettingSourceKind`: the source that won the setting's value.
//   - `bool`: true when the section and setting are both known.
func (c *Config) Provenance(section, setting string) (SettingSourceKind, bool) {

	settings, ok := c.provenance[section]
	if !ok {
		return 0, false
	}
	source, ok := settings[setting]
	return source, ok
}

// Provenances reports the source of every setting in a section, for whole-section diagnostics (config explain).
//
// The returned map is a copy; mutating it does not affect the sealed [Config].
//
// Parameters:
//   - `section`: the section name.
//
// Returns:
//   - `map[string]SettingSourceKind`: setting name to winning source; nil when the section is unknown.
func (c *Config) Provenances(section string) map[string]SettingSourceKind {

	settings, ok := c.provenance[section]
	if !ok {
		return nil
	}

	out := make(map[string]SettingSourceKind, len(settings))
	for name, source := range settings {
		out[name] = source
	}
	return out
}

// Section returns the resolved section registered under name.
//
// Parameters:
//   - `name`: the section name (e.g. "signing", "lint.copyright").
//
// Returns:
//   - `Section`: the resolved section.
//   - `bool`: true when a section is registered under name.
func (c *Config) Section(name string) (Section, bool) {
	section, ok := c.sections[name]
	return section, ok
}

// endregion

// endregion

// SectionOf returns the section of concrete type T from a [Config].
//
// It finds the one section whose concrete type matches T — Go-typed owners wrap it so consumers never assert by hand
// (e.g. signing.SectionFrom). No registry lookup is needed: there is one section per type, found by assertion.
//
// Parameters:
//   - `c`: the configuration to search.
//
// Returns:
//   - `T`: the matching section, or the zero value of T when none matches.
//   - `bool`: true when a section of type T is present.
func SectionOf[T Section](c *Config) (T, bool) {

	for _, section := range c.sections {
		if typed, ok := section.(T); ok {
			return typed, true
		}
	}

	var zero T
	return zero, false
}

// Section is the contract every configuration section satisfies: it has a name, its key within a [Config].
//
// Concrete sections embed [SectionBase] for the name and add their settings — as struct fields (a Go-typed owner like
// SigningSection) or as a key/value bag (a [DataSection]). The interface lets a [Config] hold both shapes uniformly.
type Section interface {
	Name() string
}

// SectionBase is the embeddable identity every concrete section carries — the codebase's *Base convention
// (cf. ResourceBase, OriginBase). It holds the section name, set once at construction and immutable thereafter.
type SectionBase struct {
	name string
}

// NewSectionBase returns the identity base for a section registered under name.
//
// Parameters:
//   - `name`: the section name (its key in a [Config]).
//
// Returns:
//   - `SectionBase`: the identity base to embed in a concrete section.
func NewSectionBase(name string) SectionBase {
	return SectionBase{name: name}
}

// region EXPORTED METHODS

// region State management

// Name reports the section's key within a [Config] (e.g. "signing", "lint.copyright").
//
// Returns:
//   - `string`: the section name.
func (b SectionBase) Name() string { return b.name }

// endregion

// endregion

// DataSection is a section whose settings are a typed key/value bag rather than struct fields.
//
// It is the shape runtime-discovered star extensions take — built from a [SectionSpec] — and the form any section
// crosses into Starlark as. It satisfies [starlark.Value], [starlark.Mapping], and [starlark.IterableMapping] so a
// script reads it by indexing (section["enabled"]) and iterates it like a mapping; [starlark.HasAttrs] is deliberately
// not implemented, so there is one access idiom and a missing key is a loud error, not a silent default. It is sealed
// after the build: the values are read, never written, through this type.
type DataSection struct {
	SectionBase
	values map[string]any
}

// NewDataSection builds a [DataSection] with the given name and settings.
//
// The values are retained by reference; callers must not mutate the map after construction, since the section is
// sealed. Each value is expected to already hold its declared type — instantiation happens during the build, not here.
//
// Parameters:
//   - `name`: the section name.
//   - `values`: the settings, keyed by setting name, each already of its declared type.
//
// Returns:
//   - `*DataSection`: the constructed section.
func NewDataSection(name string, values map[string]any) *DataSection {
	return &DataSection{SectionBase: NewSectionBase(name), values: values}
}

// region EXPORTED METHODS

// region State management

// Lookup returns a setting's value as its stored Go type, for dynamic Go-side reads.
//
// The value is returned as stored — already of its declared type — with no conversion. For a statically known type,
// prefer the generic [Get].
//
// Parameters:
//   - `name`: the setting name.
//
// Returns:
//   - `any`: the setting's value.
//   - `bool`: true when the setting is present.
func (d *DataSection) Lookup(name string) (any, bool) {
	value, ok := d.values[name]
	return value, ok
}

// Names returns the section's setting names in sorted order.
//
// Returns:
//   - `[]string`: the setting names, sorted.
func (d *DataSection) Names() []string {

	names := make([]string, 0, len(d.values))
	for name := range d.values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// String returns a Starlark representation of the section.
//
// Returns:
//   - `string`: the section's settings rendered as a mapping.
func (d *DataSection) String() string {

	var builder strings.Builder
	fmt.Fprintf(&builder, "config.section(%q){", d.name)
	for index, name := range d.Names() {
		if index > 0 {
			builder.WriteString(", ")
		}
		fmt.Fprintf(&builder, "%s=%v", name, d.values[name])
	}
	builder.WriteByte('}')
	return builder.String()
}

// Truth reports the section as truthy when it holds at least one setting.
//
// Returns:
//   - `starlark.Bool`: true when the section is non-empty.
func (d *DataSection) Truth() starlark.Bool { return starlark.Bool(len(d.values) > 0) }

// Type returns the Starlark type name.
//
// Returns:
//   - `string`: always "config.section".
func (d *DataSection) Type() string { return "config.section" }

// endregion

// region Behaviors

// Freeze is a no-op: a [DataSection] is already sealed by the build.
func (d *DataSection) Freeze() {}

// Get returns a setting's value as a Starlark value, implementing [starlark.Mapping] for index access.
//
// A missing key returns found=false with no error, so Starlark's indexing raises a loud "key not in" error (a schema
// typo, since the floor guarantees every declared setting is present) while membership tests still report false.
//
// Parameters:
//   - `key`: the setting name as a Starlark string.
//
// Returns:
//   - `starlark.Value`: the setting's value projected to Starlark.
//   - `bool`: true when the setting is present.
//   - `error`: non-nil only when the key is not a string or the value cannot be projected.
func (d *DataSection) Get(key starlark.Value) (starlark.Value, bool, error) {

	name, ok := starlark.AsString(key)
	if !ok {
		return nil, false, fmt.Errorf("config.section key must be a string, got %s", key.Type())
	}

	value, present := d.values[name]
	if !present {
		return nil, false, nil
	}

	projected, err := toStarlark(value)
	if err != nil {
		return nil, false, err
	}
	return projected, true, nil
}

// Hash reports the section as unhashable, matching Starlark's dict.
//
// Returns:
//   - `uint32`: always 0.
//   - `error`: always non-nil — a section is not hashable.
func (d *DataSection) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: config.section")
}

// Items returns the section's settings as (name, value) pairs in sorted order, implementing [starlark.IterableMapping].
//
// Returns:
//   - `[]starlark.Tuple`: one (name, value) tuple per setting, sorted by name.
func (d *DataSection) Items() []starlark.Tuple {

	names := d.Names()
	items := make([]starlark.Tuple, 0, len(names))
	for _, name := range names {
		value, _, _ := d.Get(starlark.String(name))
		items = append(items, starlark.Tuple{starlark.String(name), value})
	}
	return items
}

// Iterate returns an iterator over the section's setting names, implementing [starlark.IterableMapping].
//
// Returns:
//   - `starlark.Iterator`: an iterator yielding setting names in sorted order.
func (d *DataSection) Iterate() starlark.Iterator {
	return &dataSectionIterator{names: d.Names()}
}

// endregion

// endregion

// Get returns a setting from a [DataSection] as type T — an assertion over the stored value, never a parse.
//
// It is a free function, not a method, because methods cannot be generic and because the section's Starlark face
// already claims the name Get for [starlark.Mapping].
//
// Parameters:
//   - `section`: the section to read.
//   - `name`: the setting name.
//
// Returns:
//   - `T`: the setting's value, or the zero value of T when absent or of another type.
//   - `bool`: true when the setting is present and of type T.
func Get[T any](section *DataSection, name string) (T, bool) {
	value, ok := section.values[name].(T)
	return value, ok
}

// SectionSpec is the data-path schema: a section declared as data (a star extension's config block) rather than a Go
// struct. AnnounceSectionSpec turns it into a factory that builds a pre-floored [DataSection].
//
// Under the tagged-defaults model the schema is the floor: each default value's YAML tag declares its setting's type
// (Go's := applied to configuration). Defaults holds the parsed default nodes, keyed by setting name; a node's resolved
// Tag names the type and the node itself is the floor value.
type SectionSpec struct {
	Name     string
	Defaults map[string]*yaml.Node
}

// SectionConstructor builds a Go-typed section pre-floored — its builtin defaults applied — for the Go announcement
// path. The registry calls it at the config build to obtain the section the loader then overlays.
type SectionConstructor func() Section

// SettingSourceKind identifies which overlay layer won a setting's resolved value.
//
// The values are ordered low to high by precedence; every resolved setting has at least [SourceBuiltin], the floor the
// section constructor or tagged defaults provide. It applies per setting, not per section: a section's settings may
// each be won by a different layer.
type SettingSourceKind uint8

const (
	SourceBuiltin  SettingSourceKind = iota // the constructor / tagged-default floor
	SourceDefaults                          // user config.yaml, defaults: scope
	SourceApp                               // user config.yaml, <app>: scope
	SourceProject                           // app-elected project config
	SourceEnv                               // environment variables (DEVLORE_* / <APP>_*)
	SourceCLI                               // command-line flags
)

// region EXPORTED METHODS

// region State management

// String returns the layer's name, for diagnostics (config explain).
//
// Returns:
//   - `string`: the layer name (e.g. "builtin", "cli"); "unknown(N)" for an out-of-range value.
func (k SettingSourceKind) String() string {

	switch k {
	case SourceBuiltin:
		return "builtin"
	case SourceDefaults:
		return "defaults"
	case SourceApp:
		return "app"
	case SourceProject:
		return "project"
	case SourceEnv:
		return "env"
	case SourceCLI:
		return "cli"
	default:
		return fmt.Sprintf("unknown(%d)", uint8(k))
	}
}

// endregion

// endregion

// region SUPPORTING TYPES

// dataSectionIterator iterates a [DataSection]'s setting names in sorted order, satisfying [starlark.Iterator].
type dataSectionIterator struct {
	names []string
	index int
}

// region EXPORTED METHODS

// region Behaviors

// Done releases the iterator. It holds no resources, so this is a no-op.
func (it *dataSectionIterator) Done() {}

// Next yields the next setting name, implementing [starlark.Iterator].
//
// Parameters:
//   - `value`: the destination for the next setting name.
//
// Returns:
//   - `bool`: true when a name was yielded; false when the iteration is exhausted.
func (it *dataSectionIterator) Next(value *starlark.Value) bool {

	if it.index >= len(it.names) {
		return false
	}
	*value = starlark.String(it.names[it.index])
	it.index++
	return true
}

// endregion

// endregion

// endregion

// region HELPERS

// toStarlark projects a configuration value to a Starlark value.
//
// Configuration values are YAML-shaped — scalars, sequences, and string-keyed maps — plus the two yaml.v3 leaf
// extensions, time.Time and []byte. The projection is small and closed; it deliberately does not produce
// [starlark.HasAttrs] values, so a nested map becomes a plain dict.
//
// Parameters:
//   - `value`: the configuration value to project.
//
// Returns:
//   - `starlark.Value`: the projected value.
//   - `error`: non-nil when the value's type cannot be projected.
func toStarlark(value any) (starlark.Value, error) {

	switch v := value.(type) {
	case nil:
		return starlark.None, nil
	case starlark.Value:
		return v, nil
	case bool:
		return starlark.Bool(v), nil
	case string:
		return starlark.String(v), nil
	case int:
		return starlark.MakeInt(v), nil
	case int64:
		return starlark.MakeInt64(v), nil
	case float64:
		return starlark.Float(v), nil
	case time.Time:
		return starlark.String(v.Format(time.RFC3339)), nil
	case []byte:
		return starlark.Bytes(v), nil
	}

	return reflectToStarlark(reflect.ValueOf(value))
}

// reflectToStarlark projects a value of a type not handled by [toStarlark]'s switch — homogeneous sequences and
// string-keyed maps — using reflection.
//
// Parameters:
//   - `value`: the reflected configuration value.
//
// Returns:
//   - `starlark.Value`: the projected value.
//   - `error`: non-nil when the value's kind cannot be projected.
func reflectToStarlark(value reflect.Value) (starlark.Value, error) {

	switch value.Kind() {

	case reflect.Bool:
		return starlark.Bool(value.Bool()), nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return starlark.MakeInt64(value.Int()), nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return starlark.MakeUint64(value.Uint()), nil

	case reflect.Float32, reflect.Float64:
		return starlark.Float(value.Float()), nil

	case reflect.String:
		return starlark.String(value.String()), nil

	case reflect.Interface, reflect.Pointer:
		if value.IsNil() {
			return starlark.None, nil
		}
		return reflectToStarlark(value.Elem())

	case reflect.Slice, reflect.Array:
		items := make([]starlark.Value, value.Len())
		for index := range items {
			item, err := reflectToStarlark(value.Index(index))
			if err != nil {
				return nil, err
			}
			items[index] = item
		}
		return starlark.NewList(items), nil

	case reflect.Map:
		dict := starlark.NewDict(value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			key, err := reflectToStarlark(iterator.Key())
			if err != nil {
				return nil, err
			}
			mapped, err := reflectToStarlark(iterator.Value())
			if err != nil {
				return nil, err
			}
			if err := dict.SetKey(key, mapped); err != nil {
				return nil, err
			}
		}
		return dict, nil

	default:
		return nil, fmt.Errorf("devconfig: cannot project %s to a starlark value", value.Type())
	}
}

// endregion
