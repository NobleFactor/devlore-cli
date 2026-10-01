// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"bytes"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// --- AddVersionFlag ---

// TestAddVersionFlag_AnswersInOneLine pins the `--version` surface: docker's shape, one line, and nothing
// from the detail block.
//
// The one-line property is the whole point of the flag's existence beside the `version` command, so it is
// asserted directly rather than inferred from the text matching.
func TestAddVersionFlag_AnswersInOneLine(t *testing.T) {

	rootCmd := &cobra.Command{Use: "writ"}
	AddVersionFlag(rootCmd, VersionInfo{Version: "1.2.3", Commit: "abc1234", BuildDate: "2026-03-17T00:00:00Z"})

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{"--version"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if want := "writ version 1.2.3, build abc1234\n"; out.String() != want {
		t.Errorf("--version = %q, want %q", out.String(), want)
	}

	if lines := strings.Count(strings.TrimSuffix(out.String(), "\n"), "\n"); lines != 0 {
		t.Errorf("--version printed %d extra lines; the flag answers in one", lines)
	}

	// The build date belongs to `version`, not to `--version` — docker's split is the reason both exist.
	if strings.Contains(out.String(), "2026-03-17") {
		t.Error("--version reported the build date; that detail belongs to the version command")
	}
}

// TestAddVersionFlag_NamesTheChannel pins the line's channel: the one a build follows, in parentheses after the
// version, with `prerelease` beside `release` when the build takes that channel's pre-releases. Every develop build
// is a pre-release, so `develop` stands alone; a local build has no channel and the line names none.
func TestAddVersionFlag_NamesTheChannel(t *testing.T) {

	cases := []struct {
		channel    string
		prerelease bool
		want       string
	}{
		{"", false, "writ version 1.2.3, build abc1234\n"},
		{"", true, "writ version 1.2.3, build abc1234\n"},
		{"develop", true, "writ version 1.2.3 (develop), build abc1234\n"},
		{"develop", false, "writ version 1.2.3 (develop), build abc1234\n"},
		{"release", false, "writ version 1.2.3 (release), build abc1234\n"},
		{"release", true, "writ version 1.2.3 (release, prerelease), build abc1234\n"},
	}

	for _, testCase := range cases {

		rootCmd := &cobra.Command{Use: "writ"}
		AddVersionFlag(rootCmd, VersionInfo{
			Version: "1.2.3", Commit: "abc1234", Channel: testCase.channel, Prerelease: testCase.prerelease,
		})

		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetArgs([]string{"--version"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		if out.String() != testCase.want {
			t.Errorf("channel %q, prerelease %t: --version = %q, want %q",
				testCase.channel, testCase.prerelease, out.String(), testCase.want)
		}
	}
}

// TestAddVersionFlag_IsNotBoundToShorthandV pins the deliberate omission: `-v` is verbose output across this
// repository's commands, and cobra would happily take it for --version.
func TestAddVersionFlag_IsNotBoundToShorthandV(t *testing.T) {

	rootCmd := &cobra.Command{Use: "writ"}
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")
	AddVersionFlag(rootCmd, VersionInfo{Version: "1.2.3", Commit: "abc1234"})

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{"-v"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if strings.Contains(out.String(), "version") {
		t.Errorf("-v printed the version %q; it is the verbose flag", out.String())
	}
}

// --- NewVersionCmd ---

// versionProbe runs `version` with the arguments through a shared root carrying the build stamps.
//
// The root is the shared one because [Emit] renders through the common set the root registers; a command built
// standalone has no set to render with.
//
// Parameters:
//   - `t`: the test.
//   - `info`: the build stamps the root carries.
//   - `args`: the arguments after `version`.
//
// Returns:
//   - `string`: what reached stdout.
func versionProbe(t *testing.T, info VersionInfo, args ...string) string {
	t.Helper()

	root := NewRootCmd(RootConfig{
		Name:       "probe",
		Short:      "a probe",
		Version:    info.Version,
		Commit:     info.Commit,
		BuildDate:  info.BuildDate,
		Channel:    info.Channel,
		Prerelease: info.Prerelease,
	})

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(new(bytes.Buffer))
	root.SetArgs(append([]string{"version"}, args...))

	if err := root.Execute(); err != nil {
		t.Fatalf("version %s: %v", strings.Join(args, " "), err)
	}

	return out.String()
}

// stamps returns the build metadata every test in this block reports.
//
// Returns:
//   - `VersionInfo`: the stamps, on channel `release` with its pre-releases.
func stamps() VersionInfo {
	return VersionInfo{
		Version: "1.2.3", Commit: "abc1234", BuildDate: "2026-03-17T00:00:00Z", Channel: "release", Prerelease: true,
	}
}

// TestNewVersionCmd_IsAResult is #795: the version is data, so it renders through the pipeline like every other
// result. Under the json default it parses, and every stamp survives the round trip with the field names the
// struct tags declare.
func TestNewVersionCmd_IsAResult(t *testing.T) {

	var report VersionReport
	if err := json.Unmarshal([]byte(versionProbe(t, stamps())), &report); err != nil {
		t.Fatalf("the default rendering is not json: %v", err)
	}

	want := VersionReport{
		Version: "1.2.3", Channel: "release", Prerelease: true, Commit: "abc1234", Built: "2026-03-17T00:00:00Z",
		Go: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH,
	}
	if report != want {
		t.Errorf("report = %+v; want %+v", report, want)
	}
}

// TestNewVersionCmd_EveryRenderingRenders pins the whole set: a result renders under all eight, and `none`
// prints nothing at all, which is the contract that told #754 apart from a working flag.
func TestNewVersionCmd_EveryRenderingRenders(t *testing.T) {

	for _, format := range []string{"csv", "json", "list", "table", "value", "yaml", "template={{.version}}"} {
		t.Run(format, func(t *testing.T) {
			if out := versionProbe(t, stamps(), "--output", format); !strings.Contains(out, "1.2.3") {
				t.Errorf("--output %s rendered %q; the version is missing", format, out)
			}
		})
	}

	if out := versionProbe(t, stamps(), "--output", "none"); out != "" {
		t.Errorf("--output none rendered %q; its whole contract is silence", out)
	}
}

// TestNewVersionCmd_ShortIsTheVersionAlone pins `--short` and its `-s` shorthand: the scriptable form is the
// version string and nothing else, under either spelling.
func TestNewVersionCmd_ShortIsTheVersionAlone(t *testing.T) {

	for _, spelling := range []string{"--short", "-s"} {
		t.Run(spelling, func(t *testing.T) {
			out := versionProbe(t, stamps(), spelling, "--output", "value")
			if strings.TrimSpace(out) != "1.2.3" {
				t.Errorf("version %s --output value = %q; want the version alone", spelling, out)
			}
		})
	}
}

// TestNewVersionCmd_DefaultStampsSurvive pins the unstamped build: `dev`, `none` and `unknown` are values like
// any other and reach the result rather than being elided, and so do the empty channel and the false switch of a
// build that has no channel -- a consumer reads `channel` and `prerelease` from every build, the stamp proof among
// them.
func TestNewVersionCmd_DefaultStampsSurvive(t *testing.T) {

	info := VersionInfo{Version: "dev", Commit: "none", BuildDate: "unknown"}
	out := versionProbe(t, info)

	var report VersionReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("the default rendering is not json: %v", err)
	}

	if report.Version != "dev" || report.Commit != "none" || report.Built != "unknown" {
		t.Errorf("report = %+v; want the default stamps verbatim", report)
	}

	var fields map[string]any
	if err := json.Unmarshal([]byte(out), &fields); err != nil {
		t.Fatalf("the default rendering is not a json object: %v", err)
	}

	if channel, present := fields["channel"]; !present || channel != "" {
		t.Errorf("channel = %v (present %t); want an empty channel, reported", channel, present)
	}
	if prerelease, present := fields["prerelease"]; !present || prerelease != false {
		t.Errorf("prerelease = %v (present %t); want false, reported", prerelease, present)
	}
}

// TestNewVersionCmd_ChannelReachesATemplate pins what the build's stamp proof reads: the channel and the switch,
// raw, through `--output template=`, with no parser beyond the shell.
func TestNewVersionCmd_ChannelReachesATemplate(t *testing.T) {

	out := versionProbe(t, stamps(), "--output", "template={{.channel}} {{.prerelease}}")

	if out != "release true" {
		t.Errorf("version --output template = %q; want %q", out, "release true")
	}
}
