// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import "testing"

// TestToken pins the canonical dotted-token rendering, including the package-manager-family grouping.
func TestToken(t *testing.T) {

	cases := []struct {
		os     string
		distro string
		want   string
	}{
		{"darwin", "macos", "Darwin"},
		{"windows", "windows", "Windows"},
		{"linux", "debian", "Linux.Debian"},
		{"linux", "ubuntu", "Linux.Debian"},
		{"linux", "Ubuntu", "Linux.Debian"},
		{"linux", "fedora", "Linux.Fedora"},
		{"linux", "rhel", "Linux.Fedora"},
		{"linux", "centos", "Linux.Fedora"},
		{"linux", "rocky", "Linux.Fedora"},
		{"linux", "alma", "Linux.Fedora"},
		{"linux", "arch", "Linux"},
		{"linux", "", "Linux"},
	}

	for _, c := range cases {
		t.Run(c.os+"/"+c.distro, func(t *testing.T) {
			if got := Token(tokenFake{os: c.os, distro: c.distro}); got != c.want {
				t.Errorf("Token(%s, %s) = %q, want %q", c.os, c.distro, got, c.want)
			}
		})
	}
}

// tokenFake is a minimal in-package Platform stand-in for the token test.
type tokenFake struct {
	Platform

	os     string
	distro string
}

func (f tokenFake) OS() string     { return f.os }
func (f tokenFake) Distro() string { return f.distro }
