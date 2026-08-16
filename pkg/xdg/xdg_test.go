// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package xdg

import (
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// homeVariable returns the environment variable [os.UserHomeDir] reads on this platform.
//
// This is the one place in the repository that names a home-directory variable outside the package
// documentation, and it exists to prove a negative: that setting the variable does **not** move home while
// the account database can answer.
//
// Returns:
//   - `string`: "USERPROFILE" on Windows, "home" on plan9, "HOME" elsewhere.
func homeVariable() string {

	switch runtime.GOOS {
	case "windows":
		return "USERPROFILE"
	case "plan9":
		return "home"
	default:
		return "HOME"
	}
}

// requireAccountHome returns the account database's home directory, skipping when it has none.
//
// Every base directory defaults beneath this, because rung 2 outranks the environment.
//
// Parameters:
//   - `t`: the test harness.
//
// Returns:
//   - `string`: the account's home directory.
func requireAccountHome(t *testing.T) string {

	t.Helper()

	account, err := user.Current()
	if err != nil || !filepath.IsAbs(account.HomeDir) {
		t.Skipf("no user-account home available on this host: %v", err)
	}

	return account.HomeDir
}

// clearEnvironment blanks every variable this package reads, so a test sees pristine defaults.
//
// Parameters:
//   - `t`: the test harness; [testing.T.Setenv] restores each value at test end.
func clearEnvironment(t *testing.T) {

	t.Helper()

	for _, variable := range []string{
		envBinHome, envCacheHome, envConfigDirs, envConfigHome, envDataDirs, envDataHome, envStateHome,
	} {
		t.Setenv(variable, "")
	}
}

// --- UserHomeDir ---

// TestUserHomeDir_PrefersTheUserAccount pins rung 2 outranking rung 3: the account database answers even when
// the environment names somewhere else entirely.
//
// This is the property that makes home non-injectable. A test elsewhere that sets `HOME` to a sandbox and
// believes it has redirected anything is relying on an order this test forbids.
func TestUserHomeDir_PrefersTheUserAccount(t *testing.T) {

	want := requireAccountHome(t)
	t.Setenv(homeVariable(), t.TempDir())

	if got := UserHomeDir(); got != want {
		t.Errorf("UserHomeDir() = %q, want the account home %q — the environment must not outrank it", got, want)
	}
}

// TestUserHomeDir_IsAbsoluteWhicheverRungAnswers stands in for the two rungs no in-process seam can force.
//
// Rung 3 is reachable only when [user.Current] fails, and rung 4 only when the environment fails as well;
// neither can be induced from inside the process. What can be asserted on every host is the invariant they
// exist to preserve — that whichever rung answers, the answer is absolute.
func TestUserHomeDir_IsAbsoluteWhicheverRungAnswers(t *testing.T) {

	t.Setenv(homeVariable(), "")

	if got := UserHomeDir(); !filepath.IsAbs(got) {
		t.Errorf("UserHomeDir() = %q, which is relative to the working directory", got)
	}
}

// --- Base directories ---

// TestBases_DefaultBeneathHome proves each base lands at its standard XDG name under home — the same layout
// on every platform, which is the whole point of the ruling.
func TestBases_DefaultBeneathHome(t *testing.T) {

	clearEnvironment(t)

	home := requireAccountHome(t)

	for _, tc := range []struct {
		name     string
		resolve  func() string
		expected string
	}{
		{"BinHome", BinHome, filepath.Join(home, ".local", "bin")},
		{"CacheHome", CacheHome, filepath.Join(home, ".cache")},
		{"ConfigHome", ConfigHome, filepath.Join(home, ".config")},
		{"DataHome", DataHome, filepath.Join(home, ".local", "share")},
		{"StateHome", StateHome, filepath.Join(home, ".local", "state")},
	} {
		if got := tc.resolve(); got != tc.expected {
			t.Errorf("%s() = %q, want %q", tc.name, got, tc.expected)
		}
	}
}

// TestBases_AbsoluteVariableWins proves the environment overrides the default on every platform, including
// Windows — the specification's first rule.
func TestBases_AbsoluteVariableWins(t *testing.T) {

	clearEnvironment(t)

	override := t.TempDir()

	for _, tc := range []struct {
		name     string
		variable string
		resolve  func() string
	}{
		{"BinHome", envBinHome, BinHome},
		{"CacheHome", envCacheHome, CacheHome},
		{"ConfigHome", envConfigHome, ConfigHome},
		{"DataHome", envDataHome, DataHome},
		{"StateHome", envStateHome, StateHome},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.variable, override)
			if got := tc.resolve(); got != override {
				t.Errorf("%s() = %q, want the override %q", tc.name, got, override)
			}
		})
	}
}

// TestBases_RelativeVariableIsIgnored is the defect this package exists to make impossible.
//
// The specification: "If an implementation encounters a relative path in any of these variables it should
// consider the path invalid and ignore it." A relative value must therefore resolve to the home-rooted
// default, never to something the working directory decides.
func TestBases_RelativeVariableIsIgnored(t *testing.T) {

	clearEnvironment(t)

	home := requireAccountHome(t)

	for _, tc := range []struct {
		name     string
		variable string
		resolve  func() string
		expected string
	}{
		{"BinHome", envBinHome, BinHome, filepath.Join(home, ".local", "bin")},
		{"CacheHome", envCacheHome, CacheHome, filepath.Join(home, ".cache")},
		{"ConfigHome", envConfigHome, ConfigHome, filepath.Join(home, ".config")},
		{"DataHome", envDataHome, DataHome, filepath.Join(home, ".local", "share")},
		{"StateHome", envStateHome, StateHome, filepath.Join(home, ".local", "state")},
	} {
		for _, relative := range []string{".local/state", "devlore", filepath.Join("..", "escape")} {
			t.Run(tc.name+"/"+relative, func(t *testing.T) {
				t.Setenv(tc.variable, relative)
				if got := tc.resolve(); got != tc.expected {
					t.Errorf("%s() = %q with %s=%q; a relative value must be ignored, want %q",
						tc.name, got, tc.variable, relative, tc.expected)
				}
			})
		}
	}
}

// TestNoAnchorIsEverRelative is the invariant that outranks every individual case: no combination of a
// missing home variable and junk in the XDG variables may yield a path resolved against the working
// directory. On Windows before this package, `filepath.Join("", ".local", "state")` did exactly that.
func TestNoAnchorIsEverRelative(t *testing.T) {

	t.Setenv(homeVariable(), "")
	requireAccountHome(t)

	for _, variable := range []string{envBinHome, envCacheHome, envConfigHome, envDataHome, envStateHome} {
		t.Setenv(variable, filepath.Join("relative", "nonsense"))
	}

	for name, got := range map[string]string{
		"BinHome":    BinHome(),
		"CacheHome":  CacheHome(),
		"ConfigHome": ConfigHome(),
		"DataHome":   DataHome(),
		"StateHome":  StateHome(),
		"home":       UserHomeDir(),
	} {
		if !filepath.IsAbs(got) {
			t.Errorf("%s = %q, which is relative to the working directory", name, got)
		}
	}
}

// --- Path builders ---

// TestPaths_JoinElementsBeneathTheBase covers the option-D surface: the application name is simply the first
// element, and no elements yields the base directory itself.
func TestPaths_JoinElementsBeneathTheBase(t *testing.T) {

	clearEnvironment(t)

	home := requireAccountHome(t)

	for _, tc := range []struct {
		name     string
		resolve  func(...string) string
		expected string
	}{
		{"BinPath", BinPath, filepath.Join(home, ".local", "bin", "devlore")},
		{"CachePath", CachePath, filepath.Join(home, ".cache", "devlore")},
		{"ConfigPath", ConfigPath, filepath.Join(home, ".config", "devlore")},
		{"DataPath", DataPath, filepath.Join(home, ".local", "share", "devlore")},
		{"StatePath", StatePath, filepath.Join(home, ".local", "state", "devlore")},
	} {
		t.Run(tc.name, func(t *testing.T) {

			if got := tc.resolve("devlore"); got != tc.expected {
				t.Errorf("%s(\"devlore\") = %q, want %q", tc.name, got, tc.expected)
			}

			deep := tc.resolve("devlore", "signing", "ed25519")
			if want := filepath.Join(tc.expected, "signing", "ed25519"); deep != want {
				t.Errorf("%s(deep) = %q, want %q", tc.name, deep, want)
			}
		})
	}

	if got, want := StatePath(), StateHome(); got != want {
		t.Errorf("StatePath() = %q with no elements, want the base %q", got, want)
	}
}

// --- Search lists ---

// TestDirs_KeepsAbsoluteEntriesAndDropsRelativeOnes proves the search lists apply the same absolute-only rule
// as the base variables, entry by entry rather than all-or-nothing.
func TestDirs_KeepsAbsoluteEntriesAndDropsRelativeOnes(t *testing.T) {

	first, second := t.TempDir(), t.TempDir()
	mixed := strings.Join([]string{first, "relative/entry", second}, string(filepath.ListSeparator))

	t.Setenv(envConfigDirs, mixed)

	got := ConfigDirs()
	want := []string{first, second}

	if len(got) != len(want) {
		t.Fatalf("ConfigDirs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ConfigDirs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestDirs_FallsBackWhenNoAbsoluteEntrySurvives proves an unset — or entirely relative — variable yields the
// specification's defaults rather than an empty search path.
func TestDirs_FallsBackWhenNoAbsoluteEntrySurvives(t *testing.T) {

	for _, value := range []string{"", "relative/only"} {
		t.Run("value="+value, func(t *testing.T) {

			t.Setenv(envConfigDirs, value)
			t.Setenv(envDataDirs, value)

			if got := ConfigDirs(); len(got) != 1 || got[0] != "/etc/xdg" {
				t.Errorf("ConfigDirs() = %v, want the specification default [/etc/xdg]", got)
			}
			if got := DataDirs(); len(got) != 2 {
				t.Errorf("DataDirs() = %v, want the two specification defaults", got)
			}
		})
	}
}

// --- Helpers ---

// TestJoin_NoElementsReturnsTheBase pins the zero-element contract [join] gives every *Path accessor.
func TestJoin_NoElementsReturnsTheBase(t *testing.T) {

	base := filepath.Join(string(filepath.Separator), "anchor")

	if got := join(base, nil); got != base {
		t.Errorf("join(base, nil) = %q, want %q", got, base)
	}
	if got, want := join(base, []string{"a", "b"}), filepath.Join(base, "a", "b"); got != want {
		t.Errorf("join(base, [a b]) = %q, want %q", got, want)
	}
}
