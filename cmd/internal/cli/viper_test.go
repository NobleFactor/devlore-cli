// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// mappingCases are settings and the variables they read from, for every program on the shared root: a plain key, a
// nested key, a hyphenated key, a key without a section, and another program's section kept.
var mappingCases = []struct {
	program  string
	key      string
	variable string
}{
	{program: "devlore-test", key: "devlore-test.verbose", variable: "DEVLORE_TEST_VERBOSE"},
	{program: "lore", key: "lore.registry", variable: "LORE_REGISTRY"},
	{program: "star", key: "star.model-endpoint", variable: "STAR_MODEL_ENDPOINT"},
	{program: "writ", key: "lore.dry-run", variable: "WRIT_LORE_DRY_RUN"},
	{program: "writ", key: "pager", variable: "WRIT_PAGER"},
	{program: "writ", key: "writ.deploy.conflict", variable: "WRIT_DEPLOY_CONFLICT"},
	{program: "writ", key: "writ.dry-run", variable: "WRIT_DRY_RUN"},
	{program: "writ", key: "writ.repo", variable: "WRIT_REPO"},
}

// settingsFor returns a fresh viper instance that reads the environment as [InitViper] sets it up for `program`.
//
// Parameters:
//   - `t`: the test the instance serves.
//   - `program`: the program's name, such as "writ".
//
// Returns:
//   - `*viper.Viper`: the instance, touching none of the process-wide settings.
func settingsFor(t *testing.T, program string) *viper.Viper {

	t.Helper()
	settings := viper.New()
	bindEnvironment(settings, EnvironmentPrefix(program), program)
	return settings
}

// --- bindEnvironment ---

// TestBindEnvironment_ReadsThePrefixPlusTheBareKey reads each setting from the program's prefix plus its key without
// the program's own segment, dots and hyphens as underscores.
func TestBindEnvironment_ReadsThePrefixPlusTheBareKey(t *testing.T) {

	for _, testCase := range mappingCases {
		t.Run(testCase.variable, func(t *testing.T) {

			t.Setenv(testCase.variable, "from the environment")

			settings := settingsFor(t, testCase.program)
			if got := settings.GetString(testCase.key); got != "from the environment" {
				t.Errorf("%s under %s = %q, want the value of %s", testCase.key, testCase.program, got,
					testCase.variable)
			}
		})
	}
}

// TestBindEnvironment_IgnoresTheDoubledSpelling reads nothing from the variable that repeats the program's name: it
// names no setting.
func TestBindEnvironment_IgnoresTheDoubledSpelling(t *testing.T) {

	cases := []struct {
		program  string
		key      string
		variable string
	}{
		{program: "devlore-test", key: "devlore-test.verbose", variable: "DEVLORE_TEST_DEVLORE_TEST_VERBOSE"},
		{program: "lore", key: "lore.dry-run", variable: "LORE_LORE_DRY_RUN"},
		{program: "writ", key: "writ.repo", variable: "WRIT_WRIT_REPO"},
	}

	for _, testCase := range cases {
		t.Run(testCase.variable, func(t *testing.T) {

			t.Setenv(testCase.variable, "from the doubled spelling")

			settings := settingsFor(t, testCase.program)
			if got := settings.GetString(testCase.key); got != "" {
				t.Errorf("%s = %q from %s, want nothing", testCase.key, got, testCase.variable)
			}
		})
	}
}

// --- EnvironmentPrefix ---

// TestEnvironmentPrefix_TurnsAHyphenIntoAnUnderscore upper-cases a program's name, and turns the hyphen a shell
// variable cannot hold into `_`.
func TestEnvironmentPrefix_TurnsAHyphenIntoAnUnderscore(t *testing.T) {

	if got := EnvironmentPrefix("writ"); got != "WRIT" {
		t.Errorf(`EnvironmentPrefix("writ") = %q, want "WRIT"`, got)
	}
	if got := EnvironmentPrefix("devlore-test"); got != "DEVLORE_TEST" {
		t.Errorf(`EnvironmentPrefix("devlore-test") = %q, want "DEVLORE_TEST"`, got)
	}
}

// --- EnvironmentVariable ---

// TestEnvironmentVariable_NamesWhatViperReads names, for each case, the variable viper reads the setting from.
func TestEnvironmentVariable_NamesWhatViperReads(t *testing.T) {

	for _, testCase := range mappingCases {
		if got := EnvironmentVariable(testCase.program, testCase.key); got != testCase.variable {
			t.Errorf("EnvironmentVariable(%q, %q) = %q, want %q", testCase.program, testCase.key, got,
				testCase.variable)
		}
	}
}

// TestEnvironmentVariable_GivesAGlobalSettingTheSuitesPrefix names a global setting's variable under the suite's
// name, as `DEVLORE_<KEY>`.
func TestEnvironmentVariable_GivesAGlobalSettingTheSuitesPrefix(t *testing.T) {

	if got := EnvironmentVariable("devlore", "dry_run"); got != "DEVLORE_DRY_RUN" {
		t.Errorf(`EnvironmentVariable("devlore", "dry_run") = %q, want "DEVLORE_DRY_RUN"`, got)
	}
	if got := EnvironmentVariable("devlore", "model.api_key"); got != "DEVLORE_MODEL_API_KEY" {
		t.Errorf(`EnvironmentVariable("devlore", "model.api_key") = %q, want "DEVLORE_MODEL_API_KEY"`, got)
	}
}

// --- InitViper ---

// TestInitViper_DerivesThePrefixFromTheName reads devlore-test's settings from `DEVLORE_TEST_`, the prefix derived
// from a name with a hyphen, when the caller names no prefix.
func TestInitViper_DerivesThePrefixFromTheName(t *testing.T) {

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DEVLORE_TEST_DRY_RUN", "true")
	t.Cleanup(viper.Reset)

	if err := InitViper(ViperConfig{Name: "devlore-test", UseSharedConfig: true}); err != nil {
		t.Fatalf("InitViper: %v", err)
	}
	if !viper.GetBool("devlore-test.dry-run") {
		t.Errorf("devlore-test.dry-run is false, want DEVLORE_TEST_DRY_RUN's true")
	}
}

// TestInitViper_RequiresAName refuses a configuration without a program name, naming the field.
func TestInitViper_RequiresAName(t *testing.T) {

	err := InitViper(ViperConfig{})
	if err == nil {
		t.Fatal("InitViper(ViperConfig{}) succeeded, want a refusal")
	}
	if !strings.Contains(err.Error(), "ViperConfig.Name") {
		t.Errorf("InitViper(ViperConfig{}) = %q, want it to name ViperConfig.Name", err)
	}
}
