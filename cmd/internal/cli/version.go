// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

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

// NewVersionCmd creates the version command, which prints the full build detail.
//
// Output goes through [cobra.Command.OutOrStdout] rather than to [os.Stdout] directly, so a caller that
// redirects the command's output captures this like any other command's.
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
		Run: func(cmd *cobra.Command, _ []string) {

			out := cmd.OutOrStdout()

			if short {
				_, _ = fmt.Fprintln(out, info.Version) //nolint:errcheck // diagnose-ignored-error: a failed write to the command's own output has nowhere left to report; see docs/architecture/2.8-eventing-infrastructure.md
				return
			}

			//nolint:errcheck // diagnose-ignored-error: as above; see docs/architecture/2.8-eventing-infrastructure.md
			_, _ = fmt.Fprintf(out, "Version:    %s\nCommit:     %s\nBuilt:      %s\nGo version: %s\nOS/Arch:    %s/%s\n",
				info.Version, info.Commit, info.BuildDate, runtime.Version(), runtime.GOOS, runtime.GOARCH)
		},
	}

	cmd.Flags().BoolVarP(&short, "short", "s", false, "Print only the version number")

	return cmd
}
