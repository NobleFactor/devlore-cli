// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

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

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
)

// region EXPORTED TYPES

// Format names a rendering the codec produces.
//
// The stream form ([Write]) takes it explicitly — with no file name there is nothing to infer from, and the
// output syntax is not an option but a decision (the same rule op.SaveGraph follows: format is stated). The
// path form ([WriteFile]) infers it from the file extension as a convenience at the file boundary only.
type Format string

const (

	// JSON renders the document as indented JSON.
	JSON Format = "json"

	// YAML renders the document as YAML.
	YAML Format = "yaml"
)

// Option configures write behavior.
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

// Write serializes v to a stream in the given [Format] — the codec alone, owning no file creation.
//
// The seam the write side was missing (#558): [Read] separates codec from I/O and Write now mirrors it, so
// creation concerns (permissions, directories) stay with whoever owns the destination. [WithPerm] is
// meaningless here and is ignored; it belongs to [WriteFile].
//
// Parameters:
//   - `w`: the stream to write the rendered document to.
//   - `format`: the [Format] to render; [JSON] or [YAML].
//   - `v`: the value to serialize.
//   - `opts`: optional configuration ([WithIndent], [WithHeader]).
//
// Returns:
//   - `error`: an unknown format, a marshal error, or a stream write error.
func Write(w io.Writer, format Format, v any, opts ...Option) error {

	cfg := defaultWriteOpts()
	for _, o := range opts {
		o(&cfg)
	}

	data, err := encode(format, v, &cfg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write: %w", err)
	}

	return nil
}

// WriteFile serializes v to disk as a structured document, through the caller's root. Format is inferred from
// the file extension. Creates parent directories (0o750) if needed. Default file permission is 0o600; override
// with WithPerm.
//
// The root is received, never constructed (#558; #405 phase 3): file creation belongs to whoever owns the
// destination tree, and writing through [fsroot.Dir] is what makes a restrictive permission enforceable on
// Windows, where the mode bits alone protect nothing.
//
// Parameters:
//   - `dir`: the tree the document belongs to, opened by the caller.
//   - `p`: the document's path within `dir`.
//   - `v`: the value to serialize.
//   - `opts`: optional configuration ([WithPerm], [WithIndent], [WithHeader]).
//
// Returns:
//   - `error`: wraps marshal, directory creation, and write errors with the document's path for context.
func WriteFile(dir fsroot.Dir, p fsroot.Path, v any, opts ...Option) error {

	cfg := defaultWriteOpts()
	for _, o := range opts {
		o(&cfg)
	}

	data, err := encode(formatFromExt(p.Rel()), v, &cfg)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", p.Abs(), err)
	}

	if parent := filepath.Dir(p.Rel()); parent != "." {
		if err := dir.MkdirAll(dir.NewPath(parent), 0o750); err != nil {
			return fmt.Errorf("create directory %s: %w", parent, err)
		}
	}

	if err := dir.WriteFile(p, data, cfg.perm); err != nil {
		return fmt.Errorf("write %s: %w", p.Abs(), err)
	}

	return nil
}

// WithPerm overrides the default 0o600 file permission. A creation concern: it applies to [WriteFile] only
// and is ignored by the stream form, which owns no file.
//
// Parameters:
//   - `mode`: the file permission mode to use.
//
// Returns:
//   - `Option`: a write option that sets the file permission.
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

// defaultWriteOpts returns the write configuration before options apply.
//
// Returns:
//   - `writeOpts`: the defaults — 0o600 permission, two-space JSON indent, no prefix, no header.
func defaultWriteOpts() writeOpts {

	return writeOpts{
		perm:         0o600,
		jsonPrefix:   "",
		jsonIndent:   "  ",
		indentCustom: false,
	}
}

// encode renders v in the given format, with the configured header prepended.
//
// Parameters:
//   - `format`: the [Format] to render.
//   - `v`: the value to serialize.
//   - `cfg`: write options controlling indentation and the header.
//
// Returns:
//   - `[]byte`: the rendered document.
//   - `error`: an unknown format, or a marshal error from the underlying codec.
func encode(format Format, v any, cfg *writeOpts) ([]byte, error) {

	data, err := marshal(format, v, cfg)
	if err != nil {
		return nil, err
	}

	if cfg.header != "" {
		h := cfg.header
		if !strings.HasSuffix(h, "\n") {
			h += "\n"
		}
		data = append([]byte(h), data...)
	}

	return data, nil
}

// formatFromExt returns the [Format] a file extension names.
//
// Parameters:
//   - `path`: filesystem path whose extension determines the format.
//
// Returns:
//   - `Format`: [JSON] for .json files, [YAML] for everything else.
func formatFromExt(path string) Format {

	if strings.ToLower(filepath.Ext(path)) == ".json" {
		return JSON
	}
	return YAML
}

// marshal serializes v in the given format.
//
// Parameters:
//   - `format`: the [Format] to render.
//   - `v`: the value to serialize.
//   - `cfg`: write options controlling indentation.
//
// Returns:
//   - `[]byte`: serialized content.
//   - `error`: an unknown format, or a marshal error from the underlying codec.
func marshal(format Format, v any, cfg *writeOpts) ([]byte, error) {

	switch format {

	case JSON:
		data, err := json.MarshalIndent(v, cfg.jsonPrefix, cfg.jsonIndent)
		if err != nil {
			return nil, err
		}
		return append(data, '\n'), nil

	case YAML:
		return yaml.Marshal(v)

	default:
		return nil, fmt.Errorf("document: unknown format %q", format)
	}
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
