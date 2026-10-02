// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package writ

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/tree"
)

// --- PartitionByScope ---

// TestPartitionByScope_MixedSystemHome proves sources split into a System and a Home partition, each keeping the
// layer order it was given: System holds base then team, Home holds base, team, then personal.
//
// Parameters:
//   - `t`: the test harness.
func TestPartitionByScope_MixedSystemHome(t *testing.T) {
	t.Helper()

	sources := []tree.LayerSource{
		{Layer: "base", Order: 0, ScopeName: "System", TargetRoot: "/", SourceRoot: "/repo/base/System"},
		{Layer: "base", Order: 0, ScopeName: "Home", TargetRoot: "/home/user", SourceRoot: "/repo/base/Home"},
		{Layer: "team", Order: 1, ScopeName: "System", TargetRoot: "/", SourceRoot: "/repo/team/System"},
		{Layer: "team", Order: 1, ScopeName: "Home", TargetRoot: "/home/user", SourceRoot: "/repo/team/Home"},
		{Layer: "personal", Order: 2, ScopeName: "Home", TargetRoot: "/home/user", SourceRoot: "/repo/personal/Home"},
	}

	partitions := PartitionByScope(sources)

	// Two keys: System and Home
	if len(partitions) != 2 {
		t.Fatalf("got %d partitions, want 2", len(partitions))
	}

	// System partition: base, team
	sys := partitions["System"]
	if len(sys) != 2 {
		t.Fatalf("System partition has %d entries, want 2", len(sys))
	}
	if sys[0].Layer != "base" {
		t.Errorf("System[0].Layer = %q, want %q", sys[0].Layer, "base")
	}
	if sys[1].Layer != "team" {
		t.Errorf("System[1].Layer = %q, want %q", sys[1].Layer, "team")
	}

	// Home partition: base, team, personal
	home := partitions["Home"]
	if len(home) != 3 {
		t.Fatalf("Home partition has %d entries, want 3", len(home))
	}
	if home[0].Layer != "base" {
		t.Errorf("Home[0].Layer = %q, want %q", home[0].Layer, "base")
	}
	if home[1].Layer != "team" {
		t.Errorf("Home[1].Layer = %q, want %q", home[1].Layer, "team")
	}
	if home[2].Layer != "personal" {
		t.Errorf("Home[2].Layer = %q, want %q", home[2].Layer, "personal")
	}
}

// TestPartitionByScope_OnlyHome proves Home-only sources yield one Home partition in the given order and no System
// partition.
//
// Parameters:
//   - `t`: the test harness.
func TestPartitionByScope_OnlyHome(t *testing.T) {
	t.Helper()

	sources := []tree.LayerSource{
		{Layer: "base", Order: 0, ScopeName: "Home", TargetRoot: "/home/user", SourceRoot: "/repo/base/Home"},
		{Layer: "personal", Order: 2, ScopeName: "Home", TargetRoot: "/home/user", SourceRoot: "/repo/personal/Home"},
	}

	partitions := PartitionByScope(sources)

	if len(partitions) != 1 {
		t.Fatalf("got %d partitions, want 1", len(partitions))
	}

	home := partitions["Home"]
	if len(home) != 2 {
		t.Fatalf("Home partition has %d entries, want 2", len(home))
	}
	if home[0].Layer != "base" {
		t.Errorf("Home[0].Layer = %q, want %q", home[0].Layer, "base")
	}
	if home[1].Layer != "personal" {
		t.Errorf("Home[1].Layer = %q, want %q", home[1].Layer, "personal")
	}

	if _, ok := partitions["System"]; ok {
		t.Error("System partition should not exist when no System sources provided")
	}
}

// TestPartitionByScope_EmptySources proves a nil and an empty source list each yield an empty map.
//
// Parameters:
//   - `t`: the test harness.
func TestPartitionByScope_EmptySources(t *testing.T) {
	t.Helper()

	partitions := PartitionByScope(nil)

	if len(partitions) != 0 {
		t.Fatalf("got %d partitions, want 0", len(partitions))
	}

	partitions = PartitionByScope([]tree.LayerSource{})

	if len(partitions) != 0 {
		t.Fatalf("got %d partitions for empty slice, want 0", len(partitions))
	}
}

// --- ScopeHome, ScopeSystem ---

// TestScopeRoots_ReadWritScopes proves the scope roots come from `writ.scopes` (#925), keyed by the scope's name as
// the defaults document it; viper matches keys without case, so `Home` and `home` are one key.
//
// Parameters:
//   - `t`: the test harness.
func TestScopeRoots_ReadWritScopes(t *testing.T) {

	t.Cleanup(viper.Reset)
	home, system := t.TempDir(), t.TempDir()
	viper.Set("writ.scopes.Home", home)
	viper.Set("writ.scopes.System", system)

	if got := ScopeHome(); got != home {
		t.Errorf("ScopeHome() = %q, want %q from writ.scopes.Home", got, home)
	}
	if got := ScopeSystem(); got != system {
		t.Errorf("ScopeSystem() = %q, want %q from writ.scopes.System", got, system)
	}
}

// --- the scope model (#926) ---

// TestDefinedScopes_Unix proves Unix defines System at `/` and Home at the home directory, and none of the Windows
// scopes.
//
// Parameters:
//   - `t`: the test harness.
func TestDefinedScopes_Unix(t *testing.T) {

	for _, goos := range []string{"darwin", "linux"} {
		platform := testScopePlatform(goos, nil)
		want := []ScopeSpec{{SourceDir: "System", TargetRoot: "/"}, {SourceDir: "Home", TargetRoot: platform.home}}
		if got := definedScopes(platform, nil); !slices.Equal(got, want) {
			t.Errorf("%s: definedScopes = %v, want %v", goos, got, want)
		}
	}
}

// TestDefinedScopes_Windows proves Windows defines all five builtins in scope order: System beneath the system
// drive's root, Home, then each Windows scope beneath the folder its environment names.
//
// Parameters:
//   - `t`: the test harness.
func TestDefinedScopes_Windows(t *testing.T) {

	platform := testScopePlatform("windows", windowsEnvironment())
	want := []ScopeSpec{
		{SourceDir: "System", TargetRoot: `C:\`},
		{SourceDir: "Home", TargetRoot: platform.home},
		{SourceDir: "ProgramData", TargetRoot: `C:\ProgramData`},
		{SourceDir: "ProgramFiles", TargetRoot: `C:\Program Files`},
		{SourceDir: "ProgramFilesX86", TargetRoot: `C:\Program Files (x86)`},
	}
	if got := definedScopes(platform, nil); !slices.Equal(got, want) {
		t.Errorf("definedScopes = %v, want %v", got, want)
	}
}

// TestDefinedScopes_WindowsFolderUnnamed proves a Windows scope whose folder the environment does not name is not
// defined there.
//
// Parameters:
//   - `t`: the test harness.
func TestDefinedScopes_WindowsFolderUnnamed(t *testing.T) {

	environment := windowsEnvironment()
	delete(environment, "ProgramFiles(x86)")

	got := definedScopes(testScopePlatform("windows", environment), nil)
	if slices.ContainsFunc(got, func(scope ScopeSpec) bool { return scope.SourceDir == "ProgramFilesX86" }) {
		t.Errorf("ProgramFilesX86 is defined with no folder named: %v", got)
	}
	if len(got) != 4 {
		t.Errorf("got %d scopes, want the other 4: %v", len(got), got)
	}
}

// TestDefinedScopes_RelocatedAndCustom proves a builtin's key relocates the builtin, and every other key adds a custom
// scope after the builtins: alphabetically, named in lower case, its key matched without case.
//
// Parameters:
//   - `t`: the test harness.
func TestDefinedScopes_RelocatedAndCustom(t *testing.T) {

	platform := testScopePlatform("linux", nil)
	alpha, staging, system := t.TempDir(), t.TempDir(), t.TempDir()

	got := definedScopes(platform, map[string]string{"System": system, "Staging": staging, "alpha": alpha})
	want := []ScopeSpec{
		{SourceDir: "System", TargetRoot: system},
		{SourceDir: "Home", TargetRoot: platform.home},
		{SourceDir: "alpha", TargetRoot: alpha},
		{SourceDir: "staging", TargetRoot: staging},
	}
	if !slices.Equal(got, want) {
		t.Errorf("definedScopes = %v, want %v", got, want)
	}
}

// TestSelectScopes_NoneSelectsEvery proves a run that names no scope covers every scope defined here, in scope order.
//
// Parameters:
//   - `t`: the test harness.
func TestSelectScopes_NoneSelectsEvery(t *testing.T) {

	platform := testScopePlatform("windows", windowsEnvironment())
	defined := definedScopes(platform, nil)

	got, err := selectScopes(platform, defined, nil)
	if err != nil {
		t.Fatalf("selectScopes: %v", err)
	}
	if !slices.Equal(got, defined) {
		t.Errorf("selectScopes = %v, want every defined scope %v", got, defined)
	}
}

// TestSelectScopes_NamedWithoutCase proves names select scopes without case, in scope order and each once.
//
// Parameters:
//   - `t`: the test harness.
func TestSelectScopes_NamedWithoutCase(t *testing.T) {

	platform := testScopePlatform("linux", nil)
	defined := definedScopes(platform, nil)

	got, err := selectScopes(platform, defined, []string{"home", "SYSTEM", "Home"})
	if err != nil {
		t.Fatalf("selectScopes: %v", err)
	}
	if !slices.Equal(got, defined) {
		t.Errorf("selectScopes = %v, want System then Home, each once: %v", got, defined)
	}
}

// TestSelectScopes_RefusesUnknown proves a name that is no scope is a usage error naming it and the scopes defined.
//
// Parameters:
//   - `t`: the test harness.
func TestSelectScopes_RefusesUnknown(t *testing.T) {

	platform := testScopePlatform("linux", nil)

	_, err := selectScopes(platform, definedScopes(platform, nil), []string{"Bogus"})
	if code := cli.ExitCode(err); code != cli.ExitUsage {
		t.Fatalf("ExitCode = %d, want %d (EX_USAGE): %v", code, cli.ExitUsage, err)
	}
	if !strings.Contains(err.Error(), "Bogus") || !strings.Contains(err.Error(), "System, Home") {
		t.Errorf("the refusal names neither the scope nor the scopes defined: %v", err)
	}
}

// TestSelectScopes_RefusesUndefinedBuiltin proves a builtin this platform does not define is a usage error naming the
// platform.
//
// Parameters:
//   - `t`: the test harness.
func TestSelectScopes_RefusesUndefinedBuiltin(t *testing.T) {

	platform := testScopePlatform("linux", nil)

	_, err := selectScopes(platform, definedScopes(platform, nil), []string{"programfiles"})
	if code := cli.ExitCode(err); code != cli.ExitUsage {
		t.Fatalf("ExitCode = %d, want %d (EX_USAGE): %v", code, cli.ExitUsage, err)
	}
	if !strings.Contains(err.Error(), "ProgramFiles is not defined on linux") {
		t.Errorf("the refusal does not say ProgramFiles is undefined on linux: %v", err)
	}
}

// TestRefuseScopeConfiguration_UndefinedBuiltin proves a key naming a builtin this platform does not define is a
// configuration error on Unix, and relocates the scope on Windows.
//
// Parameters:
//   - `t`: the test harness.
func TestRefuseScopeConfiguration_UndefinedBuiltin(t *testing.T) {

	configured := map[string]string{"ProgramFiles": `D:\Programs`}

	err := refuseScopeConfiguration(testScopePlatform("linux", nil), configured)
	if code := cli.ExitCode(err); code != cli.ExitConfig {
		t.Fatalf("linux: ExitCode = %d, want %d (EX_CONFIG): %v", code, cli.ExitConfig, err)
	}
	if !strings.Contains(err.Error(), "ProgramFiles") {
		t.Errorf("linux: the refusal does not name ProgramFiles: %v", err)
	}

	windows := testScopePlatform("windows", windowsEnvironment())
	if err := refuseScopeConfiguration(windows, configured); err != nil {
		t.Errorf("windows: a key relocating ProgramFiles was refused: %v", err)
	}
	if got := scopeRoot(windows, configured, "ProgramFiles"); got != `D:\Programs` {
		t.Errorf("windows: ProgramFiles is beneath %q, want the relocated D:\\Programs", got)
	}
}

// TestRefuseScopeConfiguration_NoRoot proves a scope with no root is a configuration error.
//
// Parameters:
//   - `t`: the test harness.
func TestRefuseScopeConfiguration_NoRoot(t *testing.T) {

	platform := testScopePlatform("linux", nil)

	err := refuseScopeConfiguration(platform, map[string]string{"staging": ""})
	if code := cli.ExitCode(err); code != cli.ExitConfig {
		t.Fatalf("ExitCode = %d, want %d (EX_CONFIG): %v", code, cli.ExitConfig, err)
	}
	if !strings.Contains(err.Error(), "writ.scopes.staging") {
		t.Errorf("the refusal does not name writ.scopes.staging: %v", err)
	}
	if err := refuseScopeConfiguration(platform, map[string]string{"staging": "/srv/staging"}); err != nil {
		t.Errorf("a custom scope with a root was refused: %v", err)
	}
}

// TestScopeDirectory_CustomWithoutCase proves a custom scope's directory is found without case, since its name comes
// from a configuration key in lower case, and a builtin's by its own name.
//
// Parameters:
//   - `t`: the test harness.
func TestScopeDirectory_CustomWithoutCase(t *testing.T) {

	layer := t.TempDir()
	for _, name := range []string{"Staging", "System"} {
		if err := os.Mkdir(filepath.Join(layer, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if path, found := scopeDirectory(layer, "staging"); !found || path != filepath.Join(layer, "Staging") {
		t.Errorf("scopeDirectory(staging) = %q, %t; want %q", path, found, filepath.Join(layer, "Staging"))
	}
	if path, found := scopeDirectory(layer, "System"); !found || path != filepath.Join(layer, "System") {
		t.Errorf("scopeDirectory(System) = %q, %t; want %q", path, found, filepath.Join(layer, "System"))
	}
	if _, found := scopeDirectory(layer, "absent"); found {
		t.Error("scopeDirectory found a scope the layer does not hold")
	}
}

// --- helpers ---

// testScopePlatform returns a platform for a scope test: the operating system named, the environment given, and a
// fixed home directory.
//
// Parameters:
//   - `goos`: the operating system, as `runtime.GOOS` names it.
//   - `environment`: the environment variables the platform has; nil for none.
//
// Returns:
//   - `scopePlatform`: the platform.
func testScopePlatform(goos string, environment map[string]string) scopePlatform {
	return scopePlatform{
		goos:   goos,
		getenv: func(name string) string { return environment[name] },
		home:   "/home/scope-test",
	}
}

// windowsEnvironment returns the environment a stock Windows install gives its known folders.
//
// Returns:
//   - `map[string]string`: `SystemDrive`, `ProgramData`, `ProgramFiles` and `ProgramFiles(x86)`.
func windowsEnvironment() map[string]string {
	return map[string]string{
		"ProgramData":       `C:\ProgramData`,
		"ProgramFiles":      `C:\Program Files`,
		"ProgramFiles(x86)": `C:\Program Files (x86)`,
		"SystemDrive":       "C:",
	}
}
