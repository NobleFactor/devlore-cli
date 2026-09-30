// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package selector

import (
	"bufio"
	"io"
	"os"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/iox"
)

// osReleasePaths are where os-release lives, in the order the specification says to read them: /etc first, and
// /usr/lib only when /etc has none.
var osReleasePaths = []string{"/etc/os-release", "/usr/lib/os-release"}

// OSRelease holds the os-release fields selection and package-manager detection read.
//
// os-release is the freedesktop.org specification that virtually every current Linux distribution ships. ID names the
// distribution; ID_LIKE is the distribution's own statement of its lineage, closest first.
type OSRelease struct {

	// ID is the distribution's identifier, lowercase: ubuntu, debian, rhel. Empty when the file has none.
	ID string

	// IDLike lists the distributions this one is like, closest first: Rocky's is rhel, centos, fedora.
	IDLike []string

	// VersionID is the distribution's version: 26.04.
	VersionID string

	// VariantID names a variant of the distribution: server, workstation.
	VariantID string
}

// region EXPORTED FUNCTIONS

// ParseOSRelease reads os-release content.
//
// Each line is KEY=VALUE. A value may be bare, in double quotes, where a backslash escapes the character after it, or
// in single quotes, which take everything literally. Blank lines and lines that begin with # are ignored.
//
// Parameters:
//   - `r`: the content.
//
// Returns:
//   - `OSRelease`: the fields read; a field the content doesn't carry is empty.
//   - `error`: non-nil when r cannot be read.
func ParseOSRelease(r io.Reader) (OSRelease, error) {

	var release OSRelease
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		value = unquote(value)
		switch key {
		case "ID":
			release.ID = strings.ToLower(value)
		case "ID_LIKE":
			release.IDLike = strings.Fields(strings.ToLower(value))
		case "VERSION_ID":
			release.VersionID = value
		case "VARIANT_ID":
			release.VariantID = value
		}
	}

	return release, scanner.Err()
}

// ReadOSRelease reads the first os-release file of the paths given that can be read.
//
// Parameters:
//   - `paths`: the files to try, in order; `/etc/os-release` then `/usr/lib/os-release` when none is given.
//
// Returns:
//   - `OSRelease`: the fields read.
//   - `bool`: false when no file could be read, which on Linux means the distribution is unknown.
func ReadOSRelease(paths ...string) (OSRelease, bool) {

	if len(paths) == 0 {
		paths = osReleasePaths
	}

	for _, path := range paths {
		if release, err := readOSReleaseFile(path); err == nil {
			return release, true
		}
	}

	return OSRelease{}, false
}

// endregion

// region HELPER FUNCTIONS

// readOSReleaseFile reads one os-release file.
//
// Parameters:
//   - `path`: the file.
//
// Returns:
//   - `OSRelease`: the fields read.
//   - `error`: non-nil when the file cannot be opened or read.
func readOSReleaseFile(path string) (release OSRelease, err error) {

	file, err := os.Open(path)
	if err != nil {
		return OSRelease{}, err
	}

	defer iox.Close(&err, file)
	return ParseOSRelease(file)
}

// unquote removes an os-release value's quotes.
//
// Parameters:
//   - `value`: the value as written after the `=`.
//
// Returns:
//   - `string`: the value: inside double quotes with each backslash escape resolved, inside single quotes as written,
//     or bare.
func unquote(value string) string {

	if len(value) < 2 {
		return value
	}

	switch first, last := value[0], value[len(value)-1]; {
	case first == '\'' && last == '\'':
		return value[1 : len(value)-1]
	case first == '"' && last == '"':
		var b strings.Builder
		inner := value[1 : len(value)-1]
		for i := 0; i < len(inner); i++ {
			if inner[i] == '\\' && i+1 < len(inner) {
				i++
			}
			b.WriteByte(inner[i])
		}
		return b.String()
	default:
		return value
	}
}

// endregion
