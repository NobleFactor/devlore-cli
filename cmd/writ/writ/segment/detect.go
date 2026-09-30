// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package segment

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/selector"
)

// EnvVarPrefix is the prefix of the environment variables that set a segment's value: WRIT_SEGMENT_ROLE=server.
const EnvVarPrefix = "WRIT_SEGMENT_"

// region EXPORTED FUNCTIONS

// DetectSegments returns this machine's built-in segments: OS, DISTRO with its lineage, and ARCH.
//
// Returns:
//   - `Segments`: the built-ins, from [selector.Detect]. DISTRO is empty off Linux and on a Linux host whose
//     os-release names no distribution.
func DetectSegments() Segments {

	host := selector.Detect()
	return Segments{
		{Name: "OS", Value: host.OS},
		{Name: "DISTRO", Value: host.Distro, Lineage: host.Lineage},
		{Name: "ARCH", Value: host.Arch},
	}
}

// Resolve returns this machine's segments: the built-ins detection supplies, then the extras configuration declares,
// each value taken from the command line, else the environment, else configuration.
//
// An extra must be declared, and its value must be one of its declared values: a `--segment` or `WRIT_SEGMENT_`
// variable naming anything else is refused, because a misspelling would otherwise match nothing and say nothing
// (ruled 2026-09-30). The built-ins take a value without a declaration.
//
// Parameters:
//   - `declared`: the extras declared in configuration, in configured order, each with its configured value.
//   - `flags`: the `--segment NAME=value` flags, in command-line order.
//
// Returns:
//   - `Segments`: the built-ins, then the extras.
//   - `error`: the declaration's problems ([selector.ValidateSegments]), or a variable or flag the declaration refuses.
func Resolve(declared []selector.Segment, flags []string) (Segments, error) {

	if err := selector.ValidateSegments(declared); err != nil {
		return nil, fmt.Errorf("writ.segments: %w", err)
	}

	segs := DetectSegments()
	for _, extra := range declared {
		segs = append(segs, Segment{Name: extra.Name, Value: extra.Value, Values: slices.Clone(extra.Values)})
	}

	for _, entry := range os.Environ() {
		name, value, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(name, EnvVarPrefix) || value == "" {
			continue
		}
		name = strings.TrimPrefix(name, EnvVarPrefix)
		if err := segs.admits(name, value); err != nil {
			return nil, fmt.Errorf("%s%s: %w", EnvVarPrefix, name, err)
		}
		segs = segs.Set(name, value)
	}

	for _, flag := range flags {
		name, value, found := strings.Cut(flag, "=")
		if !found {
			return nil, fmt.Errorf("invalid --segment %q: expected NAME=value", flag)
		}
		if err := segs.admits(name, value); err != nil {
			return nil, fmt.Errorf("--segment %s: %w", flag, err)
		}
		segs = segs.Set(name, value)
	}

	return segs, nil
}

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// admits checks a value a variable or flag gives a segment.
//
// Parameters:
//   - `name`: the segment's name.
//   - `value`: the value.
//
// Returns:
//   - `error`: nil for a built-in, or for a declared extra and one of its values; otherwise the refusal.
func (s Segments) admits(name, value string) error {

	if slices.Contains(selector.Builtins, name) {
		return nil
	}
	for _, seg := range s {
		if seg.Name != name {
			continue
		}
		if !slices.Contains(seg.Values, value) {
			return fmt.Errorf("%q is not one of %s's declared values %v", value, name, seg.Values)
		}
		return nil
	}
	return fmt.Errorf("%s is not a declared segment; declare it in writ.segments", name)
}

// endregion

// endregion
