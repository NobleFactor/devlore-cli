// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package appnet

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Resource represents a network resource identified by a URL.
//
// The SourceURL holds the original URL as provided (with transport scheme, original casing, etc.). The canonical URI
// produced by [URI] is an opaque appnet: URI wrapping the normalized, transport-independent URL with targeted escaping
// of # and ? characters.
//
// SourceURL is persisted in JSON/YAML alongside the URI so that cross-process round-trips preserve transport scheme
// (http vs. https vs. ftp) — information lost by the transport-independent URI. Scalar forms (text, starlark) carry
// only the SourceURL string; unmarshal reconstructs both the URI and SourceURL via [NewResource].
type Resource struct {
	op.ResourceBase

	// SourceURL is the canonicalized URL with its transport scheme preserved. Custom-marshaled as a string through
	// the alias-trick pattern in MarshalJSON / MarshalYAML; `json:"-"` suppresses default *url.URL serialization.
	SourceURL *url.URL `json:"-" yaml:"-"`
}

// NewResource constructs an appnet.Resource from a string URL.
//
// Parameters:
//   - value: expected to be a string URL
//
// Returns:
//   - Resource: initialized with the parsed URL
//   - error: if v is not a string or the URL is invalid
func NewResource(ctx *op.ExecutionContext, value any) (*Resource, error) {

	sourceURL, ok := value.(string)

	if !ok {
		return nil, fmt.Errorf("appnet.Resource: expected string URL, got %T", value)
	}

	canonicalSourceURL, err := canonicalURL(sourceURL)

	if err != nil {
		return nil, err
	}

	r := Resource{
		ResourceBase: op.NewResourceBase(ctx, "appnet:"+escapeInnerURI(transportIndependentURI(canonicalSourceURL))),
		SourceURL:    canonicalSourceURL}

	return &r, nil
}

// region EXPORTED METHODS

// region Behaviors

// Equal reports whether r and other identify the same appnet resource.
//
// Strict equality: other must be a *appnet.Resource (not merely an [op.Resource] with the same URI). Once the type
// check passes, URI comparison is delegated to [op.ResourceBase.Equal].
//
// Parameters:
//   - other: the value to compare against; may be any, including nil or a non-Resource.
//
// Returns:
//   - bool: true if other is a *appnet.Resource with the same URI as r.
func (r *Resource) Equal(other any) bool {

	if other == nil {
		return false
	}

	if _, ok := other.(*Resource); !ok {
		return false
	}

	return r.ResourceBase.Equal(other)
}

// MarshalJSON marshals the resource to its JSON wire form — identity (URI) plus SourceURL as a string.
//
// SourceURL is serialized via [url.URL.String] so JSON consumers round-trip the full URL including transport
// scheme (which the canonical URI discards).
//
// Returns:
//   - []byte: JSON-encoded object.
//   - error:  any error from [json.Marshal]; none under normal conditions.
func (r *Resource) MarshalJSON() ([]byte, error) {

	var sourceURL string
	if r.SourceURL != nil {
		sourceURL = r.SourceURL.String()
	}

	type alias Resource
	return json.Marshal(&struct {
		URI       string `json:"uri"`
		SourceURL string `json:"sourceURL,omitempty"`
		*alias
	}{
		URI:       r.URI(),
		SourceURL: sourceURL,
		alias:     (*alias)(r),
	})
}

// MarshalYAML marshals the resource to its YAML wire form — identity (URI) plus SourceURL as a string.
//
// Returns:
//   - any:   the value for the YAML encoder to serialize.
//   - error: nil under normal conditions.
func (r *Resource) MarshalYAML() (any, error) {

	var sourceURL string
	if r.SourceURL != nil {
		sourceURL = r.SourceURL.String()
	}

	type alias Resource
	return &struct {
		URI       string `yaml:"uri"`
		SourceURL string `yaml:"sourceURL,omitempty"`
		*alias
	}{
		URI:       r.URI(),
		SourceURL: sourceURL,
		alias:     (*alias)(r),
	}, nil
}

// Resolve is a no-op for appnet resources.
//
// Identity is the URL; the Resource has no on-disk state to reconcile. Defined to satisfy the [op.Resource]
// interface contract.
//
// Returns:
//   - error: always nil.
func (r *Resource) Resolve() error {
	return nil
}

// String returns a debug-oriented single-line representation of the resource suitable for log lines and IDE
// debug windows.
//
// Returns:
//   - string: "appnet.Resource{uri=<URI>, sourceURL=<URL>}".
func (r *Resource) String() string {

	var sourceURL string
	if r.SourceURL != nil {
		sourceURL = r.SourceURL.String()
	}

	return fmt.Sprintf("appnet.Resource{uri=%s, sourceURL=%s}", r.URI(), sourceURL)
}

// UnmarshalJSON populates the receiver from its JSON wire form.
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.ExecutionContext] before
// invoking this method. The URL is read from the sourceURL field (authoritative — preserves transport scheme)
// and handed to [NewResource] to reconstruct both the URI and SourceURL.
//
// Parameters:
//   - data: JSON-encoded wire form.
//
// Returns:
//   - error: non-nil if the ExecutionContext is missing, the JSON does not decode, sourceURL is empty, or
//     resource construction fails.
func (r *Resource) UnmarshalJSON(data []byte) error {

	if r.ExecutionContext() == nil {
		return errors.New("appnet.Resource: UnmarshalJSON requires ExecutionContext on receiver")
	}

	type alias Resource
	aux := &struct {
		URI       string `json:"uri"`
		SourceURL string `json:"sourceURL"`
		*alias
	}{alias: (*alias)(r)}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	if aux.SourceURL == "" {
		return errors.New("appnet.Resource: UnmarshalJSON requires sourceURL field")
	}

	built, err := NewResource(r.ExecutionContext(), aux.SourceURL)
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalStarlark populates the receiver from a [starlark.String] containing a URL.
//
// Scalar form: the entire URL (with scheme) is passed through. Unmarshal reconstructs both the URI and
// SourceURL via [NewResource].
//
// Parameters:
//   - sv: a starlark value expected to be a [starlark.String].
//
// Returns:
//   - error: non-nil if the ExecutionContext is missing, the value is not a [starlark.String], or resource
//     construction fails.
func (r *Resource) UnmarshalStarlark(sv starlark.Value) error {

	if r.ExecutionContext() == nil {
		return errors.New("appnet.Resource: UnmarshalStarlark requires ExecutionContext on receiver")
	}

	s, ok := sv.(starlark.String)
	if !ok {
		return fmt.Errorf("appnet.Resource: expected starlark.String, got %s", sv.Type())
	}

	built, err := NewResource(r.ExecutionContext(), string(s))
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalText populates the receiver from raw UTF-8 bytes containing a URL.
//
// Scalar form: the entire URL (with scheme) is passed through. Unmarshal reconstructs both the URI and
// SourceURL via [NewResource].
//
// Parameters:
//   - text: UTF-8 bytes containing the URL.
//
// Returns:
//   - error: non-nil if the ExecutionContext is missing or resource construction fails.
func (r *Resource) UnmarshalText(text []byte) error {

	if r.ExecutionContext() == nil {
		return errors.New("appnet.Resource: UnmarshalText requires ExecutionContext on receiver")
	}

	built, err := NewResource(r.ExecutionContext(), string(text))
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalYAML populates the receiver from its YAML wire form.
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.ExecutionContext] before
// invoking this method. The URL is read from the sourceURL field (authoritative — preserves transport scheme)
// and handed to [NewResource] to reconstruct both the URI and SourceURL.
//
// Parameters:
//   - unmarshal: callback supplied by the YAML decoder that projects the current node into the given target.
//
// Returns:
//   - error: non-nil if the ExecutionContext is missing, the YAML does not decode, sourceURL is empty, or
//     resource construction fails.
func (r *Resource) UnmarshalYAML(unmarshal func(any) error) error {

	if r.ExecutionContext() == nil {
		return errors.New("appnet.Resource: UnmarshalYAML requires ExecutionContext on receiver")
	}

	type alias Resource
	aux := &struct {
		URI       string `yaml:"uri"`
		SourceURL string `yaml:"sourceURL"`
		*alias
	}{alias: (*alias)(r)}

	if err := unmarshal(aux); err != nil {
		return err
	}

	if aux.SourceURL == "" {
		return errors.New("appnet.Resource: UnmarshalYAML requires sourceURL field")
	}

	built, err := NewResource(r.ExecutionContext(), aux.SourceURL)
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// endregion

// endregion

// escapeInnerURI percent-encodes # and ? so they don't interfere with
// the outer URI's fragment and query parsing.
func escapeInnerURI(s string) string {
	var b []byte
	for i := range len(s) {
		switch s[i] {
		case '#':
			b = append(b, '%', '2', '3')
		case '?':
			b = append(b, '%', '3', 'F')
		default:
			b = append(b, s[i])
		}
	}
	return string(b)
}

// transportIndependentURI produces host+path+query without the transport scheme.
func transportIndependentURI(u *url.URL) string {
	s := u.Host + u.EscapedPath()
	if u.RawQuery != "" {
		s += "?" + u.RawQuery
	}
	return s
}

// canonicalURL produces the canonical host+path+query string.
//
// Normalization rules (RFC 3986):
//   - Lowercase hostname
//   - Strip default ports (80 for http, 443 for https, 21 for ftp)
//   - Normalize percent-encoding: uppercase hex digits, decode unreserved chars
//   - Strip trailing /
//   - Collapse double // in path (except leading)
//   - Sort query parameters by key
func canonicalURL(value string) (*url.URL, error) {

	sourceURL, err := url.Parse(value)

	if err != nil {
		return nil, fmt.Errorf("appnet.Resource: invalid URL %q: %w", value, err)
	}

	// Host: lowercase, strip default port

	host := strings.ToLower(sourceURL.Hostname())
	port := sourceURL.Port()

	if port != "" && !isDefaultPort(sourceURL.Scheme, port) {
		host += ":" + port
	}

	// Path: normalize encoding, collapse slashes, strip trailing /

	p := normalizePercentEncoding(sourceURL.EscapedPath())
	p = collapseSlashes(p)
	p = strings.TrimRight(p, "/")

	if p == "" {
		p = "/"
	}

	// Query: sorted by key (url.Values.Encode sorts alphabetically)

	q := sourceURL.Query().Encode()

	sourceURL.Host = host
	sourceURL.RawPath = p
	sourceURL.Path, _ = url.PathUnescape(p)
	sourceURL.RawQuery = q

	return sourceURL, nil
}

// isDefaultPort returns true if port matches the well-known default for scheme.
func isDefaultPort(scheme, port string) bool {
	defaults := map[string]string{
		"http":  "80",
		"https": "443",
		"ftp":   "21",
	}
	return defaults[scheme] == port
}

// collapseSlashes replaces runs of multiple / with a single / in the path, preserving the leading /.
func collapseSlashes(p string) string {
	if p == "" {
		return p
	}
	// Preserve leading /
	lead := ""
	rest := p
	if strings.HasPrefix(p, "/") {
		lead = "/"
		rest = strings.TrimLeft(p, "/")
	}
	if rest == "" {
		return lead
	}
	parts := strings.FieldsFunc(rest, func(r rune) bool { return r == '/' })
	return lead + strings.Join(parts, "/")
}

// normalizePercentEncoding uppercases hex digits in percent-encoded sequences and decodes unreserved characters (RFC
// 3986 §2.3: A-Z a-z 0-9 - . _ ~).
func normalizePercentEncoding(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			hi := upperHex(s[i+1])
			lo := upperHex(s[i+2])
			decoded := unhex(hi)<<4 | unhex(lo)
			if isUnreserved(decoded) {
				b.WriteByte(decoded)
			} else {
				b.WriteByte('%')
				b.WriteByte(hi)
				b.WriteByte(lo)
			}
			i += 2
		} else {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// isUnreserved returns true for RFC 3986 §2.3 unreserved characters.
func isUnreserved(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') || c == '-' || c == '.' || c == '_' || c == '~'
}

// upperHex converts a hex digit to uppercase.
func upperHex(c byte) byte {
	if c >= 'a' && c <= 'f' {
		return c - ('a' - 'A')
	}
	return c
}

// unhex converts a hex digit to its numeric value.
func unhex(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	default:
		return 0
	}
}
