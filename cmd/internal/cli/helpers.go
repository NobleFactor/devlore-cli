// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
)

// archiveName returns the name of a release's archive for a platform, as `make dist` names it.
//
// Parameters:
//   - `tag`: the release's tag.
//   - `goos`: the platform's operating system.
//   - `goarch`: the platform's architecture.
//
// Returns:
//   - `string`: `devlore-cli_<tag>_<goos>_<goarch>.zip` on Windows, and `.tar.gz` everywhere else.
func archiveName(tag, goos, goarch string) string {

	extension := tarGzExtension
	if goos == "windows" {
		extension = zipExtension
	}

	return archivePrefix + tag + "_" + goos + "_" + goarch + extension
}

// checksumsName returns the name of a release's checksums file, as `make checksums` names it.
//
// Parameters:
//   - `tag`: the release's tag.
//
// Returns:
//   - `string`: `devlore-cli_<tag>_checksums.txt`.
func checksumsName(tag string) string {
	return archivePrefix + tag + "_checksums.txt"
}

// executableName returns the tool's filename as the platform requires it.
//
// Windows will not execute a file without a recognized extension, so an install that copies the binary to
// `bin/writ` produces something the operator cannot run — a successful-looking install of a dead file. Found
// by the self-install scenario on its first Windows run (2026-08-17); every platform's `go build` output
// carries this suffix, and so must every installed copy.
//
// Parameters:
//   - `tool`: the tool name, unsuffixed.
//
// Returns:
//   - `string`: the tool name plus `.exe` on Windows, unchanged elsewhere.
func executableName(tool string) string {

	if runtime.GOOS == "windows" {
		return tool + ".exe"
	}

	return tool
}

// installedPrefixOf finds the installation prefix the running binary is installed in: the binary is
// `<prefix>/bin/<name>`, its links resolved.
//
// `self uninstall` finds the prefix it removes from this way when it is given none, and `self upgrade` finds the
// prefix it upgrades.
//
// Parameters:
//   - `executable`: the running binary's path, as [os.Executable] gives it.
//
// Returns:
//   - `prefix`: the installation prefix.
//   - `binary`: the running binary's path, its links resolved.
//   - `err`: the failure to resolve the binary's links, or an [ExitConfig] error for a binary that is not in a `bin/`
//     directory.
func installedPrefixOf(executable string) (prefix, binary string, err error) {

	binary, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", "", fmt.Errorf("cannot resolve the running executable %s: %w", executable, err)
	}

	bin := filepath.Dir(binary)
	if filepath.Base(bin) != "bin" {
		return "", "", ExitWith(ExitConfig, fmt.Errorf("cannot find the installation prefix: %s is not in a "+
			"<prefix>/bin/ directory", binary))
	}

	return filepath.Dir(bin), binary, nil
}

// joinPrograms names programs in a sentence: `lore`, `lore and writ`, `lore, star and writ`.
//
// Parameters:
//   - `programs`: the programs.
//
// Returns:
//   - `string`: their names, joined.
func joinPrograms(programs []string) string {

	switch len(programs) {
	case 0:
		return "no program"
	case 1:
		return programs[0]
	default:
		return strings.Join(programs[:len(programs)-1], ", ") + " and " + programs[len(programs)-1]
	}
}

// manifestPath returns where a tool's manifest is on disk.
//
// Parameters:
//   - `prefix`: the installation prefix.
//   - `toolName`: the installed tool.
//
// Returns:
//   - `string`: the manifest's path, within `prefix`.
func manifestPath(prefix, toolName string) string {
	return filepath.Join(prefix, relativeManifestPath(toolName))
}

// readManifest reads the manifest a tool's `self install` wrote: the record of what the tool owns in a prefix.
//
// Parameters:
//   - `prefix`: the installation prefix.
//   - `toolName`: the installed tool.
//
// Returns:
//   - `*manifest`: the record.
//   - `error`: the manifest cannot be read, or is not a manifest.
func readManifest(prefix, toolName string) (*manifest, error) {

	data, err := os.ReadFile(manifestPath(prefix, toolName))
	if err != nil {
		return nil, err
	}

	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	return &m, nil
}

// relativeManifestPath returns the manifest's path within the install prefix.
//
// Named separately because a root addresses its contents relatively: [manifestPath] answers "where is it on
// disk", this answers "where is it in the tree", and both derive from one definition.
//
// Parameters:
//   - `toolName`: the installed tool.
//
// Returns:
//   - `string`: the manifest path relative to the install prefix.
func relativeManifestPath(toolName string) string {
	return filepath.Join("share", toolName, "manifest.json")
}

// replaceBinary places `source` at `bin/<name>` in `prefixRoot` without a window in which the program is missing.
//
// Nothing may write over a running binary. Linux refuses to open one for writing (ETXTBSY), Windows refuses to
// rewrite, remove or replace one, and macOS kills the next run of a signed binary rewritten in place. What Unix
// allows is a rename over it: the name moves to the new file in one step, and a process running the old one keeps
// the file it started from. So the new binary is staged beside the target and renamed over it.
//
// The staged copy has a fixed name, `<executable>.new`, not a unique one: a run that fails between staging it and
// renaming it leaves one file behind, which the next run writes over (#947, D5).
//
// Windows refuses even the rename over a running image, but renames the image itself. When the rename is refused
// there, the running binary is renamed aside to [setAsideName], over any stale one that no longer runs, and
// the staged copy is renamed into the name it left. The program is missing between those two renames, and after
// them only if the second fails; nothing is rolled back, and a rerun places it.
//
// Every install reaches this through [installBinary]; `self upgrade` calls it to place each program it fetched
// before that program's own `self install` runs.
//
// Parameters:
//   - `prefixRoot`: the installation prefix.
//   - `source`: the binary to install, outside the root.
//   - `name`: the tool name, unsuffixed.
//
// Returns:
//   - `fsroot.Path`: the installed binary.
//   - `error`: non-nil when the source cannot be read, the copy cannot be staged, or no rename puts it in place.
func replaceBinary(prefixRoot fsroot.Dir, source, name string) (fsroot.Path, error) {

	binDir := prefixRoot.NewPath("bin")
	target := prefixRoot.NewPath("bin", executableName(name))
	staged := prefixRoot.NewPath("bin", executableName(name)+".new")

	if err := prefixRoot.MkdirAll(binDir, 0o750); err != nil {
		return fsroot.Path{}, fmt.Errorf("failed to create directory %s: %w", binDir.Abs(), err)
	}

	if err := stageBinary(prefixRoot, source, staged); err != nil {
		return fsroot.Path{}, err
	}

	err := prefixRoot.Rename(staged, target)
	if err == nil {
		return target, nil
	}

	// Refused on Windows: the running image. Anything else, or anywhere else, is reported as it is.
	aside := setAsideName(name)
	if aside == "" || !errors.Is(err, fs.ErrPermission) {
		return fsroot.Path{}, fmt.Errorf("failed to rename %s over %s: %w", staged.Abs(), target.Abs(), err)
	}

	asidePath := prefixRoot.NewPath("bin", aside)
	if asideErr := prefixRoot.Rename(target, asidePath); asideErr != nil {
		return fsroot.Path{}, fmt.Errorf("failed to replace %s, or to set it aside as %s: %w",
			target.Abs(), asidePath.Abs(), errors.Join(err, asideErr))
	}

	if err := prefixRoot.Rename(staged, target); err != nil {
		return fsroot.Path{}, fmt.Errorf("failed to rename %s into place, with the running binary set aside as %s: %w",
			staged.Abs(), asidePath.Abs(), err)
	}

	return target, nil
}

// setAsideName returns the name in `bin/` that a running binary is renamed to, so its replacement can take its place.
//
// Only Windows needs one. It refuses to replace a running image but renames it, so the image steps aside and stays
// until the next install retires it (#947, D5). Elsewhere the rename over a running binary succeeds, nothing is set
// aside, and the name is empty.
//
// Parameters:
//   - `tool`: the tool name, unsuffixed.
//
// Returns:
//   - `string`: `<tool>.exe.old` on Windows; empty elsewhere.
func setAsideName(tool string) string {

	if runtime.GOOS == "windows" {
		return executableName(tool) + ".old"
	}

	return ""
}
