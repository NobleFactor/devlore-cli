// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package segment holds writ's segments, the built-in ones detection supplies (OS, DISTRO, ARCH) and the extras
// declared in configuration, and selects a layer's directories with them through the one selector API
// (pkg/selector, #944).
package segment

import (
	"slices"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/selector"
)

// Segment is one segment: a built-in, or an extra declared in configuration.
type Segment struct {

	// Name is the segment's name: OS, DISTRO, ARCH, ROLE.
	Name string

	// Value is this machine's value: Linux, Ubuntu, arm64, desktop. Empty when unset.
	Value string

	// Values are an extra's declared values; nil for a built-in.
	Values []string

	// Lineage is DISTRO's ancestors, most general first: Debian, for Ubuntu. Nil for every other segment.
	Lineage []string
}

// Segments is this machine's segments: OS, DISTRO and ARCH, then the declared extras in configured order.
type Segments []Segment

// region EXPORTED METHODS

// region State management

// Get returns a segment's value.
//
// Parameters:
//   - `name`: the segment's name.
//
// Returns:
//   - `string`: its value; empty when unset or not a segment.
func (s Segments) Get(name string) string {

	for _, seg := range s {
		if seg.Name == name {
			return seg.Value
		}
	}
	return ""
}

// Set returns the segments with one segment's value replaced, or with the segment appended when there is none by
// that name. A replaced DISTRO keeps its lineage: an override names the distribution, not its ancestors.
//
// Parameters:
//   - `name`: the segment's name.
//   - `value`: its new value.
//
// Returns:
//   - `Segments`: a copy with the value set.
func (s Segments) Set(name, value string) Segments {

	result := slices.Clone(s)
	for i := range result {
		if result[i].Name == name {
			result[i].Value = value
			return result
		}
	}
	return append(result, Segment{Name: name, Value: value})
}

// endregion

// region Behaviors

// Extras returns the declared extras, in configured order, as the selector takes them.
//
// Returns:
//   - `[]selector.Segment`: the extras.
func (s Segments) Extras() []selector.Segment {

	var extras []selector.Segment
	for _, seg := range s {
		if !slices.Contains(selector.Builtins, seg.Name) {
			extras = append(extras, selector.Segment{Name: seg.Name, Values: seg.Values, Value: seg.Value})
		}
	}
	return extras
}

// Host returns the machine the built-in segments describe, overrides included.
//
// Returns:
//   - `selector.Host`: the machine: its chain is Unix when the OS is a Unix, the OS, then DISTRO's lineage and DISTRO.
func (s Segments) Host() selector.Host {

	var lineage []string
	for _, seg := range s {
		if seg.Name == "DISTRO" {
			lineage = seg.Lineage
		}
	}
	return selector.NewHostFromWords(s.Get("OS"), s.Get("DISTRO"), lineage, s.Get("ARCH"))
}

// Selector returns the selector for these segments and a list of projects.
//
// Parameters:
//   - `projects`: the projects, in the order they're applied.
//
// Returns:
//   - `selector.Selector`: the selector.
func (s Segments) Selector(projects []string) selector.Selector {

	return selector.Selector{Host: s.Host(), Projects: projects, Segments: s.Extras()}
}

// String returns the set segments as NAME=value pairs, in order.
//
// Returns:
//   - `string`: the pairs, comma-separated.
func (s Segments) String() string {

	var parts []string
	for _, seg := range s {
		if seg.Value != "" {
			parts = append(parts, seg.Name+"="+seg.Value)
		}
	}
	return strings.Join(parts, ", ")
}

// endregion

// endregion
