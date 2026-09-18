// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestExitCodes(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		expected int
	}{
		{"ExitOK", ExitOK, 0},
		{"ExitError", ExitError, 1},
		{"ExitUsage", ExitUsage, 64},
		{"ExitDataErr", ExitDataErr, 65},
		{"ExitNoInput", ExitNoInput, 66},
		{"ExitUnavailable", ExitUnavailable, 69},
		{"ExitSoftware", ExitSoftware, 70},
		{"ExitCantCreate", ExitCantCreate, 73},
		{"ExitIOErr", ExitIOErr, 74},
		{"ExitNoPerm", ExitNoPerm, 77},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.code != tt.expected {
				t.Errorf("expected %s = %d, got %d", tt.name, tt.expected, tt.code)
			}
		})
	}
}

func TestExitWith(t *testing.T) {
	baseErr := errors.New("file not found")
	err := ExitWith(ExitNoInput, baseErr)

	if err == nil {
		t.Fatal("expected error to be non-nil")
	}

	// Should preserve the original error message
	if err.Error() != "file not found" {
		t.Errorf("expected error message 'file not found', got %q", err.Error())
	}

	// Should unwrap to base error
	if !errors.Is(err, baseErr) {
		t.Error("expected wrapped error to unwrap to base error")
	}
}

func TestExitCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected int
	}{
		{"nil error", nil, ExitOK},
		{"plain error", errors.New("generic error"), ExitError},
		{"ExitNoInput wrapped", ExitWith(ExitNoInput, errors.New("not found")), ExitNoInput},
		{"ExitNoPerm wrapped", ExitWith(ExitNoPerm, errors.New("permission denied")), ExitNoPerm},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := ExitCode(tt.err)
			if code != tt.expected {
				t.Errorf("expected exit code %d, got %d", tt.expected, code)
			}
		})
	}
}

func TestAddOutputFlagsBindsTheCommonSet(t *testing.T) {

	cmd := &cobra.Command{Use: "test"}
	var opts SinkOptions
	addOutputFlags(cmd, &opts)

	for _, name := range []string{"filter", "jq", "output", "store"} {
		if cmd.PersistentFlags().Lookup(name) == nil {
			t.Errorf("--%s is not bound; the common set is registered in full or not at all", name)
		}
	}
}

func TestAddOutputFlagsOutputDefaultsToJSON(t *testing.T) {

	cmd := &cobra.Command{Use: "test"}
	var opts SinkOptions
	addOutputFlags(cmd, &opts)

	flag := cmd.PersistentFlags().Lookup("output")
	if flag == nil {
		t.Fatal("--output not bound")
	}
	if flag.DefValue != "json" {
		t.Errorf("--output default = %q, want %q", flag.DefValue, "json")
	}
}

func TestBuildPipelineProducesPipelineForKnownFormat(t *testing.T) {

	var buf bytes.Buffer
	pipeline, err := BuildPipeline(SinkOptions{Format: "json"}, &buf)
	if err != nil {
		t.Fatalf("BuildPipeline: %v", err)
	}
	if pipeline == nil {
		t.Errorf("BuildPipeline returned nil, want *result.Pipeline")
	}
}

func TestBuildPipelineEmitsJSONForJSONFormat(t *testing.T) {

	var buf bytes.Buffer
	sink, err := BuildPipeline(SinkOptions{Format: "json"}, &buf)
	if err != nil {
		t.Fatalf("BuildPipeline: %v", err)
	}

	if err := sink.Emit(map[string]any{"k": "v"}); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, `"k": "v"`) {
		t.Errorf("output = %q, want substring \"k\": \"v\"", got)
	}
}

func TestBuildPipelineAppliesFieldFilterBeforeJQ(t *testing.T) {

	var buf bytes.Buffer
	sink, err := BuildPipeline(
		SinkOptions{
			Format:  "json",
			Filters: []string{"kind=file"},
			JQ:      ".[].name",
		},
		&buf,
	)
	if err != nil {
		t.Fatalf("BuildPipeline: %v", err)
	}

	in := []map[string]any{
		{"kind": "file", "name": "a"},
		{"kind": "dir", "name": "b"},
		{"kind": "file", "name": "c"},
	}
	if err := sink.Emit(in); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	got := buf.String()
	for _, want := range []string{`"a"`, `"c"`} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q; full output:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"b"`) {
		t.Errorf("output should have filtered out b; got:\n%s", got)
	}
}

func TestBuildPipelineReportsUnknownFormat(t *testing.T) {

	var buf bytes.Buffer
	_, err := BuildPipeline(SinkOptions{Format: "xml"}, &buf)
	if err == nil {
		t.Fatal("expected error for unknown format; got nil")
	}
	if !strings.Contains(err.Error(), "unknown formatter") {
		t.Errorf("error text = %q, want substring 'unknown formatter'", err.Error())
	}
}

func TestBuildPipelineReportsFilterParseError(t *testing.T) {

	var buf bytes.Buffer
	_, err := BuildPipeline(SinkOptions{Format: "json", Filters: []string{"nope"}}, &buf)
	if err == nil {
		t.Fatal("expected field-parse error; got nil")
	}
}

func TestBuildPipelineReportsJQParseError(t *testing.T) {

	var buf bytes.Buffer
	_, err := BuildPipeline(SinkOptions{Format: "json", JQ: "((("}, &buf)
	if err == nil {
		t.Fatal("expected jq-parse error; got nil")
	}
}

func TestFailureReturnsError(t *testing.T) {
	err := Failure("test error: %s", "detail")
	if err == nil {
		t.Fatal("expected Failure to return error")
	}

	expected := "test error: detail"
	if err.Error() != expected {
		t.Errorf("expected error message %q, got %q", expected, err.Error())
	}
}

// --- SilentRequested ---

// TestSilentRequested pins the pre-parse read of `--silent`: the flag takes effect before cobra parses
// anything, because a program that narrates while building its command tree narrates before then (#828).
// Cobra's own boolean spellings are honored, and a `--` ends the flags.
func TestSilentRequested(t *testing.T) {

	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"absent", []string{"version"}, false},
		{"bare", []string{"--silent", "version"}, true},
		{"after the command", []string{"version", "--silent"}, true},
		{"explicitly true", []string{"--silent=true"}, true},
		{"explicitly false", []string{"--silent=false"}, false},
		{"among others", []string{"--output", "json", "--silent", "version"}, true},
		{"after a terminator is an operand", []string{"run", "--", "--silent"}, false},
		{"a different flag that starts the same", []string{"--silently"}, false},
		{"no arguments", nil, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SilentRequested(c.args); got != c.want {
				t.Errorf("SilentRequested(%q) = %v; want %v", c.args, got, c.want)
			}
		})
	}
}

// TestNewRootCmd_InstallsTheNarratorBeforeParsing pins the consequence: building a root installs the narrator
// from the raw arguments, so a narration during construction -- before cobra has parsed anything -- is silenced
// when `--silent` was asked for and heard when it was not.
func TestNewRootCmd_InstallsTheNarratorBeforeParsing(t *testing.T) {

	for _, c := range []struct {
		name  string
		args  []string
		heard bool
	}{
		{"with --silent, construction is silent", []string{"probe", "--silent", "version"}, false},
		{"without it, construction speaks", []string{"probe", "version"}, true},
	} {
		t.Run(c.name, func(t *testing.T) {

			previousArgs, previousUI := os.Args, UI()
			t.Cleanup(func() { os.Args = previousArgs; SetUI(previousUI) })

			os.Args = c.args

			// The capture wraps the construction, because the narrator binds os.Stderr when it is built:
			// swapping the file afterwards would leave the narrator holding the real one. Narrating
			// inside the capture is what a report during construction does.
			spoke := captureStderr(t, func() {
				NewRootCmd(RootConfig{Name: "probe"})
				Note("a module surface report")
			})

			if heard := strings.Contains(spoke, "module surface report"); heard != c.heard {
				t.Errorf("narration heard = %v, want %v; stderr was %q", heard, c.heard, spoke)
			}
		})
	}
}

// captureStderr runs fn and returns what it wrote to os.Stderr.
func captureStderr(t *testing.T, fn func()) string {

	t.Helper()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	original := os.Stderr
	os.Stderr = writer

	fn()

	_ = writer.Close()
	os.Stderr = original

	buffer := make([]byte, 4096)
	n, _ := reader.Read(buffer)
	_ = reader.Close()

	return string(buffer[:n])
}

// --- ExitCode ---

// TestExitCode_CobrasRefusalsAreUsageErrors pins the mapping ruled 2026-09-12: the suite reports the sysexits
// set, and a command line the program cannot run exits EX_USAGE rather than the generic 1 every failure used to
// return. A coded error keeps its own code, and a command that ran and failed keeps 1.
func TestExitCode_CobrasRefusalsAreUsageErrors(t *testing.T) {

	cases := []struct {
		name string
		err  error
		want int
	}{
		{"no error", nil, ExitOK},
		{"unknown command", errors.New(`unknown command "nosuchcommand" for "writ"`), ExitUsage},
		{"unknown flag", errors.New("unknown flag: --bogus"), ExitUsage},
		{"unknown shorthand", errors.New("unknown shorthand flag: 'q' in -q"), ExitUsage},
		{"a flag missing its argument", errors.New("flag needs an argument: --output"), ExitUsage},
		{"a bad flag value", errors.New(`invalid argument "bogus" for "--output"`), ExitUsage},
		{"too many arguments", errors.New("accepts 1 arg(s), received 3"), ExitUsage},
		{"too few arguments", errors.New("requires at least 1 arg(s), only received 0"), ExitUsage},
		{
			"mutually exclusive flags",
			errors.New("if any flags in the group [interactive unattended] are set none of the others can be; " +
				"[interactive unattended] were all set"),
			ExitUsage,
		},
		{
			"flags that must come together",
			errors.New("if any flags in the group [a b] are set they must all be set; missing [b]"),
			ExitUsage,
		},
		{
			"a group needing one of its flags",
			errors.New("at least one of the flags in the group [a b] is required"),
			ExitUsage,
		},
		{"a coded error keeps its code", ExitWith(ExitDataErr, errors.New("key not found")), ExitDataErr},
		{"a command that ran and failed", errors.New("verification failed for two documents"), ExitError},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ExitCode(c.err); got != c.want {
				t.Errorf("ExitCode(%v) = %d; want %d", c.err, got, c.want)
			}
		})
	}
}

// TestExitCode_TheSysexitsSetIsComplete pins the set itself against Declare-BashScript's thirteen, so a program
// and the shell scripts beside it cannot drift apart on what a status means.
func TestExitCode_TheSysexitsSetIsComplete(t *testing.T) {

	for name, code := range map[string]int{
		"EX_USAGE": ExitUsage, "EX_DATAERR": ExitDataErr, "EX_NOINPUT": ExitNoInput,
		"EX_UNAVAILABLE": ExitUnavailable, "EX_SOFTWARE": ExitSoftware, "EX_OSERR": ExitOSErr,
		"EX_OSFILE": ExitOSFile, "EX_CANTCREAT": ExitCantCreate, "EX_IOERR": ExitIOErr,
		"EX_TEMPFAIL": ExitTempFail, "EX_PROTOCOL": ExitProtocol, "EX_NOPERM": ExitNoPerm,
		"EX_CONFIG": ExitConfig,
	} {
		want := map[string]int{
			"EX_USAGE": 64, "EX_DATAERR": 65, "EX_NOINPUT": 66, "EX_UNAVAILABLE": 69, "EX_SOFTWARE": 70,
			"EX_OSERR": 71, "EX_OSFILE": 72, "EX_CANTCREAT": 73, "EX_IOERR": 74, "EX_TEMPFAIL": 75,
			"EX_PROTOCOL": 76, "EX_NOPERM": 77, "EX_CONFIG": 78,
		}[name]
		if code != want {
			t.Errorf("%s = %d; Declare-BashScript defines %d", name, code, want)
		}
	}
}
