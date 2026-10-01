// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	securejoin "github.com/cyphar/filepath-securejoin"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
)

// A release carries one archive per platform and one checksums file (Makefile, dist-all and checksums):
// `devlore-cli_<tag>_<os>_<arch>.tar.gz`, `.zip` on Windows, and `devlore-cli_<tag>_checksums.txt`. The archive holds
// the programs at its root and star's extensions under `share/`. An upgrade verifies the archive against its line in
// the checksums file and, from GitHub, the asset's digest, and only then unpacks it, contained, into the run's scratch
// directory, laid out as the installers lay it out (#947, Requirements 5, 6 and 10).

// archivePrefix, tarGzExtension and zipExtension spell a release archive's name.
const (
	archivePrefix  = "devlore-cli_"
	tarGzExtension = ".tar.gz"
	zipExtension   = ".zip"
)

// sha256Prefix is how GitHub labels an asset's SHA-256 in its `digest`.
const sha256Prefix = "sha256:"

// region HELPER FUNCTIONS

// Fallible actions

// carriesSuite checks that the unpacked package carries every program of the suite, in `pkg/bin`.
//
// The suite is upgraded together or not at all: an archive that lacks one of the prefix's programs would leave it on
// the old release while the rest moved, so it is refused before anything is placed.
//
// Parameters:
//   - `scratch`: the run's scratch directory.
//   - `pkg`: the unpacked package within it.
//   - `archive`: the archive's name, for the refusal.
//   - `suite`: the programs the prefix holds.
//
// Returns:
//   - `error`: an [ExitDataErr] error naming the archive and every program it lacks; nil when it carries them all.
func carriesSuite(scratch fsroot.Dir, pkg fsroot.Path, archive string, suite []string) error {

	var missing []string
	for _, program := range suite {
		info, err := scratch.Lstat(scratch.NewPath(pkg.Rel(), "bin", executableName(program)))
		if err != nil || !info.Mode().IsRegular() {
			missing = append(missing, program)
		}
	}

	if len(missing) > 0 {
		return ExitWith(ExitDataErr, fmt.Errorf("%s does not carry %s, which this prefix holds: the programs in a "+
			"prefix are upgraded together", archive, joinPrograms(missing)))
	}

	return nil
}

// containedEntry returns where an archive entry unpacks within `pkg`, or refuses an entry that would land elsewhere.
//
// Containment is the archive provider's, ruled 2026-07-18 (3.5.1-archive-provider.md §10, ruling 3), in its three
// layers. Policy: a name that is absolute, rooted, on a volume, or that climbs out with `..` after cleaning is
// refused, naming it. Resolution: [securejoin.SecureJoin] resolves the name against the links already on disk, and
// any difference from the plain join means a symbolic link would redirect the entry, which is refused, never
// followed. Enforcement: every write goes through `scratch`, an [fsroot.Dir], whose [os.Root] refuses an escape at
// the system call, closing the window between this check and the write.
//
// Parameters:
//   - `scratch`: the run's scratch directory.
//   - `pkg`: the directory the archive unpacks into, within `scratch`.
//   - `name`: the entry's name, as the archive records it.
//
// Returns:
//   - `fsroot.Path`: the entry's place within `scratch`.
//   - `error`: an error naming the entry when it would escape `pkg` or pass through a symbolic link.
func containedEntry(scratch fsroot.Dir, pkg fsroot.Path, name string) (fsroot.Path, error) {

	cleaned := filepath.Clean(filepath.FromSlash(name))

	if filepath.IsAbs(cleaned) || filepath.VolumeName(cleaned) != "" ||
		strings.HasPrefix(cleaned, string(filepath.Separator)) ||
		cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return fsroot.Path{}, fmt.Errorf("entry %q escapes the directory the archive unpacks into", name)
	}

	lexical := filepath.Join(pkg.Abs(), cleaned)

	resolved, err := securejoin.SecureJoin(pkg.Abs(), cleaned)
	if err != nil {
		return fsroot.Path{}, fmt.Errorf("entry %q cannot be resolved within %s: %w", name, pkg.Abs(), err)
	}

	if resolved != lexical {
		return fsroot.Path{}, fmt.Errorf("entry %q passes through a symbolic link: it resolves to %s, not %s",
			name, resolved, lexical)
	}

	return scratch.NewPath(pkg.Rel(), cleaned), nil
}

// extractEntry unpacks one archive entry, a directory or a regular file, into `pkg`, contained.
//
// The archive's permission bits are kept, as tar and unzip keep them, with the owner always able to read and write:
// the programs' own `self install` reads what lies here, and star copies its directories' modes into the prefix.
//
// Parameters:
//   - `scratch`: the run's scratch directory.
//   - `pkg`: the directory the archive unpacks into.
//   - `name`: the entry's name, as the archive records it.
//   - `directory`: whether the entry is a directory; otherwise it is a regular file.
//   - `mode`: the entry's mode, as the archive records it.
//   - `body`: a regular file's content.
//
// Returns:
//   - `error`: the containment refusal, or a failure to create the entry.
func extractEntry(
	scratch fsroot.Dir, pkg fsroot.Path, name string, directory bool, mode fs.FileMode, body io.Reader,
) error {

	target, err := containedEntry(scratch, pkg, name)
	if err != nil {
		return err
	}

	if directory {
		if err := scratch.MkdirAll(target, mode.Perm()|0o700); err != nil {
			return fmt.Errorf("cannot unpack %q: %w", name, err)
		}
		return nil
	}

	if err := scratch.MkdirAll(scratch.NewPath(path.Dir(target.Rel())), 0o750); err != nil {
		return fmt.Errorf("cannot unpack %q: %w", name, err)
	}

	return writeEntry(scratch, target, mode.Perm()|0o600, body)
}

// extractTarGz unpacks a gzip-compressed tar archive into `pkg`, entry by entry.
//
// Parameters:
//   - `scratch`: the run's scratch directory.
//   - `archive`: the archive within it.
//   - `pkg`: the directory it unpacks into.
//
// Returns:
//   - `error`: an unreadable archive, an entry of a kind a release does not carry, or [extractEntry]'s.
func extractTarGz(scratch fsroot.Dir, archive, pkg fsroot.Path) (err error) {

	file, err := scratch.Open(archive)
	if err != nil {
		return err
	}
	defer iox.Close(&err, file)

	decompressed, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("%s is not gzip-compressed: %w", archive.Rel(), err)
	}
	defer iox.Close(&err, decompressed)

	entries := tar.NewReader(decompressed)
	for {
		header, err := entries.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%s cannot be read: %w", archive.Rel(), err)
		}

		if header.Typeflag != tar.TypeDir && header.Typeflag != tar.TypeReg {
			return unsupportedEntry(archive.Rel(), header.Name, tarEntryKind(header.Typeflag))
		}

		// No cap on an entry's size: the filesystem is the budget (3.5.1-archive-provider.md §10, ruling 2).
		mode := fs.FileMode(header.Mode).Perm() //nolint:gosec // G115: permission bits, masked to nine
		if err := extractEntry(scratch, pkg, header.Name, header.Typeflag == tar.TypeDir, mode, entries); err != nil {
			return err
		}
	}
}

// extractZip unpacks a zip archive into `pkg`, entry by entry.
//
// Parameters:
//   - `scratch`: the run's scratch directory.
//   - `archive`: the archive within it.
//   - `pkg`: the directory it unpacks into.
//
// Returns:
//   - `error`: an unreadable archive, an entry of a kind a release does not carry, or [extractEntry]'s.
func extractZip(scratch fsroot.Dir, archive, pkg fsroot.Path) (err error) {

	file, err := scratch.Open(archive)
	if err != nil {
		return err
	}
	defer iox.Close(&err, file)

	info, err := file.Stat()
	if err != nil {
		return err
	}

	entries, err := zip.NewReader(file, info.Size())
	if err != nil {
		return fmt.Errorf("%s is not a zip archive: %w", archive.Rel(), err)
	}

	for _, entry := range entries.File {
		if err := extractZipEntry(scratch, archive, pkg, entry); err != nil {
			return err
		}
	}

	return nil
}

// extractZipEntry unpacks one zip entry into `pkg`.
//
// Parameters:
//   - `scratch`: the run's scratch directory.
//   - `archive`: the archive, for a refusal.
//   - `pkg`: the directory it unpacks into.
//   - `entry`: the entry.
//
// Returns:
//   - `error`: an entry of a kind a release does not carry, an unreadable entry, or [extractEntry]'s.
func extractZipEntry(scratch fsroot.Dir, archive, pkg fsroot.Path, entry *zip.File) (err error) {

	mode := entry.Mode()
	if !mode.IsDir() && !mode.IsRegular() {
		return unsupportedEntry(archive.Rel(), entry.Name, zipEntryKind(mode))
	}

	if mode.IsDir() {
		return extractEntry(scratch, pkg, entry.Name, true, mode, nil)
	}

	body, err := entry.Open()
	if err != nil {
		return fmt.Errorf("%s: entry %q cannot be read: %w", archive.Rel(), entry.Name, err)
	}
	defer iox.Close(&err, body)

	// No cap on an entry's size: the filesystem is the budget (3.5.1-archive-provider.md §10, ruling 2).
	return extractEntry(scratch, pkg, entry.Name, false, mode, body)
}

// layOutPackage lays out an unpacked archive as the installers do (install.sh:318-345): every regular file at the
// package's root is a program, and moves to `pkg/bin`, executable; everything else stays where the archive put it.
//
// Each program's `self install` then finds `pkg/share` at `<exeDir>/../share`, where it looks for what it installs
// beside its binary.
//
// Parameters:
//   - `scratch`: the run's scratch directory.
//   - `pkg`: the unpacked package.
//
// Returns:
//   - `error`: a failure to create `pkg/bin`, read the package, or move or mark a program.
func layOutPackage(scratch fsroot.Dir, pkg fsroot.Path) error {

	bin := scratch.NewPath(pkg.Rel(), "bin")
	if err := scratch.MkdirAll(bin, 0o750); err != nil {
		return err
	}

	entries, err := fs.ReadDir(scratch.FS(), pkg.Rel())
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}

		program := scratch.NewPath(bin.Rel(), entry.Name())
		if err := scratch.Rename(scratch.NewPath(pkg.Rel(), entry.Name()), program); err != nil {
			return err
		}
		if err := scratch.Chmod(program, 0o750); err != nil {
			return err
		}
	}

	return nil
}

// localRelease describes the release `--from` names: an archive on disk, with its checksums file beside it.
//
// The tag comes from the archive's name, and the name must be this platform's archive, so an archive built for
// another system or architecture is refused, as is one with no checksums file beside it: nothing is installed that
// is not verified. A file on disk carries no digest; only GitHub's API gives one (#947, Requirement 10).
//
// Parameters:
//   - `archive`: the archive's path, as `--from` gives it.
//   - `goos`: this platform's operating system.
//   - `goarch`: this platform's architecture.
//
// Returns:
//   - `upgradeRelease`: the release: its tag, and the two files on disk.
//   - `error`: an [ExitUsage] error for a name that is not this platform's release archive, or an [ExitNoInput]
//     error for an archive, or a checksums file beside it, that is not there.
func localRelease(archive, goos, goarch string) (upgradeRelease, error) {

	absolute, err := filepath.Abs(archive)
	if err != nil {
		return upgradeRelease{}, ExitWith(ExitNoInput, fmt.Errorf("--from %s: %w", archive, err))
	}

	name := filepath.Base(absolute)
	tag, archiveGOOS, archiveGOARCH, ok := parseArchiveName(name)
	if !ok {
		return upgradeRelease{}, ExitWith(ExitUsage, fmt.Errorf("--from %s is not a release archive: a release "+
			"names its archives %s<tag>_<os>_<arch>%s, or %s on Windows", archive, archivePrefix, tarGzExtension,
			zipExtension))
	}

	if want := archiveName(tag, goos, goarch); name != want {
		return upgradeRelease{}, ExitWith(ExitUsage, fmt.Errorf("--from %s is release %s's archive for %s/%s; "+
			"this is %s/%s, whose archive is %s", archive, tag, archiveGOOS, archiveGOARCH, goos, goarch, want))
	}

	checksums := filepath.Join(filepath.Dir(absolute), checksumsName(tag))

	// Unsandboxed: both files are wherever the operator put them, not in a tree this program owns.
	if _, err := os.Stat(absolute); err != nil {
		return upgradeRelease{}, ExitWith(ExitNoInput, fmt.Errorf("--from %s: %w", archive, err))
	}
	if _, err := os.Stat(checksums); err != nil {
		return upgradeRelease{}, ExitWith(ExitNoInput, fmt.Errorf("--from %s: the checksums file %s must sit beside "+
			"it, so the archive can be verified: %w", archive, checksumsName(tag), err))
	}

	return upgradeRelease{
		tag:       tag,
		archive:   releaseFile{name: name, path: absolute},
		checksums: releaseFile{name: checksumsName(tag), path: checksums},
	}, nil
}

// unpackArchive unpacks a verified release archive in `scratch` into `scratch/pkg`, laid out as the installers lay
// it out.
//
// The format is the one its name says, which the release chose: `.zip` on Windows, `.tar.gz` everywhere else.
//
// Parameters:
//   - `scratch`: the run's scratch directory.
//   - `archive`: the archive's name, at the root of `scratch`.
//
// Returns:
//   - `fsroot.Path`: the unpacked package, `pkg`, with the programs in `pkg/bin`.
//   - `error`: [extractTarGz]'s, [extractZip]'s or [layOutPackage]'s.
func unpackArchive(scratch fsroot.Dir, archive string) (fsroot.Path, error) {

	pkg := scratch.NewPath("pkg")
	if err := scratch.MkdirAll(pkg, 0o750); err != nil {
		return fsroot.Path{}, err
	}

	extract := extractTarGz
	if strings.HasSuffix(archive, zipExtension) {
		extract = extractZip
	}

	if err := extract(scratch, scratch.NewPath(archive), pkg); err != nil {
		return fsroot.Path{}, err
	}

	if err := layOutPackage(scratch, pkg); err != nil {
		return fsroot.Path{}, fmt.Errorf("cannot lay out %s: %w", archive, err)
	}

	return pkg, nil
}

// unsupportedEntry refuses an entry of a kind a release archive does not carry, naming the entry and its kind.
//
// `make dist` packs regular files and their directories and checks the archive holds exactly those, so anything else
// is not something the build made; it is refused rather than interpreted, and nothing is skipped in silence
// (3.5.1-archive-provider.md §10, ruling 1).
//
// Parameters:
//   - `archive`: the archive.
//   - `name`: the entry's name.
//   - `kind`: its kind, in words.
//
// Returns:
//   - `error`: an [ExitDataErr] error.
func unsupportedEntry(archive, name, kind string) error {
	return ExitWith(ExitDataErr, fmt.Errorf("%s: entry %q is a %s, which a release archive does not carry",
		archive, name, kind))
}

// verifyArchive checks an archive's SHA-256 against its line in the checksums file and, from GitHub, the asset's
// digest; nothing is unpacked until both agree (#947, Requirement 5).
//
// The line is found by the archive's exact name. Both the checksums file and the digest come from the release's
// own job, so this proves the bytes are the ones it published, not who published them.
//
// Parameters:
//   - `archive`: the archive; a file on disk carries no digest, and is checked against the checksums file alone.
//   - `sum`: the archive's SHA-256, in hex, taken as it was written to scratch.
//   - `checksums`: the checksums file's content.
//   - `checksumsFile`: the checksums file's name, for a refusal.
//
// Returns:
//   - `error`: an [ExitDataErr] error when the checksums file has no line for the archive, GitHub gives no SHA-256
//     digest for it, or either disagrees with the archive.
func verifyArchive(archive releaseFile, sum string, checksums []byte, checksumsFile string) error {

	listed, found := listedChecksum(checksums, archive.name)
	if !found {
		return ExitWith(ExitDataErr, fmt.Errorf("%s has no line for %s, so the archive cannot be verified",
			checksumsFile, archive.name))
	}

	if listed != sum {
		return ExitWith(ExitDataErr, fmt.Errorf("%s does not match %s: its SHA-256 is %s, and the checksums file "+
			"lists %s", archive.name, checksumsFile, sum, listed))
	}

	if archive.path != "" {
		return nil
	}

	digest, found := strings.CutPrefix(archive.digest, sha256Prefix)
	if !found {
		return ExitWith(ExitDataErr, fmt.Errorf("GitHub gives no SHA-256 digest for %s (its digest is %q), so the "+
			"archive cannot be verified", archive.name, archive.digest))
	}

	if !strings.EqualFold(digest, sum) {
		return ExitWith(ExitDataErr, fmt.Errorf("%s does not match GitHub's digest: its SHA-256 is %s, and GitHub's "+
			"is %s", archive.name, sum, digest))
	}

	return nil
}

// writeEntry writes a regular file's content to `target`, replacing any file an earlier entry of the same name wrote.
//
// Parameters:
//   - `scratch`: the run's scratch directory.
//   - `target`: the file, contained.
//   - `perm`: its permission bits.
//   - `body`: its content.
//
// Returns:
//   - `error`: a failure to create, write or close the file.
func writeEntry(scratch fsroot.Dir, target fsroot.Path, perm fs.FileMode, body io.Reader) (err error) {

	file, err := scratch.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("cannot unpack %s: %w", target.Rel(), err)
	}
	defer iox.Close(&err, file)

	if _, err := io.Copy(file, body); err != nil {
		return fmt.Errorf("cannot unpack %s: %w", target.Rel(), err)
	}

	return nil
}

// Actions

// listedChecksum returns the SHA-256 a checksums file lists for `name`, found by exact name.
//
// A line is `<hex>  <name>`, or `<hex> *<name>` for a file hashed in binary mode, as `shasum` and `sha256sum` write
// them. A name that merely contains `name`, or matches it as a pattern, is a different file.
//
// Parameters:
//   - `checksums`: the checksums file's content.
//   - `name`: the file to find.
//
// Returns:
//   - `string`: the listed SHA-256, lower-cased.
//   - `bool`: false when no well-formed line names exactly `name`.
func listedChecksum(checksums []byte, name string) (string, bool) {

	for line := range strings.SplitSeq(string(checksums), "\n") {
		sum, rest, found := strings.Cut(strings.TrimRight(line, "\r"), " ")
		if !found || len(sum) != 64 || rest == "" || (rest[0] != ' ' && rest[0] != '*') || rest[1:] != name {
			continue
		}
		if strings.Trim(strings.ToLower(sum), "0123456789abcdef") != "" {
			continue
		}
		return strings.ToLower(sum), true
	}

	return "", false
}

// parseArchiveName reads the tag and platform from a release archive's name.
//
// The platform is the name's last two fields, so a tag may itself hold an underscore.
//
// Parameters:
//   - `name`: the file's name.
//
// Returns:
//   - `tag`: the release's tag.
//   - `goos`: the operating system it was built for.
//   - `goarch`: the architecture it was built for.
//   - `ok`: false when the name is not `devlore-cli_<tag>_<os>_<arch>.tar.gz` or `.zip`.
func parseArchiveName(name string) (tag, goos, goarch string, ok bool) {

	rest, found := strings.CutPrefix(name, archivePrefix)
	if !found {
		return "", "", "", false
	}

	trimmed, isTarGz := strings.CutSuffix(rest, tarGzExtension)
	if !isTarGz {
		if trimmed, found = strings.CutSuffix(rest, zipExtension); !found {
			return "", "", "", false
		}
	}

	platform := strings.LastIndex(trimmed, "_")
	if platform < 0 {
		return "", "", "", false
	}
	goarch = trimmed[platform+1:]

	system := strings.LastIndex(trimmed[:platform], "_")
	if system < 0 {
		return "", "", "", false
	}
	goos, tag = trimmed[system+1:platform], trimmed[:system]

	return tag, goos, goarch, tag != "" && goos != "" && goarch != ""
}

// tarEntryKind names a tar entry's kind, for a refusal.
//
// Parameters:
//   - `typeflag`: the entry's tar typeflag.
//
// Returns:
//   - `string`: the kind, in words.
func tarEntryKind(typeflag byte) string {

	switch typeflag {
	case tar.TypeSymlink:
		return "symbolic link"
	case tar.TypeLink:
		return "hard link"
	case tar.TypeChar:
		return "character device"
	case tar.TypeBlock:
		return "block device"
	case tar.TypeFifo:
		return "FIFO"
	default:
		return fmt.Sprintf("tar entry of typeflag %q", typeflag)
	}
}

// zipEntryKind names a zip entry's kind, for a refusal.
//
// Parameters:
//   - `mode`: the entry's mode.
//
// Returns:
//   - `string`: the kind, in words.
func zipEntryKind(mode fs.FileMode) string {

	if mode&fs.ModeSymlink != 0 {
		return "symbolic link"
	}

	return fmt.Sprintf("special file (mode %s)", mode)
}

// endregion
