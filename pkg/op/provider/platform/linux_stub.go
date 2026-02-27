// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !linux

package platform

import "github.com/NobleFactor/devlore-cli/pkg/op"

func newLinux() *op.Platform {
	panic("newLinux called on non-linux platform")
}
