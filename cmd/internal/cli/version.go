// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// VersionReport is `version`'s result: the build stamps this binary carries and the toolchain and platform it
// was built with. A result like any other, so `--output json` feeds a script, `--output yaml` reads by eye, and
// `--output value` gives the six values alone (10-command-line-interface.md §5, §7).
type VersionReport struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Built   string `json:"built"`
	Go      string `json:"go"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

// The two version surfaces, split the way docker splits them: `--version` answers in one line and
// `version` prints the detail. `version --short` is the scriptable form of the first, which is why all three
// are built here rather than drifting apart in separate files.

// VersionInfo contains version metadata set at build time.
type VersionInfo struct {
	Version   string // Semantic version (e.g., "0.1.0")
	Commit    string // Git commit hash
	BuildDate string // Build timestamp
}

// AddVersionFlag installs `--version` on a root command, answering in one line.
//
// Cobra generates the flag as soon as [cobra.Command.Version] is set; the template fixes the wording to
// docker's — `writ version 0.4.0, build ed6f468` — a single line that contacts nothing and exits. `-v` is
// deliberately not a shorthand for it: this repository's commands already use `-v` for verbose output, and
// the collision would be worse than the missing convenience.
//
// Parameters:
//   - `rootCmd`: the root command the flag is installed on.
//   - `info`: the build-time metadata; `Version` and `Commit` appear in the line.
func AddVersionFlag(rootCmd *cobra.Command, info VersionInfo) {

	rootCmd.Version = info.Version

	// The template is rendered by text/template, so it must carry no action delimiters of its own. Every
	// value here is a build stamp, and a stamp containing "{{" is not a case worth defending against.
	rootCmd.SetVersionTemplate(fmt.Sprintf("%s version %s, build %s\n", rootCmd.Name(), info.Version, info.Commit))
}

// NewVersionCmd creates the version command, whose result is the build detail.
//
// The report goes through [Emit] like every other result, so the renderings and the filter stage apply to it:
// `--output json` parses, `--output yaml` reads, `--output value` gives the six values, `--output none` prints
// nothing. `--short` narrows the result to the version string alone, which is the scriptable form; the
// `--version` flag keeps cobra's one-line answer, which is not a result (#795).
//
// Parameters:
//   - `info`: the build-time metadata to report.
//
// Returns:
//   - `*cobra.Command`: the `version` command, carrying its `--short` flag.
func NewVersionCmd(info VersionInfo) *cobra.Command {
	var short bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, _ []string) error {

			if short {
				return Emit(cmd, info.Version)
			}

			return Emit(cmd, VersionReport{
				Version: info.Version,
				Commit:  info.Commit,
				Built:   info.BuildDate,
				Go:      runtime.Version(),
				OS:      runtime.GOOS,
				Arch:    runtime.GOARCH,
			})
		},
	}

	cmd.Flags().BoolVarP(&short, "short", "s", false, "Print only the version number")

	return cmd
}
