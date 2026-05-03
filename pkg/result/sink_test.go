// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package result

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// region NoOpFilter

func TestNoOpFilterPassesThrough(t *testing.T) {

	got, err := NoOpFilter{}.Apply("hello")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got != "hello" {
		t.Errorf("Apply returned %v, want hello", got)
	}
}

func TestNoOpFilterPreservesNil(t *testing.T) {

	got, err := NoOpFilter{}.Apply(nil)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got != nil {
		t.Errorf("Apply returned %v, want nil", got)
	}
}

// endregion

// region UnconfiguredSink

func TestUnconfiguredSinkErrorsLoudly(t *testing.T) {

	err := UnconfiguredSink{}.Emit("anything")
	if err == nil {
		t.Fatal("UnconfiguredSink.Emit returned nil error, want non-nil")
	}
	if !strings.Contains(err.Error(), "unconfigured") {
		t.Errorf("error text = %q, want substring %q", err.Error(), "unconfigured")
	}
}

// endregion

// region Pipeline composition

// stubFilter doubles every string by appending "!" — used to verify the filter stage runs before the
// formatter stage in Pipeline.Emit.
type stubFilter struct{}

func (stubFilter) Apply(value any) (any, error) {
	if s, ok := value.(string); ok {
		return s + "!", nil
	}
	return value, nil
}

// errFilter always returns an error — used to verify Pipeline propagates filter errors.
type errFilter struct{}

func (errFilter) Apply(_ any) (any, error) {
	return nil, errors.New("filter rejected")
}

// captureFormatter records the value handed to Format; the bytes written to w are the value's string
// form. Used to verify Pipeline calls the formatter after the filter and feeds the filtered value.
type captureFormatter struct {
	got any
}

func (cf *captureFormatter) Format(value any, w io.Writer) error {
	cf.got = value
	if s, ok := value.(string); ok {
		_, err := io.WriteString(w, s)
		return err
	}
	return nil
}

func TestPipelineFiltersThenFormats(t *testing.T) {

	var buf bytes.Buffer
	formatter := &captureFormatter{}

	pipeline := NewPipeline(stubFilter{}, formatter, &buf)

	if err := pipeline.Emit("hello"); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	if formatter.got != "hello!" {
		t.Errorf("formatter received %v, want hello!", formatter.got)
	}
	if buf.String() != "hello!" {
		t.Errorf("buffer = %q, want hello!", buf.String())
	}
}

func TestPipelineNilFilterDefaultsToNoOp(t *testing.T) {

	var buf bytes.Buffer
	formatter := &captureFormatter{}

	pipeline := NewPipeline(nil, formatter, &buf)

	if err := pipeline.Emit("hello"); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	if formatter.got != "hello" {
		t.Errorf("formatter received %v, want hello (NoOp filter pass-through)", formatter.got)
	}
}

func TestPipelinePropagatesFilterError(t *testing.T) {

	var buf bytes.Buffer
	formatter := &captureFormatter{}

	pipeline := NewPipeline(errFilter{}, formatter, &buf)

	err := pipeline.Emit("hello")

	if err == nil {
		t.Fatal("Emit returned nil error, want filter error")
	}
	if !strings.Contains(err.Error(), "filter rejected") {
		t.Errorf("error text = %q, want substring %q", err.Error(), "filter rejected")
	}
	if formatter.got != nil {
		t.Errorf("formatter ran despite filter error, got %v", formatter.got)
	}
}

// endregion
