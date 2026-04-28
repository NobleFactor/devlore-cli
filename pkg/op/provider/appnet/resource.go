// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package appnet

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Resource represents a network resource identified by a URL.
//
// The URI IS the canonical full URL — scheme, host, path, and query. The scheme is preserved because it's
// reachability-critical (http vs. https vs. ftp are distinct endpoints). Canonicalization normalizes host casing,
// strips default ports, normalizes percent-encoding, strips trailing slashes, collapses repeated slashes, and sorts
// query parameters — all semantics-preserving transforms over the given URL.
//
// Two URLs that differ only in the transport scheme (e.g., "http://x" vs. "https://x") produce distinct resources.
// This is a deliberate consequence of "identity ensures reachability." Consumers that want transport-independent
// addressing should factor that into their own logic; appnet.Resource does not.
//
// SourceURL is a non-persisted *url.URL view of the URI, populated at construction (and reparsed on unmarshal) for
// callers that want structured URL access. It always equals url.Parse(URI).
type Resource struct {
	op.ResourceBase

	// SourceURL is a parsed view of the URI. Derived from URI at construction and on unmarshal; not
	// persisted (the URI on ResourceBase is authoritative).
	SourceURL *url.URL `json:"-" yaml:"-"`
}

// NewResource constructs an appnet.Resource from a string URL.
//
// The URL is canonicalized (lowercase host, strip default port, normalize percent-encoding, strip trailing slash,
// collapse double slashes, sort query parameters) while preserving the transport scheme. The canonicalized URL becomes
// the Resource's URI; SourceURL is populated by reparsing it.
//
// Parameters:
//   - ctx:   execution context.
//   - value: a string URL with a transport scheme (http, https, ftp, ssh, etc.).
//
// Returns:
//   - *Resource: the constructed resource.
//   - error:     if value is not a string, does not parse as a URL, or has no scheme.
func NewResource(ctx *op.ExecutionContext, value any) (*Resource, error) {

	raw, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("appnet.Resource: expected string URL, got %T", value)
	}

	canonical, err := canonicalURL(raw)
	if err != nil {
		return nil, err
	}

	if canonical.Scheme == "" {
		return nil, fmt.Errorf("appnet.Resource: URL missing transport scheme: %q", raw)
	}

	base, err := op.NewResourceBase(ctx, canonical.String(), reflect.TypeFor[*Resource]())
	if err != nil {
		return nil, err
	}

	return &Resource{
		ResourceBase: base,
		SourceURL:    canonical,
	}, nil
}

// region EXPORTED METHODS

// region Behaviors

// Equal reports whether r and other identify the same appnet resource.
//
// Strict equality: other must be a *appnet.Resource (not merely an [op.Resource] with the same URI). Once the type
// check passes, URI comparison is delegated to [op.ResourceBase.Equal].
func (r *Resource) Equal(other any) bool {

	if other == nil {
		return false
	}

	if _, ok := other.(*Resource); !ok {
		return false
	}

	return r.ResourceBase.Equal(other)
}

// Resolve is a no-op for appnet resources — identity is the URL; the Resource has no on-disk state to reconcile.
func (r *Resource) Resolve() error {
	return nil
}

// String returns a debug-oriented single-line representation of the resource.
func (r *Resource) String() string {
	return fmt.Sprintf("appnet.Resource{uri=%s}", r.URI())
}

// UnmarshalJSON populates the receiver from its JSON wire form (a bare URL string).
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.ExecutionContext] before invoking
// this method. The URL alone is sufficient — identity IS reachability.
func (r *Resource) UnmarshalJSON(data []byte) error {

	if r.ExecutionContext() == nil {
		return errors.New("appnet.Resource: UnmarshalJSON requires ExecutionContext on receiver")
	}

	var uri string
	if err := json.Unmarshal(data, &uri); err != nil {
		return err
	}

	built, err := NewResource(r.ExecutionContext(), uri)
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalStarlark populates the receiver from a [starlark.String] containing the URL.
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

// UnmarshalText populates the receiver from raw UTF-8 bytes containing the URL.
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

// UnmarshalYAML populates the receiver from its YAML wire form (a bare URL scalar).
func (r *Resource) UnmarshalYAML(unmarshal func(any) error) error {

	if r.ExecutionContext() == nil {
		return errors.New("appnet.Resource: UnmarshalYAML requires ExecutionContext on receiver")
	}

	var uri string
	if err := unmarshal(&uri); err != nil {
		return err
	}

	built, err := NewResource(r.ExecutionContext(), uri)
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// endregion

// endregion

// canonicalURL parses value, normalizes it in place, and returns the result.
//
// Normalization (RFC 3986 semantics-preserving):
//   - Lowercase hostname.
//   - Strip default port for the transport (80 for http, 443 for https, 21 for ftp).
//   - Normalize percent-encoding: uppercase hex digits, decode unreserved characters.
//   - Strip trailing /.
//   - Collapse repeated // in path (preserving leading /).
//   - Sort query parameters by key (url.Values.Encode sorts alphabetically).
//
// The transport scheme is preserved — it's reachability-critical and part of the resource's identity.
func canonicalURL(value string) (*url.URL, error) {

	sourceURL, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("appnet.Resource: invalid URL %q: %w", value, err)
	}

	host := strings.ToLower(sourceURL.Hostname())
	port := sourceURL.Port()

	if port != "" && !isDefaultPort(sourceURL.Scheme, port) {
		host += ":" + port
	}

	p := normalizePercentEncoding(sourceURL.EscapedPath())
	p = collapseSlashes(p)
	p = strings.TrimRight(p, "/")

	if p == "" {
		p = "/"
	}

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

// normalizePercentEncoding uppercases hex digits in percent-encoded sequences and decodes unreserved characters.
//
// Unreserved characters are the characters that are allowed in a URI without being percent-encoded. According to
// RFC 3986 §2.3, the unreserved set consists of:
//
// - Uppercase and Lowercase Letters: A–Z and a–z
// - Decimal Digits: 0–9
// - Hyphen: -
// - Period: .
// - Underscore: _
// - Tilde: ~
//
// See: https://datatracker.ietf.org/doc/html/rfc3986#section-2.3
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
