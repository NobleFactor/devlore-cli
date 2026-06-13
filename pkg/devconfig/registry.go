// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package devconfig

import (
	"fmt"
	"reflect"
	"sort"
	"sync"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// announcements is the package-level registry of announced section schemas — Go-typed constructors (the Go path) and
// data-path specs — keyed by section name.
//
// One singleton, [announced], accumulates every section announced at init() (Go owners) or extension-discovery time
// (data path). The loader snapshots it once per application process at the config build. The registry holds only inert
// schema — constructors and specs, never resolved values — so the process-wide singleton is safe, mirroring
// op.announced. The mutex serializes registration against the snapshot reads.
type announcements struct {
	mu      sync.Mutex
	entries map[string]announcement
}

// announcement is one section's announced schema: exactly one of construct (Go path) or spec (data path) is set, and
// sectionType records the Go-typed section's concrete type, nil on the data path.
type announcement struct {
	sectionType reflect.Type
	construct   SectionConstructor
	spec        *SectionSpec
}

// announced is the package singleton, populated at init() and extension-discovery time and snapshotted by the loader.
var announced = &announcements{entries: make(map[string]announcement)}

// region UNEXPORTED METHODS — announcements

// region State management

// constructorFor returns the Go-path constructor announced under name.
//
// Parameters:
//   - `name`: the section name.
//
// Returns:
//   - `SectionConstructor`: the announced constructor, or nil when name is absent or was announced as a data spec.
//   - `bool`: true when name has a Go-path constructor.
func (a *announcements) constructorFor(name string) (SectionConstructor, bool) {

	a.mu.Lock()
	defer a.mu.Unlock()

	entry, ok := a.entries[name]
	if !ok {
		return nil, false
	}
	return entry.construct, entry.construct != nil
}

// names returns the announced section names in sorted order.
//
// Returns:
//   - `[]string`: the section names, sorted.
func (a *announcements) names() []string {

	a.mu.Lock()
	defer a.mu.Unlock()

	names := make([]string, 0, len(a.entries))
	for name := range a.entries {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// specFor returns the data-path spec announced under name.
//
// Parameters:
//   - `name`: the section name.
//
// Returns:
//   - `SectionSpec`: the announced spec, or the zero value when name is absent or was announced as a Go constructor.
//   - `bool`: true when name has a data-path spec.
func (a *announcements) specFor(name string) (SectionSpec, bool) {

	a.mu.Lock()
	defer a.mu.Unlock()

	entry, ok := a.entries[name]
	if !ok || entry.spec == nil {
		return SectionSpec{}, false
	}
	return *entry.spec, true
}

// endregion

// region Behaviors

// register inserts entry under name, rejecting a duplicate.
//
// Parameters:
//   - `name`: the section name; the registry key.
//   - `entry`: the announced schema to store.
//
// Returns:
//   - `error`: non-nil when a section is already announced under name, naming both claimants.
func (a *announcements) register(name string, entry announcement) error {

	a.mu.Lock()
	defer a.mu.Unlock()

	if existing, exists := a.entries[name]; exists {
		return fmt.Errorf("config section %q already announced by %s; rejected %s",
			name, claimant(existing), claimant(entry))
	}

	a.entries[name] = entry
	return nil
}

// endregion

// endregion

// AnnounceSection registers a Go-typed section's constructor — the Go announcement path, called from a package init().
//
// The section is keyed by the name its constructor reports. A duplicate name is a programmer error — two compiled-in
// packages claiming one name — so it is **fatal** at announce time, both claimants named: the Go Must idiom, mirroring
// op.AnnounceProvider. The constructor is invoked once here to read the name, and is retained for the loader to call
// again at the config build.
//
// Parameters:
//   - `sectionType`: the section's concrete `reflect.Type`, recorded for collision diagnostics.
//   - `construct`: the factory that builds the section pre-floored; must be non-nil.
func AnnounceSection(sectionType reflect.Type, construct SectionConstructor) {

	label := fmt.Sprintf("AnnounceSection(%s)", sectionType)
	assert.Truef(construct != nil, "%s: construct must not be nil", label)

	err := announced.register(construct().Name(), announcement{sectionType: sectionType, construct: construct})
	assert.NoError(label, err)
}

// AnnounceSectionSpec registers a data-path section schema — the data announcement path, called at
// extension-discovery time.
//
// The spec is user-supplied data (a star extension), so a duplicate name — two extensions, or one shadowing a
// framework name — is a user error, **returned, never fatal**; the first writer keeps the name.
//
// Parameters:
//   - `spec`: the data-path schema; its `Name` is the registry key and must not be empty.
//
// Returns:
//   - `error`: non-nil when the name is empty or already announced.
func AnnounceSectionSpec(spec SectionSpec) error {

	if spec.Name == "" {
		return fmt.Errorf("AnnounceSectionSpec: section name must not be empty")
	}

	stored := spec
	return announced.register(spec.Name, announcement{spec: &stored})
}

// AnnouncedSectionNames returns the names of all announced sections in sorted order, for diagnostics and tests.
//
// Returns:
//   - `[]string`: the announced section names, sorted.
func AnnouncedSectionNames() []string {
	return announced.names()
}

// ConstructorFor returns the Go-path constructor announced under name. The loader calls it at the config build.
//
// Parameters:
//   - `name`: the section name.
//
// Returns:
//   - `SectionConstructor`: the announced constructor, or nil when name has no Go-path constructor.
//   - `bool`: true when name has a Go-path constructor.
func ConstructorFor(name string) (SectionConstructor, bool) {
	return announced.constructorFor(name)
}

// SpecFor returns the data-path spec announced under name. The loader calls it at the config build.
//
// Parameters:
//   - `name`: the section name.
//
// Returns:
//   - `SectionSpec`: the announced spec, or the zero value when name has no data-path spec.
//   - `bool`: true when name has a data-path spec.
func SpecFor(name string) (SectionSpec, bool) {
	return announced.specFor(name)
}

// region HELPERS

// claimant describes an announcement for collision diagnostics: the Go section's type, or that it is a data spec.
//
// Parameters:
//   - `a`: the announcement to describe.
//
// Returns:
//   - `string`: the concrete section type for the Go path, or a fixed phrase for the data path.
func claimant(a announcement) string {

	if a.sectionType != nil {
		return a.sectionType.String()
	}
	return "a data-path section spec"
}

// endregion
