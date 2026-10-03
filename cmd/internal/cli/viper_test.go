// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// settingsFor returns a fresh viper instance that reads the environment as [InitViper] sets it up for `program`.
//
// Parameters:
//   - `t`: the test the instance serves.
//   - `prefix`: the program's environment prefix, such as "WRIT".
//   - `program`: the program's name, such as "writ".
//
// Returns:
//   - `*viper.Viper`: the instance, touching none of the process-wide settings.
func settingsFor(t *testing.T, prefix, program string) *viper.Viper {

	t.Helper()
	settings := viper.New()
	bindEnvironment(settings, prefix, program)
	return settings
}

// --- bindEnvironment ---

// TestBindEnvironment_ReadsThePrefixPlusTheBareKey reads each setting from the program's prefix plus its key without
// the program's own segment, dots and hyphens as underscores, for every program on the shared root.
func TestBindEnvironment_ReadsThePrefixPlusTheBareKey(t *testing.T) {

	cases := []struct {
		prefix   string
		program  string
		key      string
		variable string
	}{
		{prefix: "WRIT", program: "writ", key: "writ.repo", variable: "WRIT_REPO"},
		{prefix: "WRIT", program: "writ", key: "writ.deploy.conflict", variable: "WRIT_DEPLOY_CONFLICT"},
		{prefix: "WRIT", program: "writ", key: "writ.dry-run", variable: "WRIT_DRY_RUN"},
		{prefix: "WRIT", program: "writ", key: "pager", variable: "WRIT_PAGER"},
		{prefix: "LORE", program: "lore", key: "lore.registry", variable: "LORE_REGISTRY"},
		{prefix: "STAR", program: "star", key: "star.model-endpoint", variable: "STAR_MODEL_ENDPOINT"},
		{prefix: "DEVLORE_TEST", program: "devlore-test", key: "devlore-test.verbose",
			variable: "DEVLORE_TEST_VERBOSE"},
	}

	for _, testCase := range cases {
		t.Run(testCase.variable, func(t *testing.T) {

			t.Setenv(testCase.variable, "from the environment")

			settings := settingsFor(t, testCase.prefix, testCase.program)
			if got := settings.GetString(testCase.key); got != "from the environment" {
				t.Errorf("%s = %q, want the value of %s", testCase.key, got, testCase.variable)
			}
		})
	}
}

// TestBindEnvironment_IgnoresTheDoubledSpelling reads nothing from the variable that repeats the program's name: it
// names no setting.
func TestBindEnvironment_IgnoresTheDoubledSpelling(t *testing.T) {

	cases := []struct {
		prefix   string
		program  string
		key      string
		variable string
	}{
		{prefix: "WRIT", program: "writ", key: "writ.repo", variable: "WRIT_WRIT_REPO"},
		{prefix: "LORE", program: "lore", key: "lore.dry-run", variable: "LORE_LORE_DRY_RUN"},
		{prefix: "DEVLORE_TEST", program: "devlore-test", key: "devlore-test.verbose",
			variable: "DEVLORE_TEST_DEVLORE_TEST_VERBOSE"},
	}

	for _, testCase := range cases {
		t.Run(testCase.variable, func(t *testing.T) {

			t.Setenv(testCase.variable, "from the doubled spelling")

			settings := settingsFor(t, testCase.prefix, testCase.program)
			if got := settings.GetString(testCase.key); got != "" {
				t.Errorf("%s = %q from %s, want nothing", testCase.key, got, testCase.variable)
			}
		})
	}
}

// TestBindEnvironment_KeepsAnotherProgramsSegment drops only the running program's own segment: a key under another
// program's name keeps it, under the running program's prefix.
func TestBindEnvironment_KeepsAnotherProgramsSegment(t *testing.T) {

	t.Setenv("WRIT_LORE_DRY_RUN", "from the environment")

	settings := settingsFor(t, "WRIT", "writ")
	if got := settings.GetString("lore.dry-run"); got != "from the environment" {
		t.Errorf("lore.dry-run under writ = %q, want the value of WRIT_LORE_DRY_RUN", got)
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
