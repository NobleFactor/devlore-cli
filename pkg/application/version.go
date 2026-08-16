// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package application

// Build-time identity, stamped by the linker and shared by every command.
//
// These are the `-X` targets, and they live here — beside the program name and the tool's other identity —
// because a build stamps the product once, not once per binary. The build stanzas name exactly these three
// symbols:
//
//	-X github.com/NobleFactor/devlore-cli/pkg/application.Version=$(VERSION)
//	-X github.com/NobleFactor/devlore-cli/pkg/application.Commit=$(COMMIT)
//	-X github.com/NobleFactor/devlore-cli/pkg/application.BuildDate=$(BUILD_DATE)
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
