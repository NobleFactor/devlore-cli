// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package application

// Build-time identity, stamped by the linker and shared by every command.
//
// These are the `-X` targets, and they live here — beside the program name and the tool's other identity —
// because a build stamps the product once, not once per binary. The build stanzas name exactly these five
// symbols:
//
//	-X github.com/NobleFactor/devlore-cli/pkg/application.Version=$(VERSION)
//	-X github.com/NobleFactor/devlore-cli/pkg/application.Commit=$(COMMIT)
//	-X github.com/NobleFactor/devlore-cli/pkg/application.BuildDate=$(BUILD_DATE)
//	-X github.com/NobleFactor/devlore-cli/pkg/application.Channel=$(CHANNEL)
//	-X github.com/NobleFactor/devlore-cli/pkg/application.Prerelease=$(PRERELEASE)
//
// **The path is load-bearing and silently so.** `-X` against a symbol that does not exist is not an error —
// the linker ignores it and the binary reports its compiled-in default. Every release built before
// 2026-08-16 shipped that way, because the stanzas named `internal/cli.Version`, where `Version` was only ever
// a struct field. A package move that leaves these paths behind produces working, testable, correct-looking
// binaries that report `dev`; only a build-time check comparing what was stamped against what the binary
// prints can catch it (docs/plans/version-stamping.md).
//
// The defaults are what an unstamped build honestly reports — `go run`, an IDE build, `go build` without the
// Makefile — and they are deliberately not empty strings.
var (
	// Version is the semantic version, e.g. "v0.4.0"; "dev" in an unstamped build.
	Version = "dev"

	// Commit is the short git commit hash; "none" in an unstamped build or outside a repository.
	Commit = "none"

	// BuildDate is the RFC 3339 UTC build timestamp; "unknown" in an unstamped build.
	BuildDate = "unknown"
)

// The channel a build follows, stamped by the release that builds it (#947): `develop` for a build from develop,
// `release` for one from main, a `release/*` branch or a `v*` tag.
//
// These two are the exception to the rule above, and deliberately empty by default. A local build has no channel
// (the Makefile stamps both empty unless CHANNEL and PRERELEASE are given), and an unstamped build is a local one,
// so the empty string is the honest report of both; `self upgrade` refuses to guess at a channel a build does not
// have. The build proves these two bind whenever CHANNEL is given, for the same reason it proves the version.
//
// Prerelease is a string because `-X` sets only strings: a `bool` there fails the link. [IsPrerelease] reads it.
var (
	// Channel is the channel `self upgrade` follows by default: "develop" or "release"; empty in a local build.
	Channel string

	// Prerelease is "true" when the build takes its channel's pre-releases and "false" when it does not; empty in
	// a local build.
	Prerelease string
)

// region EXPORTED FUNCTIONS

// IsPrerelease reports whether the build takes its channel's pre-releases.
//
// The stamp is the word the build writes, so only "true" is true. "false", the empty string of a local build, and
// any other spelling are false: a switch the build did not set is off.
//
// Returns:
//   - `bool`: true when [Prerelease] is exactly "true".
func IsPrerelease() bool {
	return Prerelease == "true"
}

// endregion
