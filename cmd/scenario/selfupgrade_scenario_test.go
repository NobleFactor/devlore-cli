// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package scenario

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// fixtureVersion is the release the upgrade scenario builds, packs and upgrades to: any version but the one
// `make build` stamps into build/, so that the upgrade has something to do and where it ends can be told from where
// it started.
const fixtureVersion = "v0.0.0-scenario.1"

// versionPackage is the package holding the stamped version, the one the Makefile's VERSION_PACKAGE names.
const versionPackage = "github.com/NobleFactor/devlore-cli/pkg/application"

// addTarEntry writes one staged file or directory into a tar archive, under `name`, keeping its mode.
//
// Parameters:
//   - `entries`: the archive being written.
//   - `name`: the entry's name within the archive, slash-separated.
//   - `info`: the staged file or directory.
//   - `source`: the staged file's path; read only for a regular file.
//
// Returns:
//   - `error`: a failure to describe, write or read the entry.
func addTarEntry(entries *tar.Writer, name string, info fs.FileInfo, source string) error {

	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}

	header.Name = name
	if info.IsDir() {
		header.Name += "/"
	}

	if err := entries.WriteHeader(header); err != nil {
		return err
	}

	if info.IsDir() {
		return nil
	}

	return copyInto(entries, source)
}

// addZipEntry writes one staged file or directory into a zip archive, under `name`, keeping its mode.
//
// Parameters:
//   - `entries`: the archive being written.
//   - `name`: the entry's name within the archive, slash-separated.
//   - `info`: the staged file or directory.
//   - `source`: the staged file's path; read only for a regular file.
//
// Returns:
//   - `error`: a failure to describe, write or read the entry.
func addZipEntry(entries *zip.Writer, name string, info fs.FileInfo, source string) error {

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}

	header.Name = name
	if info.IsDir() {
		header.Name += "/"
	} else {
		header.Method = zip.Deflate
	}

	entry, err := entries.CreateHeader(header)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return nil
	}

	return copyInto(entry, source)
}

// buildFixture builds the release the scenario upgrades to, as `make dist` builds one for this platform.
//
// lore, star and writ are built for the runner's platform, stamped [fixtureVersion], and staged as `make dist` stages
// them (Makefile, dist-all): the programs at the stage's root, the tracked files of star/extensions under
// share/devlore. The stage is packed as devlore-cli_<version>_<goos>_<goarch>.tar.gz, or .zip on Windows, with the
// checksums file beside it in shasum's format. Not `make dist` itself, which would restamp build/ or carry its stamp.
//
// The go toolchain builds the fixture, as cmd/devlore-test's suite builds the binary it drives.
//
// Parameters:
//   - `t`: the test harness.
//   - `dir`: the directory the stage, the archive and the checksums file are written in.
//
// Returns:
//   - `archive`: the archive's path.
//   - `stage`: the stage, holding each program the archive carries, as built.
func buildFixture(t *testing.T, dir string) (archive, stage string) {

	t.Helper()

	root := repositoryRoot(t)
	stage = filepath.Join(dir, "stage")

	if err := os.MkdirAll(filepath.Join(stage, "share", "devlore"), 0o750); err != nil {
		t.Fatal(err)
	}

	// `-o` naming a directory builds each package into it under the package's name, with the platform's suffix.
	args := []string{"build", "-ldflags", "-X " + versionPackage + ".Version=" + fixtureVersion,
		"-o", stage + string(filepath.Separator)}
	for _, tool := range selfInstallTools {
		args = append(args, "./cmd/"+tool)
	}

	build := exec.CommandContext(t.Context(), "go", args...)
	build.Dir = root
	build.Env = append(os.Environ(), "GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH, "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("cannot build the fixture: %v\n%s", err, out)
	}

	stageExtensions(t, root, filepath.Join(stage, "share", "devlore"))

	extension := ".tar.gz"
	if runtime.GOOS == "windows" {
		extension = ".zip"
	}

	name := "devlore-cli_" + fixtureVersion + "_" + runtime.GOOS + "_" + runtime.GOARCH + extension
	archive = filepath.Join(dir, name)
	packStage(t, stage, archive)

	checksums := filepath.Join(dir, "devlore-cli_"+fixtureVersion+"_checksums.txt")
	if err := os.WriteFile(checksums, []byte(fileDigest(t, archive)+"  "+name+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	return archive, stage
}

// changedEntries returns every path whose state differs between two snapshots of a prefix, sorted, including a path
// that is in only one of them.
//
// Parameters:
//   - `before`: the earlier snapshot, from [snapshotPrefix].
//   - `after`: the later snapshot.
//
// Returns:
//   - `[]string`: the paths that were added, removed or changed; empty when nothing was.
func changedEntries(before, after map[string]string) []string {

	var changed []string
	for path, state := range before {
		if now, found := after[path]; !found || now != state {
			changed = append(changed, path)
		}
	}
	for path := range after {
		if _, found := before[path]; !found {
			changed = append(changed, path)
		}
	}

	slices.Sort(changed)

	return changed
}

// copyInto copies a file's content into `w`.
//
// Parameters:
//   - `w`: where the content goes.
//   - `source`: the file.
//
// Returns:
//   - `error`: a failure to open or read the file, or to write its content.
func copyInto(w io.Writer, source string) error {

	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(w, file)

	return err
}

// fileDigest returns a file's SHA-256, in hex.
//
// Parameters:
//   - `t`: the test harness.
//   - `path`: the file.
//
// Returns:
//   - `string`: the file's SHA-256, lower-case hex.
func fileDigest(t *testing.T, path string) string {

	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		t.Fatalf("cannot read %s: %v", path, err)
	}

	return hex.EncodeToString(hash.Sum(nil))
}

// manifestVersion returns the version an installed program's manifest records, the one its `self install` wrote
// from its stamp.
//
// Parameters:
//   - `t`: the test harness.
//   - `prefix`: the installation prefix.
//   - `tool`: the installed program.
//
// Returns:
//   - `string`: the recorded version.
func manifestVersion(t *testing.T, prefix, tool string) string {

	t.Helper()

	data, err := os.ReadFile(filepath.Join(prefix, "share", tool, "manifest.json"))
	if err != nil {
		t.Fatalf("read %s manifest: %v", tool, err)
	}

	var m struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse %s manifest: %v", tool, err)
	}

	return m.Version
}

// packStage packs a staged release into `archive` as `make dist` packs its stage: every entry beneath `stage`, named
// by its path relative to it, in a gzip-compressed tar or, when the archive's name says so, a zip.
//
// Parameters:
//   - `t`: the test harness.
//   - `stage`: the staged release: the programs at its root, share/ beside them.
//   - `archive`: the archive to write, ending .tar.gz or .zip.
func packStage(t *testing.T, stage, archive string) {

	t.Helper()

	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}

	var add func(name string, info fs.FileInfo, source string) error
	var closers []io.Closer

	if strings.HasSuffix(archive, ".zip") {
		entries := zip.NewWriter(file)
		add = func(name string, info fs.FileInfo, source string) error {
			return addZipEntry(entries, name, info, source)
		}
		closers = []io.Closer{entries, file}
	} else {
		compressed := gzip.NewWriter(file)
		entries := tar.NewWriter(compressed)
		add = func(name string, info fs.FileInfo, source string) error {
			return addTarEntry(entries, name, info, source)
		}
		closers = []io.Closer{entries, compressed, file}
	}

	err = filepath.WalkDir(stage, func(source string, entry fs.DirEntry, err error) error {
		if err != nil || source == stage {
			return err
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		name, err := filepath.Rel(stage, source)
		if err != nil {
			return err
		}

		return add(filepath.ToSlash(name), info, source)
	})

	// Every writer is closed, innermost first, whatever went wrong before: each flushes into the next.
	for _, closer := range closers {
		err = errors.Join(err, closer.Close())
	}
	if err != nil {
		t.Fatalf("cannot pack %s: %v", archive, err)
	}
}

// repositoryRoot returns the repository's root, two levels above this package, where the scenario runs.
//
// Parameters:
//   - `t`: the test harness.
//
// Returns:
//   - `string`: the repository's root, absolute.
func repositoryRoot(t *testing.T) string {

	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	return root
}

// snapshotPrefix returns what is in a prefix: every entry beneath it, by its path relative to it, with its mode, its
// modification time and, for a regular file, its SHA-256.
//
// Two snapshots are equal only when nothing beneath the prefix was added, removed, rewritten or touched.
//
// Parameters:
//   - `t`: the test harness.
//   - `prefix`: the installation prefix.
//
// Returns:
//   - `map[string]string`: each entry's state, by its path relative to `prefix`.
func snapshotPrefix(t *testing.T, prefix string) map[string]string {

	t.Helper()

	snapshot := map[string]string{}

	err := filepath.WalkDir(prefix, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		relative, err := filepath.Rel(prefix, path)
		if err != nil {
			return err
		}

		state := fmt.Sprintf("%v %d", info.Mode(), info.ModTime().UnixNano())
		if info.Mode().IsRegular() {
			state += " " + fileDigest(t, path)
		}
		snapshot[relative] = state

		return nil
	})
	if err != nil {
		t.Fatalf("cannot read %s: %v", prefix, err)
	}

	return snapshot
}

// stageExtensions copies star's extensions into a stage's share/devlore, as `make dist` stages them: only tracked
// files, so a dirty checkout cannot reach the archive, each at its path in the repository.
//
// Parameters:
//   - `t`: the test harness.
//   - `root`: the repository's root.
//   - `share`: the stage's share/devlore directory.
func stageExtensions(t *testing.T, root, share string) {

	t.Helper()

	listing := exec.CommandContext(t.Context(), "git", "ls-files", "-z", "--", "star/extensions")
	listing.Dir = root

	out, err := listing.Output()
	if err != nil {
		t.Fatalf("cannot list star's tracked extensions: %v", err)
	}

	for name := range strings.SplitSeq(string(out), "\x00") {
		if name == "" {
			continue
		}

		source := filepath.Join(root, filepath.FromSlash(name))
		target := filepath.Join(share, filepath.FromSlash(name))

		info, err := os.Stat(source)
		if err != nil {
			t.Fatalf("cannot stage %s: %v", name, err)
		}

		data, err := os.ReadFile(source)
		if err != nil {
			t.Fatalf("cannot stage %s: %v", name, err)
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, info.Mode().Perm()); err != nil {
			t.Fatalf("cannot stage %s: %v", name, err)
		}
	}
}

// --- self upgrade ---

// TestSelfUpgradeScenario drives `self upgrade --from` through the real binaries, from the installed copy, on
// whatever platform the runner is (#947, Requirement 13).
//
// lore, star and writ are installed from build/ into a sandbox prefix, and the installed writ upgrades all three to a
// release the test built and packed itself. The point is the running binary: writ replaces itself while it runs,
// which no build before #947 could do, and on Windows only by setting itself aside as writ.exe.old, which writ's own
// `self install` then records. A second run, by the upgraded writ, finds every program at the release, says so, and
// changes nothing.
//
// No network: an archive on disk is the offline path, which asks GitHub nothing.
func TestSelfUpgradeScenario(t *testing.T) {

	if os.Getenv("DEVLORE_SCENARIO_RUN") == "" {
		t.Skip("scenario runs under make test-scenario (DEVLORE_SCENARIO_RUN=1)")
	}

	sandbox := t.TempDir()
	prefix := filepath.Join(sandbox, "prefix")
	environment := sandboxEnvironment(sandbox)
	archive, stage := buildFixture(t, t.TempDir())

	// Install the suite from build/. Its stamp must not be the fixture's, or the upgrade would find nothing to do.
	for _, tool := range selfInstallTools {
		out, err := run(t, toolBinary(t, tool), environment, "self", "install", prefix, "--shell", "bash")
		if err != nil {
			t.Fatalf("%s self install failed: %v\n%s", tool, err, out)
		}

		if version := manifestVersion(t, prefix, tool); version == fixtureVersion {
			t.Fatalf("build/%s is stamped %s, the fixture's version, so the upgrade would prove nothing", tool, version)
		}
	}

	// The upgrade runs from the installed copy, which it replaces while it runs.
	upgrader := filepath.Join(prefix, "bin", "writ"+exeSuffix())

	out, err := run(t, upgrader, environment, "self", "upgrade", "--from", archive, "--shell", "bash")
	t.Logf("self upgrade:\n%s", out)
	if err != nil {
		t.Fatalf("self upgrade failed: %v\n%s", err, out)
	}

	for _, tool := range selfInstallTools {
		binary := filepath.Join("bin", tool+exeSuffix())

		installed := fileDigest(t, filepath.Join(prefix, binary))
		if built := fileDigest(t, filepath.Join(stage, tool+exeSuffix())); installed != built {
			t.Errorf("%s is not the fixture's %s: its SHA-256 is %s, and the fixture's is %s", binary, tool, installed,
				built)
		}

		if version := manifestVersion(t, prefix, tool); version != fixtureVersion {
			t.Errorf("%s manifest names %s, not %s", tool, version, fixtureVersion)
		}

		if _, err := os.Stat(filepath.Join(prefix, binary+".new")); !os.IsNotExist(err) {
			t.Errorf("%s.new, the staged copy, was left in the prefix (err = %v)", binary, err)
		}
	}

	// Windows renames a running image but will not replace it, so writ set itself aside, and its own install recorded
	// the file it could not yet remove.
	if runtime.GOOS == "windows" {
		aside := filepath.Join("bin", "writ.exe.old")

		if _, err := os.Stat(filepath.Join(prefix, aside)); err != nil {
			t.Errorf("writ, replaced while it ran, was not set aside as %s: %v", aside, err)
		}

		if !slices.Contains(manifestFiles(t, prefix, "writ"), aside) {
			t.Errorf("writ's manifest does not record %s", aside)
		}
	}

	// The upgraded writ, run again, finds every program at the release.
	before := snapshotPrefix(t, prefix)

	out, err = run(t, upgrader, environment, "self", "upgrade", "--from", archive, "--shell", "bash")
	t.Logf("self upgrade, again:\n%s", out)
	if err != nil {
		t.Fatalf("the second self upgrade failed: %v\n%s", err, out)
	}

	if !strings.Contains(out, "Nothing to upgrade") {
		t.Errorf("the second self upgrade did not say there was nothing to upgrade:\n%s", out)
	}

	if changed := changedEntries(before, snapshotPrefix(t, prefix)); len(changed) > 0 {
		t.Errorf("the second self upgrade changed %s", strings.Join(changed, ", "))
	}
}
