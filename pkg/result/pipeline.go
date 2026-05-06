// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package result owns the primary output channel — the structured-data stream the framework emits
// to the user (or to downstream tooling, when piped). Distinct from the side channel ([pkg/status])
// that carries categorized narration.
//
// The pipeline shape is `value → result.Filter → result.Formatter → sink.Sink`. [NewPipeline]
// composes the three stages; callers Emit values through it. Three formatters ship today
// ([JSONFormatter], [YAMLFormatter], [CSVFormatter]) plus [TemplateFormatter] for caller-supplied
// Go templates. Three filters ship: [NoOpFilter] pass-through, [FieldFilter] selection,
// [JQFilter] for jq expressions.
package result

import (
	"io"

	"github.com/NobleFactor/devlore-cli/pkg/sink"
)

// Filter is the selection stage of the result pipeline. Implementations narrow, transform, or
// pass-through the value before it reaches the formatter.
type Filter interface {

	// Apply returns a (possibly modified) value, or an error if the filter expression is invalid
	// for the input shape.
	Apply(value any) (any, error)
}

// Formatter is the rendering stage of the result pipeline. Implementations encode the (filtered)
// value as bytes on the writer.
//
// The writer parameter is typed as [io.Writer] rather than [sink.Sink] because formatters operate
// on a byte stream — they don't need TTY-awareness, Close lifecycle, or any other Sink-specific
// concerns. [Pipeline.Emit] passes its [sink.Sink] (which is an [io.Writer]) to Format directly;
// stdlib encoders ([encoding/json], [gopkg.in/yaml.v3], etc.) plug in without adapter shims.
type Formatter interface {

	// Format renders value to w in the implementation's format (JSON, YAML, CSV, template, etc.).
	Format(value any, w io.Writer) error
}

// Pipeline composes a [Filter] and a [Formatter] against a [sink.Sink]. Callers Emit values; the
// Pipeline applies the filter, hands the result to the formatter, and the formatter writes through
// the sink.
//
// All fields are unexported and set at construction by [NewPipeline]; the value is immutable from
// the caller's perspective. To suppress all output, construct with [sink.Discard] as the sink.
type Pipeline struct {
	filter    Filter
	formatter Formatter
	sink      sink.Sink
}

// NewPipeline constructs an immutable [Pipeline] writing through the supplied sink.
//
// Parameters:
//   - filter:    the selection stage. Pass nil to use [NoOpFilter] pass-through.
//   - formatter: the rendering stage. Must not be nil.
//   - s:         the [sink.Sink] to write through. Must not be nil. Pass [sink.Discard] to suppress
//     all emissions; pass [sink.Stdout] for the standard cli case.
//
// Returns:
//   - *Pipeline: the constructed pipeline.
func NewPipeline(filter Filter, formatter Formatter, s sink.Sink) *Pipeline {

	if filter == nil {
		filter = NoOpFilter{}
	}

	return &Pipeline{
		filter:    filter,
		formatter: formatter,
		sink:      s,
	}
}

// region EXPORTED METHODS

// region Behaviors

// Fallible actions

// Emit runs value through the filter, then hands the filtered value to the formatter, which writes
// through the sink.
//
// Parameters:
//   - value: the value to emit.
//
// Returns:
//   - error: non-nil if the filter rejects value, or if the formatter fails to encode it.
func (p *Pipeline) Emit(value any) error {

	filtered, err := p.filter.Apply(value)
	if err != nil {
		return err
	}
	return p.formatter.Format(filtered, p.sink)
}

// endregion

// endregion

// region NoOpFilter

// NoOpFilter is the pass-through [Filter]; Apply returns its argument unchanged.
type NoOpFilter struct{}

// Compile-time interface guard.
var _ Filter = NoOpFilter{}

// Apply returns value unchanged.
//
// Parameters:
//   - value: the value to pass through.
//
// Returns:
//   - any:   value unchanged.
//   - error: always nil.
func (NoOpFilter) Apply(value any) (any, error) { return value, nil }

// endregion