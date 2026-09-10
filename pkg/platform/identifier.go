// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package platform

import (
	"fmt"
	"strings"
)

// purlScheme is the canonical purl's scheme, checked before the manager-prefix form: the two grammars are otherwise
// ambiguous, since `pkg` reads as a legal manager prefix (#813).
const purlScheme = "pkg:"

// ParseIdentifier parses a package identifier into a [PURL] whose type is canonical for `plat`.
//
// Three forms are accepted, and the canonical one is checked first. A string that declares the scheme is a purl: a
// parse failure is reported as one and never re-read as a manager prefix, since a fallback would turn a typo into a
// package named after it.
//
//   - `pkg:{type}/{namespace}/{name}@{version}?{qualifiers}`: the purl; its type resolves through
//     [Platform.ResolvePurlType].
//   - `{manager}:{name}[@{version}]`: the manager-prefix form; the prefix resolves the same way.
//   - `{name}[@{version}]`: the bare name, on the platform's default manager ([Platform.DefaultPurlType]).
//
// The result is the identifier's full coordinates, version included. Identity is versionless: a caller that interns
// or compares packages clears `Version` first, so `jq` and `jq@1.7` are one package at two versions.
//
// This is the one grammar for package identifiers. The pkg provider builds its resources from it and writ's manifest
// merge keys claims by it, so what is planned and what is compared are the same parse (#814).
//
// Parameters:
//   - `plat`: the target platform; resolves manager prefixes and supplies the default type.
//   - `raw`: the identifier, in any of the three forms.
//
// Returns:
//   - `PURL`: the parsed coordinates, with a canonical type.
//   - `error`: a malformed purl, or a purl type or manager prefix no manager on `plat` answers to.
func ParseIdentifier(plat Platform, raw string) (PURL, error) {

	if strings.HasPrefix(raw, purlScheme) {
		parsed, err := ParsePURL(raw)
		if err != nil {
			return PURL{}, err
		}

		resolved, known := plat.ResolvePurlType(parsed.Type)
		if !known {
			return PURL{}, fmt.Errorf("unknown package manager %q", parsed.Type)
		}

		parsed.Type = resolved
		return *parsed, nil
	}

	// The manager-prefix form and the bare name.

	purlType := plat.DefaultPurlType()

	if prefix, after, ok := strings.Cut(raw, ":"); ok {
		resolved, known := plat.ResolvePurlType(prefix)
		if !known {
			return PURL{}, fmt.Errorf("unknown package manager %q", prefix)
		}
		purlType = resolved
		raw = after
	}

	name, version, _ := strings.Cut(raw, "@")

	return PURL{Type: purlType, Name: name, Version: version}, nil
}
