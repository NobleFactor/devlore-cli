// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package file

import (
	"encoding/json"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// ResourceKind is the kind a discovery or resolution asserts (4-resource-management.md §5.7, ruled
// 2026-08-22).
//
// The `entry` zero value is the permissive default: the short spelling accepts whatever kind the disk
// holds, and asserting a specific kind is opt-in strictness whose verdict sharpens at the asserting
// action's own node. Kinds are lstat-strict (step 23, ruling 5e): a symbolic link to a regular file is
// kind symbolic-link, never regular — [Provider.Resolve] is the explicit follow.
type ResourceKind int

const (
	// ResourceKindAny is the zero value and the default — permissive: any taxonomy kind is accepted.
	ResourceKindAny ResourceKind = 0

	// ResourceKindRegular asserts a regular file.
	ResourceKindRegular ResourceKind = 1

	// ResourceKindDirectory asserts a directory.
	ResourceKindDirectory ResourceKind = 2

	// ResourceKindSymbolicLink asserts a symbolic link — the link itself, lstat semantics.
	ResourceKindSymbolicLink ResourceKind = 3
)

// region EXPORTED METHODS

// MarshalJSON serializes the kind as its canonical lowercase string — a document carries "regular",
// never a bare ordinal (the typed-value rule: no value degrades to its least-typed rendering).
//
// Returns:
//   - `[]byte`: the JSON string form.
//   - `error`: any error from the underlying marshal.
func (k ResourceKind) MarshalJSON() ([]byte, error) { return json.Marshal(k.String()) }

// MarshalYAML serializes the kind as its canonical lowercase string scalar.
//
// Returns:
//   - `any`: the string form.
//   - `error`: always nil.
func (k ResourceKind) MarshalYAML() (any, error) { return k.String(), nil }

// String returns the canonical lowercase rendering of the kind.
//
// Returns:
//   - `string`: "any", "regular", "directory", or "symbolic_link".
func (k ResourceKind) String() string {

	switch k {
	case ResourceKindAny:
		return "any"
	case ResourceKindRegular:
		return "regular"
	case ResourceKindDirectory:
		return "directory"
	case ResourceKindSymbolicLink:
		return "symbolic_link"
	}

	assert.Unreachable(fmt.Sprintf("file.ResourceKind.String: invalid kind value %d", int(k)))
	return ""
}

// UnmarshalJSON deserializes the canonical string form.
//
// Parameters:
//   - `data`: the JSON bytes; must be a string.
//
// Returns:
//   - `error`: non-nil on a non-string value or an unknown kind.
func (k *ResourceKind) UnmarshalJSON(data []byte) error {

	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("file.ResourceKind: %w", err)
	}
	return k.parse(s)
}

// UnmarshalText deserializes the canonical string form — the seam [op.Convert]'s text-unmarshal step and
// the defaults vocabulary use, so an authored `kind="regular"` (and the `kind=entry` default) land as
// typed values.
//
// Parameters:
//   - `text`: the canonical lowercase form.
//
// Returns:
//   - `error`: non-nil on an unknown kind.
func (k *ResourceKind) UnmarshalText(text []byte) error { return k.parse(string(text)) }

// UnmarshalYAML deserializes the canonical string scalar form.
//
// Parameters:
//   - `value`: the YAML node; must be a string scalar.
//
// Returns:
//   - `error`: non-nil on a non-scalar node or an unknown kind.
func (k *ResourceKind) UnmarshalYAML(value *yaml.Node) error {

	var s string
	if err := value.Decode(&s); err != nil {
		return fmt.Errorf("file.ResourceKind: %w", err)
	}
	return k.parse(s)
}

// endregion

// region PRIVATE METHODS

// admits reports whether the lstat-observed `mode` satisfies this kind assertion.
//
// Kinds are lstat-strict: the symlink bit decides symbolic-link before the type bits are consulted, so a
// link to a regular file never admits as regular. The permissive `entry` admits every taxonomy kind —
// and only taxonomy kinds; a FIFO, socket, or device admits nothing (the taxonomy has no variant).
//
// Parameters:
//   - `mode`: the lstat-observed [os.FileMode].
//
// Returns:
//   - `bool`: true when the observed kind satisfies the assertion.
func (k ResourceKind) admits(mode os.FileMode) bool {

	switch k {
	case ResourceKindAny:
		return mode&os.ModeSymlink != 0 || mode.IsDir() || mode.IsRegular()
	case ResourceKindRegular:
		return mode.IsRegular()
	case ResourceKindDirectory:
		return mode.IsDir()
	case ResourceKindSymbolicLink:
		return mode&os.ModeSymlink != 0
	}

	return false
}

// parse assigns the kind from its canonical string form.
//
// Parameters:
//   - `s`: the candidate string.
//
// Returns:
//   - `error`: non-nil when `s` is not a canonical kind.
func (k *ResourceKind) parse(s string) error {

	switch s {
	case "any":
		*k = ResourceKindAny
	case "regular":
		*k = ResourceKindRegular
	case "directory":
		*k = ResourceKindDirectory
	case "symbolic_link":
		*k = ResourceKindSymbolicLink
	default:
		return fmt.Errorf("file.ResourceKind: unknown kind %q (want any, regular, directory, or symbolic_link)", s)
	}

	return nil
}

// endregion
