// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// PURL is a structured Package URL (purl) identity, modeled after url.URL.
//
// The purl specification defines the canonical format:
//
//	pkg:{type}/{namespace}/{name}@{version}?{qualifiers}#{subpath}
//
// Type and Name are required. All other components are optional.
type PURL struct {
	Type       string            // package ecosystem ("brew", "deb", "winget", etc.)
	Namespace  string            // owner/group prefix (winget publisher, npm scope, Maven group ID)
	Name       string            // the package name
	Version    string            // release version
	Qualifiers map[string]string // key=value pairs (OS, arch, repository, distro)
	Subpath    string            // path within the package
}

// String returns the canonical purl string representation.
//
// Returns:
//   - string: the purl URI (e.g., "pkg:brew/jq@1.7").
func (p PURL) String() string {

	var b strings.Builder

	b.WriteString("pkg:")
	b.WriteString(p.Type)
	b.WriteByte('/')

	if p.Namespace != "" {
		b.WriteString(p.Namespace)
		b.WriteByte('/')
	}

	b.WriteString(p.Name)

	if p.Version != "" {
		b.WriteByte('@')
		b.WriteString(p.Version)
	}

	if len(p.Qualifiers) > 0 {
		b.WriteByte('?')
		keys := make([]string, 0, len(p.Qualifiers))
		for k := range p.Qualifiers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			if i > 0 {
				b.WriteByte('&')
			}
			b.WriteString(k)
			b.WriteByte('=')
			b.WriteString(p.Qualifiers[k])
		}
	}

	if p.Subpath != "" {
		b.WriteByte('#')
		b.WriteString(p.Subpath)
	}

	return b.String()
}

// ParsePURL parses a purl string into its components.
//
// Parameters:
//   - raw: the purl string (e.g., "pkg:brew/jq@1.7").
//
// Returns:
//   - *PURL: the parsed components, or nil on error.
//   - error: non-nil if the string is not a valid purl.
func ParsePURL(raw string) (*PURL, error) {

	if !strings.HasPrefix(raw, "pkg:") {
		return nil, fmt.Errorf("purl: missing pkg: scheme: %q", raw)
	}

	remainder := raw[4:]

	// Split off subpath.

	var subpath string

	if i := strings.IndexByte(remainder, '#'); i >= 0 {
		subpath = remainder[i+1:]
		remainder = remainder[:i]
	}

	// Split off qualifiers.

	var qualifiers map[string]string

	if i := strings.IndexByte(remainder, '?'); i >= 0 {
		qs := remainder[i+1:]
		remainder = remainder[:i]
		qualifiers = make(map[string]string)
		for _, pair := range strings.Split(qs, "&") {
			k, v, _ := strings.Cut(pair, "=")
			qualifiers[k] = v
		}
	}

	// Split off version.

	var version string

	if i := strings.LastIndexByte(remainder, '@'); i >= 0 {
		version, _ = url.PathUnescape(remainder[i+1:])
		remainder = remainder[:i]
	}

	// Split type from namespace/name.

	typeName, rest, ok := strings.Cut(remainder, "/")

	if !ok || typeName == "" {
		return nil, fmt.Errorf("purl: missing type: %q", raw)
	}

	// Split namespace from name (last segment is name).

	var namespace, name string

	if i := strings.LastIndexByte(rest, '/'); i >= 0 {
		namespace = rest[:i]
		name = rest[i+1:]
	} else {
		name = rest
	}

	if name == "" {
		return nil, fmt.Errorf("purl: missing name: %q", raw)
	}

	return &PURL{
		Type:       typeName,
		Namespace:  namespace,
		Name:       name,
		Version:    version,
		Qualifiers: qualifiers,
		Subpath:    subpath,
	}, nil
}
