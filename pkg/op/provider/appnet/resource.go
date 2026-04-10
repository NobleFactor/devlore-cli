// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package appnet

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Resource represents a network resource identified by a URL.
//
// The SourceURL holds the original URL as provided (with transport scheme, original casing, etc.). The canonical URI
// produced by [URI] is an opaque appnet: URI wrapping the normalized, transport-independent URL with targeted escaping
// of # and ? characters.
type Resource struct {
	op.ResourceBase
	SourceURL *url.URL
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

// String returns a compact JSON representation of the resource.
func (r *Resource) String() string { return r.Format(r) }

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
