// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package file

import (
	"fmt"
	"path/filepath"
	"reflect"

	// Aliased: plan-space paths are a slash-form language on every platform; their canonical form comes
	// from the slash-path Clean, never the OS one.
	slashpath "path"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// init registers the file scheme's plan-space grammar for every taxonomy variant, beside the generated
// announcements — an authored string bound to a file-resource parameter at plan time passes through
// [NormalizePlanSpacePath] before construction.
func init() {
	for _, t := range []reflect.Type{
		reflect.TypeFor[AnyKind](),
		reflect.TypeFor[Regular](),
		reflect.TypeFor[Directory](),
		reflect.TypeFor[SymbolicLink](),
		reflect.TypeFor[*resource](),
	} {
		op.RegisterPlanPathNormalizer(t, NormalizePlanSpacePath)
	}
}

// NormalizePlanSpacePath renders an authored plan path into its canonical rel, or refuses it.
//
// The git model (#584, ruled 2026-08-20; docs/architecture/4-resource-management.md §5.2): plan-space paths
// are a portable little language. `foo/bar` and `/foo/bar` both name rel `foo/bar` — the leading slash is
// the anchored spelling; machine-absoluteness is inexpressible in a plan (it arises only from the run's root
// choice). The refusals:
//
//   - a volume or drive-letter spelling (`C:\x`, `C:/x`) — malformed plan input;
//   - a UNC spelling (`//server/share`, `\\server\share`) — malformed plan input;
//   - any backslash — plan space is slash-form on every platform; a backslash is a native-spelling leak;
//   - a root-qualified spelling (`@name/…`) — reserved for the named multi-root design (#597);
//   - a rel that escapes (`../…`) or names the root itself — intent confinement can never satisfy.
//
// Parameters:
//   - `path`: the authored plan path.
//
// Returns:
//   - `string`: the slash-canonical rel.
//   - `error`: non-nil for every refusal above.
func NormalizePlanSpacePath(path string) (string, error) {

	if len(path) >= 2 && path[1] == ':' && isASCIILetter(path[0]) {
		return "", fmt.Errorf(
			"file: plan path %q carries a volume spelling — machine-absoluteness is inexpressible in a plan; "+
				"name the path relative to the run's root", path)
	}

	if strings.HasPrefix(path, "//") || strings.HasPrefix(path, `\\`) {
		return "", fmt.Errorf(
			"file: plan path %q carries a UNC spelling — machine-absoluteness is inexpressible in a plan; "+
				"name the path relative to the run's root", path)
	}

	if strings.Contains(path, `\`) {
		return "", fmt.Errorf(
			"file: plan path %q carries a backslash — plan-space paths are slash-form on every platform", path)
	}

	if strings.HasPrefix(path, "@") {
		return "", fmt.Errorf(
			"file: plan path %q is root-qualified — the @name spelling is reserved for the named multi-root "+
				"design; only run-root-relative paths are expressible today", path)
	}

	// The anchored spelling: a leading slash names the same rel.
	cleaned := slashpath.Clean(strings.TrimPrefix(path, "/"))

	if cleaned == "." || cleaned == "" {
		return "", fmt.Errorf("file: plan path %q names the run's root itself, not a resource under it", path)
	}

	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf(
			"file: plan path %q escapes the run's root — intent confinement can never satisfy it", path)
	}

	return cleaned, nil
}

// NormalizeRuntimePath renders a run-computed path into the slash-canonical rel the run catalog speaks —
// the runtime dialect of the plan-space grammar (4-resource-management.md §5.7 rule 3, ruled 2026-08-22).
//
// Rels normalize exactly as authored plan-space paths do, with the same refusals: escapes, the reserved
// @name spelling, the bare root, backslashes. The dialect's one addition: a machine-absolute input is
// interpretable here because the run's root is bound — machine-absoluteness arises only from the run's
// root choice (§5.2), and the discovery actions sit on the far side of that choice — so an absolute path
// under `root` rebases to its rel, and one outside `root` (another volume and UNC spellings included)
// refuses as a confinement violation.
//
// Dialect sharpening, recorded at PR 3: on unix a leading slash is ambiguous between the plan-space
// anchored spelling and machine-absoluteness — at run time the machine reading wins (tools emit machine
// absolutes), so the two readings agree under the root and an out-of-root absolute refuses rather than
// silently confining. Authors of literals write bare rels.
//
// Parameters:
//   - `root`: the run's bound root.
//   - `path`: the run-computed path in any spelling.
//
// Returns:
//   - `string`: the slash-canonical rel.
//   - `error`: the dialect's refusal.
func NormalizeRuntimePath(root fsroot.Dir, path string) (string, error) {

	if isMachineAbsolute(path) {
		rel, within := fsroot.RelWithin(root.Name(), path)
		if !within {
			return "", fmt.Errorf(
				"file: runtime path %q lies outside the run's root %q — confinement admits no resource beyond the root",
				path, root.Name())
		}
		return rel, nil
	}

	return NormalizePlanSpacePath(path)
}

// isASCIILetter reports whether b is an ASCII letter — the drive-letter half of a volume spelling.
//
// Parameters:
//   - `b`: the byte to test.
//
// Returns:
//   - `bool`: true for A-Z and a-z.
func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// isMachineAbsolute reports whether `path` is machine-absolute in any spelling a host tool might emit —
// the platform's own IsAbs judgment, a volume spelling, or a UNC spelling (the latter two matter
// off-Windows, where [filepath.IsAbs] does not recognize them).
//
// Parameters:
//   - `path`: the path to judge.
//
// Returns:
//   - `bool`: true for any machine-absolute spelling.
func isMachineAbsolute(path string) bool {

	if filepath.IsAbs(path) {
		return true
	}
	if len(path) >= 2 && path[1] == ':' && isASCIILetter(path[0]) {
		return true
	}
	return strings.HasPrefix(path, `\\`)
}
