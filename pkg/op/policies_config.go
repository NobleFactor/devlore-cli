// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"

	"github.com/NobleFactor/devlore-cli/pkg/devconfig"
)

var _ devconfig.Section = (*PoliciesConfig)(nil) // Interface Guard.

func init() {
	devconfig.AnnounceSection(reflect.TypeFor[PoliciesConfig](), func() devconfig.Section {
		return NewPoliciesConfig()
	})
}

// PoliciesConfig is the op-owned "policies" section — the home of every executor-enforced run policy.
//
// It follows the [RuntimeEnvironmentConfig] precedent: announced at init() with its builtin floor and read live from
// [application.Application.Config] (the floor now, enriched with file / env / cli once the loader resolves those
// sources). It carries the two policies the graph executor consults: [PoliciesConfig.Retry] — the default
// [RetryPolicy] for subgraph combinators (step 35's tri-state) — and [PoliciesConfig.Transition] — the
// [TransitionPolicy] that decides continue / pause / stop on each aberrant condition flip.
type PoliciesConfig struct {
	devconfig.SectionBase

	// Retry is the DEFAULT retry policy for subgraph combinators (the step-35 tri-state resolves an unset unit policy
	// here for subgraphs; every other unit resolves to none).
	Retry RetryPolicy `json:"retry" yaml:"retry"`

	// Transition is the reaction policy consulted at each aberrant [Condition] flip.
	Transition TransitionPolicy `json:"transition" yaml:"transition"`
}

// NewPoliciesConfig returns the policies section at its builtin floor.
//
// Floor: [TransitionPolicy] degraded → continue, failed_execution → stop, failed_compensation → stop (the
// unattended-execution baseline — stop delivers the consistent pre-run state); [RetryPolicy] zero-value (no retry —
// the subgraph-combinator default is layered by step 35's resolution, not by the floor).
//
// Returns:
//   - `*PoliciesConfig`: the policies section at its builtin floor.
func NewPoliciesConfig() *PoliciesConfig {

	return &PoliciesConfig{
		SectionBase: devconfig.NewSectionBase("policies"),
		Transition: TransitionPolicy{
			Degraded:           ReactionContinue,
			ExecutionFailed:    ReactionStop,
			CompensationFailed: ReactionStop,
		},
	}
}

// PoliciesFrom fetches the [*PoliciesConfig] section from a resolved [devconfig.Config].
//
// The typed wrapper over [devconfig.SectionOf] the design prescribes so consumers never type-assert by hand.
// [PoliciesConfig] is announced at init(), so a config that snapshotted the registry carries it.
//
// Parameters:
//   - `config`: the resolved configuration container.
//
// Returns:
//   - `*PoliciesConfig`: the policies section, or nil when absent.
//   - `bool`: true when the section was present.
func PoliciesFrom(config *devconfig.Config) (*PoliciesConfig, bool) {
	return devconfig.SectionOf[*PoliciesConfig](config)
}

// Validate reports whether the policies section is internally consistent.
//
// Delegates to [TransitionPolicy.Validate] — the loader calls each section's Validate() as it walks the resolved
// tree. The sole invariant today: `continue` is illegal for `failed_compensation`.
//
// Returns:
//   - `error`: non-nil when the transition policy is invalid.
func (c *PoliciesConfig) Validate() error {
	return c.Transition.Validate()
}

// region SUPPORTING TYPES

// TransitionPolicy maps each aberrant [Condition] to the [Reaction] the executor takes when the run's condition
// flips to it.
//
// The floor (from [NewPoliciesConfig]) is degraded → continue, failed_execution → stop, failed_compensation → stop:
// the author chose to degrade, so degradation continues; a failure stops at its saga boundary with the consistent
// pre-run state. Pause is the attended-mode override for the two failure conditions, layered in via profile / app
// config. `continue` is never legal for `failed_compensation` — you cannot walk on past a dirty unwind —
// [TransitionPolicy.Validate] enforces it.
type TransitionPolicy struct {

	// Degraded is the reaction when the run's condition flips to [ConditionDegraded]. Floor: [ReactionContinue].
	Degraded Reaction `json:"degraded" yaml:"degraded"`

	// ExecutionFailed is the reaction when the run's condition flips to [ConditionExecutionFailed]. Floor:
	// [ReactionStop]. The Go field reads subject-verb; the serialized key keeps the settled `failed_execution` form.
	ExecutionFailed Reaction `json:"failed_execution" yaml:"failed_execution"`

	// CompensationFailed is the reaction when the run's condition flips to [ConditionCompensationFailed]. Floor:
	// [ReactionStop]; [ReactionContinue] is rejected by [TransitionPolicy.Validate].
	CompensationFailed Reaction `json:"failed_compensation" yaml:"failed_compensation"`
}

// Reaction is a [TransitionPolicy]'s response to an aberrant [Condition] flip: continue, pause, or stop.
//
// Serialized over [reactionNames] in both document formats — [Reaction.MarshalText] for JSON, [Reaction.MarshalYAML]
// for gopkg.in/yaml.v3, which does not honor [encoding.TextMarshaler].
type Reaction int

const (

	// ReactionContinue keeps the run walking in its (now aberrant) condition; the zero value.
	ReactionContinue Reaction = iota

	// ReactionPause parks the whole run, resumable — the attended-mode override that preserves the failure scene.
	ReactionPause

	// ReactionStop unwinds the boundary's stack and lands it stopped × condition, returning to the parent for
	// bubble-up adjudication.
	ReactionStop
)

// reactionNames maps each [Reaction] to its serialized name.
var reactionNames = [...]string{
	ReactionContinue: "continue",
	ReactionPause:    "pause",
	ReactionStop:     "stop",
}

// endregion

// region EXPORTED METHODS

// region Behaviors

// ReactionFor reports the configured [Reaction] for a condition flip.
//
// The healthy condition is not aberrant and never flips through a policy consultation, so it maps to
// [ReactionContinue] alongside any unrecognized value.
//
// Parameters:
//   - `condition`: the [Condition] the run flipped to.
//
// Returns:
//   - `Reaction`: the reaction configured for `condition`.
func (p TransitionPolicy) ReactionFor(condition Condition) Reaction {

	switch condition {
	case ConditionDegraded:
		return p.Degraded
	case ConditionExecutionFailed:
		return p.ExecutionFailed
	case ConditionCompensationFailed:
		return p.CompensationFailed
	default:
		return ReactionContinue
	}
}

// Validate reports whether the policy is internally consistent.
//
// The sole invariant: `continue` is never legal for `failed_compensation` — walking on past a dirty unwind is
// forbidden. Pause and stop are both legal there (pause holds the dirty residue for inspection after the best-effort
// unwind completes).
//
// Returns:
//   - `error`: non-nil when [TransitionPolicy.CompensationFailed] is [ReactionContinue].
func (p TransitionPolicy) Validate() error {

	if p.CompensationFailed == ReactionContinue {
		return fmt.Errorf("op.TransitionPolicy: failed_compensation may not be 'continue' — a dirty unwind cannot be walked past")
	}

	return nil
}

// MarshalText encodes this reaction as its serialized name.
//
// Satisfies [encoding.TextMarshaler], so JSON documents carry "continue" / "pause" / "stop" rather than a bare
// integer.
//
// Returns:
//   - `[]byte`: the name from [reactionNames].
//   - `error`: non-nil when the value is out of range.
func (r Reaction) MarshalText() ([]byte, error) {

	if int(r) < 0 || int(r) >= len(reactionNames) {
		return nil, fmt.Errorf("op.Reaction: unknown value %d", int(r))
	}

	return []byte(reactionNames[r]), nil
}

// MarshalYAML encodes this reaction as its serialized name.
//
// gopkg.in/yaml.v3 does not honor [encoding.TextMarshaler], so YAML documents need this companion to carry
// "continue" / "pause" / "stop" rather than a bare integer.
//
// Returns:
//   - `any`: the name from [reactionNames], as a string.
//   - `error`: non-nil when the value is out of range.
func (r Reaction) MarshalYAML() (any, error) {

	text, err := r.MarshalText()
	if err != nil {
		return nil, err
	}

	return string(text), nil
}

// String returns this reaction's serialized name.
//
// Returns:
//   - `string`: the name from [reactionNames], or "Reaction(<n>)" for an out-of-range value.
func (r Reaction) String() string {

	if int(r) >= 0 && int(r) < len(reactionNames) {
		return reactionNames[r]
	}

	return fmt.Sprintf("Reaction(%d)", int(r))
}

// UnmarshalText decodes a reaction from its serialized name.
//
// Satisfies [encoding.TextUnmarshaler] for JSON documents.
//
// Parameters:
//   - `text`: one of the [reactionNames] entries.
//
// Returns:
//   - `error`: non-nil when `text` names no reaction.
func (r *Reaction) UnmarshalText(text []byte) error {

	name := string(text)

	for value, candidate := range reactionNames {
		if candidate == name {
			*r = Reaction(value)
			return nil
		}
	}

	return fmt.Errorf("op.Reaction: unknown name %q", name)
}

// UnmarshalYAML decodes a reaction from its serialized name.
//
// gopkg.in/yaml.v3 does not honor [encoding.TextUnmarshaler], so YAML documents need this companion.
//
// Parameters:
//   - `unmarshal`: the YAML node decoder supplied by the yaml package.
//
// Returns:
//   - `error`: non-nil when the node is not a string or names no reaction.
func (r *Reaction) UnmarshalYAML(unmarshal func(any) error) error {

	var name string
	if err := unmarshal(&name); err != nil {
		return err
	}

	return r.UnmarshalText([]byte(name))
}

// endregion

// endregion
