// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/sink"
	"github.com/NobleFactor/devlore-cli/pkg/status"
)

// upgradedTo is the release every upgrade in these tests installs; the prefixes start at `v1`.
const upgradedTo = "v2"

// narratingUpgradeChildEnv, set to 1 in a test binary's environment, turns [TestRunUpgradeChild_NarratingChild] into
// a child that narrates on stderr as a program's `self install` does.
const narratingUpgradeChildEnv = "DEVLORE_TEST_NARRATING_UPGRADE_CHILD"

// childRun is one child a `self upgrade` started, as [recordingChild] saw it.
type childRun struct {
	dir    string   // where it ran
	binary string   // what ran
	args   []string // its arguments
	placed string   // what the prefix's binary for its program held when it started
}

// cancelingTransport makes each request through another transport, and wraps the body of one asset's download so
// that reading it to its end cancels the run.
type cancelingTransport struct {
	next   http.RoundTripper  // the transport that makes the request
	asset  string             // the asset whose end cancels the run
	cancel context.CancelFunc // cancels the run
}

// RoundTrip makes the request, wrapping the body when it is the asset's download.
//
// Parameters:
//   - `request`: the request.
//
// Returns:
//   - `*http.Response`: the response, its body wrapped for the asset.
//   - `error`: the other transport's.
func (c cancelingTransport) RoundTrip(request *http.Request) (*http.Response, error) {

	response, err := c.next.RoundTrip(request)
	if err != nil || path.Base(request.URL.Path) != c.asset {
		return response, err
	}

	response.Body = &cancelingBody{ReadCloser: response.Body, cancel: c.cancel}

	return response, nil
}

// cancelingBody is a download's body that cancels the run when a read reaches its end.
type cancelingBody struct {
	io.ReadCloser                    // the download's own body
	cancel        context.CancelFunc // cancels the run
}

// Read reads the body, and cancels the run once the read reaches its end; the read itself still succeeds.
//
// Parameters:
//   - `p`: the buffer.
//
// Returns:
//   - `int`: the bytes read.
//   - `error`: the body's own, [io.EOF] at its end.
func (b *cancelingBody) Read(p []byte) (int, error) {

	n, err := b.ReadCloser.Read(p)
	if errors.Is(err, io.EOF) {
		b.cancel()
	}

	return n, err
}

// upgradeScratch points the temporary directory at a fresh one for the length of the test, and fails the test if
// anything is left in it at the end.
//
// Every platform's variable is set: TMPDIR on Unix, TMP and TEMP on Windows.
//
// Parameters:
//   - `t`: the test harness.
func upgradeScratch(t *testing.T) {

	t.Helper()

	temporary := t.TempDir()
	for _, variable := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(variable, temporary)
	}

	t.Cleanup(func() {
		left, err := os.ReadDir(temporary)
		if err != nil {
			t.Errorf("reading the temporary directory: %v", err)
			return
		}
		for _, entry := range left {
			t.Errorf("the upgrade left %s in the temporary directory", entry.Name())
		}
	})
}

// installedPrefix lays out a prefix as `self install` leaves it: a binary in `bin/` for each program in `binaries`,
// holding `<program> old`, and a manifest naming its version for each program in `versions`.
//
// Parameters:
//   - `t`: the test harness.
//   - `versions`: each program's recorded version; a program absent here has no manifest.
//   - `binaries`: the programs whose binaries are in `bin/`.
//
// Returns:
//   - `string`: the prefix, its links resolved, as the upgrade finds it from the running binary.
func installedPrefix(t *testing.T, versions map[string]string, binaries ...string) string {

	t.Helper()

	prefix, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	prefixRoot := openPrefix(t, prefix)

	for _, program := range binaries {
		writeTestFile(t, filepath.Join(prefix, "bin", executableName(program)), program+" old")
	}
	for program, version := range versions {
		if err := writeManifest(prefixRoot, program, version, nil); err != nil {
			t.Fatalf("writeManifest %s: %v", program, err)
		}
	}

	return prefix
}

// recordingChild returns a child runner that records each run and does what a `self install` of [upgradedTo]
// records: it rewrites the program's manifest to name the release. The run for `failing` fails instead, recording
// nothing.
//
// A run whose context is already done starts nothing and records nothing, and returns the context's error, as
// [exec.Cmd.Start] does for a command made with [exec.CommandContext].
//
// Parameters:
//   - `t`: the test harness.
//   - `failing`: the program whose `self install` fails; "" for none.
//
// Returns:
//   - `run`: the runner.
//   - `runs`: the runs so far, as a function the test calls once the runner has run.
func recordingChild(
	t *testing.T, failing string,
) (run func(context.Context, string, string, []string) error, runs func() []childRun) {

	t.Helper()

	var mu sync.Mutex
	var recorded []childRun

	run = func(ctx context.Context, dir, binary string, args []string) error {

		if err := ctx.Err(); err != nil {
			return err
		}

		program := strings.TrimSuffix(filepath.Base(binary), ".exe")
		prefix := args[2]

		placed, err := os.ReadFile(filepath.Join(prefix, "bin", executableName(program)))
		if err != nil {
			return fmt.Errorf("the child for %s found no binary in place: %w", program, err)
		}

		mu.Lock()
		recorded = append(recorded, childRun{dir: dir, binary: binary, args: args, placed: string(placed)})
		mu.Unlock()

		if program == failing {
			return fmt.Errorf("process: %s exited with code 1", binary)
		}

		prefixRoot, err := fsroot.OpenExisting(prefix)
		if err != nil {
			return err
		}
		defer func() { _ = prefixRoot.Close() }()

		return writeManifest(prefixRoot, program, upgradedTo,
			[]string{filepath.Join("bin", executableName(program))})
	}

	runs = func() []childRun {
		mu.Lock()
		defer mu.Unlock()
		return append([]childRun(nil), recorded...)
	}

	return run, runs
}

// cancelingAtTheEndOf returns a client that cancels the run the moment the download of `asset` is read to its end:
// once the download is done, and before anything verifies or unpacks it.
//
// Parameters:
//   - `client`: the client whose transport makes every request.
//   - `asset`: the name of the asset whose end cancels the run.
//   - `cancel`: cancels the run.
//
// Returns:
//   - `*http.Client`: the client.
func cancelingAtTheEndOf(client *http.Client, asset string, cancel context.CancelFunc) *http.Client {
	return &http.Client{Transport: cancelingTransport{next: client.Transport, asset: asset, cancel: cancel}}
}

// upgradeAt returns the environment of an upgrade run as `program` from `prefix`, against the fake GitHub.
//
// Parameters:
//   - `fake`: GitHub, as the test serves it.
//   - `prefix`: the installation prefix the running binary is in.
//   - `program`: the running program.
//   - `runChild`: the child runner.
//
// Returns:
//   - `upgradeEnvironment`: the environment.
func upgradeAt(
	fake *fakeGitHub, prefix, program string, runChild func(context.Context, string, string, []string) error,
) upgradeEnvironment {

	env := fake.environment("")
	env.executable = filepath.Join(prefix, "bin", executableName(program))
	env.runChild = runChild

	return env
}

// binaryHolds reports what a program's binary in the prefix holds, or "" when it has none.
//
// Parameters:
//   - `t`: the test harness.
//   - `prefix`: the installation prefix.
//   - `program`: the program.
//
// Returns:
//   - `string`: the binary's content.
func binaryHolds(t *testing.T, prefix, program string) string {

	t.Helper()

	content, err := os.ReadFile(filepath.Join(prefix, "bin", executableName(program)))
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	return string(content)
}

// useUpgradeEnvironment makes every `self upgrade` the test runs reach `env` in place of the real one.
//
// Parameters:
//   - `t`: the test harness.
//   - `env`: the environment.
func useUpgradeEnvironment(t *testing.T, env upgradeEnvironment) {

	t.Helper()

	previous := upgradeEnvironmentFor
	upgradeEnvironmentFor = func() (upgradeEnvironment, error) { return env, nil }
	t.Cleanup(func() { upgradeEnvironmentFor = previous })
}

// upgradeProbe runs `<program> self upgrade <args>` through a root built as the program builds its own.
//
// The caller sandboxes the shared configuration first, with [installSandbox], so no setting of the machine's decides
// the run, and sets DEVLORE_VERSION as the case needs it.
//
// Parameters:
//   - `t`: the test harness.
//   - `program`: the program the root is built for.
//   - `channel`: the channel stamped into the build; "" for a local build.
//   - `args`: the arguments after `self upgrade`.
//
// Returns:
//   - `stdout`: what reached stdout: the result.
//   - `stderr`: what reached stderr: the narration.
//   - `err`: what the command returned.
func upgradeProbe(t *testing.T, program, channel string, args ...string) (stdout, stderr string, err error) {

	t.Helper()

	previousUI := UI()
	t.Cleanup(func() { SetUI(previousUI) })

	var result bytes.Buffer

	stderr = captureStderr(t, func() {
		root := NewRootCmd(RootConfig{Name: program, Short: "a probe", Version: "v1", Channel: channel})
		root.SetOut(&result)
		root.SetErr(new(bytes.Buffer))
		root.SetArgs(append([]string{"self", "upgrade"}, args...))
		err = root.Execute()
	})

	return result.String(), stderr, err
}

// decodeReport parses the result a `self upgrade` emitted as JSON.
//
// Parameters:
//   - `t`: the test harness.
//   - `stdout`: what reached stdout.
//
// Returns:
//   - `upgradeReport`: the result.
func decodeReport(t *testing.T, stdout string) upgradeReport {

	t.Helper()

	var report upgradeReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("the result is not JSON: %v\n%s", err, stdout)
	}

	return report
}

// reportsEqual compares two results, field by field.
//
// Parameters:
//   - `got`: the result a run returned.
//   - `want`: the result expected.
//
// Returns:
//   - `bool`: true when every field matches.
func reportsEqual(got, want upgradeReport) bool {

	return got.From == want.From && got.To == want.To && got.Channel == want.Channel &&
		got.Prerelease == want.Prerelease && got.Prefix == want.Prefix && slices.Equal(got.Programs, want.Programs)
}

// --- the narrating child ---

// TestRunUpgradeChild_NarratingChild is the child [TestRunUpgradeChild_RelaysNarrationAsNotes] runs, not a test.
//
// It returns at once unless [narratingUpgradeChildEnv] is set. When it is set, it narrates on stderr as a program's
// `self install` does, and exits before the test framework can add output of its own.
func TestRunUpgradeChild_NarratingChild(t *testing.T) {

	if os.Getenv(narratingUpgradeChildEnv) != "1" {
		return
	}

	fmt.Fprintln(os.Stderr, "[lore] [+] Installed lore to /prefix")
	os.Exit(0)
}

// --- shippedPrograms ---

// TestShippedPrograms_AreTheMakefilesProducts keeps the suite's list and the archive's in step.
//
// The suite is decided before anything is downloaded, so it cannot be read from the archive; the Makefile's PRODUCTS
// is what `make dist` packs, and the two must name the same programs.
func TestShippedPrograms_AreTheMakefilesProducts(t *testing.T) {

	makefile, err := os.ReadFile(filepath.Join("..", "..", "..", "Makefile"))
	if err != nil {
		t.Fatalf("reading the Makefile: %v", err)
	}

	for line := range strings.SplitSeq(string(makefile), "\n") {
		if products, found := strings.CutPrefix(line, "PRODUCTS := "); found {
			if got := strings.Fields(products); !slices.Equal(got, shippedPrograms) {
				t.Errorf("the Makefile's PRODUCTS are %v; shippedPrograms is %v", got, shippedPrograms)
			}
			return
		}
	}

	t.Fatal("the Makefile has no PRODUCTS line")
}

// --- upgradePrefix ---

// TestUpgradePrefix_IsTheDirectoryAboveBin finds the prefix from the running binary, as `self uninstall` does.
func TestUpgradePrefix_IsTheDirectoryAboveBin(t *testing.T) {

	prefix := installedPrefix(t, nil, "writ")

	got, err := upgradePrefix(filepath.Join(prefix, "bin", executableName("writ")), "writ")
	if err != nil {
		t.Fatalf("upgradePrefix: %v", err)
	}
	if got != prefix {
		t.Errorf("upgradePrefix = %q, want %q", got, prefix)
	}
}

// TestUpgradePrefix_RefusesAProgramNoReleaseCarries is Requirement 7's first refusal: devlore-test is not shipped.
func TestUpgradePrefix_RefusesAProgramNoReleaseCarries(t *testing.T) {

	prefix := installedPrefix(t, nil, "devlore-test")

	_, err := upgradePrefix(filepath.Join(prefix, "bin", executableName("devlore-test")), "devlore-test")
	if err == nil {
		t.Fatal("upgradePrefix accepted devlore-test")
	}
	if ExitCode(err) != ExitUsage {
		t.Errorf("exit %d, want %d", ExitCode(err), ExitUsage)
	}
	for _, word := range []string{"devlore-test", "lore, star and writ"} {
		if !strings.Contains(err.Error(), word) {
			t.Errorf("error = %v; want it to name %s", err, word)
		}
	}
}

// TestUpgradePrefix_RefusesARenamedBinary is Requirement 7's second refusal.
//
// The upgrade places each program under its own name, so a binary renamed away from its program's would be left as
// it is while the run reported success.
func TestUpgradePrefix_RefusesARenamedBinary(t *testing.T) {

	prefix := installedPrefix(t, nil)
	renamed := filepath.Join(prefix, "bin", executableName("writ-dev"))
	writeTestFile(t, renamed, "writ old")

	_, err := upgradePrefix(renamed, "writ")
	if err == nil {
		t.Fatal("upgradePrefix accepted a renamed binary")
	}
	if ExitCode(err) != ExitConfig {
		t.Errorf("exit %d, want %d", ExitCode(err), ExitConfig)
	}
	for _, word := range []string{executableName("writ-dev"), executableName("writ")} {
		if !strings.Contains(err.Error(), word) {
			t.Errorf("error = %v; want it to name %s", err, word)
		}
	}
}

// TestUpgradePrefix_RefusesABinaryOutsideBin pins that a binary not in `<prefix>/bin/` has no prefix to upgrade.
func TestUpgradePrefix_RefusesABinaryOutsideBin(t *testing.T) {

	elsewhere := filepath.Join(t.TempDir(), executableName("writ"))
	writeTestFile(t, elsewhere, "writ old")

	if _, err := upgradePrefix(elsewhere, "writ"); err == nil || !strings.Contains(err.Error(), "bin") {
		t.Errorf("upgradePrefix = %v; want a refusal naming bin/", err)
	}
}

// --- heldPrograms ---

// TestHeldPrograms_IsWhatThePrefixOwns pins Requirement 7's suite: each program with a manifest, and the one running.
//
// The manifest is the record of what a program owns (#933), so a binary of the same name with no manifest is
// someone else's, `star` the tar archiver for one, and the upgrade leaves it alone. The running program is held
// whatever its manifest says, because it is the binary the upgrade runs from.
func TestHeldPrograms_IsWhatThePrefixOwns(t *testing.T) {

	prefix := installedPrefix(t, map[string]string{"star": "v1"}, "lore", "devlore-test")
	prefixRoot := openPrefix(t, prefix)

	cases := []struct {
		running string
		want    []string
	}{
		{"writ", []string{"star", "writ"}},
		{"lore", []string{"lore", "star"}},
		{"star", []string{"star"}},
	}

	for _, testCase := range cases {
		if got := heldPrograms(prefixRoot, testCase.running); !slices.Equal(got, testCase.want) {
			t.Errorf("heldPrograms(%s) = %v, want %v", testCase.running, got, testCase.want)
		}
	}
}

// TestHeldPrograms_LeavesAForeignBinaryAlone pins that a binary with no manifest, not running, is not the suite's.
func TestHeldPrograms_LeavesAForeignBinaryAlone(t *testing.T) {

	prefix := installedPrefix(t, map[string]string{"writ": "v1"}, "star", "writ")
	prefixRoot := openPrefix(t, prefix)

	if got, want := heldPrograms(prefixRoot, "writ"), []string{"writ"}; !slices.Equal(got, want) {
		t.Errorf("heldPrograms = %v, want %v: bin/star has no manifest, so it is not devlore's", got, want)
	}
}

// --- upgrade ---

// TestUpgrade_UpgradesEveryProgramThePrefixHolds is Requirements 3 to 7 together, from a develop build on GitHub.
//
// The prefix holds lore and writ; star is not added. Each program's binary is placed before its own `self install`
// runs from `pkg/`, with `--shell` passed through, and the API is asked once.
func TestUpgrade_UpgradesEveryProgramThePrefixHolds(t *testing.T) {

	upgradeScratch(t)
	prefix := installedPrefix(t, map[string]string{"lore": "v1", "writ": "v1"}, "lore", "writ")

	fake := newFakeGitHub(t)
	fake.publish(fake.release(t, "v2", developRef, shippedPrograms...))
	runChild, runs := recordingChild(t, "")

	report, err := upgrade(context.Background(), upgradeAt(fake, prefix, "writ", runChild), upgradeRequest{
		program: "writ",
		prefix:  prefix, version: "v1", target: channelTargetOf(channelDevelop, true), shells: []string{"bash"},
	})
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}

	want := upgradeReport{From: "v1", To: "v2", Channel: channelDevelop, Prerelease: true, Prefix: prefix,
		Programs: []string{"lore", "writ"}}
	if !reportsEqual(report, want) {
		t.Errorf("report = %+v, want %+v", report, want)
	}

	for _, program := range []string{"lore", "writ"} {
		if got := binaryHolds(t, prefix, program); got != program+" v2" {
			t.Errorf("bin/%s holds %q, want the release's", program, got)
		}
	}
	if got := binaryHolds(t, prefix, "star"); got != "" {
		t.Errorf("bin/star holds %q; the prefix did not hold star, so it is not added", got)
	}

	done := runs()
	if len(done) != 2 {
		t.Fatalf("%d children ran, want 2: %+v", len(done), done)
	}
	for i, program := range []string{"lore", "writ"} {
		run := done[i]
		if filepath.Base(run.dir) != "pkg" || run.binary != filepath.Join(run.dir, "bin", executableName(program)) {
			t.Errorf("child %d ran %s from %s; want pkg/bin/%s from pkg/", i, run.binary, run.dir, program)
		}
		if want := []string{"self", "install", prefix, "--shell", "bash"}; !slices.Equal(run.args, want) {
			t.Errorf("child %d's arguments = %v, want %v", i, run.args, want)
		}
		if run.placed != program+" v2" {
			t.Errorf("%s's self install started with bin/%s holding %q; the release's is placed first",
				program, program, run.placed)
		}
	}

	if got := fake.apiRequests(""); got != 1 {
		t.Errorf("%d API requests, want 1", got)
	}
	if got := fake.assetRequests(); got != 2 {
		t.Errorf("%d downloads, want 2: the archive and its checksums", got)
	}
}

// TestUpgrade_PutsBackAProgramLeftWithoutItsBinary is Requirement 7: one an interrupted run left without its binary
// is put back, and is not current while its binary is missing, whatever its manifest says.
func TestUpgrade_PutsBackAProgramLeftWithoutItsBinary(t *testing.T) {

	upgradeScratch(t)
	prefix := installedPrefix(t, map[string]string{"lore": "v2", "star": "v2", "writ": "v2"}, "lore", "writ")

	fake := newFakeGitHub(t)
	fake.publish(fake.release(t, "v2", developRef, shippedPrograms...))
	runChild, runs := recordingChild(t, "")

	report, err := upgrade(context.Background(), upgradeAt(fake, prefix, "writ", runChild), upgradeRequest{
		program: "writ", prefix: prefix, version: "v2", target: channelTargetOf(channelDevelop, true),
	})
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}

	if got := binaryHolds(t, prefix, "star"); got != "star v2" {
		t.Errorf("bin/star holds %q; want it put back", got)
	}
	if !slices.Equal(report.Programs, shippedPrograms) || len(runs()) != 3 {
		t.Errorf("programs = %v, %d children; want all three", report.Programs, len(runs()))
	}
}

// TestUpgrade_AlreadyCurrentIsANoOp is Requirement 8: every program at the release, nothing downloaded or changed.
func TestUpgrade_AlreadyCurrentIsANoOp(t *testing.T) {

	upgradeScratch(t)
	prefix := installedPrefix(t, map[string]string{"lore": "v2", "writ": "v2"}, "lore", "writ")

	fake := newFakeGitHub(t)
	fake.publish(fake.release(t, "v2", developRef, shippedPrograms...))
	runChild, runs := recordingChild(t, "")

	report, err := upgrade(context.Background(), upgradeAt(fake, prefix, "writ", runChild), upgradeRequest{
		program: "writ", prefix: prefix, version: "v2", target: channelTargetOf(channelDevelop, true),
	})
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}

	if report.To != "v2" || !slices.Equal(report.Programs, []string{"lore", "writ"}) {
		t.Errorf("report = %+v", report)
	}
	if got := fake.assetRequests(); got != 0 {
		t.Errorf("%d downloads; already current downloads nothing", got)
	}
	if got := len(runs()); got != 0 {
		t.Errorf("%d children ran; already current changes nothing", got)
	}
	if got := binaryHolds(t, prefix, "writ"); got != "writ old" {
		t.Errorf("bin/writ holds %q; already current changes nothing", got)
	}
}

// TestUpgrade_AnUnreadableManifestIsNotCurrent is Requirement 8: a manifest that cannot be read is not current.
func TestUpgrade_AnUnreadableManifestIsNotCurrent(t *testing.T) {

	upgradeScratch(t)
	prefix := installedPrefix(t, map[string]string{"lore": "v2", "writ": "v2"}, "lore", "writ")
	writeTestFile(t, manifestPath(prefix, "lore"), "{ not a manifest")

	fake := newFakeGitHub(t)
	fake.publish(fake.release(t, "v2", developRef, shippedPrograms...))
	runChild, runs := recordingChild(t, "")

	if _, err := upgrade(context.Background(), upgradeAt(fake, prefix, "writ", runChild), upgradeRequest{
		program: "writ", prefix: prefix, version: "v2", target: channelTargetOf(channelDevelop, true),
	}); err != nil {
		t.Fatalf("upgrade: %v", err)
	}

	if got := len(runs()); got != 2 {
		t.Errorf("%d children ran, want 2: lore's manifest cannot be read, so the suite is not current", got)
	}
}

// TestUpgrade_DryRunChangesNothing is Requirement 11: the release is found, downloaded and verified, and nothing is
// placed and no child starts.
func TestUpgrade_DryRunChangesNothing(t *testing.T) {

	upgradeScratch(t)
	prefix := installedPrefix(t, map[string]string{"lore": "v1", "writ": "v1"}, "lore", "writ")

	fake := newFakeGitHub(t)
	fake.publish(fake.release(t, "v2", developRef, shippedPrograms...))
	runChild, runs := recordingChild(t, "")

	report, err := upgrade(context.Background(), upgradeAt(fake, prefix, "writ", runChild), upgradeRequest{
		program: "writ", prefix: prefix, version: "v1", target: channelTargetOf(channelDevelop, true), dryRun: true,
	})
	if err != nil {
		t.Fatalf("upgrade --dry-run: %v", err)
	}

	want := upgradeReport{From: "v1", To: "v2", Channel: channelDevelop, Prerelease: true, Prefix: prefix,
		Programs: []string{"lore", "writ"}}
	if !reportsEqual(report, want) {
		t.Errorf("report = %+v, want %+v", report, want)
	}
	if got := fake.assetRequests(); got != 2 {
		t.Errorf("%d downloads, want 2: a dry run verifies what it would install", got)
	}
	if got := len(runs()); got != 0 {
		t.Errorf("%d children ran under --dry-run", got)
	}
	for _, program := range []string{"lore", "writ"} {
		if got := binaryHolds(t, prefix, program); got != program+" old" {
			t.Errorf("bin/%s holds %q after a dry run", program, got)
		}
	}
}

// TestUpgrade_TheFirstFailureStops is Requirement 7: the first program whose install fails stops the run, by name.
func TestUpgrade_TheFirstFailureStops(t *testing.T) {

	upgradeScratch(t)
	prefix := installedPrefix(t, map[string]string{"lore": "v1", "writ": "v1"}, "lore", "writ")

	fake := newFakeGitHub(t)
	fake.publish(fake.release(t, "v2", developRef, shippedPrograms...))
	runChild, runs := recordingChild(t, "lore")

	_, err := upgrade(context.Background(), upgradeAt(fake, prefix, "writ", runChild), upgradeRequest{
		program: "writ", prefix: prefix, version: "v1", target: channelTargetOf(channelDevelop, true),
	})
	if err == nil {
		t.Fatal("upgrade succeeded though lore's install failed")
	}
	if !strings.Contains(err.Error(), "lore") || !strings.Contains(err.Error(), "again") {
		t.Errorf("error = %v; want it to name lore and say a rerun finishes the job", err)
	}
	if got := len(runs()); got != 1 {
		t.Errorf("%d children ran; the run stops at the first failure", got)
	}
	if got := binaryHolds(t, prefix, "writ"); got != "writ old" {
		t.Errorf("bin/writ holds %q; nothing after the failure is touched", got)
	}
}

// TestUpgrade_RefusesAnArchiveWithoutAProgramThePrefixHolds is Requirement 7: the suite is upgraded together or not.
func TestUpgrade_RefusesAnArchiveWithoutAProgramThePrefixHolds(t *testing.T) {

	upgradeScratch(t)
	prefix := installedPrefix(t, map[string]string{"lore": "v1", "star": "v1", "writ": "v1"}, "lore", "star", "writ")

	fake := newFakeGitHub(t)
	fake.publish(fake.release(t, "v2", developRef, "lore", "writ"))
	runChild, runs := recordingChild(t, "")

	_, err := upgrade(context.Background(), upgradeAt(fake, prefix, "writ", runChild), upgradeRequest{
		program: "writ", prefix: prefix, version: "v1", target: channelTargetOf(channelDevelop, true),
	})
	if err == nil || !strings.Contains(err.Error(), "star") {
		t.Fatalf("upgrade = %v; want a refusal naming star", err)
	}
	if got := len(runs()); got != 0 {
		t.Errorf("%d children ran; nothing is placed until the archive is known to carry the suite", got)
	}
}

// TestUpgrade_AMismatchUnpacksNothing is Requirement 5: an archive that does not match its checksums installs nothing.
func TestUpgrade_AMismatchUnpacksNothing(t *testing.T) {

	upgradeScratch(t)
	prefix := installedPrefix(t, map[string]string{"writ": "v1"}, "writ")

	fake := newFakeGitHub(t)
	fake.publish(fake.release(t, "v2", developRef, shippedPrograms...))
	fake.serve(checksumsName("v2"), []byte(sha256Of([]byte("another archive"))+"  "+
		archiveName("v2", fake.environment("").goos, fake.environment("").goarch)+"\n"))
	runChild, runs := recordingChild(t, "")

	_, err := upgrade(context.Background(), upgradeAt(fake, prefix, "writ", runChild), upgradeRequest{
		program: "writ", prefix: prefix, version: "v1", target: channelTargetOf(channelDevelop, true),
	})
	if ExitCode(err) != ExitDataErr {
		t.Fatalf("upgrade = %v (exit %d); want a verification failure", err, ExitCode(err))
	}
	if got := len(runs()); got != 0 {
		t.Errorf("%d children ran from an unverified archive", got)
	}
	if got := binaryHolds(t, prefix, "writ"); got != "writ old" {
		t.Errorf("bin/writ holds %q after a failed verification", got)
	}
}

// TestUpgrade_FromAnArchiveOnDisk is Requirement 10: `--from` installs a local archive and never asks GitHub.
func TestUpgrade_FromAnArchiveOnDisk(t *testing.T) {

	upgradeScratch(t)
	prefix := installedPrefix(t, map[string]string{"lore": "v1", "writ": "v1"}, "lore", "writ")

	dir := t.TempDir()
	name, content := packRelease(t, "v2", shippedPrograms...)
	writeTestFile(t, filepath.Join(dir, name), string(content))
	writeTestFile(t, filepath.Join(dir, checksumsName("v2")), string(checksumsFor(map[string][]byte{name: content})))

	fake := newFakeGitHub(t)
	runChild, runs := recordingChild(t, "")

	report, err := upgrade(context.Background(), upgradeAt(fake, prefix, "writ", runChild), upgradeRequest{
		program: "writ",
		prefix:  prefix, version: "v1", target: upgradeTarget{mode: upgradeModeArchive}, from: filepath.Join(dir, name),
	})
	if err != nil {
		t.Fatalf("upgrade --from: %v", err)
	}

	want := upgradeReport{From: "v1", To: "v2", Prefix: prefix, Programs: []string{"lore", "writ"}}
	if !reportsEqual(report, want) {
		t.Errorf("report = %+v, want %+v", report, want)
	}
	if got := fake.apiRequests("") + fake.assetRequests(); got != 0 {
		t.Errorf("%d requests to GitHub; --from asks it nothing", got)
	}
	if got := len(runs()); got != 2 {
		t.Errorf("%d children ran, want 2", got)
	}
	if got := binaryHolds(t, prefix, "lore"); got != "lore v2" {
		t.Errorf("bin/lore holds %q, want the archive's", got)
	}

	// Again: the archive's release is installed, so the second run is a no-op.
	if _, err := upgrade(context.Background(), upgradeAt(fake, prefix, "writ", runChild), upgradeRequest{
		program: "writ",
		prefix:  prefix, version: "v2", target: upgradeTarget{mode: upgradeModeArchive}, from: filepath.Join(dir, name),
	}); err != nil {
		t.Fatalf("upgrade --from, again: %v", err)
	}
	if got := len(runs()); got != 2 {
		t.Errorf("%d children ran in all; the second run is a no-op", got)
	}
}

// TestUpgrade_FromAnArchiveOnDiskIsVerified is Requirement 10: a local archive is verified as a downloaded one is.
func TestUpgrade_FromAnArchiveOnDiskIsVerified(t *testing.T) {

	upgradeScratch(t)
	prefix := installedPrefix(t, map[string]string{"writ": "v1"}, "writ")

	dir := t.TempDir()
	name, content := packRelease(t, "v2", shippedPrograms...)
	writeTestFile(t, filepath.Join(dir, name), string(content))
	writeTestFile(t, filepath.Join(dir, checksumsName("v2")), sha256Of([]byte("another"))+"  "+name+"\n")

	runChild, runs := recordingChild(t, "")

	_, err := upgrade(context.Background(), upgradeAt(newFakeGitHub(t), prefix, "writ", runChild), upgradeRequest{
		program: "writ",
		prefix:  prefix, version: "v1", target: upgradeTarget{mode: upgradeModeArchive}, from: filepath.Join(dir, name),
	})
	if ExitCode(err) != ExitDataErr {
		t.Fatalf("upgrade --from = %v (exit %d); want a verification failure", err, ExitCode(err))
	}
	if got := len(runs()); got != 0 {
		t.Errorf("%d children ran from an unverified archive", got)
	}
}

// TestUpgrade_AnInterruptLeavesNoScratch is Requirement 6: an interrupt in the middle of a download cancels it, and
// the scratch directory goes with the command.
func TestUpgrade_AnInterruptLeavesNoScratch(t *testing.T) {

	upgradeScratch(t)
	prefix := installedPrefix(t, map[string]string{"writ": "v1"}, "writ")

	fake := newFakeGitHub(t)
	fake.publish(fake.release(t, "v2", developRef, shippedPrograms...))
	fake.stallOn(archiveName("v2", fake.environment("").goos, fake.environment("").goarch))
	runChild, runs := recordingChild(t, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-fake.stalled
		cancel()
	}()

	_, err := upgrade(ctx, upgradeAt(fake, prefix, "writ", runChild), upgradeRequest{
		program: "writ", prefix: prefix, version: "v1", target: channelTargetOf(channelDevelop, true),
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("upgrade = %v; want the interrupt's cancellation", err)
	}
	if got := len(runs()); got != 0 {
		t.Errorf("%d children ran after an interrupt", got)
	}
}

// TestUpgrade_AnInterruptAfterTheDownloadPlacesNothing is Requirement 6: an interrupt that lands once the download is
// done, while the archive is verified and unpacked, stops the run before it places a binary or starts a child.
func TestUpgrade_AnInterruptAfterTheDownloadPlacesNothing(t *testing.T) {

	upgradeScratch(t)
	prefix := installedPrefix(t, map[string]string{"lore": "v1", "writ": "v1"}, "lore", "writ")

	fake := newFakeGitHub(t)
	fake.publish(fake.release(t, "v2", developRef, shippedPrograms...))
	runChild, runs := recordingChild(t, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := upgradeAt(fake, prefix, "writ", runChild)
	env.client = cancelingAtTheEndOf(env.client, archiveName("v2", env.goos, env.goarch), cancel)

	_, err := upgrade(ctx, env, upgradeRequest{
		program: "writ", prefix: prefix, version: "v1", target: channelTargetOf(channelDevelop, true),
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("upgrade = %v; want the interrupt's cancellation", err)
	}

	for _, program := range []string{"lore", "writ"} {
		if got := binaryHolds(t, prefix, program); got != program+" old" {
			t.Errorf("bin/%s holds %q after the interrupt; nothing is placed once the run is interrupted",
				program, got)
		}
	}
	if got := len(runs()); got != 0 {
		t.Errorf("%d children ran after an interrupt", got)
	}
}

// TestUpgrade_AnInterruptedDryRunFails is Requirement 6 under `--dry-run`: an interrupt once the download is done
// fails the run, which reports nothing as though it had finished.
func TestUpgrade_AnInterruptedDryRunFails(t *testing.T) {

	upgradeScratch(t)
	prefix := installedPrefix(t, map[string]string{"writ": "v1"}, "writ")

	fake := newFakeGitHub(t)
	fake.publish(fake.release(t, "v2", developRef, shippedPrograms...))
	runChild, _ := recordingChild(t, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := upgradeAt(fake, prefix, "writ", runChild)
	env.client = cancelingAtTheEndOf(env.client, archiveName("v2", env.goos, env.goarch), cancel)

	_, err := upgrade(ctx, env, upgradeRequest{
		program: "writ", prefix: prefix, version: "v1", target: channelTargetOf(channelDevelop, true), dryRun: true,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("upgrade --dry-run = %v; want the interrupt's cancellation", err)
	}
}

// TestUpgrade_AnInterruptBetweenProgramsStopsAtTheNext is Requirement 6 in the middle of the suite: an interrupt
// while one program installs itself stops the run before the next program's binary is placed, naming that program.
func TestUpgrade_AnInterruptBetweenProgramsStopsAtTheNext(t *testing.T) {

	upgradeScratch(t)
	prefix := installedPrefix(t, map[string]string{"lore": "v1", "writ": "v1"}, "lore", "writ")

	fake := newFakeGitHub(t)
	fake.publish(fake.release(t, "v2", developRef, shippedPrograms...))
	recording, runs := recordingChild(t, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	interrupting := func(ctx context.Context, dir, binary string, args []string) error {
		err := recording(ctx, dir, binary, args)
		cancel()
		return err
	}

	_, err := upgrade(ctx, upgradeAt(fake, prefix, "writ", interrupting), upgradeRequest{
		program: "writ", prefix: prefix, version: "v1", target: channelTargetOf(channelDevelop, true),
	})
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "upgrading writ") {
		t.Fatalf("upgrade = %v; want the interrupt's cancellation, stopping at writ", err)
	}

	if got := binaryHolds(t, prefix, "lore"); got != "lore v2" {
		t.Errorf("bin/lore holds %q; lore finished before the interrupt", got)
	}
	if got := binaryHolds(t, prefix, "writ"); got != "writ old" {
		t.Errorf("bin/writ holds %q; nothing is placed once the run is interrupted", got)
	}
	if got := len(runs()); got != 1 {
		t.Errorf("%d children ran; the run stops at the interrupt", got)
	}
}

// TestUpgrade_ReportsAReleaseWithoutThisPlatformsArchive is Requirement 3's last bullet, through the whole run.
func TestUpgrade_ReportsAReleaseWithoutThisPlatformsArchive(t *testing.T) {

	upgradeScratch(t)
	prefix := installedPrefix(t, map[string]string{"writ": "v1"}, "writ")

	fake := newFakeGitHub(t)
	fake.publish(bare("v2", developRef))
	runChild, _ := recordingChild(t, "")

	_, err := upgrade(context.Background(), upgradeAt(fake, prefix, "writ", runChild), upgradeRequest{
		program: "writ", prefix: prefix, version: "v1", target: channelTargetOf(channelDevelop, true),
	})
	if err == nil || !strings.Contains(err.Error(), checksumsName("v2")) {
		t.Errorf("upgrade = %v; want a refusal naming the missing files", err)
	}
}

// --- runUpgradeChild ---

// TestRunUpgradeChild_RelaysNarrationAsNotes is Requirement 7's relay: a child's narration reaches the operator as
// notes, not warnings.
func TestRunUpgradeChild_RelaysNarrationAsNotes(t *testing.T) {

	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	captured, narration := sink.Capture()
	previousUI := UI()
	SetUI(status.NewNarrator("writ", captured))
	t.Cleanup(func() { SetUI(previousUI) })

	t.Setenv(narratingUpgradeChildEnv, "1")

	if err := runUpgradeChild(context.Background(), t.TempDir(), self,
		[]string{"-test.run=^TestRunUpgradeChild_NarratingChild$"}); err != nil {
		t.Fatalf("runUpgradeChild: %v", err)
	}

	if !strings.Contains(narration.String(), "[writ] [+] [lore] [+] Installed lore to /prefix") {
		t.Errorf("the child's narration was not relayed as a note:\n%s", narration)
	}
	if strings.Contains(narration.String(), "[△]") {
		t.Errorf("the child's narration was relayed as a warning:\n%s", narration)
	}
}

// --- self upgrade, end to end ---

// TestSelfUpgradeCmd_NothingToDecideIsAUsageError is D6, through the command: a local build with nothing to say what
// to upgrade to refuses, naming --channel and self.channel, before anything is touched.
func TestSelfUpgradeCmd_NothingToDecideIsAUsageError(t *testing.T) {

	installSandbox(t)
	t.Setenv(pinVariable, "")

	prefix := installedPrefix(t, map[string]string{"writ": "v1"}, "writ")
	fake := newFakeGitHub(t)
	runChild, _ := recordingChild(t, "")
	useUpgradeEnvironment(t, upgradeAt(fake, prefix, "writ", runChild))

	_, _, err := upgradeProbe(t, "writ", "")
	if err == nil {
		t.Fatal("self upgrade ran with nothing to decide what to")
	}
	if ExitCode(err) != ExitUsage {
		t.Errorf("exit %d, want %d", ExitCode(err), ExitUsage)
	}
	for _, word := range []string{"--channel", selfChannelKey} {
		if !strings.Contains(err.Error(), word) {
			t.Errorf("error = %v; want it to name %s", err, word)
		}
	}
}

// TestSelfUpgradeCmd_APinSetAsideIsNoted is Requirement 2, through the command: a pin beside --channel is ignored,
// and the operator is told; the result is emitted.
func TestSelfUpgradeCmd_APinSetAsideIsNoted(t *testing.T) {

	installSandbox(t)
	upgradeScratch(t)
	t.Setenv(pinVariable, "v9.9.9")

	prefix := installedPrefix(t, map[string]string{"writ": "v2"}, "writ")
	fake := newFakeGitHub(t)
	fake.publish(fake.release(t, "v2", "refs/tags/v2", shippedPrograms...))
	runChild, _ := recordingChild(t, "")
	useUpgradeEnvironment(t, upgradeAt(fake, prefix, "writ", runChild))

	stdout, stderr, err := upgradeProbe(t, "writ", channelDevelop, "--channel", channelRelease)
	if err != nil {
		t.Fatalf("self upgrade --channel release: %v", err)
	}

	if !strings.Contains(stderr, "DEVLORE_VERSION=v9.9.9 is ignored") {
		t.Errorf("stderr does not note the pin set aside:\n%s", stderr)
	}
	if report := decodeReport(t, stdout); report.To != "v2" || report.Channel != channelRelease {
		t.Errorf("result = %+v; want the release channel's v2", report)
	}
	if got := fake.apiRequests("/releases/tags/v9.9.9"); got != 0 {
		t.Errorf("the pin was asked for %d times; --channel outranks it", got)
	}
}

// TestSelfUpgradeCmd_DryRunEmitsTheResult is Requirement 11, through the command: the result is emitted, and nothing
// is placed or started.
func TestSelfUpgradeCmd_DryRunEmitsTheResult(t *testing.T) {

	installSandbox(t)
	upgradeScratch(t)
	t.Setenv(pinVariable, "")

	prefix := installedPrefix(t, map[string]string{"lore": "v1", "writ": "v1"}, "lore", "writ")
	fake := newFakeGitHub(t)
	fake.publish(fake.release(t, "v2", developRef, shippedPrograms...))
	runChild, runs := recordingChild(t, "")
	useUpgradeEnvironment(t, upgradeAt(fake, prefix, "writ", runChild))

	stdout, _, err := upgradeProbe(t, "writ", channelDevelop, "--dry-run")
	if err != nil {
		t.Fatalf("self upgrade --dry-run: %v", err)
	}

	want := upgradeReport{From: "v1", To: "v2", Channel: channelDevelop, Prerelease: true, Prefix: prefix,
		Programs: []string{"lore", "writ"}}
	if report := decodeReport(t, stdout); !reportsEqual(report, want) {
		t.Errorf("result = %+v, want %+v", report, want)
	}
	if got := len(runs()); got != 0 {
		t.Errorf("%d children ran under --dry-run", got)
	}
}

// TestSelfUpgradeCmd_FromExcludesChannelAndPrerelease is Requirement 10's flag: an archive names its own release.
func TestSelfUpgradeCmd_FromExcludesChannelAndPrerelease(t *testing.T) {

	installSandbox(t)
	useUpgradeEnvironment(t, upgradeEnvironment{})

	for _, c := range []struct {
		args  []string
		other string
	}{
		{[]string{"--from", "archive.tar.gz", "--channel", channelDevelop}, "channel"},
		{[]string{"--from", "archive.tar.gz", "--prerelease"}, "prerelease"},
	} {
		_, _, err := upgradeProbe(t, "writ", channelDevelop, c.args...)
		if err == nil {
			t.Errorf("self upgrade %v ran", c.args)
			continue
		}
		if ExitCode(err) != ExitUsage || strings.Contains(err.Error(), "unknown flag") ||
			!strings.Contains(err.Error(), "from") || !strings.Contains(err.Error(), c.other) {
			t.Errorf("self upgrade %v: exit %d, %v; want a usage error refusing --from with --%s",
				c.args, ExitCode(err), err, c.other)
		}
	}
}

// TestSelfUpgradeCmd_RefusesAProgramNoReleaseCarries is Requirement 7's refusal, through the command.
func TestSelfUpgradeCmd_RefusesAProgramNoReleaseCarries(t *testing.T) {

	installSandbox(t)
	upgradeScratch(t)

	prefix := installedPrefix(t, nil, "devlore-test")
	fake := newFakeGitHub(t)
	runChild, _ := recordingChild(t, "")
	useUpgradeEnvironment(t, upgradeAt(fake, prefix, "devlore-test", runChild))

	_, _, err := upgradeProbe(t, "devlore-test", "", "--channel", channelDevelop)
	if ExitCode(err) != ExitUsage || !strings.Contains(fmt.Sprint(err), "devlore-test") {
		t.Errorf("devlore-test self upgrade: exit %d, %v; want a usage error naming it", ExitCode(err), err)
	}
	if got := fake.apiRequests(""); got != 0 {
		t.Errorf("%d API requests; a refusal asks GitHub nothing", got)
	}
}

// TestSelfUpgradeCmd_RefusesAProgramNoReleaseCarriesBeforeDeciding is Requirement 7's refusal of devlore-test, given
// before anything is decided: a local build with nothing to decide what to upgrade to is told why devlore-test cannot
// be upgraded, not advised to pass a channel that would be refused anyway.
func TestSelfUpgradeCmd_RefusesAProgramNoReleaseCarriesBeforeDeciding(t *testing.T) {

	installSandbox(t)
	t.Setenv(pinVariable, "")

	prefix := installedPrefix(t, nil, "devlore-test")
	fake := newFakeGitHub(t)
	runChild, _ := recordingChild(t, "")
	useUpgradeEnvironment(t, upgradeAt(fake, prefix, "devlore-test", runChild))

	_, _, err := upgradeProbe(t, "devlore-test", "")
	if ExitCode(err) != ExitUsage {
		t.Errorf("exit %d, want %d: %v", ExitCode(err), ExitUsage, err)
	}
	for _, word := range []string{"devlore-test", "lore, star and writ"} {
		if !strings.Contains(fmt.Sprint(err), word) {
			t.Errorf("error = %v; want it to name %s", err, word)
		}
	}
	if strings.Contains(fmt.Sprint(err), "--channel") {
		t.Errorf("error = %v; it advises --channel, which would be refused anyway", err)
	}
}

// TestSelfUpgradeCmd_RefusesARenamedBinaryBeforeDeciding is Requirement 7's refusal of a renamed binary, given before
// anything is decided: a local build is told the binary's name is the problem, not advised to pass a channel.
func TestSelfUpgradeCmd_RefusesARenamedBinaryBeforeDeciding(t *testing.T) {

	installSandbox(t)
	t.Setenv(pinVariable, "")

	prefix := installedPrefix(t, nil)
	writeTestFile(t, filepath.Join(prefix, "bin", executableName("writ-dev")), "writ old")
	fake := newFakeGitHub(t)
	runChild, _ := recordingChild(t, "")
	useUpgradeEnvironment(t, upgradeAt(fake, prefix, "writ-dev", runChild))

	_, _, err := upgradeProbe(t, "writ", "")
	if ExitCode(err) != ExitConfig {
		t.Errorf("exit %d, want %d: %v", ExitCode(err), ExitConfig, err)
	}
	if !strings.Contains(fmt.Sprint(err), executableName("writ-dev")) {
		t.Errorf("error = %v; want it to name the renamed binary", err)
	}
	if strings.Contains(fmt.Sprint(err), "--channel") {
		t.Errorf("error = %v; it advises --channel, which would be refused anyway", err)
	}
}

// TestSelfUpgradeCmd_ARefusalNotesNothing pins that a refused run says only why: a pin beside --channel is not noted
// as set aside by a run that goes nowhere.
func TestSelfUpgradeCmd_ARefusalNotesNothing(t *testing.T) {

	installSandbox(t)
	t.Setenv(pinVariable, "v9.9.9")

	prefix := installedPrefix(t, nil, "devlore-test")
	fake := newFakeGitHub(t)
	runChild, _ := recordingChild(t, "")
	useUpgradeEnvironment(t, upgradeAt(fake, prefix, "devlore-test", runChild))

	_, stderr, err := upgradeProbe(t, "devlore-test", "", "--channel", channelDevelop)
	if ExitCode(err) != ExitUsage {
		t.Errorf("exit %d, want %d: %v", ExitCode(err), ExitUsage, err)
	}
	if strings.Contains(stderr, "is ignored") {
		t.Errorf("a refused run noted the pin it set aside:\n%s", stderr)
	}
}

// --- newUpgradeCmd ---

// TestNewUpgradeCmd_PromisesOneRelease is D7 in the help: the upgrade installs the release that is decided, which a
// switch of channel or a pin can make older than the running build, so the help never promises a newer one.
func TestNewUpgradeCmd_PromisesOneRelease(t *testing.T) {

	cmd := newUpgradeCmd(&cobra.Command{Use: "writ"}, SelfInstallInfo{Name: "writ"})

	if !strings.Contains(cmd.Short, "to one release") {
		t.Errorf("Short = %q; want it to promise one release", cmd.Short)
	}
	if strings.Contains(cmd.Short+cmd.Long, "newer") {
		t.Errorf("the help promises a newer release, which D7 does not check for:\n%s\n%s", cmd.Short, cmd.Long)
	}
}

// TestNewUpgradeCmd_HelpOfAProgramNoReleaseCarries pins that devlore-test's help says the command refuses it, and
// why, and promises no upgrade: every run of it is refused (Requirement 7).
func TestNewUpgradeCmd_HelpOfAProgramNoReleaseCarries(t *testing.T) {

	cmd := newUpgradeCmd(&cobra.Command{Use: "devlore-test"}, SelfInstallInfo{Name: "devlore-test"})

	if !strings.Contains(cmd.Short, "devlore-test is not in a release") {
		t.Errorf("Short = %q; want it to say devlore-test is not in a release", cmd.Short)
	}
	if !strings.Contains(cmd.Long, "lore, star and writ") {
		t.Errorf("Long = %q; want it to name what a release carries", cmd.Long)
	}
	for _, text := range []string{cmd.Short, cmd.Long} {
		if strings.Contains(text, "Upgrade devlore-test") || strings.Contains(text, "Examples:") {
			t.Errorf("devlore-test's help promises an upgrade it refuses:\n%s", text)
		}
	}
}
