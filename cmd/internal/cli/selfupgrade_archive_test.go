// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
)

// packedEntry is one entry of an archive a test packs: a file, a directory, or a kind no release archive carries.
type packedEntry struct {
	name     string // the entry's name, as the archive records it
	body     string // a regular file's content
	mode     int64  // the entry's permission bits; 0o644 when zero
	typeflag byte   // the tar entry's kind; tar.TypeReg when zero, and a directory when the name ends in a slash
	linkname string // a link's target
}

// packTarGz packs `entries` into a gzip-compressed tar, as `make dist` packs a release for every platform but Windows.
//
// Parameters:
//   - `t`: the test harness.
//   - `entries`: what the archive holds, in order.
//
// Returns:
//   - `[]byte`: the archive.
func packTarGz(t *testing.T, entries []packedEntry) []byte {

	t.Helper()

	var packed bytes.Buffer
	compressed := gzip.NewWriter(&packed)
	archive := tar.NewWriter(compressed)

	for _, entry := range entries {
		header := &tar.Header{Name: entry.name, Mode: entry.mode, Typeflag: entry.typeflag, Linkname: entry.linkname}
		if header.Mode == 0 {
			header.Mode = 0o644
		}
		switch {
		case header.Typeflag == 0 && strings.HasSuffix(entry.name, "/"):
			header.Typeflag = tar.TypeDir
		case header.Typeflag == 0:
			header.Typeflag = tar.TypeReg
		}
		if header.Typeflag == tar.TypeReg {
			header.Size = int64(len(entry.body))
		}
		if err := archive.WriteHeader(header); err != nil {
			t.Fatalf("packing %s: %v", entry.name, err)
		}
		if _, err := archive.Write([]byte(entry.body)); err != nil {
			t.Fatalf("packing %s: %v", entry.name, err)
		}
	}

	if err := archive.Close(); err != nil {
		t.Fatalf("closing the tar: %v", err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatalf("closing the gzip: %v", err)
	}

	return packed.Bytes()
}

// packZip packs `entries` into a zip, as `make dist` packs a release for Windows.
//
// A tar.TypeSymlink entry becomes a zip symbolic link: its target as its body and the link bit in its mode.
//
// Parameters:
//   - `t`: the test harness.
//   - `entries`: what the archive holds, in order.
//
// Returns:
//   - `[]byte`: the archive.
func packZip(t *testing.T, entries []packedEntry) []byte {

	t.Helper()

	var packed bytes.Buffer
	archive := zip.NewWriter(&packed)

	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Deflate}
		mode := fs.FileMode(entry.mode)
		if mode == 0 {
			mode = 0o644
		}
		body := entry.body
		switch {
		case strings.HasSuffix(entry.name, "/"):
			mode |= fs.ModeDir
		case entry.typeflag == tar.TypeSymlink:
			mode |= fs.ModeSymlink
			body = entry.linkname
		}
		header.SetMode(mode)

		writer, err := archive.CreateHeader(header)
		if err != nil {
			t.Fatalf("packing %s: %v", entry.name, err)
		}
		if _, err := writer.Write([]byte(body)); err != nil {
			t.Fatalf("packing %s: %v", entry.name, err)
		}
	}

	if err := archive.Close(); err != nil {
		t.Fatalf("closing the zip: %v", err)
	}

	return packed.Bytes()
}

// packRelease packs a release archive for this platform, laid out as `make dist` lays one out.
//
// The programs sit at the archive's root, each holding `<program> <tag>` so a test can tell which build it has, and
// star's extensions sit under `share/`.
//
// Parameters:
//   - `t`: the test harness.
//   - `tag`: the release the archive belongs to.
//   - `programs`: the programs the archive carries.
//
// Returns:
//   - `name`: the archive's name, as the release names it.
//   - `content`: the archive.
func packRelease(t *testing.T, tag string, programs ...string) (name string, content []byte) {

	t.Helper()

	var entries []packedEntry
	for _, program := range programs {
		entries = append(entries, packedEntry{name: executableName(program), body: program + " " + tag, mode: 0o755})
	}
	entries = append(entries,
		packedEntry{name: "share/"},
		packedEntry{name: "share/devlore/"},
		packedEntry{name: "share/devlore/star/extensions/probe/extension.yaml", body: "name: probe\n"})

	name = archiveName(tag, runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		return name, packZip(t, entries)
	}

	return name, packTarGz(t, entries)
}

// sha256Of returns the hex SHA-256 of `content`, as a checksums file and GitHub's digest give it.
//
// Parameters:
//   - `content`: the bytes to hash.
//
// Returns:
//   - `string`: the lower-case hex digest.
func sha256Of(content []byte) string {

	sum := sha256.Sum256(content)

	return hex.EncodeToString(sum[:])
}

// checksumsFor returns a checksums file listing each named content, in the format `shasum -a 256` writes.
//
// Parameters:
//   - `files`: each file's name and content.
//
// Returns:
//   - `[]byte`: the checksums file.
func checksumsFor(files map[string][]byte) []byte {

	var listing strings.Builder
	for name, content := range files {
		fmt.Fprintf(&listing, "%s  %s\n", sha256Of(content), name)
	}

	return []byte(listing.String())
}

// unpackInto writes `content` to a scratch directory the test owns and unpacks it there.
//
// Parameters:
//   - `t`: the test harness.
//   - `name`: the archive's name, which says its format.
//   - `content`: the archive.
//
// Returns:
//   - `fsroot.Dir`: the scratch directory, open until the test ends.
//   - `fsroot.Path`: the unpacked package, as [unpackArchive] returns it.
//   - `error`: [unpackArchive]'s.
func unpackInto(t *testing.T, name string, content []byte) (fsroot.Dir, fsroot.Path, error) {

	t.Helper()

	scratch, err := fsroot.OpenExisting(t.TempDir())
	if err != nil {
		t.Fatalf("OpenExisting: %v", err)
	}
	t.Cleanup(func() { _ = scratch.Close() })

	if err := scratch.WriteFile(scratch.NewPath(name), content, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	pkg, err := unpackArchive(scratch, name)

	return scratch, pkg, err
}

// --- archiveName and checksumsName ---

// TestArchiveName_EveryPlatform pins the six archive names `make dist` publishes (Makefile, dist-all).
func TestArchiveName_EveryPlatform(t *testing.T) {

	tag := "v0.1.0-dev.20260930194506"
	for _, c := range []struct{ goos, goarch, want string }{
		{"darwin", "amd64", "devlore-cli_v0.1.0-dev.20260930194506_darwin_amd64.tar.gz"},
		{"darwin", "arm64", "devlore-cli_v0.1.0-dev.20260930194506_darwin_arm64.tar.gz"},
		{"linux", "amd64", "devlore-cli_v0.1.0-dev.20260930194506_linux_amd64.tar.gz"},
		{"linux", "arm64", "devlore-cli_v0.1.0-dev.20260930194506_linux_arm64.tar.gz"},
		{"windows", "amd64", "devlore-cli_v0.1.0-dev.20260930194506_windows_amd64.zip"},
		{"windows", "arm64", "devlore-cli_v0.1.0-dev.20260930194506_windows_arm64.zip"},
	} {
		if got := archiveName(tag, c.goos, c.goarch); got != c.want {
			t.Errorf("archiveName(%s/%s) = %q, want %q", c.goos, c.goarch, got, c.want)
		}
	}

	if got, want := checksumsName(tag), "devlore-cli_v0.1.0-dev.20260930194506_checksums.txt"; got != want {
		t.Errorf("checksumsName = %q, want %q", got, want)
	}
}

// TestParseArchiveName_ReadsTagAndPlatform is how `--from` learns what it was given.
func TestParseArchiveName_ReadsTagAndPlatform(t *testing.T) {

	for _, c := range []struct{ name, tag, goos, goarch string }{
		{"devlore-cli_v0.1.0-dev.20260930194506_linux_arm64.tar.gz", "v0.1.0-dev.20260930194506", "linux", "arm64"},
		{"devlore-cli_v1.2.3_windows_amd64.zip", "v1.2.3", "windows", "amd64"},
	} {
		tag, goos, goarch, ok := parseArchiveName(c.name)
		if !ok || tag != c.tag || goos != c.goos || goarch != c.goarch {
			t.Errorf("parseArchiveName(%q) = %q, %q, %q, %t", c.name, tag, goos, goarch, ok)
		}
	}

	for _, name := range []string{
		"devlore-cli_v1.2.3_checksums.txt",
		"devlore-cli_v1.2.3_linux_arm64.tar.xz",
		"writ_v1.2.3_linux_arm64.tar.gz",
		"devlore-cli_linux_arm64.tar.gz",
		"devlore-cli__linux_arm64.tar.gz",
	} {
		if _, _, _, ok := parseArchiveName(name); ok {
			t.Errorf("parseArchiveName(%q) took it for a release archive", name)
		}
	}
}

// --- listedChecksum ---

// TestListedChecksum_FindsTheExactName is Requirement 5's first half: the line for the archive is found by exact name.
//
// The installers match with a regular expression, in which a dot matches anything and a longer name containing the
// archive's matches too; neither may decide which line verifies an archive.
func TestListedChecksum_FindsTheExactName(t *testing.T) {

	archive := "devlore-cli_v1.2.3_linux_arm64.tar.gz"
	text := strings.Repeat("a", 64) + "  devlore-cli_v1.2.3_linux_arm64.tar.gz.sig\n" +
		strings.Repeat("b", 64) + "  devlore-cli_v1.2.3_linux-arm64.tar.gz\n" +
		strings.Repeat("C", 64) + "  " + archive + "\n"

	if got, ok := listedChecksum([]byte(text), archive); !ok || got != strings.Repeat("c", 64) {
		t.Errorf("listedChecksum = %q, %t; want the exact line's sum, lower-cased", got, ok)
	}

	if got, ok := listedChecksum([]byte(strings.Repeat("d", 64)+" *"+archive+"\r\n"), archive); !ok ||
		got != strings.Repeat("d", 64) {
		t.Errorf("listedChecksum of a binary-mode line = %q, %t; want its sum", got, ok)
	}

	for _, text := range []string{
		"",
		strings.Repeat("a", 64) + "  other.tar.gz\n",
		strings.Repeat("a", 63) + "  " + archive + "\n",
		strings.Repeat("g", 64) + "  " + archive + "\n",
		strings.Repeat("a", 64) + " " + archive + "\n",
	} {
		if got, ok := listedChecksum([]byte(text), archive); ok {
			t.Errorf("listedChecksum(%q) = %q; want no line", text, got)
		}
	}
}

// --- verifyArchive ---

// TestVerifyArchive_AcceptsAMatch pins the verified path, from GitHub and from disk.
func TestVerifyArchive_AcceptsAMatch(t *testing.T) {

	name, content := "devlore-cli_v1.2.3_linux_arm64.tar.gz", []byte("an archive")
	checksums := checksumsFor(map[string][]byte{name: content})

	fromGitHub := releaseFile{name: name, url: "https://example.invalid/a", digest: "sha256:" + sha256Of(content)}
	if err := verifyArchive(fromGitHub, sha256Of(content), checksums, "checksums.txt"); err != nil {
		t.Errorf("verifyArchive from GitHub: %v", err)
	}

	fromDisk := releaseFile{name: name, path: "/somewhere/" + name}
	if err := verifyArchive(fromDisk, sha256Of(content), checksums, "checksums.txt"); err != nil {
		t.Errorf("verifyArchive from disk, which carries no digest: %v", err)
	}
}

// TestVerifyArchive_RefusesEachMissingPieceAndMismatch is Requirement 5's second half: nothing unverified is unpacked.
func TestVerifyArchive_RefusesEachMissingPieceAndMismatch(t *testing.T) {

	name, content := "devlore-cli_v1.2.3_linux_arm64.tar.gz", []byte("an archive")
	sum := sha256Of(content)
	listed := checksumsFor(map[string][]byte{name: content})
	other := sha256Of([]byte("another archive"))

	for _, c := range []struct {
		label     string
		file      releaseFile
		checksums []byte
		want      []string
	}{
		{"no line for it", releaseFile{name: name, digest: "sha256:" + sum, url: "u"},
			checksumsFor(map[string][]byte{"other.tar.gz": content}), []string{"checksums.txt", "no line", name}},
		{"checksum mismatch", releaseFile{name: name, digest: "sha256:" + sum, url: "u"},
			[]byte(other + "  " + name + "\n"), []string{name, sum, other}},
		{"digest mismatch", releaseFile{name: name, digest: "sha256:" + other, url: "u"},
			listed, []string{"GitHub", sum, other}},
		{"no digest from GitHub", releaseFile{name: name, url: "u"},
			listed, []string{"GitHub", "digest", name}},
		{"a digest that is not SHA-256", releaseFile{name: name, digest: "md5:abc", url: "u"},
			listed, []string{"GitHub", "digest", name}},
	} {
		err := verifyArchive(c.file, sum, c.checksums, "checksums.txt")
		if err == nil {
			t.Errorf("%s: verifyArchive accepted it", c.label)
			continue
		}
		if ExitCode(err) != ExitDataErr {
			t.Errorf("%s: exit %d, want %d", c.label, ExitCode(err), ExitDataErr)
		}
		for _, word := range c.want {
			if !strings.Contains(err.Error(), word) {
				t.Errorf("%s: %v; want it to name %s", c.label, err, word)
			}
		}
	}
}

// --- unpackArchive ---

// TestUnpackArchive_LaysOutAsTheInstallersDo is Requirement 6's layout (install.sh:318-345).
//
// The programs at the archive's root move to `pkg/bin`, executable, so each one's `self install` finds `pkg/share`
// at `<exeDir>/../share`; everything else stays where the archive put it.
func TestUnpackArchive_LaysOutAsTheInstallersDo(t *testing.T) {

	for _, format := range []string{"tar.gz", "zip"} {
		t.Run(format, func(t *testing.T) {

			entries := []packedEntry{
				{name: "lore", body: "lore v1", mode: 0o755},
				{name: "writ", body: "writ v1", mode: 0o755},
				{name: "share/"},
				{name: "share/devlore/star/extensions/probe/extension.yaml", body: "name: probe\n"},
			}
			name, content := "devlore-cli_v1_linux_arm64.tar.gz", packTarGz(t, entries)
			if format == "zip" {
				name, content = "devlore-cli_v1_windows_arm64.zip", packZip(t, entries)
			}

			scratch, pkg, err := unpackInto(t, name, content)
			if err != nil {
				t.Fatalf("unpackArchive: %v", err)
			}
			if pkg.Rel() != "pkg" {
				t.Errorf("unpacked into %q, want pkg", pkg.Rel())
			}

			for relative, want := range map[string]string{
				"pkg/bin/lore": "lore v1",
				"pkg/bin/writ": "writ v1",
				"pkg/share/devlore/star/extensions/probe/extension.yaml": "name: probe\n",
			} {
				got, err := scratch.ReadFile(scratch.NewPath(relative))
				if err != nil || string(got) != want {
					t.Errorf("%s = %q, %v; want %q", relative, got, err, want)
				}
			}

			for _, gone := range []string{"pkg/lore", "pkg/writ"} {
				if _, err := scratch.Lstat(scratch.NewPath(gone)); !os.IsNotExist(err) {
					t.Errorf("%s is still at the package's root (stat error = %v)", gone, err)
				}
			}

			if runtime.GOOS != "windows" {
				info, err := scratch.Stat(scratch.NewPath("pkg/bin/lore"))
				if err != nil || info.Mode().Perm()&0o100 == 0 {
					t.Errorf("pkg/bin/lore is not executable: %v, %v", info, err)
				}
			}
		})
	}
}

// TestUnpackArchive_RefusesAnEntryThatEscapes is Requirement 6's containment, ruling 3's first layer.
//
// An absolute name, and a `..` that climbs out after cleaning, are refused by name; a `..` that stays inside is not.
func TestUnpackArchive_RefusesAnEntryThatEscapes(t *testing.T) {

	for _, name := range []string{"../outside", "share/../../outside", "/etc/devlore-escape"} {
		t.Run(name, func(t *testing.T) {

			content := packTarGz(t, []packedEntry{{name: "lore", body: "lore"}, {name: name, body: "escaped"}})

			scratch, _, err := unpackInto(t, "devlore-cli_v1_linux_arm64.tar.gz", content)
			if err == nil {
				t.Fatalf("unpackArchive accepted %q", name)
			}
			if !strings.Contains(err.Error(), name) {
				t.Errorf("error = %v; want it to name %q", err, name)
			}
			if _, statErr := os.Stat(filepath.Join(filepath.Dir(scratch.Name()), "outside")); !os.IsNotExist(statErr) {
				t.Errorf("the entry landed outside the scratch directory (stat error = %v)", statErr)
			}
		})
	}

	content := packTarGz(t, []packedEntry{{name: "share/devlore/../inside", body: "stays inside"}})
	scratch, _, err := unpackInto(t, "devlore-cli_v1_linux_arm64.tar.gz", content)
	if err != nil {
		t.Fatalf("unpackArchive refused a .. that stays inside: %v", err)
	}
	if got, err := scratch.ReadFile(scratch.NewPath("pkg/share/inside")); err != nil || string(got) != "stays inside" {
		t.Errorf("pkg/share/inside = %q, %v", got, err)
	}
}

// TestUnpackArchive_RefusesAPathThroughASymbolicLink is ruling 3's second layer: a link never redirects an entry.
//
// A release archive carries no links, so the link here is planted in the directory the archive unpacks into, as
// something other than the archive would have to plant it; the entry through it is refused, naming the entry.
func TestUnpackArchive_RefusesAPathThroughASymbolicLink(t *testing.T) {

	scratch, err := fsroot.OpenExisting(t.TempDir())
	if err != nil {
		t.Fatalf("OpenExisting: %v", err)
	}
	t.Cleanup(func() { _ = scratch.Close() })

	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(scratch.Name(), "pkg"), 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(scratch.Name(), "pkg", "share")); err != nil {
		t.Skipf("this platform will not create the link: %v", err)
	}

	name := "devlore-cli_v1_linux_arm64.tar.gz"
	content := packTarGz(t, []packedEntry{{name: "share/planted", body: "redirected"}})
	if err := scratch.WriteFile(scratch.NewPath(name), content, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err = unpackArchive(scratch, name)
	if err == nil {
		t.Fatal("unpackArchive wrote through a symbolic link")
	}
	if !strings.Contains(err.Error(), "share/planted") || !strings.Contains(err.Error(), "symbolic link") {
		t.Errorf("error = %v; want it to name the entry and the link", err)
	}
	if _, statErr := os.Stat(filepath.Join(outside, "planted")); !os.IsNotExist(statErr) {
		t.Errorf("the entry landed where the link points (stat error = %v)", statErr)
	}
}

// TestUnpackArchive_RefusesAKindAReleaseDoesNotCarry is ruling 1's "nothing silent": an entry is unpacked or refused.
//
// `make dist` packs regular files and their directories, and checks the archive holds exactly those; a link or a
// device in one is not something the build makes, so it is refused by name and kind rather than interpreted.
func TestUnpackArchive_RefusesAKindAReleaseDoesNotCarry(t *testing.T) {

	for _, c := range []struct {
		label  string
		format string
		entry  packedEntry
		kind   string
	}{
		{"tar symbolic link", "tar.gz",
			packedEntry{name: "link", typeflag: tar.TypeSymlink, linkname: "lore"}, "symbolic link"},
		{"tar hard link", "tar.gz", packedEntry{name: "link", typeflag: tar.TypeLink, linkname: "lore"}, "hard link"},
		{"tar FIFO", "tar.gz", packedEntry{name: "fifo", typeflag: tar.TypeFifo}, "FIFO"},
		{"zip symbolic link", "zip",
			packedEntry{name: "link", typeflag: tar.TypeSymlink, linkname: "lore"}, "symbolic link"},
	} {
		t.Run(c.label, func(t *testing.T) {

			entries := []packedEntry{{name: "lore", body: "lore"}, c.entry}
			name, content := "devlore-cli_v1_linux_arm64.tar.gz", packTarGz(t, entries)
			if c.format == "zip" {
				name, content = "devlore-cli_v1_windows_arm64.zip", packZip(t, entries)
			}

			_, _, err := unpackInto(t, name, content)
			if err == nil {
				t.Fatalf("unpackArchive accepted a %s", c.kind)
			}
			if !strings.Contains(err.Error(), c.entry.name) || !strings.Contains(err.Error(), c.kind) {
				t.Errorf("error = %v; want it to name %q and %s", err, c.entry.name, c.kind)
			}
		})
	}
}

// --- carriesSuite ---

// TestCarriesSuite_NamesWhatIsMissing is Requirement 7's check that the archive carries every program in the suite.
func TestCarriesSuite_NamesWhatIsMissing(t *testing.T) {

	name, content := packRelease(t, "v1", "lore", "writ")
	scratch, pkg, err := unpackInto(t, name, content)
	if err != nil {
		t.Fatalf("unpackArchive: %v", err)
	}

	if err := carriesSuite(scratch, pkg, name, []string{"lore", "writ"}); err != nil {
		t.Errorf("carriesSuite of what it carries: %v", err)
	}

	err = carriesSuite(scratch, pkg, name, []string{"lore", "star", "writ"})
	if err == nil {
		t.Fatal("carriesSuite accepted an archive without star")
	}
	if !strings.Contains(err.Error(), "star") || !strings.Contains(err.Error(), name) {
		t.Errorf("error = %v; want it to name star and the archive", err)
	}
}

// --- localRelease ---

// TestLocalRelease_ReadsTheTagFromTheName is Requirement 10: the tag comes from the archive's name, and the checksums
// file beside it is the one it is verified against. A file on disk carries no digest.
func TestLocalRelease_ReadsTheTagFromTheName(t *testing.T) {

	dir := t.TempDir()
	name := archiveName("v1.2.3", "linux", "arm64")
	writeTestFile(t, filepath.Join(dir, name), "an archive")
	writeTestFile(t, filepath.Join(dir, checksumsName("v1.2.3")), "")

	release, err := localRelease(filepath.Join(dir, name), "linux", "arm64")
	if err != nil {
		t.Fatalf("localRelease: %v", err)
	}

	if release.tag != "v1.2.3" {
		t.Errorf("tag = %q, want v1.2.3", release.tag)
	}
	if release.archive.name != name || release.archive.path != filepath.Join(dir, name) || release.archive.url != "" {
		t.Errorf("archive = %+v", release.archive)
	}
	if release.checksums.path != filepath.Join(dir, checksumsName("v1.2.3")) {
		t.Errorf("checksums = %+v", release.checksums)
	}
	if release.archive.digest != "" {
		t.Errorf("digest = %q; a file on disk carries none", release.archive.digest)
	}
}

// TestLocalRelease_RefusesAnotherPlatformsArchive is Requirement 10: an archive for another platform is refused.
func TestLocalRelease_RefusesAnotherPlatformsArchive(t *testing.T) {

	dir := t.TempDir()
	for _, name := range []string{
		archiveName("v1.2.3", "linux", "amd64"),
		archiveName("v1.2.3", "darwin", "arm64"),
		"devlore-cli_v1.2.3_linux_arm64.zip",
	} {
		writeTestFile(t, filepath.Join(dir, name), "an archive")
		writeTestFile(t, filepath.Join(dir, checksumsName("v1.2.3")), "")

		_, err := localRelease(filepath.Join(dir, name), "linux", "arm64")
		if err == nil {
			t.Errorf("localRelease accepted %s on linux/arm64", name)
			continue
		}
		if ExitCode(err) != ExitUsage || !strings.Contains(err.Error(), "linux/arm64") {
			t.Errorf("%s: exit %d, %v; want a usage error naming this platform", name, ExitCode(err), err)
		}
	}
}

// TestLocalRelease_RequiresTheChecksumsFileBesideIt is Requirement 10: nothing is installed that is not verified.
func TestLocalRelease_RequiresTheChecksumsFileBesideIt(t *testing.T) {

	dir := t.TempDir()
	name := archiveName("v1.2.3", "linux", "arm64")
	writeTestFile(t, filepath.Join(dir, name), "an archive")

	_, err := localRelease(filepath.Join(dir, name), "linux", "arm64")
	if err == nil {
		t.Fatal("localRelease accepted an archive with no checksums file beside it")
	}
	if ExitCode(err) != ExitNoInput || !strings.Contains(err.Error(), checksumsName("v1.2.3")) {
		t.Errorf("exit %d, %v; want a missing-input error naming the checksums file", ExitCode(err), err)
	}
}

// TestLocalRelease_RefusesWhatIsNotAReleaseArchive covers a name that is no release's and a file that is not there.
func TestLocalRelease_RefusesWhatIsNotAReleaseArchive(t *testing.T) {

	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "writ.tar.gz"), "an archive")

	if _, err := localRelease(filepath.Join(dir, "writ.tar.gz"), "linux", "arm64"); ExitCode(err) != ExitUsage {
		t.Errorf("a name that is no release's: exit %d, %v; want a usage error", ExitCode(err), err)
	}

	missing := filepath.Join(dir, archiveName("v1.2.3", "linux", "arm64"))
	if _, err := localRelease(missing, "linux", "arm64"); ExitCode(err) != ExitNoInput {
		t.Errorf("an archive that is not there: exit %d, %v; want a missing-input error", ExitCode(err), err)
	}
}
