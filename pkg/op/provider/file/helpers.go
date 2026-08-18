// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package file

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	osuser "os/user"

	// Aliased: three functions in this file take a `path` parameter, and the slash-form glob
	// matcher needs the stdlib path package beside them.
	slashpath "path"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// errKindMismatch marks a taxonomy kind-mismatch (phase-8 step 23, ruling 5e).
//
// The plan asserted one file kind, the disk shows another. Wrapped by [kindMismatchError]; test with
// errors.Is.
var errKindMismatch = errors.New("file kind mismatch")

// applyOwnership changes the owner and/or group of path.
//
// Both sides empty is a no-op — the function returns nil without invoking any system call, which is the contract that
// lets the file-provider write methods always call applyOwnership unconditionally and rely on the short-circuit.
//
// Either side accepts a name (resolved via os/user) or a decimal integer (passed to os.Chown directly), and either may
// be empty to leave that side unchanged. Mixed forms are allowed: `user="alice"`, `group="1000"` resolves alice's uid
// and uses gid 1000.
//
// The two are separate parameters rather than one Dockerfile-style `"user:group"` string because the surface they
// project into is Starlark, where Python's `shutil.chown(path, user=…, group=…)` is the familiar shape — and because a
// colon-delimited string makes the caller build a value the callee immediately takes apart.
//
// Parameters:
//   - `path`: the filesystem path whose ownership changes.
//   - `user`: the owner to set, by name or decimal uid; empty leaves the owner unchanged.
//   - `group`: the group to set, by name or decimal gid; empty leaves the group unchanged.
//
// Returns:
//   - `error`: non-nil if a name does not resolve, or os.Chown fails.
func applyOwnership(path, user, group string) error {

	if user == "" && group == "" {
		return nil
	}

	uid, gid, err := resolveOwnership(user, group)
	if err != nil {
		return fmt.Errorf("ownership %q: %w", path, err)
	}

	if err := os.Chown(path, uid, gid); err != nil {
		return fmt.Errorf("ownership %q: %w", path, err)
	}

	return nil
}

// buildCandidateAs validates `value`, parses any file URI per RFC 8089, and constructs the [Resource] base.
//
// The shared trunk of the variant constructors (phase-8 step 23): the returned base is embedded into the variant by
// the caller, so the minted [op.ResourceBase] must already carry the variant's canonical type id — the key the
// framework dispatches on (rehydration constructors, the pre-flight resolve pass's staging gate). This function does
// not touch the resource catalog; callers route the wrapped candidate through [internEntry] themselves. The
// catch-all's [buildCandidate] delegates here with the base type id (sealed in-package use only).
//
// Parameters:
//   - `runtimeEnvironment`: the session's runtime environment; supplies `Root` for path canonicalization and is
//     embedded via [op.NewResourceBase].
//   - `value`: an `any` carrying a string filesystem path (or the provider's own emitted identity
//     specific, `file://` + path, on the rehydration round-trip); other dynamic types are rejected.
//   - `resourceType`: the concrete variant pointer type (e.g. `reflect.TypeFor[*Regular]()`) minted into the base.
//
// Returns:
//   - `*Resource`: the constructed candidate base, ready for embedding. Not interned in the catalog.
//   - `error`: non-nil if `value` is not a string or [op.NewResourceBase] fails.
func buildCandidateAs(
	runtimeEnvironment *op.RuntimeEnvironment,
	value any,
	resourceType reflect.Type,
) (resource *Resource, err error) {

	path, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("file.Resource: expected string, got %T", value)
	}

	// The input is a filesystem path. One internal round-trip also lands here: catalog rehydration hands
	// back this provider's own emitted identity specific ("file://" + path) — strip our own prefix and it
	// is a path again. No URI parsing: the provider decodes only what it mints (readback does the same).
	path = strings.TrimPrefix(path, "file://")
	sourcePath := runtimeEnvironment.Root().NewPath(path)
	var base op.ResourceBase

	// NOTE: this identity is OS-native, so on Windows it carries backslashes inside a URI — see #547.
	// Normalizing HERE was tried on 2026-08-18 and reverted: it fixed the identity string and broke four
	// consumers that key on the native form, because the transform belongs at resource construction with
	// the resource owning both forms, not at one site along the way.
	base, err = op.NewResourceBase(runtimeEnvironment, "file://"+sourcePath.Abs(), resourceType)
	if err != nil {
		return nil, err
	}

	return &Resource{
		ResourceBase: base,
		SourcePath:   sourcePath,
	}, nil
}

// candidateOfMode builds an unlinked taxonomy candidate for `abs`, kinded by an already-observed mode.
//
// The un-interned counterpart of the observed-kind constructors: receipts and other bookkeeping need a typed
// identity handle without a catalog claim. An entry of any other kind (FIFO, socket, device) is an error.
//
// Parameters:
//   - `runtimeEnvironment`: the session runtime environment.
//   - `abs`: the absolute path the candidate identifies.
//   - `mode`: the observed [os.FileMode] choosing the variant.
//
// Returns:
//   - `Entry`: the unlinked variant candidate.
//   - `error`: an unsupported entry kind, or a construction failure.
func candidateOfMode(runtimeEnvironment *op.RuntimeEnvironment, abs string, mode os.FileMode) (Entry, error) {

	switch {
	case mode&os.ModeSymlink != 0:
		base, err := buildCandidateAs(runtimeEnvironment, abs, reflect.TypeFor[*SymbolicLink]())
		if err != nil {
			return nil, err
		}
		return &SymbolicLink{Resource: *base}, nil
	case mode.IsDir():
		base, err := buildCandidateAs(runtimeEnvironment, abs, reflect.TypeFor[*Directory]())
		if err != nil {
			return nil, err
		}
		return &Directory{Resource: *base}, nil
	case mode.IsRegular():
		base, err := buildCandidateAs(runtimeEnvironment, abs, reflect.TypeFor[*Regular]())
		if err != nil {
			return nil, err
		}
		return &Regular{Resource: *base}, nil
	default:
		return nil, fmt.Errorf("file: %s: unsupported entry kind %s (no taxonomy variant)", abs, mode)
	}
}

// checksumBytes computes the "sha256:<hex>" checksum string for `data`.
//
// Parameters:
//   - `data`: the bytes to hash.
//
// Returns:
//   - `string`: the checksum in "sha256:<hex>" form.
func checksumBytes(data []byte) string {
	h := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(h[:])
}

// checksumFile reads the file at `path` and returns its "sha256:<hex>" checksum.
//
// Parameters:
//   - `root`: the [fsroot.Dir] used to read `path`.
//   - `path`: the path to hash.
//
// Returns:
//   - `string`: the checksum in "sha256:<hex>" form, or "" when the file cannot be read.
func checksumFile(root fsroot.Dir, path string) string {

	data, err := root.ReadFile(root.NewPath(path))
	if err != nil {
		return ""
	}

	return checksumBytes(data)
}

// contentDigest computes the streamed sha256 of the regular file at `abs` (no full-file allocation).
//
// Parameters:
//   - `root`: the [fsroot.Dir] used to open `abs`.
//   - `abs`: the absolute path of the regular file to hash.
//
// Returns:
//   - `op.Digest`: sha256 algorithm with 32 raw bytes.
//   - `error`: any open or read error.
func contentDigest(root fsroot.Dir, abs string) (digest op.Digest, err error) {

	f, err := root.Open(root.NewPath(abs))
	if err != nil {
		return op.Digest{}, err
	}
	defer iox.Close(&err, f)

	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return op.Digest{}, err
	}

	return op.Digest{Algorithm: "sha256", Bytes: h.Sum(nil)}, nil
}

// internEntry routes a constructed taxonomy candidate through the session catalog.
//
// It asserts the canonical entry's concrete type. The shared trunk of the variant constructors (phase-8
// step 23). `claim` selects the catalog verb: true routes
// through [op.ResourceCatalog.GetOrCreate] (a production claim stamped with `producerID`); false routes through
// [op.ResourceCatalog.Discover] (no production claim; `producerID` is ignored). Nil-Catalog tolerance:
// the unlinked candidate is returned as-is. A catalog entry of a different concrete type —
// the same URI claimed as two different kinds — is an error, surfacing cross-kind plan conflicts at the earliest
// moment.
//
// Parameters:
//   - `runtimeEnvironment`: the session runtime environment.
//   - `producerID`: the producing caller's id for a production claim; "" for discovery or caller-less dispatch.
//   - `claim`: true to claim production (GetOrCreate); false to discover.
//   - `candidate`: the constructed, not-yet-interned variant.
//
// Returns:
//   - `E`: the canonical catalog entry (or the unlinked candidate when no catalog is present).
//   - `error`: a catalog assertion failure, or a cross-kind collision on the URI.
func internEntry[E Entry](
	runtimeEnvironment *op.RuntimeEnvironment,
	producerID string,
	claim bool,
	candidate E,
) (E, error) {

	if runtimeEnvironment.ResourceCatalog == nil {
		return candidate, nil
	}

	factory := func() (op.Resource, error) { return candidate, nil }

	var got op.Resource
	var err error

	if claim {
		got, err = runtimeEnvironment.ResourceCatalog.GetOrCreate(producerID, candidate.URI(), factory)
	} else {
		got, err = runtimeEnvironment.ResourceCatalog.Discover(candidate.URI(), factory)
	}

	if err != nil {
		var zero E
		return zero, err
	}

	canonical, ok := got.(E)
	if !ok {
		var zero E
		return zero, fmt.Errorf("file: catalog entry for %q is %T, want %T", candidate.URI(), got, candidate)
	}

	return canonical, nil
}

// isDirNotEmpty reports whether `err` is the "directory not empty" (ENOTEMPTY) error.
//
// Parameters:
//   - `err`: the error to test.
//
// Returns:
//   - `bool`: true when `err` wraps [syscall.ENOTEMPTY].
func isDirNotEmpty(err error) bool {
	return errors.Is(err, syscall.ENOTEMPTY)
}

// kindMismatchError builds the taxonomy kind-mismatch error (phase-8 step 23, ruling 5e).
//
// The plan asserted one file kind, the disk shows another.
//
// Parameters:
//   - `typeName`: the asserting variant's display name (e.g. "file.Regular").
//   - `path`: the absolute path whose observed kind diverges.
//   - `observed`: the [os.FileMode] the disk actually shows.
//
// Returns:
//   - `error`: wraps [errKindMismatch]; test with errors.Is.
func kindMismatchError(typeName, path string, observed os.FileMode) error {
	return fmt.Errorf("%s: %s: %w: the plan asserted this kind, the disk shows mode %s",
		typeName, path, errKindMismatch, observed)
}

// matchDoubleStar reports whether `relPath` matches `pattern`, supporting `**` recursive wildcards.
//
// A pattern with no `**` is delegated to [path.Match] semantics; a single `**` is handled segment-by-segment; and
// multiple `**` fall back to matching the trailing component against the path's base name. Both inputs are
// slash-form on every platform — glob patterns are a slash-form language, and the caller converts walked paths at
// the boundary.
//
// Parameters:
//   - `pattern`: the slash-form glob pattern, which may contain `**`.
//   - `relPath`: the slash-form path to test.
//
// Returns:
//   - `bool`: true when `relPath` matches `pattern`.
func matchDoubleStar(pattern, relPath string) bool {

	parts := strings.Split(pattern, "**")
	if len(parts) == 1 {
		return pathMatch(pattern, relPath)
	}

	if len(parts) == 2 {
		return matchDoubleStarSingle(parts[0], parts[1], relPath)
	}

	tail := strings.TrimLeft(parts[len(parts)-1], "/")
	return pathMatch(tail, slashpath.Base(relPath))
}

// matchDoubleStarSingle reports whether `relPath` matches a single-`**` pattern split into `rawPrefix` and
// `rawSuffix`.
//
// The prefix must match the head of `relPath`; the suffix is then matched against every trailing sub-path so `**`
// spans zero or more intermediate segments. All inputs are slash-form on every platform.
//
// Parameters:
//   - `rawPrefix`: the slash-form pattern text before the `**`.
//   - `rawSuffix`: the slash-form pattern text after the `**`.
//   - `relPath`: the slash-form path to test.
//
// Returns:
//   - `bool`: true when `relPath` matches the prefix/suffix around `**`.
func matchDoubleStarSingle(rawPrefix, rawSuffix, relPath string) bool {

	prefix := strings.TrimRight(rawPrefix, "/")
	suffix := strings.TrimLeft(rawSuffix, "/")

	if prefix != "" {
		if !strings.HasPrefix(relPath, prefix+"/") && relPath != prefix {
			return false
		}
		relPath = strings.TrimPrefix(relPath, prefix+"/")
	}

	segments := strings.Split(relPath, "/")

	for i := range segments {
		tail := strings.Join(segments[i:], "/")
		if pathMatch(suffix, tail) {
			return true
		}
	}

	return false
}

// resolveOwnership resolves a user and a group into uid and gid integers suitable for os.Chown.
//
// Each side resolves either a name via os/user or a numeric form via strconv. An empty side produces -1 — the os.Chown
// sentinel for "leave this side unchanged."
//
// Parameters:
//   - `user`: the owner, by name or decimal uid; empty leaves the owner unchanged.
//   - `group`: the group, by name or decimal gid; empty leaves the group unchanged.
//
// Returns:
//   - `int`: resolved uid, or -1 when `user` is empty.
//   - `int`: resolved gid, or -1 when `group` is empty.
//   - `error`: non-nil if either side fails to resolve, or if both are empty.
func resolveOwnership(user, group string) (uid, gid int, err error) {

	uid = -1
	if user != "" {
		resolved, err := resolveUser(user)
		if err != nil {
			return 0, 0, err
		}
		uid = resolved
	}

	gid = -1
	if group != "" {
		resolved, err := resolveGroup(group)
		if err != nil {
			return 0, 0, err
		}
		gid = resolved
	}

	if uid == -1 && gid == -1 {
		return 0, 0, fmt.Errorf("invalid ownership: at least one of user or group must be present")
	}

	return uid, gid, nil
}

// pathMatch wraps [path.Match], treating a malformed-pattern error as no match.
//
// [path.Match], not [filepath.Match]: glob patterns are a slash-form language on every platform
// (the same contract as [io/fs] and the canonical [fsroot.Path] rel form), and the matcher's
// inputs are slash-normalized before they arrive here. [filepath.Match] would flip separator and
// escape semantics on Windows — the defect that made Find return nothing there.
//
// Parameters:
//   - `pattern`: the slash-form [path.Match] pattern.
//   - `name`: the slash-form name to test.
//
// Returns:
//   - `bool`: true when `name` matches `pattern` and the pattern is well-formed.
func pathMatch(pattern, name string) bool {
	ok, err := slashpath.Match(pattern, name)
	return err == nil && ok
}

// preArchiveDigest computes the digest of the bytes at `path` before archival.
//
// Returns the zero [op.Digest] (not an error) when the file cannot be hashed — symlinks, unreadable files, etc.
// Callers can record the digest when available without blocking the archive when not.
//
// Parameters:
//   - `root`: the [fsroot.Dir] used to read `path`.
//   - `path`: the absolute path whose bytes are hashed.
//
// Returns:
//   - `op.Digest`: the parsed digest, or the zero value when the bytes cannot be hashed or parsed.
func preArchiveDigest(root fsroot.Dir, path string) op.Digest {

	checksum := checksumFile(root, path)
	if checksum == "" {
		return op.Digest{}
	}

	digest, err := op.ParseDigest(checksum)
	if err != nil {
		return op.Digest{}
	}

	return digest
}

// resolveGroup converts the group side of a chown spec into a gid. Numeric input passes through.
//
// A name is looked up via os/user.LookupGroup.
//
// Parameters:
//   - s: the group side; non-empty.
//
// Returns:
//   - int:   the resolved gid.
//   - error: non-nil if the name doesn't resolve or the numeric form is out of range.
func resolveGroup(s string) (int, error) {

	if gid, err := strconv.Atoi(s); err == nil {
		return gid, nil
	}

	g, err := osuser.LookupGroup(s)
	if err != nil {
		return 0, fmt.Errorf("lookup group %q: %w", s, err)
	}

	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return 0, fmt.Errorf("group %q has non-numeric gid %q: %w", s, g.Gid, err)
	}

	return gid, nil
}

// resolveUser converts the user side of a chown spec into a uid. Numeric input passes through.
//
// A name is looked up via os/user.Lookup.
//
// Parameters:
//   - s: the user side; non-empty.
//
// Returns:
//   - int:   the resolved uid.
//   - error: non-nil if the name doesn't resolve or the numeric form is out of range.
func resolveUser(s string) (int, error) {

	if uid, err := strconv.Atoi(s); err == nil {
		return uid, nil
	}

	u, err := osuser.Lookup(s)
	if err != nil {
		return 0, fmt.Errorf("lookup user %q: %w", s, err)
	}

	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return 0, fmt.Errorf("user %q has non-numeric uid %q: %w", s, u.Uid, err)
	}

	return uid, nil
}

// splitFindPattern splits `pattern` into a base directory and the match expression beneath it.
//
// The pattern is normalized to slash form first — glob patterns are a slash-form language on every platform, and the
// downstream matcher is slash-native. When the pattern contains `**`, the base is everything before it; otherwise
// the base is the pattern's directory and the match is its base name.
//
// Parameters:
//   - `pattern`: the find pattern to split; any separator form.
//
// Returns:
//   - `root`: the base directory portion, slash-form.
//   - `match`: the match expression portion, slash-form.
func splitFindPattern(pattern string) (root, match string) {

	pattern = filepath.ToSlash(pattern)

	idx := strings.Index(pattern, "**")
	if idx < 0 {
		return slashpath.Dir(pattern), slashpath.Base(pattern)
	}

	root = strings.TrimRight(pattern[:idx], "/")
	match = pattern[idx:]

	return root, match
}

// statTupleEtag packs the stat tuple (size, mtime_ns, ino) little-endian and returns its sha256 as lowercase hex.
//
// The shared change-detection token form for every taxonomy variant and the catch-all base: an inexpensive signal
// the catalog uses to trigger the full [op.Resource.Digest] comparison. The inode comes from [statIdentity] and is
// zero on Windows, whose stat carries no inode.
//
// Parameters:
//   - `info`: the stat (or lstat) result to pack; the caller chooses follow semantics.
//
// Returns:
//   - `string`: lowercase hex sha256 of the packed stat tuple.
func statTupleEtag(info os.FileInfo) string {

	inode, _ := statIdentity(info)

	var buf [24]byte

	binary.LittleEndian.PutUint64(buf[0:8], uint64(info.Size())) //nolint:gosec // file sizes are non-negative
	binary.LittleEndian.PutUint64(buf[8:16], uint64(info.ModTime().UnixNano()))
	binary.LittleEndian.PutUint64(buf[16:24], inode)

	h := sha256.Sum256(buf[:])
	return hex.EncodeToString(h[:])
}
