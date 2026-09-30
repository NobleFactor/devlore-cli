// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package selector

import (
	"runtime"
	"slices"
)

// Host is what a selector word can name on one machine: its operating system and that system's lineage, and its
// architecture.
type Host struct {

	// OS is the operating system's word: Linux, Darwin, Windows.
	OS string

	// Distro is the distribution's word, from os-release's ID: Ubuntu. Empty off Linux, and on a Linux host whose
	// os-release names no distribution.
	Distro string

	// Lineage is the words of the distributions Distro is like, most general first: os-release's ID_LIKE reversed.
	// Ubuntu's is Debian.
	Lineage []string

	// Chain is the OS part's words, most general first. On Linux it follows the lineage the distribution states in
	// os-release: Unix, Linux, ID_LIKE reversed, then ID, so Ubuntu's is Unix, Linux, Debian, Ubuntu.
	Chain []string

	// Arch is the architecture, as runtime.GOARCH names it: arm64, amd64.
	Arch string
}

// region EXPORTED FUNCTIONS

// Detect describes this machine.
//
// Detection has no side effects and never fails: an os-release it cannot read means "Linux, distribution unknown", and
// the chain stops at Linux.
//
// Returns:
//   - `Host`: this machine.
func Detect() Host {

	var release OSRelease
	if runtime.GOOS == "linux" {
		release, _ = ReadOSRelease()
	}
	return NewHost(runtime.GOOS, runtime.GOARCH, release)
}

// NewHost describes a machine from its operating system, architecture and os-release.
//
// Parameters:
//   - `goos`: the operating system, as runtime.GOOS names it.
//   - `goarch`: the architecture, as runtime.GOARCH names it.
//   - `release`: the machine's os-release; the zero value when it has none. Read only on Linux.
//
// Returns:
//   - `Host`: the machine.
func NewHost(goos, goarch string, release OSRelease) Host {

	var distro string
	var lineage []string
	if goos == "linux" && release.ID != "" {
		for i := len(release.IDLike) - 1; i >= 0; i-- {
			lineage = append(lineage, DistroWord(release.IDLike[i]))
		}
		distro = DistroWord(release.ID)
	}
	return NewHostFromWords(OSWord(goos), distro, lineage, goarch)
}

// NewHostFromWords describes a machine from its words: the ones a user states when overriding detection, or a test
// states to describe a machine it isn't running on.
//
// Parameters:
//   - `os`: the operating system's word: Linux.
//   - `distro`: the distribution's word: Ubuntu. Empty for none.
//   - `lineage`: the words of the distributions it is like, most general first: Debian. Ignored when distro is empty.
//   - `arch`: the architecture, in either spelling: arm64 or aarch64.
//
// Returns:
//   - `Host`: the machine. Its chain is Unix when the OS is a Unix, the OS, then the lineage and the distribution,
//     each word once, at its most specific position.
func NewHostFromWords(os, distro string, lineage []string, arch string) Host {

	host := Host{OS: os, Arch: arch}
	if canonical, ok := archOf(arch); ok {
		host.Arch = canonical
	}

	if unixWords[os] {
		host.Chain = append(host.Chain, "Unix")
	}
	host.Chain = append(host.Chain, os)

	if distro != "" {
		host.Distro = distro
		host.Lineage = slices.Clone(lineage)
		host.Chain = append(host.Chain, lineage...)
		host.Chain = keepMostSpecific(append(host.Chain, distro))
	}

	return host
}

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// depth gives a word's position in the host's chain.
//
// Parameters:
//   - `word`: a selector word.
//
// Returns:
//   - `int`: the word's 1-based position, most general first; 0 when the chain doesn't hold it.
func (h Host) depth(word string) int {

	for i, link := range h.Chain {
		if link == word {
			return i + 1
		}
	}
	return 0
}

// endregion

// endregion

// region HELPER FUNCTIONS

// keepMostSpecific removes a word that repeats in a chain from every position but its last, the most specific.
//
// openSUSE Leap's os-release gives ID opensuse-leap and ID_LIKE "suse opensuse", and both IDs spell OpenSUSE.
//
// Parameters:
//   - `chain`: the words, most general first.
//
// Returns:
//   - `[]string`: the words, each once, most general first.
func keepMostSpecific(chain []string) []string {

	seen := make(map[string]bool, len(chain))
	kept := make([]string, 0, len(chain))
	for i := len(chain) - 1; i >= 0; i-- {
		if !seen[chain[i]] {
			seen[chain[i]] = true
			kept = append(kept, chain[i])
		}
	}

	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}
	return kept
}

// endregion
