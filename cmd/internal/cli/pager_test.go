// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/NobleFactor/devlore-cli/pkg/result"
)

// region Tests

// TestShouldPage_OnlyWhatAPersonReads walks §10's rule.
//
// The group is what decides: Records and Document are laid out for a person, and the rest exist to be consumed
// by a program. `--paginate` overrides the group, `--no-pager` overrides everything, and a destination that is
// not a terminal is never paged whatever the flags say -- that last one is what keeps a rendering the same
// bytes piped as it is on screen.
func TestShouldPage_OnlyWhatAPersonReads(t *testing.T) {

	for _, testCase := range []struct {
		name     string
		isTTY    bool
		group    result.Group
		paginate bool
		noPager  bool
		want     bool
	}{
		{"a table on a terminal", true, result.GroupRecords, false, false, true},
		{"a document on a terminal", true, result.GroupDocument, false, false, true},
		{"json on a terminal", true, result.GroupSerialized, false, false, false},
		{"a composed shape on a terminal", true, result.GroupComposed, false, false, false},
		{"nothing on a terminal", true, result.GroupNothing, false, false, false},
		{"a table down a pipe", false, result.GroupRecords, false, false, false},
		{"a document down a pipe", false, result.GroupDocument, false, false, false},
		{"--paginate makes json page", true, result.GroupSerialized, true, false, true},
		{"--paginate down a pipe still does not", false, result.GroupSerialized, true, false, false},
		{"--no-pager refuses a table", true, result.GroupRecords, false, true, false},
		{"--no-pager beats --paginate", true, result.GroupDocument, true, true, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got := shouldPage(testCase.isTTY, testCase.group, testCase.paginate, testCase.noPager)
			if got != testCase.want {
				t.Errorf("shouldPage(%v, %q, paginate=%v, noPager=%v) = %v, want %v",
					testCase.isTTY, testCase.group, testCase.paginate, testCase.noPager, got, testCase.want)
			}
		})
	}
}

// TestPagerCommand_FollowsGitsOrder pins the environment's say.
//
// This suite's variable first, then `PAGER`, then the fallback -- the order `git-var(1)` documents for
// `GIT_PAGER`, `core.pager` and `PAGER`. A variable set to the empty string disables paging, as `GIT_PAGER=`
// does, and the suite's variable disables it even when `PAGER` is set: the nearer answer wins whatever it says.
func TestPagerCommand_FollowsGitsOrder(t *testing.T) {

	for _, testCase := range []struct {
		name    string
		devlore *string
		config  *string
		pager   *string
		want    []string
	}{
		{name: "nothing set", want: strings.Fields(pagerFallback)},
		{name: "PAGER alone", pager: value("more"), want: []string{"more"}},
		{name: "the config key beats PAGER", config: value("bat -p"), pager: value("more"), want: []string{"bat", "-p"}},
		{
			name:    "the suite's variable beats both",
			devlore: value("less -R"),
			config:  value("bat -p"),
			pager:   value("more"),
			want:    []string{"less", "-R"},
		},
		{name: "an empty variable disables", devlore: value(""), pager: value("more"), want: nil},
		{name: "an empty config key disables", config: value(""), pager: value("more"), want: nil},
	} {
		t.Run(testCase.name, func(t *testing.T) {

			t.Setenv(pagerVariable, "")
			t.Setenv("PAGER", "")
			os.Unsetenv(pagerVariable) //nolint:errcheck // t.Setenv restores it after the test
			os.Unsetenv("PAGER")       //nolint:errcheck // t.Setenv restores it after the test

			if testCase.devlore != nil {
				t.Setenv(pagerVariable, *testCase.devlore)
			}
			if testCase.pager != nil {
				t.Setenv("PAGER", *testCase.pager)
			}
			if testCase.config != nil {
				viper.Set("probe.pager", *testCase.config)
				t.Cleanup(func() { viper.Set("probe.pager", nil) })
			}

			got := pagerCommand("probe")
			if strings.Join(got, " ") != strings.Join(testCase.want, " ") {
				t.Errorf("pagerCommand() = %q, want %q", got, testCase.want)
			}
		})
	}
}

// value returns a pointer to s, so a table can tell "not set" from "set to empty" -- the difference between
// falling through to the next answer and disabling paging outright.
func value(s string) *string { return &s }

// TestOpenPager_RunsThePagerAndWaitsForIt covers the machinery: the child process, the pipe, and the wait.
//
// `cat` stands in for a pager because it is the one every platform has and it copies its input through
// unchanged, so the bytes that come out the far side are the proof that the pipe and the wait both work. The
// wait is the part that matters in production: a command that exits before its pager has drained leaves the
// shell prompt fighting the pager for the screen.
func TestOpenPager_RunsThePagerAndWaitsForIt(t *testing.T) {

	t.Setenv(pagerVariable, "cat")

	var out bytes.Buffer
	session, err := openPager(&out, "probe")
	if err != nil {
		t.Fatalf("openPager: %v", err)
	}

	const report = "a report\nwith two lines\n"
	if _, err := io.WriteString(session, report); err != nil {
		t.Fatalf("write to the pager: %v", err)
	}

	if err := session.Close(); err != nil {
		t.Fatalf("close the pager: %v", err)
	}

	if out.String() != report {
		t.Errorf("the pager wrote %q, want %q", out.String(), report)
	}
}

// TestCloseActivePager_IsSafeToCallTwice pins the post-run's contract.
//
// The post-run runs on every command, paged or not, and a command may Emit more than once; closing what was
// never opened, or closing twice, must both be quiet.
func TestCloseActivePager_IsSafeToCallTwice(t *testing.T) {

	t.Cleanup(func() { activePager = nil })

	if err := closeActivePager(); err != nil {
		t.Errorf("closing an unopened pager: %v", err)
	}

	t.Setenv(pagerVariable, "cat")
	var out bytes.Buffer
	session, err := openPager(&out, "probe")
	if err != nil {
		t.Fatalf("openPager: %v", err)
	}
	activePager = session

	if err := closeActivePager(); err != nil {
		t.Errorf("closing the pager: %v", err)
	}
	if activePager != nil {
		t.Error("the pager survived its close")
	}
	if err := closeActivePager(); err != nil {
		t.Errorf("closing twice: %v", err)
	}
}

// TestEmitWriter_ABufferIsNeverPaged is §7's rule at the seam where it is decided.
//
// A destination that is not a terminal comes back untouched, whatever the rendering: that is what makes a
// piped result the same bytes as a redirected one.
func TestEmitWriter_ABufferIsNeverPaged(t *testing.T) {

	t.Setenv(pagerVariable, "cat")
	t.Cleanup(func() { activePager = nil })

	for _, format := range []string{"table", "markdown", "terminal", "json", "value"} {
		t.Run(format, func(t *testing.T) {

			var buffer bytes.Buffer
			writer, err := emitWriter(&buffer, SinkOptions{Format: format}, "probe")
			if err != nil {
				t.Fatalf("emitWriter: %v", err)
			}
			if writer != io.Writer(&buffer) {
				t.Errorf("%s was routed somewhere other than its destination", format)
			}
			if activePager != nil {
				t.Errorf("%s opened a pager for a buffer", format)
			}
		})
	}
}

// TestEmitWriter_RefusesAnUnknownRendering keeps the failure where a user can read it.
//
// The group decides whether to page, so the rendering is resolved here as well as in [BuildPipeline]; a bad
// name must fail rather than quietly skip the decision.
func TestEmitWriter_RefusesAnUnknownRendering(t *testing.T) {

	var buffer bytes.Buffer
	if _, err := emitWriter(&buffer, SinkOptions{Format: "bogus"}, "probe"); err == nil {
		t.Error("an unknown rendering was accepted")
	}
}

// TestOpenPager_ReportsAPagerThatWillNotStart is the machine without `less`.
//
// The command still has a result to deliver, so [emitWriter] treats this as "no pager" rather than a failure;
// what is asserted here is that the failure is reported to its caller rather than swallowed at this level.
func TestOpenPager_ReportsAPagerThatWillNotStart(t *testing.T) {

	t.Setenv(pagerVariable, "no-such-pager-devlore-test")

	var out bytes.Buffer
	if _, err := openPager(&out, "probe"); err == nil {
		t.Error("a pager that does not exist started")
	}
}

// TestPagerSession_CloseIsQuietWhenTheReaderQuitsEarly pins the exit-status rule.
//
// Quitting a pager before its input is drained is how people read: `q` at the first screen. The pager then
// exits non-zero with its input pipe broken, and that is not an error to report -- the reader said they were
// done. `false` stands in for it, exiting non-zero immediately.
func TestPagerSession_CloseIsQuietWhenTheReaderQuitsEarly(t *testing.T) {

	t.Setenv(pagerVariable, "false")

	var out bytes.Buffer
	session, err := openPager(&out, "probe")
	if err != nil {
		t.Fatalf("openPager: %v", err)
	}

	// The write races the child's exit, so its error is not the subject and is deliberately ignored.
	_, _ = io.WriteString(session, "a report the reader did not wait for\n") //nolint:errcheck // see above

	if err := session.Close(); err != nil {
		t.Errorf("a pager the reader quit early reported %v, want silence", err)
	}
}

// endregion
