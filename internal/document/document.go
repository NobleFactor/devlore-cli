// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package document provides structured document I/O for YAML and JSON files. It encapsulates the read-deserialize and
// serialize-write patterns used throughout the codebase, with consistent error wrapping, permission modes, directory
// creation, and format detection.
package document

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// region EXPORTED TYPES

// Option configures Write behavior.
type Option func(*writeOpts)

// endregion

// region EXPORTED FUNCTIONS

// region Behaviors

// Fallible actions

// Read deserializes a structured document from a reader. YAML decoding is used unconditionally since JSON is a valid
// subset of YAML.
//
// Type Parameters:
//   - T: the target type for deserialization
//
// Parameters:
//   - r: the reader to read from
//
// Returns:
//   - *T: pointer to the deserialized value
//   - error: wraps read and parse errors
func Read[T any](r io.Reader) (*T, error) {

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	var v T
	if err := yaml.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	return &v, nil
}

// ReadFile deserializes a structured document from disk. Format is inferred from the file extension: .json → JSON,
// .yaml/.yml/anything else → YAML.
//
// Type Parameters:
//   - T: the target type for deserialization
//
// Parameters:
//   - path: filesystem path to the document
//
// Returns:
//   - *T: pointer to the deserialized value
//   - error: wraps both I/O and parse errors with the file path for context
func ReadFile[T any](path string) (*T, error) {

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var v T
	if err := unmarshal(path, data, &v); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	return &v, nil
}

// Write serializes v to disk as a structured document. Format is inferred from the file extension. Creates parent
// directories (0o750) if needed. Default file permission is 0o600; override with WithPerm.
//
// Parameters:
//   - path: filesystem path for the output document
//   - v: value to serialize
//   - opts: optional configuration (WithPerm, WithIndent, WithHeader)
//
// Returns:
//   - error: wraps marshal, directory creation, and write errors with the file path for context
func Write(path string, v any, opts ...Option) error {

	cfg := writeOpts{
		perm:         0o600,
		jsonPrefix:   "",
		jsonIndent:   "  ",
		indentCustom: false,
	}
	for _, o := range opts {
		o(&cfg)
	}

	data, err := marshal(path, v, &cfg)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}

	if cfg.header != "" {
		h := cfg.header
		if !strings.HasSuffix(h, "\n") {
			h += "\n"
		}
		data = append([]byte(h), data...)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	if err := os.WriteFile(path, data, cfg.perm); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	return nil
}

// WithPerm overrides the default 0o600 file permission.
//
// Parameters:
//   - mode: the file permission mode to use
//
// Returns:
//   - Option: a write option that sets the file permission
func WithPerm(mode os.FileMode) Option {

	return func(o *writeOpts) {
		o.perm = mode
	}
}

// WithIndent controls JSON indentation. Ignored for YAML. Default is 2-space indent with no prefix.
//
// Parameters:
//   - prefix: prefix string prepended to each line (typically empty)
//   - indent: indent string used for each level of nesting
//
// Returns:
//   - Option: a write option that sets JSON indentation
func WithIndent(prefix, indent string) Option {

	return func(o *writeOpts) {
		o.jsonPrefix = prefix
		o.jsonIndent = indent
		o.indentCustom = true
	}
}

// WithHeader prepends a literal string before the serialized content. A trailing newline is appended if not present.
//
// Parameters:
//   - header: text to prepend (e.g., a generated-file comment or disclaimer)
//
// Returns:
//   - Option: a write option that sets the header
func WithHeader(header string) Option {

	return func(o *writeOpts) {
		o.header = header
	}
}

// endregion

// endregion

// region UNEXPORTED TYPES

// writeOpts holds configuration for Write.
type writeOpts struct {
	perm         os.FileMode // file permission mode (default: 0o600)
	jsonPrefix   string      // JSON indent prefix (default: "")
	jsonIndent   string      // JSON indent string (default: "  ")
	indentCustom bool        // true when WithIndent was called explicitly
	header       string      // literal text prepended before serialized content
}

// endregion

// region HELPER FUNCTIONS

// region Behaviors

// formatFromExt returns "json" or "yaml" based on the file extension.
//
// Parameters:
//   - path: filesystem path whose extension determines the format
//
// Returns:
//   - string: "json" for .json files, "yaml" for everything else
func formatFromExt(path string) string {

	if strings.ToLower(filepath.Ext(path)) == ".json" {
		return "json"
	}
	return "yaml"
}

// marshal serializes v according to the format inferred from the file extension.
//
// Parameters:
//   - path: filesystem path whose extension determines the format
//   - v: value to serialize
//   - cfg: write options controlling indentation
//
// Returns:
//   - []byte: serialized content
//   - error: marshal error from the underlying codec
func marshal(path string, v any, cfg *writeOpts) ([]byte, error) {

	if formatFromExt(path) == "json" {
		data, err := json.MarshalIndent(v, cfg.jsonPrefix, cfg.jsonIndent)
		if err != nil {
			return nil, err
		}
		return append(data, '\n'), nil
	}

	return yaml.Marshal(v)
}

// unmarshal deserializes data into v according to the format inferred from the file extension.
//
// Parameters:
//   - path: filesystem path whose extension determines the format
//   - data: raw file content
//   - v: pointer to the target value for deserialization
//
// Returns:
//   - error: unmarshal error from the underlying codec
func unmarshal(path string, data []byte, v any) error {

	if formatFromExt(path) == "json" {
		return json.Unmarshal(data, v)
	}
	return yaml.Unmarshal(data, v)
}

// endregion

// endregion
