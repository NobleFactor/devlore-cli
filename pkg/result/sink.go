// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package result owns the primary output channel — the stdout-equivalent stream of structured data
// the framework emits to the user (or to downstream tooling, when piped). Distinct from the side
// channel ([pkg/status]) that carries categorized status messages.
//
// The pipeline shape is `any → structured document → result.Filter → result.Formatter → io.Writer`.
// [NewPipeline] composes the three stages into a [Sink] that callers Emit into. Three formatters
// ship in commit 1: [JSONFormatter] and [YAMLFormatter] (CSV and template formatters land in commit
// 2). One filter ships in commit 1: [NoOpFilter] pass-through (FieldFilter and JQFilter land in
// commit 2).
package result

import (
	"errors"
	"io"
)

// Sink is the terminal of the result pipeline. Every command's primary output goes through Emit.
//
// Implementations are typically built via [NewPipeline] which composes a [Filter] and a [Formatter]
// against a writer. Test impls can capture without filter/formatter; the loud-failure default
// [UnconfiguredSink] errors on every Emit so unconfigured environments don't silently swallow.
type Sink interface {

	// Emit projects the value through the sink's configured filter and formatter, writing the
	// result bytes to its underlying writer.
	Emit(value any) error
}

// Filter is the selection stage of the result pipeline. Implementations narrow, transform, or
// pass-through the value before it reaches the formatter.
type Filter interface {

	// Apply returns a (possibly modified) value, or an error if the filter expression is invalid
	// for the input shape.
	Apply(value any) (any, error)
}

// Formatter is the rendering stage of the result pipeline. Implementations encode the (filtered)
// value as bytes on the writer.
type Formatter interface {

	// Format renders value to w in the implementation's format (JSON, YAML, CSV, template, etc.).
	Format(value any, w io.Writer) error
}

// Pipeline composes a [Filter] and a [Formatter] against a writer to satisfy [Sink].
type Pipeline struct {
	filter    Filter
	formatter Formatter
	writer    io.Writer
}

// Compile-time interface guard.
var _ Sink = (*Pipeline)(nil)

// NewPipeline returns a [Pipeline] sink that applies filter then formatter on each Emit.
//
// Parameters:
//   - filter: the selection stage. Pass nil to use [NoOpFilter] pass-through.
//   - formatter: the rendering stage. Must not be nil.
//   - w: the destination writer.
//
// Returns:
//   - *Pipeline: the composed sink.
func NewPipeline(filter Filter, formatter Formatter, w io.Writer) *Pipeline {

	if filter == nil {
		filter = NoOpFilter{}
	}
	return &Pipeline{
		filter:    filter,
		formatter: formatter,
		writer:    w,
	}
}

// Emit runs value through filter then formatter, writing to the configured writer.
func (p *Pipeline) Emit(value any) error {

	filtered, err := p.filter.Apply(value)
	if err != nil {
		return err
	}
	return p.formatter.Format(filtered, p.writer)
}

// NoOpFilter is the pass-through [Filter]; Apply returns its argument unchanged.
type NoOpFilter struct{}

// Compile-time interface guard.
var _ Filter = NoOpFilter{}

// Apply returns value unchanged.
func (NoOpFilter) Apply(value any) (any, error) { return value, nil }

// UnconfiguredSink is the default [Sink] for runtime environment specs that have not had a real sink
// installed. Every Emit returns a loud error so unconfigured tools fail fast rather than silently
// swallowing emissions.
type UnconfiguredSink struct{}

// Compile-time interface guard.
var _ Sink = UnconfiguredSink{}

// Emit returns an error explaining that no result sink was configured.
func (UnconfiguredSink) Emit(_ any) error {
	return errors.New("result.Sink: unconfigured; set RuntimeEnvironmentSpec.Result before invoking")
}
