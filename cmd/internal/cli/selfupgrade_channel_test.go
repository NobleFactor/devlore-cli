// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/schema"
)

// stampedAs returns the inputs of a run in which only the build's stamp speaks.
//
// No archive, no flag, no pin and no setting: every precedence case starts here and adds the source it is about.
//
// Parameters:
//   - `channel`: the stamped channel; "" for a local build.
//   - `prerelease`: the stamped pre-release switch.
//
// Returns:
//   - `upgradeInputs`: the inputs.
func stampedAs(channel string, prerelease bool) upgradeInputs {
	return upgradeInputs{stampedChannel: channel, stampedPrerelease: prerelease}
}

// mustResolve resolves the inputs and fails the test on an error.
//
// Parameters:
//   - `t`: the test.
//   - `inputs`: the inputs to resolve.
//
// Returns:
//   - `upgradeTarget`: what the run upgrades to.
func mustResolve(t *testing.T, inputs upgradeInputs) upgradeTarget {
	t.Helper()

	target, err := resolveUpgradeTarget(inputs)
	if err != nil {
		t.Fatalf("resolveUpgradeTarget(%+v) error = %v", inputs, err)
	}

	return target
}

// gatherFrom gathers the inputs `self upgrade` reads for a stamped build and a command line.
//
// It builds the command for a build stamped with the channel and switch, parses the arguments into its flags, and
// gathers the inputs from it, as the command itself would.
//
// Parameters:
//   - `t`: the test.
//   - `channel`: the stamped channel.
//   - `prerelease`: the stamped pre-release switch.
//   - `args`: the command line after `self upgrade`.
//
// Returns:
//   - `upgradeInputs`: what the command reads.
func gatherFrom(t *testing.T, channel string, prerelease bool, args ...string) upgradeInputs {
	t.Helper()

	info := SelfInstallInfo{Name: "probe", Version: "1.2.3", Channel: channel, Prerelease: prerelease}
	cmd := newUpgradeCmd(&cobra.Command{Use: "probe"}, info)

	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("ParseFlags(%v) error = %v", args, err)
	}

	return gatherUpgradeInputs(cmd, info)
}

// withSettings sets self.channel and self.prerelease in viper for the length of the test.
//
// Both are always set, the empty channel and the false switch included, so a value in the configuration the process
// loaded cannot reach the test; the empty channel is what an unset one reads as.
//
// Parameters:
//   - `t`: the test.
//   - `channel`: self.channel's value; "" for unset.
//   - `prerelease`: self.prerelease's value; false for unset.
func withSettings(t *testing.T, channel string, prerelease bool) {
	t.Helper()

	viper.Set(selfChannelKey, channel)
	viper.Set(selfPrereleaseKey, prerelease)
	t.Cleanup(func() {
		viper.Set(selfChannelKey, nil)
		viper.Set(selfPrereleaseKey, nil)
	})
}

// --- resolveUpgradeTarget: precedence ---

// TestResolveUpgradeTarget_FromWinsOverEverything pins rung 1: an archive decides, and no other source is read.
func TestResolveUpgradeTarget_FromWinsOverEverything(t *testing.T) {

	inputs := stampedAs(channelDevelop, true)
	inputs.from = true
	inputs.channelFlag, inputs.channelFlagGiven = channelRelease, true
	inputs.pin = "v0.1.0-dev.20260930000000"
	inputs.channelSetting = channelRelease

	target := mustResolve(t, inputs)

	if target.mode != upgradeModeArchive {
		t.Errorf("mode = %q; want %q", target.mode, upgradeModeArchive)
	}
	if target.channel != "" || target.tag != "" {
		t.Errorf("target = %+v; an archive resolves no channel and no tag", target)
	}
}

// TestResolveUpgradeTarget_ChannelFlagWinsOverPinSettingAndStamp pins rung 2, where a pin is set aside with a note.
//
// A flag always wins, and the run says what it ignored.
func TestResolveUpgradeTarget_ChannelFlagWinsOverPinSettingAndStamp(t *testing.T) {

	inputs := stampedAs(channelDevelop, true)
	inputs.channelFlag, inputs.channelFlagGiven = channelRelease, true
	inputs.pin = "v0.1.0-dev.20260930000000"
	inputs.channelSetting = channelDevelop

	target := mustResolve(t, inputs)

	if target.mode != upgradeModeChannel || target.channel != channelRelease {
		t.Errorf("target = %+v; want the release channel", target)
	}
	if target.tag != "" {
		t.Errorf("tag = %q; the pin is ignored beside --channel", target.tag)
	}
	for _, word := range []string{"DEVLORE_VERSION", "v0.1.0-dev.20260930000000", "--channel"} {
		if !strings.Contains(target.note, word) {
			t.Errorf("note = %q; want it to name %s", target.note, word)
		}
	}
}

// TestResolveUpgradeTarget_ChannelFlagWithoutPinSaysNothing pins that the note is only for a pin set aside.
func TestResolveUpgradeTarget_ChannelFlagWithoutPinSaysNothing(t *testing.T) {

	for _, pin := range []string{"", pinLatest} {
		inputs := stampedAs(channelDevelop, true)
		inputs.channelFlag, inputs.channelFlagGiven = channelRelease, true
		inputs.pin = pin

		if target := mustResolve(t, inputs); target.note != "" {
			t.Errorf("DEVLORE_VERSION=%q: note = %q; want none, since no pin was set aside", pin, target.note)
		}
	}
}

// TestResolveUpgradeTarget_PinWinsOverSettingAndStamp pins rung 3: that exact release, whatever its channel.
func TestResolveUpgradeTarget_PinWinsOverSettingAndStamp(t *testing.T) {

	inputs := stampedAs(channelDevelop, true)
	inputs.pin = "v1.2.3"
	inputs.channelSetting = channelRelease

	target := mustResolve(t, inputs)

	if target.mode != upgradeModePin || target.tag != "v1.2.3" {
		t.Errorf("target = %+v; want the pin v1.2.3", target)
	}
	if target.channel != "" || target.note != "" {
		t.Errorf("target = %+v; a pin resolves no channel and sets nothing aside", target)
	}
}

// TestResolveUpgradeTarget_PrereleaseBesideAPinSaysSo pins the note for a --prerelease a pin leaves with no effect.
//
// A pin names one release, so there is nothing for the switch to add; the operator is told, not ignored.
func TestResolveUpgradeTarget_PrereleaseBesideAPinSaysSo(t *testing.T) {

	inputs := stampedAs(channelRelease, false)
	inputs.pin = "v1.2.3"
	inputs.prereleaseFlag, inputs.prereleaseFlagGiven = true, true

	target := mustResolve(t, inputs)

	if target.mode != upgradeModePin || target.tag != "v1.2.3" {
		t.Errorf("target = %+v; want the pin v1.2.3", target)
	}
	for _, word := range []string{"--prerelease", pinVariable, "v1.2.3"} {
		if !strings.Contains(target.note, word) {
			t.Errorf("note = %q; want it to name %s", target.note, word)
		}
	}
}

// TestResolveUpgradeTarget_LatestAndEmptyAreNoPin pins that `latest`, the installers' default, and "" pin nothing.
func TestResolveUpgradeTarget_LatestAndEmptyAreNoPin(t *testing.T) {

	for _, pin := range []string{"", pinLatest} {
		inputs := stampedAs(channelRelease, false)
		inputs.pin = pin

		if target := mustResolve(t, inputs); target.mode != upgradeModeChannel || target.channel != channelRelease {
			t.Errorf("DEVLORE_VERSION=%q: target = %+v; want the stamped channel", pin, target)
		}
	}
}

// TestResolveUpgradeTarget_SettingWinsOverStamp pins rung 4: the standing choice outranks the build's own channel.
func TestResolveUpgradeTarget_SettingWinsOverStamp(t *testing.T) {

	inputs := stampedAs(channelDevelop, true)
	inputs.channelSetting = channelRelease

	if target := mustResolve(t, inputs); target.mode != upgradeModeChannel || target.channel != channelRelease {
		t.Errorf("target = %+v; want self.channel's release", target)
	}
}

// TestResolveUpgradeTarget_StampDecidesAlone pins rung 5: with nothing else said, a build keeps its own channel.
func TestResolveUpgradeTarget_StampDecidesAlone(t *testing.T) {

	for _, channel := range []string{channelDevelop, channelRelease} {
		if target := mustResolve(t, stampedAs(channel, false)); target.mode != upgradeModeChannel ||
			target.channel != channel {
			t.Errorf("stamped %s: target = %+v; want the stamped channel", channel, target)
		}
	}
}

// TestResolveUpgradeTarget_RefusesWhenNothingDecides pins D6: with nothing to decide, the command refuses.
//
// A local build has no channel, so with no archive, no channel and no pin the refusal names the two ways to give one.
func TestResolveUpgradeTarget_RefusesWhenNothingDecides(t *testing.T) {

	inputs := stampedAs("", true)
	inputs.prereleaseFlag, inputs.prereleaseFlagGiven = true, true
	inputs.prereleaseSetting = true
	inputs.pin = pinLatest

	_, err := resolveUpgradeTarget(inputs)
	if err == nil {
		t.Fatal("resolveUpgradeTarget() error = nil; want a refusal")
	}
	for _, word := range []string{"--channel", "self.channel"} {
		if !strings.Contains(err.Error(), word) {
			t.Errorf("error = %q; want it to name %s", err, word)
		}
	}
	if code := ExitCode(err); code != ExitUsage {
		t.Errorf("ExitCode = %d; want %d (EX_USAGE)", code, ExitUsage)
	}
}

// --- resolveUpgradeTarget: channel values ---

// TestResolveUpgradeTarget_RefusesAnyOtherChannel pins that a channel is develop or release, whatever gives it.
//
// The refusal names both channels along with the source that gave the value.
func TestResolveUpgradeTarget_RefusesAnyOtherChannel(t *testing.T) {

	flag := stampedAs(channelRelease, false)
	flag.channelFlag, flag.channelFlagGiven = "main", true

	empty := stampedAs(channelRelease, false)
	empty.channelFlagGiven = true

	setting := stampedAs(channelRelease, false)
	setting.channelSetting = "stable"

	cases := []struct {
		name   string
		inputs upgradeInputs
		source string
		code   int
	}{
		{"flag", flag, "--channel", ExitUsage},
		{"empty flag", empty, "--channel", ExitUsage},
		{"setting", setting, "self.channel", ExitConfig},
		{"stamp", stampedAs("preview", false), "stamped channel", ExitSoftware},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {

			_, err := resolveUpgradeTarget(testCase.inputs)
			if err == nil {
				t.Fatal("resolveUpgradeTarget() error = nil; want a refusal")
			}
			for _, word := range []string{channelDevelop, channelRelease, testCase.source} {
				if !strings.Contains(err.Error(), word) {
					t.Errorf("error = %q; want it to name %s", err, word)
				}
			}
			if code := ExitCode(err); code != testCase.code {
				t.Errorf("ExitCode = %d; want %d", code, testCase.code)
			}
		})
	}
}

// TestResolveUpgradeTarget_OnlyTheDecidingChannelIsChecked pins that a setting a flag outranks is not checked.
//
// A flag always wins, so a bad value in the setting does not stop the run the flag asked for.
func TestResolveUpgradeTarget_OnlyTheDecidingChannelIsChecked(t *testing.T) {

	inputs := stampedAs("preview", false)
	inputs.channelFlag, inputs.channelFlagGiven = channelRelease, true
	inputs.channelSetting = "stable"

	if target := mustResolve(t, inputs); target.channel != channelRelease {
		t.Errorf("target = %+v; want --channel's release", target)
	}
}

// --- resolveUpgradeTarget: the pre-release switch ---

// TestResolveUpgradeTarget_PrereleaseComesWithTheChannel pins the pre-release rule, source by source.
//
// --prerelease given means its value; otherwise false with --channel, self.prerelease with self.channel, and the
// stamp's with the stamp's. Never assembled from two places.
func TestResolveUpgradeTarget_PrereleaseComesWithTheChannel(t *testing.T) {

	// Which sources speak: --channel or self.channel (else the stamp decides), the switch each carries, and
	// --prerelease given with its value.
	type sources struct {
		channelFlag, channelSetting, setting, stamp, flagGiven, flagValue bool
	}

	cases := []struct {
		name string
		in   sources
		want bool
	}{
		{"--channel alone, whatever setting and stamp", sources{channelFlag: true, setting: true, stamp: true}, false},
		{"--channel with --prerelease", sources{channelFlag: true, flagGiven: true, flagValue: true}, true},
		{"self.channel takes self.prerelease", sources{channelSetting: true, setting: true}, true},
		{"self.channel alone, whatever the stamp", sources{channelSetting: true, stamp: true}, false},
		{"self.channel with --prerelease", sources{channelSetting: true, flagGiven: true, flagValue: true}, true},
		{"the stamp takes its own switch", sources{stamp: true}, true},
		{"the stamp's switch, not self.prerelease", sources{setting: true}, false},
		{"the stamp with --prerelease", sources{flagGiven: true, flagValue: true}, true},
		{"--prerelease=false over the stamp", sources{stamp: true, flagGiven: true}, false},
		{"--prerelease=false over the setting", sources{channelSetting: true, setting: true, flagGiven: true}, false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {

			inputs := stampedAs(channelRelease, testCase.in.stamp)
			inputs.prereleaseSetting = testCase.in.setting
			inputs.prereleaseFlag, inputs.prereleaseFlagGiven = testCase.in.flagValue, testCase.in.flagGiven
			if testCase.in.channelFlag {
				inputs.channelFlag, inputs.channelFlagGiven = channelRelease, true
			}
			if testCase.in.channelSetting {
				inputs.channelSetting = channelRelease
			}

			if target := mustResolve(t, inputs); target.prerelease != testCase.want {
				t.Errorf("prerelease = %t; want %t (target %+v)", target.prerelease, testCase.want, target)
			}
		})
	}
}

// TestResolveUpgradeTarget_DevelopIsAlwaysPrerelease pins that develop takes pre-releases from every source.
//
// Every develop build is a pre-release, so the switch means something only on release.
func TestResolveUpgradeTarget_DevelopIsAlwaysPrerelease(t *testing.T) {

	fromFlag := stampedAs(channelRelease, false)
	fromFlag.channelFlag, fromFlag.channelFlagGiven = channelDevelop, true
	fromFlag.prereleaseFlagGiven = true

	fromSetting := stampedAs(channelRelease, false)
	fromSetting.channelSetting = channelDevelop

	for name, inputs := range map[string]upgradeInputs{
		"--channel develop --prerelease=false": fromFlag,
		"self.channel develop":                 fromSetting,
		"stamped develop, switch false":        stampedAs(channelDevelop, false),
	} {
		if target := mustResolve(t, inputs); !target.prerelease {
			t.Errorf("%s: prerelease = false; every develop build is a pre-release", name)
		}
	}
}

// --- gatherUpgradeInputs ---

// TestGatherUpgradeInputs_ReadsEverySource pins what `self upgrade` reads, from each source.
//
// The flags as given, DEVLORE_VERSION, the two shared settings through viper, and the build's stamp from the
// command's info.
func TestGatherUpgradeInputs_ReadsEverySource(t *testing.T) {

	t.Setenv(pinVariable, "v1.2.3")
	withSettings(t, channelDevelop, true)

	got := gatherFrom(t, channelRelease, true, "--channel", "develop", "--prerelease")

	want := upgradeInputs{
		channelFlag:         channelDevelop,
		channelFlagGiven:    true,
		prereleaseFlag:      true,
		prereleaseFlagGiven: true,
		pin:                 "v1.2.3",
		channelSetting:      channelDevelop,
		prereleaseSetting:   true,
		stampedChannel:      channelRelease,
		stampedPrerelease:   true,
	}
	if got != want {
		t.Errorf("inputs = %+v; want %+v", got, want)
	}
}

// TestGatherUpgradeInputs_DefaultsAreNotGiven pins that a flag the command line did not name is not given.
//
// This is the precedence rule's premise: cobra holds a default for every flag, and an unset setting reads as unset.
func TestGatherUpgradeInputs_DefaultsAreNotGiven(t *testing.T) {

	t.Setenv(pinVariable, "")
	withSettings(t, "", false)

	got := gatherFrom(t, "", false)

	if got != (upgradeInputs{}) {
		t.Errorf("inputs = %+v; want nothing given, nothing set and nothing stamped", got)
	}
}

// TestGatherUpgradeInputs_FlagEqualToItsDefaultIsGiven pins that a flag named with its default value is given.
//
// `--prerelease=false` and `--channel ""` each equal their default. A flag always wins, including then
// (10-command-line-interface.md §11).
func TestGatherUpgradeInputs_FlagEqualToItsDefaultIsGiven(t *testing.T) {

	t.Setenv(pinVariable, "")
	withSettings(t, "", false)

	got := gatherFrom(t, "", false, "--prerelease=false", "--channel=")

	if !got.prereleaseFlagGiven || !got.channelFlagGiven {
		t.Errorf("inputs = %+v; want both flags given", got)
	}
}

// TestNewUpgradeCmd_CarriesChannelAndPrerelease pins the two flags on `self upgrade`, where the channel is chosen.
func TestNewUpgradeCmd_CarriesChannelAndPrerelease(t *testing.T) {

	cmd := newUpgradeCmd(&cobra.Command{Use: "probe"}, SelfInstallInfo{Name: "probe", Version: "1.2.3"})

	if flag := cmd.Flags().Lookup("channel"); flag == nil || flag.Value.Type() != "string" {
		t.Errorf("--channel = %+v; want a string flag", flag)
	}
	if flag := cmd.Flags().Lookup("prerelease"); flag == nil || flag.Value.Type() != "bool" {
		t.Errorf("--prerelease = %+v; want a bool flag", flag)
	}
}

// --- the settings in the schema ---

// TestSchema_DeclaresSelfChannelAndPrerelease pins self.channel as a string and self.prerelease as a boolean.
//
// `config set` coerces by the schema's type, so without them the setting the upgrade reads cannot be written.
func TestSchema_DeclaresSelfChannelAndPrerelease(t *testing.T) {

	channel, err := coerceValue(schema.DevloreSchema, selfChannelKey, channelRelease)
	if err != nil || channel != channelRelease {
		t.Errorf("coerceValue(%s) = %v, %v; want the string %q", selfChannelKey, channel, err, channelRelease)
	}

	prerelease, err := coerceValue(schema.DevloreSchema, selfPrereleaseKey, "true")
	if err != nil || prerelease != true {
		t.Errorf("coerceValue(%s) = %v, %v; want the boolean true", selfPrereleaseKey, prerelease, err)
	}
}

// TestSharedDefaults_SetNoSelfSetting pins that the shared defaults document the settings and set neither.
//
// The shared defaults become a new install's config.yaml, and a self.channel set there would outrank every build's
// stamp.
func TestSharedDefaults_SetNoSelfSetting(t *testing.T) {

	var defaults map[string]any
	if err := yaml.Unmarshal(schema.SharedDefaultConfig, &defaults); err != nil {
		t.Fatalf("the shared defaults do not parse: %v", err)
	}

	if _, set := defaults["self"]; set {
		t.Errorf("the shared defaults set self: %v; they may only document it", defaults["self"])
	}

	for _, key := range []string{selfChannelKey, selfPrereleaseKey} {
		if !strings.Contains(string(schema.SharedDefaultConfig), strings.TrimPrefix(key, "self.")+":") {
			t.Errorf("the shared defaults do not document %s", key)
		}
	}
}
