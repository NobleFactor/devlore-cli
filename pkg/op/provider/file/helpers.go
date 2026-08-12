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
	"os/user"
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

// applyChown changes the owner and/or group of path according to the Dockerfile-style ownership string spec.
//
// An empty spec is a no-op — the function returns nil without invoking any system call, which is the contract that lets
// the four file-provider write methods always call applyChown unconditionally and rely on the empty-string
// short-circuit.
//
// Accepted spec shapes:
//   - ""             — no change (short-circuit; no syscall)
//   - "user"         — change owner only; group unchanged
//   - "user:group"   — change owner and group
//   - ":group"       — change group only; owner unchanged
//   - "uid"          — numeric form of "user"
//   - "uid:gid"      — numeric form of "user:group"
//   - ":gid"         — numeric form of ":group"
//
// User and group sides accept either a name (resolved via os/user) or a decimal integer (passed to os.Chown directly).
//
// Mixed forms are allowed: `"alice:1000"` resolves alice's uid and uses gid 1000.
//
// Parameters:
//   - path: the filesystem path to chown.
//   - spec: the Dockerfile-style ownership string.
//
// Returns:
//   - error: non-nil if spec is malformed, a name doesn't resolve, or os.Chown fails.
func applyChown(path, spec string) error {

	if spec == "" {
		return nil
	}

	uid, gid, err := parseChown(spec)
	if err != nil {
		return fmt.Errorf("chown %q: %w", path, err)
	}

	if err := os.Chown(path, uid, gid); err != nil {
		return fmt.Errorf("chown %q: %w", path, err)
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
	sourcePath := runtimeEnvironment.Root.NewPath(path)
	var base op.ResourceBase

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
//   - `root`: the [fsroot.Root] used to read `path`.
//   - `path`: the path to hash.
//
// Returns:
//   - `string`: the checksum in "sha256:<hex>" form, or "" when the file cannot be read.
func checksumFile(root fsroot.Root, path string) string {

	data, err := root.ReadFile(root.NewPath(path))
	if err != nil {
		return ""
	}

	return checksumBytes(data)
}

// contentDigest computes the streamed sha256 of the regular file at `abs` (no full-file allocation).
//
// Parameters:
//   - `root`: the [fsroot.Root] used to open `abs`.
//   - `abs`: the absolute path of the regular file to hash.
//
// Returns:
//   - `op.Digest`: sha256 algorithm with 32 raw bytes.
//   - `error`: any open or read error.
func contentDigest(root fsroot.Root, abs string) (digest op.Digest, err error) {

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

// matchDoubleStar reports whether `path` matches `pattern`, supporting `**` recursive wildcards.
//
// A pattern with no `**` is delegated to [filepath.Match] semantics; a single `**` is handled segment-by-segment; and
// multiple `**` fall back to matching the trailing component against the path's base name.
//
// Parameters:
//   - `pattern`: the glob pattern, which may contain `**`.
//   - `path`: the path to test.
//
// Returns:
//   - `bool`: true when `path` matches `pattern`.
func matchDoubleStar(pattern, path string) bool {

	parts := strings.Split(pattern, "**")
	if len(parts) == 1 {
		return pathMatch(pattern, path)
	}

	if len(parts) == 2 {
		return matchDoubleStarSingle(parts[0], parts[1], path)
	}

	tail := strings.TrimLeft(parts[len(parts)-1], string(filepath.Separator))
	return pathMatch(tail, filepath.Base(path))
}

// matchDoubleStarSingle reports whether `path` matches a single-`**` pattern split into `rawPrefix` and `rawSuffix`.
//
// The prefix must match the head of `path`; the suffix is then matched against every trailing sub-path so `**` spans
// zero or more intermediate segments.
//
// Parameters:
//   - `rawPrefix`: the pattern text before the `**`.
//   - `rawSuffix`: the pattern text after the `**`.
//   - `path`: the path to test.
//
// Returns:
//   - `bool`: true when `path` matches the prefix/suffix around `**`.
func matchDoubleStarSingle(rawPrefix, rawSuffix, path string) bool {

	prefix := strings.TrimRight(rawPrefix, string(filepath.Separator))
	suffix := strings.TrimLeft(rawSuffix, string(filepath.Separator))

	if prefix != "" {
		if !strings.HasPrefix(path, prefix+string(filepath.Separator)) && path != prefix {
			return false
		}
		path = strings.TrimPrefix(path, prefix+string(filepath.Separator))
	}

	segments := strings.Split(path, string(filepath.Separator))

	for i := range segments {
		tail := strings.Join(segments[i:], string(filepath.Separator))
		if pathMatch(suffix, tail) {
			return true
		}
	}

	return false
}

// parseChown splits a Dockerfile-style ownership string into uid and gid integers suitable for os.Chown.
//
// Each side resolves either a name via os/user or a numeric form via strconv. Empty sides produce -1 — the os.Chown
// sentinel for "leave this side unchanged."
//
// Parameters:
//   - spec: the ownership string; must be non-empty (callers short-circuit on empty before calling).
//
// Returns:
//   - int:   resolved uid, or -1 if the user side is empty.
//   - int:   resolved gid, or -1 if the group side is empty.
//   - error: non-nil if either side fails to resolve.
func parseChown(spec string) (uid, gid int, err error) {

	userSide, groupSide, hasColon := strings.Cut(spec, ":")

	uid = -1
	if userSide != "" {
		resolved, err := resolveUser(userSide)
		if err != nil {
			return 0, 0, err
		}
		uid = resolved
	}

	gid = -1
	if hasColon && groupSide != "" {
		resolved, err := resolveGroup(groupSide)
		if err != nil {
			return 0, 0, err
		}
		gid = resolved
	}

	if uid == -1 && gid == -1 {
		return 0, 0, fmt.Errorf("invalid ownership %q: at least one of user or group must be present", spec)
	}

	return uid, gid, nil
}

// pathMatch wraps [filepath.Match], treating a malformed-pattern error as no match.
//
// Parameters:
//   - `pattern`: the [filepath.Match] pattern.
//   - `name`: the name to test.
//
// Returns:
//   - `bool`: true when `name` matches `pattern` and the pattern is well-formed.
func pathMatch(pattern, name string) bool {
	ok, err := filepath.Match(pattern, name)
	return err == nil && ok
}

// preArchiveDigest computes the digest of the bytes at `path` before archival.
//
// Returns the zero [op.Digest] (not an error) when the file cannot be hashed — symlinks, unreadable files, etc.
// Callers can record the digest when available without blocking the archive when not.
//
// Parameters:
//   - `root`: the [fsroot.Root] used to read `path`.
//   - `path`: the absolute path whose bytes are hashed.
//
// Returns:
//   - `op.Digest`: the parsed digest, or the zero value when the bytes cannot be hashed or parsed.
func preArchiveDigest(root fsroot.Root, path string) op.Digest {

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

	g, err := user.LookupGroup(s)
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

	u, err := user.Lookup(s)
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
// When the pattern contains `**`, the base is everything before it; otherwise the base is the pattern's directory and
// the match is its base name.
//
// Parameters:
//   - `pattern`: the find pattern to split.
//
// Returns:
//   - `string`: the base directory portion.
//   - `string`: the match expression portion.
func splitFindPattern(pattern string) (root, match string) {

	idx := strings.Index(pattern, "**")
	if idx < 0 {
		return filepath.Dir(pattern), filepath.Base(pattern)
	}

	root = strings.TrimRight(pattern[:idx], string(filepath.Separator))
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
