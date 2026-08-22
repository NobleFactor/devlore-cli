// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// MissingResourcePolicy is a consumer's declared response to a missing resource (ruled 2026-08-22;
// docs/architecture/4-resource-management.md §3, the claims taxonomy).
//
// The parameter's TYPE is the declaration — no directive: a method with a MissingResourcePolicy-typed
// parameter and exactly one consumed (resource-typed) parameter links the two at announcement. A warning
// is produced whenever a missing resource is detected, under every policy. Aggregation across the
// consumers of one entry: Stop wins. A Skip variant ("do not dispatch") was considered and DROPPED (ruled
// 2026-08-22): its undo story is trivially clean — nothing ran, nothing to undo — but its forward side
// (nil-valued promises to downstream consumers; a trace that cannot tell "skipped" from "ran and produced
// nothing") buys machinery that Ignore never needs. Re-adding it later is purely additive.
type MissingResourcePolicy int

const (
	// MissingResourcePolicyStop is the zero value and the default — fail-safe: a missing resource fails
	// the consuming scope as unmet intent. An unset policy can never accidentally tolerate.
	MissingResourcePolicyStop MissingResourcePolicy = 0

	// MissingResourcePolicyIgnore makes the call anyway: the provider sees the absence and handles it
	// (a remove no-ops), and the receipt records that the target was already absent.
	MissingResourcePolicyIgnore MissingResourcePolicy = 1
)

// String returns the canonical lowercase rendering of the policy.
//
// Returns:
//   - `string`: "stop" or "ignore".
func (p MissingResourcePolicy) String() string {

	switch p {
	case MissingResourcePolicyStop:
		return "stop"
	case MissingResourcePolicyIgnore:
		return "ignore"
	}

	assert.Unreachable(fmt.Sprintf("op.MissingResourcePolicy.String: invalid policy value %d", int(p)))
	return ""
}

// MarshalJSON serializes the policy as its canonical lowercase string — a document carries "stop", never a
// bare ordinal (the typed-value rule: no value degrades to its least-typed rendering in an artifact).
//
// Returns:
//   - `[]byte`: the JSON string form.
//   - `error`: any error from the underlying marshal.
func (p MissingResourcePolicy) MarshalJSON() ([]byte, error) { return json.Marshal(p.String()) }

// MarshalYAML serializes the policy as its canonical lowercase string, mirroring
// [MissingResourcePolicy.MarshalJSON].
//
// Returns:
//   - `any`: the string form for the YAML encoder.
//   - `error`: always nil; present to satisfy the [yaml.Marshaler] interface.
func (p MissingResourcePolicy) MarshalYAML() (any, error) { return p.String(), nil }

// UnmarshalJSON deserializes the canonical string form.
//
// Parameters:
//   - `data`: the JSON bytes to decode.
//
// Returns:
//   - `error`: a malformed JSON string, or an unknown policy name.
func (p *MissingResourcePolicy) UnmarshalJSON(data []byte) error {

	var name string
	if err := json.Unmarshal(data, &name); err != nil {
		return err
	}

	return p.parse(name)
}

// UnmarshalYAML deserializes the canonical string form, mirroring [MissingResourcePolicy.UnmarshalJSON].
//
// Parameters:
//   - `value`: the YAML node to decode.
//
// Returns:
//   - `error`: a malformed YAML scalar, or an unknown policy name.
func (p *MissingResourcePolicy) UnmarshalYAML(value *yaml.Node) error {

	var name string
	if err := value.Decode(&name); err != nil {
		return err
	}

	return p.parse(name)
}

// UnmarshalText deserializes the canonical string form — the seam [Convert]'s text-unmarshal step uses to
// turn an authored "skip" into the typed policy at slot fill.
//
// Parameters:
//   - `text`: the policy name as UTF-8 bytes.
//
// Returns:
//   - `error`: non-nil for an unknown name.
func (p *MissingResourcePolicy) UnmarshalText(text []byte) error { return p.parse(string(text)) }

// parse assigns the policy named by `name`.
//
// Parameters:
//   - `name`: the canonical lowercase policy name.
//
// Returns:
//   - `error`: non-nil for an unknown name.
func (p *MissingResourcePolicy) parse(name string) error {

	switch name {
	case "stop":
		*p = MissingResourcePolicyStop
	case "ignore":
		*p = MissingResourcePolicyIgnore
	default:
		return fmt.Errorf("op.MissingResourcePolicy: unknown policy %q", name)
	}

	return nil
}
