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
// documentation: a test that proves the fallback ladder has to be able to silence the rung above it.
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

// TestUserHomeDir_UsesTheEnvironmentWhenItAnswers pins rung 2: the platform's own variable, whatever it is
// called here.
func TestUserHomeDir_UsesTheEnvironmentWhenItAnswers(t *testing.T) {

	want := t.TempDir()
	t.Setenv(homeVariable(), want)

	if got := UserHomeDir(); got != want {
		t.Errorf("UserHomeDir() = %q, want %q", got, want)
	}
}

// TestUserHomeDir_FallsBackToTheUserAccount pins rung 3 — the rung that exists for Windows services and
// scheduled tasks, where the environment carries no home at all.
//
// Rung 4 (the assert) is deliberately not exercised: reaching it requires the user account lookup to fail
// too, which no in-process seam can force. The invariant that stands in for it is asserted by
// [TestNoAnchorIsEverRelative] — whatever rung answers, the result is absolute.
func TestUserHomeDir_FallsBackToTheUserAccount(t *testing.T) {

	t.Setenv(homeVariable(), "")

	account, err := user.Current()
	if err != nil || account.HomeDir == "" {
		t.Skipf("no user-account home available on this host: %v", err)
	}

	if got := UserHomeDir(); got != account.HomeDir {
		t.Errorf("UserHomeDir() = %q, want the account home %q", got, account.HomeDir)
	}
}

// --- Base directories ---

// TestBases_DefaultBeneathHome proves each base lands at its standard XDG name under home — the same layout
// on every platform, which is the whole point of the ruling.
func TestBases_DefaultBeneathHome(t *testing.T) {

	clearEnvironment(t)

	home := t.TempDir()
	t.Setenv(homeVariable(), home)

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
	t.Setenv(homeVariable(), t.TempDir())

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

	home := t.TempDir()
	t.Setenv(homeVariable(), home)

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

	if _, err := user.Current(); err != nil {
		t.Skipf("no user-account home available on this host: %v", err)
	}

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

	home := t.TempDir()
	t.Setenv(homeVariable(), home)

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
