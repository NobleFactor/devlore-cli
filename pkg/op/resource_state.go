// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// ResourceState is the lifecycle state of a catalog entry.
//
// Three states: Pending (initial — entry exists in the namespace but the underlying resource has not yet been
// observed or produced), Active (observation succeeded or the producer created the resource; metadata is
// populated), Gone (the resource is no longer there — an existence check failed, or a mutating consumer
// destroyed the resource and reported it; a later consumer of a Gone entry sees the state rather than
// rediscovering the loss).
//
// The state field is mutated by catalog code only; provider implementations have no setter. See
// docs/architecture/4-resource-management.md §3 (states + the behavior matrix) for the full lifecycle spec.
type ResourceState int

const (
	// Pending is the zero value; every new catalog entry is born here.
	Pending ResourceState = iota

	// Active means the resource has been observed (discovery path) or freshly created (production path).
	Active

	// Gone means the resource is no longer there: an existence check failed, or a mutating consumer
	// destroyed the resource and reported it. A later consumer sees Gone from the catalog rather than
	// rediscovering the loss.
	Gone
)

// String returns the canonical lowercase rendering of the state.
//
// Returns:
//   - `string`: "pending", "active", or "gone".
func (s ResourceState) String() string {

	switch s {
	case Pending:
		return "pending"
	case Active:
		return "active"
	case Gone:
		return "gone"
	}

	assert.Unreachable(fmt.Sprintf("op.ResourceState.String: invalid state value %d", int(s)))
	return ""
}

// MarshalJSON serializes the state as its canonical lowercase string — a document carries "pending", never a
// bare ordinal (the typed-value rule: no value degrades to its least-typed rendering in an artifact).
//
// Returns:
//   - `[]byte`: the JSON string form.
//   - `error`: any error from the underlying marshal.
func (s ResourceState) MarshalJSON() ([]byte, error) { return json.Marshal(s.String()) }

// MarshalYAML serializes the state as its canonical lowercase string, mirroring [ResourceState.MarshalJSON].
//
// Returns:
//   - `any`: the string form for the YAML encoder.
//   - `error`: always nil; present to satisfy the [yaml.Marshaler] interface.
func (s ResourceState) MarshalYAML() (any, error) { return s.String(), nil }

// UnmarshalJSON deserializes the canonical string form.
//
// Parameters:
//   - `data`: the JSON bytes to decode.
//
// Returns:
//   - `error`: a malformed JSON string, or an unknown state name.
func (s *ResourceState) UnmarshalJSON(data []byte) error {

	var name string
	if err := json.Unmarshal(data, &name); err != nil {
		return err
	}

	return s.parse(name)
}

// UnmarshalYAML deserializes the canonical string form, mirroring [ResourceState.UnmarshalJSON].
//
// Parameters:
//   - `value`: the YAML node to decode.
//
// Returns:
//   - `error`: a malformed YAML scalar, or an unknown state name.
func (s *ResourceState) UnmarshalYAML(value *yaml.Node) error {

	var name string
	if err := value.Decode(&name); err != nil {
		return err
	}

	return s.parse(name)
}

// parse assigns the state named by `name`.
//
// Parameters:
//   - `name`: the canonical lowercase state name.
//
// Returns:
//   - `error`: non-nil for an unknown name.
func (s *ResourceState) parse(name string) error {

	switch name {
	case "pending":
		*s = Pending
	case "active":
		*s = Active
	case "gone":
		*s = Gone
	default:
		return fmt.Errorf("op.ResourceState: unknown state %q", name)
	}

	return nil
}
