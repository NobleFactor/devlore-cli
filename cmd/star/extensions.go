// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package main

import "embed"

// bundledExtensions contains the star extensions compiled into the binary.
// These are discovered and loaded before any filesystem-based extensions.
//
//go:embed extensions
var bundledExtensions embed.FS
