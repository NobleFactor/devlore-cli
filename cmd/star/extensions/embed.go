// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package bundled carries the star extensions compiled into the binary: every `com.*` directory beside this
// file, discovered and loaded before any filesystem-based extension. It is its own package because an embed
// can only name paths under its package's directory, and star's root -- an importable package since #787 --
// loads from it.
package bundled

import "embed"

// FS is the extensions compiled into the binary; its root holds one directory per extension.
//
//go:embed com.*
var FS embed.FS
