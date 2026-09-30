// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package selector

import "strings"

var (
	// osWords spells each operating system, as runtime.GOOS names it, as a selector word.
	osWords = map[string]string{
		"darwin":  "Darwin",
		"freebsd": "FreeBSD",
		"linux":   "Linux",
		"netbsd":  "NetBSD",
		"openbsd": "OpenBSD",
		"windows": "Windows",
	}

	// unixWords are the operating systems, by their words, whose chain begins with Unix.
	unixWords = map[string]bool{"Darwin": true, "FreeBSD": true, "Linux": true, "NetBSD": true, "OpenBSD": true}

	// distroWords spells each distribution, as its os-release ID names it, as a selector word. It holds every ID
	// pkg/platform lists, in both the form os-release gives and the form pkg/platform aliases it to.
	distroWords = map[string]string{
		"almalinux":           "AlmaLinux",
		"alpine":              "Alpine",
		"arch":                "Arch",
		"centos":              "CentOS",
		"centos-stream":       "CentOS",
		"debian":              "Debian",
		"fedora":              "Fedora",
		"linuxmint":           "Mint",
		"manjaro":             "Manjaro",
		"mint":                "Mint",
		"opensuse":            "OpenSUSE",
		"opensuse-leap":       "OpenSUSE",
		"opensuse-tumbleweed": "OpenSUSE",
		"rhel":                "RHEL",
		"rocky":               "Rocky",
		"suse":                "SUSE",
		"ubuntu":              "Ubuntu",
	}

	// archWords maps each spelling of an architecture to its name as runtime.GOARCH gives it: `aarch64` is what the
	// Linux kernel reports for the architecture Go, Apple, Debian and Windows call `arm64`.
	archWords = map[string]string{
		"aarch64": "arm64",
		"amd64":   "amd64",
		"arm64":   "arm64",
		"x86_64":  "amd64",
	}
)

// region EXPORTED FUNCTIONS

// DistroWord spells an os-release ID as a selector word.
//
// Parameters:
//   - `id`: the ID, lowercase, as os-release gives it: ubuntu, rhel, linuxmint.
//
// Returns:
//   - `string`: the word the table gives (Ubuntu, RHEL, Mint), or, for an ID the table doesn't list, the ID with its
//     first letter uppercased (pop gives Pop).
func DistroWord(id string) string {

	if word, ok := distroWords[id]; ok {
		return word
	}
	return capitalized(id)
}

// OSWord spells an operating system as a selector word.
//
// Parameters:
//   - `goos`: the operating system, as runtime.GOOS names it: darwin, linux, windows.
//
// Returns:
//   - `string`: the word the table gives (Darwin, Linux, Windows, FreeBSD), or, for another operating system, its name
//     with the first letter uppercased.
func OSWord(goos string) string {

	if word, ok := osWords[goos]; ok {
		return word
	}
	return capitalized(goos)
}

// endregion

// region HELPER FUNCTIONS

// archOf names the architecture a word spells.
//
// Parameters:
//   - `word`: a selector word.
//
// Returns:
//   - `string`: the architecture, as runtime.GOARCH names it.
//   - `bool`: false when the word spells no architecture the vocabulary knows.
func archOf(word string) (string, bool) {

	arch, ok := archWords[word]
	return arch, ok
}

// capitalized uppercases a name's first letter.
//
// Parameters:
//   - `name`: the name.
//
// Returns:
//   - `string`: the name with its first letter uppercased; empty when the name is.
func capitalized(name string) string {

	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// isPlatformWord reports whether a word is one the vocabulary gives the OS part: Unix, an operating system, or a
// distribution the table spells.
//
// Parameters:
//   - `word`: a selector word.
//
// Returns:
//   - `bool`: true when the word belongs to the OS part.
func isPlatformWord(word string) bool {

	if word == "Unix" {
		return true
	}
	for _, w := range osWords {
		if w == word {
			return true
		}
	}
	for _, w := range distroWords {
		if w == word {
			return true
		}
	}
	return false
}

// endregion
