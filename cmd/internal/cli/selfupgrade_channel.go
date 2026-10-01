// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// What a `self upgrade` installs is decided by one source, first to last (#947): an archive, `--from`; a channel on
// the command line, `--channel`; a pinned release, DEVLORE_VERSION; the channel in the shared configuration,
// `self.channel`; and the channel stamped into the build. The pre-release switch comes with the channel that
// decides, never assembled from two places. The decision is pure, so every rung is proven without a network, a
// prefix or a terminal; finding and fetching the release it names is the upgrade's.

// channelDevelop and channelRelease are the two channels (ruled 2026-09-30). `develop` is every build from develop,
// each a pre-release; `release` is GitHub's latest release, and with the pre-releases switched on, the pre-releases
// built from main and `release/*` too.
const (
	channelDevelop = "develop"
	channelRelease = "release"
)

// pinVariable names the environment variable that pins one release, the one the installers and the release report
// already read; pinLatest is the value that pins nothing, the installers' default.
const (
	pinVariable = "DEVLORE_VERSION"
	pinLatest   = "latest"
)

// selfChannelKey and selfPrereleaseKey are the standing choice of a channel, in the configuration every program
// shares: the channel belongs to the suite, not to one program (D3).
const (
	selfChannelKey    = "self.channel"
	selfPrereleaseKey = "self.prerelease"
)

// upgradeModeArchive, upgradeModePin and upgradeModeChannel are what decides a run: a local archive, one pinned
// release, or the newest release on a channel.
const (
	upgradeModeArchive upgradeMode = "archive"
	upgradeModePin     upgradeMode = "pin"
	upgradeModeChannel upgradeMode = "channel"
)

// region SUPPORTING TYPES

// upgradeMode names what decides the release a `self upgrade` installs.
type upgradeMode string

// upgradeInputs is everything that can decide what a `self upgrade` installs, as the command line, the environment,
// the shared configuration and the build's stamp give it. Holding a value decides nothing by itself; which source
// decides is [resolveUpgradeTarget]'s to say.
type upgradeInputs struct {
	from                bool   // --from was given: an archive decides
	channelFlag         string // --channel's value; read only when channelFlagGiven
	channelFlagGiven    bool   // --channel was given, whatever its value
	prereleaseFlag      bool   // --prerelease's value; read only when prereleaseFlagGiven
	prereleaseFlagGiven bool   // --prerelease was given, whatever its value
	pin                 string // DEVLORE_VERSION as found; "" and "latest" pin nothing
	channelSetting      string // self.channel; "" when unset
	prereleaseSetting   bool   // self.prerelease; false when unset
	stampedChannel      string // the channel stamped into this build; "" in a local build
	stampedPrerelease   bool   // the pre-release switch stamped into this build
}

// upgradeTarget is what a `self upgrade` installs: a local archive, one pinned release, or the newest release on a
// channel, and what the operator is told about a source the decision set aside.
type upgradeTarget struct {
	mode       upgradeMode // what decides the run
	channel    string      // develop or release; "" unless mode is upgradeModeChannel
	prerelease bool        // whether the channel's pre-releases count; always true on develop
	tag        string      // the pinned release's tag; "" unless mode is upgradeModePin
	note       string      // a source set aside, said to the operator; "" when none was
}

// endregion

// region HELPER FUNCTIONS

// Fallible actions

// channelTarget returns the target of a run that follows a channel, refusing any channel but the two.
//
// Every develop build is a pre-release, so the switch means something only on `release`; on `develop` the target
// takes pre-releases whatever the switch says.
//
// Parameters:
//   - `channel`: the channel the deciding source gives.
//   - `source`: the deciding source as the operator knows it, for the refusal: `--channel`, `self.channel`, or the
//     build's stamp.
//   - `code`: the exit status of a refusal, which says whose the bad value is.
//   - `prerelease`: the pre-release switch that comes with the source.
//
// Returns:
//   - `upgradeTarget`: the newest release on the channel, with or without its pre-releases.
//   - `error`: an [ExitWith] error with `code`, naming the source and both channels, when the channel is neither.
func channelTarget(channel, source string, code int, prerelease bool) (upgradeTarget, error) {

	switch channel {
	case channelDevelop:
		return upgradeTarget{mode: upgradeModeChannel, channel: channelDevelop, prerelease: true}, nil
	case channelRelease:
		return upgradeTarget{mode: upgradeModeChannel, channel: channelRelease, prerelease: prerelease}, nil
	default:
		return upgradeTarget{}, ExitWith(code, fmt.Errorf("%s is %q: the channels are %s and %s",
			source, channel, channelDevelop, channelRelease))
	}
}

// resolveUpgradeTarget decides what a `self upgrade` installs, from the first source that speaks.
//
// First to last: `--from`, which needs no channel; `--channel`, beside which a pin is set aside with a note, because
// a flag always wins; DEVLORE_VERSION, that exact release whatever its channel; `self.channel`; and the build's
// stamped channel. The pre-release switch comes with the channel: `--prerelease` given means its value; otherwise it
// is false with `--channel`, `self.prerelease` with `self.channel`, and the stamp's with the stamp's. A pin names one
// release, so `--prerelease` beside it has no effect, and a note says so. Only the channel that decides is checked,
// so a bad value a flag outranks does not stop the run the flag asked for.
//
// Parameters:
//   - `inputs`: every source, as found.
//
// Returns:
//   - `upgradeTarget`: what the run installs.
//   - `error`: a channel that is neither `develop` nor `release`, exiting [ExitUsage] from `--channel`,
//     [ExitConfig] from `self.channel` and [ExitSoftware] from the stamp; or, when nothing decides, an [ExitUsage]
//     refusal naming `--channel` and `self.channel`.
func resolveUpgradeTarget(inputs upgradeInputs) (upgradeTarget, error) {

	if inputs.from {
		return upgradeTarget{mode: upgradeModeArchive}, nil
	}

	pinned := inputs.pin != "" && inputs.pin != pinLatest

	if inputs.channelFlagGiven {
		target, err := channelTarget(inputs.channelFlag, "--channel", ExitUsage, prereleaseOr(inputs, false))
		if err != nil {
			return upgradeTarget{}, err
		}
		if pinned {
			target.note = fmt.Sprintf("%s=%s is ignored: --channel %s was given, and a flag always wins",
				pinVariable, inputs.pin, target.channel)
		}
		return target, nil
	}

	if pinned {
		target := upgradeTarget{mode: upgradeModePin, tag: inputs.pin}
		if inputs.prereleaseFlagGiven {
			target.note = fmt.Sprintf("--prerelease has no effect: %s=%s pins one release", pinVariable, inputs.pin)
		}
		return target, nil
	}

	if inputs.channelSetting != "" {
		return channelTarget(inputs.channelSetting, selfChannelKey, ExitConfig,
			prereleaseOr(inputs, inputs.prereleaseSetting))
	}

	if inputs.stampedChannel != "" {
		return channelTarget(inputs.stampedChannel, "this build's stamped channel", ExitSoftware,
			prereleaseOr(inputs, inputs.stampedPrerelease))
	}

	return upgradeTarget{}, ExitWith(ExitUsage, fmt.Errorf("nothing says what to upgrade to: this build has no "+
		"channel of its own; pass --channel %s or --channel %s, or set %s", channelDevelop, channelRelease,
		selfChannelKey))
}

// Actions

// gatherUpgradeInputs reads every source that can decide what a `self upgrade` installs.
//
// A flag is given when the command line names it, as [pflag.FlagSet.Changed] tells, and never because cobra holds
// a default for it: `--prerelease=false` is given, and a flag always wins, including when its value equals its
// default (10-command-line-interface.md §11). The two settings are read through viper, as every shared setting is,
// from the configuration the root loaded; a flag the command does not carry is never given.
//
// Parameters:
//   - `cmd`: the `self upgrade` command, its flags parsed.
//   - `info`: the program's install metadata, carrying the build's stamp.
//
// Returns:
//   - `upgradeInputs`: every source, as found.
func gatherUpgradeInputs(cmd *cobra.Command, info SelfInstallInfo) upgradeInputs {

	flags := cmd.Flags()

	inputs := upgradeInputs{
		from:                flags.Changed("from"),
		channelFlagGiven:    flags.Changed("channel"),
		prereleaseFlagGiven: flags.Changed("prerelease"),
		pin:                 os.Getenv(pinVariable),
		channelSetting:      viper.GetString(selfChannelKey),
		prereleaseSetting:   viper.GetBool(selfPrereleaseKey),
		stampedChannel:      info.Channel,
		stampedPrerelease:   info.Prerelease,
	}

	if inputs.channelFlagGiven {
		inputs.channelFlag = assert.Must(flags.GetString("channel"))
	}
	if inputs.prereleaseFlagGiven {
		inputs.prereleaseFlag = assert.Must(flags.GetBool("prerelease"))
	}

	return inputs
}

// prereleaseOr returns `--prerelease`'s value when it was given, and `fallback` otherwise.
//
// Parameters:
//   - `inputs`: every source, as found.
//   - `fallback`: the switch that comes with the deciding channel.
//
// Returns:
//   - `bool`: whether the run takes the channel's pre-releases.
func prereleaseOr(inputs upgradeInputs, fallback bool) bool {

	if inputs.prereleaseFlagGiven {
		return inputs.prereleaseFlag
	}

	return fallback
}

// endregion
