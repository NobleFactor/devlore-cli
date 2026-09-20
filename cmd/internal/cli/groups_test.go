// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// region Tests

// TestOperandsOf_TellsAFlagFromAVerb is the part of #897 most easily got wrong.
//
// `writ repo --output json` must reach help: `json` is a flag's value, not a verb. `writ repo add` must be
// refused. Cobra decides this with `stripFlags`, which is unexported, so this is our copy of the rule and this
// table is what keeps the two in step.
func TestOperandsOf_TellsAFlagFromAVerb(t *testing.T) {

	command := &cobra.Command{Use: "noun"}
	command.Flags().String("output", "json", "a flag that takes a value")
	command.Flags().StringP("store", "s", "", "a flag with a shorthand")
	command.Flags().Bool("silent", false, "a flag that takes none")

	for _, testCase := range []struct {
		name      string
		arguments []string
		want      []string
	}{
		{"nothing", nil, nil},
		{"a verb", []string{"add"}, []string{"add"}},
		{"a verb and its arguments", []string{"add", "team", "~/x"}, []string{"add", "team", "~/x"}},
		{"a flag and its value", []string{"--output", "json"}, nil},
		{"a flag with an equals", []string{"--output=json"}, nil},
		{"a shorthand and its value", []string{"-s", "/tmp/store"}, nil},
		{"a boolean flag", []string{"--silent"}, nil},
		{"a boolean flag then a verb", []string{"--silent", "add"}, []string{"add"}},
		{"a flag, its value, then a verb", []string{"--output", "json", "add"}, []string{"add"}},
		{"a verb then a flag", []string{"add", "--silent"}, []string{"add"}},
		{"everything after the terminator", []string{"--", "--output", "add"}, []string{"--output", "add"}},
		{"a lone dash", []string{"-"}, []string{"-"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got := operandsOf(command, testCase.arguments)
			if strings.Join(got, " ") != strings.Join(testCase.want, " ") {
				t.Errorf("operandsOf(%q) = %q, want %q", testCase.arguments, got, testCase.want)
			}
		})
	}
}

// TestValidateCommandLine_RefusesAVerbAGroupDoesNotHave walks every shape the pre-flight must judge.
//
// It refuses one thing and one only: an operand standing where a group's verb belongs. A leaf's arguments are
// [cobra.Command.ValidateArgs]'s business, the root's are cobra's own `legacyArgs`, and a bare group is the
// noun invoked bare, which §14's invariant 7 answers with help.
func TestValidateCommandLine_RefusesAVerbAGroupDoesNotHave(t *testing.T) {

	build := func() *cobra.Command {
		root := &cobra.Command{Use: "probe"}
		group := &cobra.Command{Use: "noun"}
		group.PersistentFlags().String("output", "json", "a flag that takes a value")
		group.AddCommand(&cobra.Command{
			Use:  "verb",
			Args: cobra.ArbitraryArgs,
			RunE: func(*cobra.Command, []string) error { return nil },
		})
		root.AddCommand(group)
		root.AddCommand(&cobra.Command{Use: "leaf", Args: cobra.ArbitraryArgs, RunE: func(*cobra.Command, []string) error { return nil }})
		return root
	}

	for _, testCase := range []struct {
		name     string
		argv     []string
		refused  bool
		mentions string
	}{
		{name: "a bare group prints help", argv: []string{"noun"}},
		{name: "a group and a flag", argv: []string{"noun", "--output", "yaml"}},
		{name: "a known verb", argv: []string{"noun", "verb"}},
		{name: "a known verb with arguments", argv: []string{"noun", "verb", "anything"}},
		{name: "a leaf with arguments", argv: []string{"leaf", "anything"}},
		{name: "an unknown verb at the root", argv: []string{"nosuchnoun"}},
		{name: "nothing at all", argv: nil},
		{name: "an unknown verb", argv: []string{"noun", "add"}, refused: true, mentions: `"add"`},
		{
			name:     "an unknown verb after a flag",
			argv:     []string{"noun", "--output", "yaml", "add"},
			refused:  true,
			mentions: `"add"`,
		},
		{name: "an unknown verb with arguments", argv: []string{"noun", "add", "team"}, refused: true, mentions: `"add"`},
		{
			name:     "a near miss suggests the verb",
			argv:     []string{"noun", "vreb"},
			refused:  true,
			mentions: "Did you mean this?\n\tverb",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {

			err := ValidateCommandLine(build(), testCase.argv)

			switch {
			case testCase.refused && err == nil:
				t.Fatalf("%q was accepted", testCase.argv)
			case !testCase.refused && err != nil:
				t.Fatalf("%q was refused: %v", testCase.argv, err)
			case !testCase.refused:
				return
			}

			if !strings.Contains(err.Error(), testCase.mentions) {
				t.Errorf("the refusal does not name %s: %v", testCase.mentions, err)
			}
			if !strings.Contains(err.Error(), "probe noun") {
				t.Errorf("the refusal does not name the group: %v", err)
			}
			if code := ExitCode(err); code != ExitUsage {
				t.Errorf("a refused verb exits %d, want %d", code, ExitUsage)
			}
		})
	}
}

// TestValidateCommandLine_LeavesTheRealTreesAlone guards against a pre-flight that refuses what works.
//
// Every command line in a program's own help must survive it: the examples are what a reader types first.
func TestValidateCommandLine_LeavesTheRealTreesAlone(t *testing.T) {

	root := sharedRoot()
	group := &cobra.Command{Use: "noun"}
	group.AddCommand(&cobra.Command{Use: "verb", RunE: func(*cobra.Command, []string) error { return nil }})
	root.AddCommand(group)

	for _, argv := range [][]string{
		{"--help"},
		{"noun"},
		{"noun", "--help"},
		{"noun", "verb"},
		{"noun", "verb", "--output", "yaml"},
		{"version"},
	} {
		if err := ValidateCommandLine(root, argv); err != nil {
			t.Errorf("%q was refused: %v", argv, err)
		}
	}
}

// endregion
