// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package selector

import (
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// hostFrom builds a host from synthetic os-release content.
//
// Parameters:
//   - `t`: the test.
//   - `goos`: the operating system, as runtime.GOOS spells it.
//   - `goarch`: the architecture, as runtime.GOARCH spells it.
//   - `osRelease`: the os-release lines; empty for a host that has none.
//
// Returns:
//   - `Host`: the host.
func hostFrom(t *testing.T, goos, goarch, osRelease string) Host {

	t.Helper()
	release, err := ParseOSRelease(strings.NewReader(osRelease))
	if err != nil {
		t.Fatalf("parse the os-release fixture: %v", err)
	}
	return NewHost(goos, goarch, release)
}

// --- NewHost ---

// TestNewHost_Chains is the issue's table, one row per distribution, each from a synthetic os-release; then the
// fallback case, a distribution the table doesn't list, and the operating systems that aren't Linux.
func TestNewHost_Chains(t *testing.T) {
	tests := []struct {
		name      string
		goos      string
		osRelease string
		want      []string
	}{
		{"Debian", "linux", "ID=debian\n", []string{"Unix", "Linux", "Debian"}},
		{"Ubuntu", "linux", "ID=ubuntu\nID_LIKE=debian\n", []string{"Unix", "Linux", "Debian", "Ubuntu"}},
		{"Linux Mint", "linux", "ID=linuxmint\nID_LIKE=\"ubuntu debian\"\n",
			[]string{"Unix", "Linux", "Debian", "Ubuntu", "Mint"}},
		{"Fedora", "linux", "ID=fedora\n", []string{"Unix", "Linux", "Fedora"}},
		{"RHEL", "linux", "ID=\"rhel\"\nID_LIKE=\"fedora\"\n", []string{"Unix", "Linux", "Fedora", "RHEL"}},
		{"CentOS Stream", "linux", "ID=\"centos\"\nID_LIKE=\"rhel fedora\"\n",
			[]string{"Unix", "Linux", "Fedora", "RHEL", "CentOS"}},
		{"Rocky", "linux", "ID=\"rocky\"\nID_LIKE=\"rhel centos fedora\"\n",
			[]string{"Unix", "Linux", "Fedora", "CentOS", "RHEL", "Rocky"}},
		{"no os-release", "linux", "", []string{"Unix", "Linux"}},
		{"os-release without an ID", "linux", "NAME=Linux\n", []string{"Unix", "Linux"}},
		{"Pop!_OS, not in the table", "linux", "ID=pop\nID_LIKE=\"ubuntu debian\"\n",
			[]string{"Unix", "Linux", "Debian", "Ubuntu", "Pop"}},
		{"openSUSE Leap, a repeated link", "linux", "ID=\"opensuse-leap\"\nID_LIKE=\"suse opensuse\"\n",
			[]string{"Unix", "Linux", "SUSE", "OpenSUSE"}},
		{"Darwin", "darwin", "", []string{"Unix", "Darwin"}},
		{"Windows", "windows", "", []string{"Windows"}},
		{"FreeBSD", "freebsd", "", []string{"Unix", "FreeBSD"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hostFrom(t, tt.goos, "arm64", tt.osRelease).Chain; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Chain = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewHost_ArchIsGoArch(t *testing.T) {
	if got := hostFrom(t, "linux", "arm64", "ID=ubuntu\n").Arch; got != "arm64" {
		t.Errorf("Arch = %q, want arm64", got)
	}
}

// --- Detect ---

// TestDetect_ChainEndsWithTheOSOrItsDistribution: on the machine running the test, the chain's last link is its
// distribution when os-release names one, and its OS otherwise; the OS is in the chain; the architecture is Go's.
func TestDetect_ChainEndsWithTheOSOrItsDistribution(t *testing.T) {
	host := Detect()
	if len(host.Chain) == 0 {
		t.Fatal("Detect() returned an empty chain")
	}

	want := host.OS
	if host.Distro != "" {
		want = host.Distro
	}
	if got := host.Chain[len(host.Chain)-1]; got != want {
		t.Errorf("Chain = %v ends with %q, want %q: the distribution when os-release names one, else the OS",
			host.Chain, got, want)
	}
	if want := OSWord(runtime.GOOS); host.OS != want || !slices.Contains(host.Chain, want) {
		t.Errorf("OS = %q in chain %v, want %q, runtime.GOOS's word, in the chain", host.OS, host.Chain, want)
	}
	if host.Arch != runtime.GOARCH {
		t.Errorf("Arch = %q, want %q", host.Arch, runtime.GOARCH)
	}
}

// --- NewHostFromWords ---

func TestNewHostFromWords_BuildsTheSameChainAsDetection(t *testing.T) {
	host := NewHostFromWords("Linux", "Ubuntu", []string{"Debian"}, "aarch64")
	if want := []string{"Unix", "Linux", "Debian", "Ubuntu"}; !reflect.DeepEqual(host.Chain, want) {
		t.Errorf("Chain = %v, want %v", host.Chain, want)
	}
	if host.Arch != "arm64" {
		t.Errorf("Arch = %q, want arm64: aarch64 is arm64's other spelling", host.Arch)
	}
	if !reflect.DeepEqual(host, hostFrom(t, "linux", "arm64", "ID=ubuntu\nID_LIKE=debian\n")) {
		t.Errorf("NewHostFromWords = %+v, want what NewHost gives from the same os-release", host)
	}
}
