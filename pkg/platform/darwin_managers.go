// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import "strings"

// Darwin managers (brew, port, launchd) split across three files for cross-host build support:
//
//   - darwin_managers.go         types + pure methods (this file, always compiled)
//   - darwin_managers_darwin.go  real shell-out implementations (Darwin only)
//   - darwin_managers_other.go   stub shell-out implementations (every non-Darwin host)
//
// On Darwin: this file + darwin_managers_darwin.go combine; methods are real.
// On any other host: this file + darwin_managers_other.go combine; shell-out methods return error
// PlatformResults so cross-host fixtures construct successfully but fail loudly at runtime.

// Compile-time interface guards — on every host, each type implements its full interface (real on
// Darwin, stubbed on every other host).
var (
	_ PackageManager = (*brewManager)(nil)
	_ PackageManager = (*portManager)(nil)
	_ ServiceManager = (*launchdManager)(nil)
)

// =============================================================================
// Homebrew Package Manager
// =============================================================================

type brewManager struct{}

func (m *brewManager) Name() string { return "brew" }

func (m *brewManager) ParsePURL(id string) PURL {

	name, version, _ := strings.Cut(id, "@")
	return PURL{Type: "brew", Name: name, Version: version}
}

func (m *brewManager) NeedsSudo() bool { return false }

// =============================================================================
// MacPorts Package Manager
// =============================================================================

type portManager struct{}

func (m *portManager) Name() string { return "port" }

func (m *portManager) ParsePURL(id string) PURL {

	name, version, _ := strings.Cut(id, "@")
	return PURL{Type: "port", Name: name, Version: version}
}

func (m *portManager) NeedsSudo() bool { return true }

// =============================================================================
// launchd Service Manager
// =============================================================================

type launchdManager struct{}

func (m *launchdManager) NeedsSudo() bool { return false }
