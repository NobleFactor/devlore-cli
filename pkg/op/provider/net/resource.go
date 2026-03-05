// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package net //nolint:revive // package name is domain-specific

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func init() {
	// Both constructors are identical — no I/O for URLs.
	ctor := func(v any) (Resource, error) {
		s, ok := v.(string)
		if !ok {
			return Resource{}, fmt.Errorf("net.Resource: expected string URL, got %T", v)
		}
		u, err := url.Parse(s)
		if err != nil {
			return Resource{}, fmt.Errorf("net.Resource: invalid URL %q: %w", s, err)
		}
		return Resource{SourceURL: u}, nil
	}
	op.RegisterConstructor(ctor)
	op.RegisterPlanTimeConstructor(ctor)
}

// Resource represents a network resource identified by a URL.
//
// The SourceURL holds the original URL as provided (with transport scheme,
// original casing, etc.). The canonical URI produced by [URI] strips the
// transport scheme and normalizes the authority and path components for
// catalog deduplication.
type Resource struct {
	op.ResourceBase
	SourceURL *url.URL
}

// URI returns the canonical net:// URI for catalog lookups.
//
// The URI is transport-independent: http://example.com/f and
// https://example.com/f produce the same URI. The original transport
// scheme is available via SourceURL.Scheme.
func (r *Resource) URI() string { return "net://" + r.canonicalAuthority() }

// Scheme returns "net".
func (r *Resource) Scheme() string { return op.SchemeNet }

// Host returns the lowercased hostname from the source URL.
func (r *Resource) Host() string { return strings.ToLower(r.SourceURL.Hostname()) }

// Path returns the raw path from the source URL.
func (r *Resource) Path() string { return r.SourceURL.Path }

// canonicalAuthority produces the canonical host+path+query string.
//
// Normalization rules (RFC 3986):
//   - Lowercase hostname
//   - Strip default ports (80 for http, 443 for https, 21 for ftp)
//   - Normalize percent-encoding: uppercase hex digits, decode unreserved chars
//   - Strip trailing /
//   - Collapse double // in path (except leading)
//   - Sort query parameters by key
func (r *Resource) canonicalAuthority() string {
	// Host: lowercase, strip default port
	host := strings.ToLower(r.SourceURL.Hostname())
	port := r.SourceURL.Port()
	if port != "" && !isDefaultPort(r.SourceURL.Scheme, port) {
		host += ":" + port
	}

	// Path: normalize encoding, collapse slashes, strip trailing /
	p := normalizePercentEncoding(r.SourceURL.EscapedPath())
	p = collapseSlashes(p)
	p = strings.TrimRight(p, "/")
	if p == "" {
		p = "/"
	}

	// Query: sorted by key (url.Values.Encode sorts alphabetically)
	q := r.SourceURL.Query().Encode()

	result := host + p
	if q != "" {
		result += "?" + q
	}
	return result
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

// collapseSlashes replaces runs of multiple / with a single / in the path,
// preserving the leading /.
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

// normalizePercentEncoding uppercases hex digits in percent-encoded sequences
// and decodes unreserved characters (RFC 3986 §2.3: A-Z a-z 0-9 - . _ ~).
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
