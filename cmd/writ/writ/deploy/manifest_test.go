// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package deploy

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/internal/lorepackage"
	"github.com/NobleFactor/devlore-cli/pkg/platform"
)

// writeManifestFile writes a packages-manifest.yaml with the given body and returns its path.
//
// Parameters:
//   - `t`: the test.
//   - `body`: the manifest's YAML.
//
// Returns:
//   - `string`: the file's path.
func writeManifestFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "packages-manifest.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// testPlatform builds the platform the identifiers parse against.
//
// Parameters:
//   - `t`: the test.
//   - `spec`: the platform's spec, e.g. [platform.Darwin].
//
// Returns:
//   - `platform.Platform`: the platform.
func testPlatform(t *testing.T, spec *platform.Spec) platform.Platform {
	t.Helper()
	p, err := platform.New(spec)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// testRegistry builds a registry client over a temporary cache holding the named packages.
//
// Parameters:
//   - `t`: the test.
//   - `packages`: the registry packages to create, each with an empty lifecycle.yaml.
//
// Returns:
//   - `*lorepackage.Registry`: the client.
func testRegistry(t *testing.T, packages ...string) *lorepackage.Registry {
	t.Helper()
	dir := t.TempDir()
	for _, name := range packages {
		pkgDir := filepath.Join(dir, "packages", name)
		if err := os.MkdirAll(pkgDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pkgDir, "lifecycle.yaml"), []byte("name: "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return lorepackage.New("test", nil, dir)
}

// mergeNames merges the manifests on Darwin against an empty registry and returns the merged claims' identifiers.
//
// Parameters:
//   - `t`: the test.
//   - `paths`: the manifest files, in contribution order.
//
// Returns:
//   - `[]claim`: the merged claims.
//   - `[]Duplicate`: the merge's notes.
func mergeOnDarwin(t *testing.T, paths ...string) ([]claim, []Duplicate) {
	t.Helper()
	claims, duplicates, _, err := mergeManifests(testPlatform(t, platform.Darwin()), testRegistry(t), paths)
	if err != nil {
		t.Fatalf("mergeManifests: %v", err)
	}
	return claims, duplicates
}

// names projects the merged claims to their package names.
//
// Parameters:
//   - `claims`: the merged claims.
//
// Returns:
//   - `[]string`: the names, in order.
func names(claims []claim) []string {
	var out []string
	for i := range claims {
		out = append(out, claims[i].purl.Name)
	}
	return out
}

// TestMergeManifests_APackageClaimedTwiceIsPlannedOnce is #814's merge: manifests combine, so a package a general
// manifest and a platform manifest both claim contributes one product, keeping the position of its first claim.
func TestMergeManifests_APackageClaimedTwiceIsPlannedOnce(t *testing.T) {

	general := writeManifestFile(t, "packages:\n  - name: git\n  - name: jq\n")
	specific := writeManifestFile(t, "packages:\n  - name: jq\n  - name: ripgrep\n")

	claims, _ := mergeOnDarwin(t, general, specific)

	if want := []string{"git", "jq", "ripgrep"}; !reflect.DeepEqual(names(claims), want) {
		t.Errorf("merged = %v; want %v -- a union, each package once, in first-claim order", names(claims), want)
	}
}

// TestMergeManifests_FeaturesUnion pins the feature rule: a platform manifest adding a feature to a package the
// general manifest already claims means both features, not the later claim overwriting the earlier.
func TestMergeManifests_FeaturesUnion(t *testing.T) {

	general := writeManifestFile(t, "packages:\n  - name: vim\n    with: [python]\n")
	specific := writeManifestFile(t, "packages:\n  - name: vim\n    with: [lua, python]\n")

	claims, _ := mergeOnDarwin(t, general, specific)

	if len(claims) != 1 {
		t.Fatalf("merged %d claims, want 1", len(claims))
	}
	if want := []string{"python", "lua"}; !reflect.DeepEqual(claims[0].with, want) {
		t.Errorf("features = %v; want %v -- the union, in first-seen order, no duplicates", claims[0].with, want)
	}
}

// TestMergeManifests_APinSatisfiesAnUnpinnedClaim pins the version rule, which is satisfaction rather than
// precedence: a bare name asks for any version, so a pinned claim satisfies both and is kept -- whichever order the
// two claims arrive in. No claim overrides another, which is the ruling on #814.
func TestMergeManifests_APinSatisfiesAnUnpinnedClaim(t *testing.T) {

	for _, testCase := range []struct{ name, first, second string }{
		{"the pin arrives second", "packages:\n  - name: jq\n", "packages:\n  - name: jq@1.7\n"},
		{"the pin arrives first", "packages:\n  - name: jq@1.7\n", "packages:\n  - name: jq\n"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			claims, _ := mergeOnDarwin(t, writeManifestFile(t, testCase.first), writeManifestFile(t, testCase.second))
			if len(claims) != 1 {
				t.Fatalf("merged %d claims, want 1 -- jq and jq@1.7 are one package", len(claims))
			}
			if got := claims[0].identifier(); got != "pkg:brew/jq@1.7" {
				t.Errorf("identifier = %q; want pkg:brew/jq@1.7 -- the pin satisfies the unpinned claim too", got)
			}
		})
	}
}

// TestMergeManifests_TwoDifferentPinsAreRefused pins the INTERIM answer to the case satisfaction cannot resolve.
//
// The ruled algorithm (devlore-cli#872) asks the relevant package manager whether two versions of the package can
// coexist here: yes, both install; no, a version conflict, refused naming both manifests. Until the broker's
// question surface exists the merge cannot ask, and every manager writ targets today -- apt, winget, brew -- answers
// no, so the merge refuses. When the broker lands, this hard-coded "no" becomes a per-manager answer and this test
// becomes that manager's case; it is not a rule of the merge. Last-wins would have silently dropped a claim.
//
// The emergent design is recorded in three places, each for its audience: the user guide
// (docs/guides/writ/packages-manifest.md, "Layer merging"), the design record (#868, the pkg provider's design
// document, with docs/architecture/4.1-resource-identity.md for identity), and the plans
// (docs/plans/fix/814-manifests-combine.md, and #872 where the rulings were made).
func TestMergeManifests_TwoDifferentPinsAreRefused(t *testing.T) {

	general := writeManifestFile(t, "packages:\n  - name: jq@1.6\n")
	specific := writeManifestFile(t, "packages:\n  - name: jq@1.7\n")

	_, _, _, err := mergeManifests(testPlatform(t, platform.Darwin()), testRegistry(t), []string{general, specific})
	if err == nil {
		t.Fatal("mergeManifests accepted two different version pins; want a refusal")
	}
	for _, want := range []string{"jq", "1.6", "1.7", general, specific} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal %q does not name %s", err, want)
		}
	}
}

// TestMergeManifests_OrderIsDeterministic pins why order is kept at all: a graph's identity is a checksum over its
// content, so merging the same manifests twice must yield the same sequence.
func TestMergeManifests_OrderIsDeterministic(t *testing.T) {

	first := writeManifestFile(t, "packages:\n  - name: a\n  - name: b\n")
	second := writeManifestFile(t, "packages:\n  - name: c\n  - name: b\n")

	var seen []string
	for range 8 {
		claims, _ := mergeOnDarwin(t, first, second)
		got := names(claims)
		if seen == nil {
			seen = got
			continue
		}
		if !reflect.DeepEqual(got, seen) {
			t.Fatalf("merge order varies between runs: %v then %v", seen, got)
		}
	}
	if want := []string{"a", "b", "c"}; !reflect.DeepEqual(seen, want) {
		t.Errorf("merged = %v; want %v", seen, want)
	}
}

// TestMergeManifests_ThreeSpellingsAreOneProduct pins identity as what the catalog interns, by the pkg provider's own
// grammar: `jq`, `brew:jq` and `pkg:brew/jq` are one product on a brew platform, and `port:jq` is a second, on another
// manager (#872, delta 2).
func TestMergeManifests_ThreeSpellingsAreOneProduct(t *testing.T) {

	claims, duplicates := mergeOnDarwin(t,
		writeManifestFile(t, "packages:\n  - name: jq\n"),
		writeManifestFile(t, "packages:\n  - name: brew:jq\n"),
		writeManifestFile(t, "packages:\n  - name: pkg:brew/jq\n"),
		writeManifestFile(t, "packages:\n  - name: port:jq\n"),
	)

	var got []string
	for _, c := range claims {
		got = append(got, c.identifier())
	}
	if want := []string{"pkg:brew/jq", "pkg:port/jq"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("merged = %v; want %v -- three spellings of one product; a second manager is a second product",
			got, want)
	}
	if len(claims[0].manifests) != 3 {
		t.Errorf("brew jq claimed by %d manifests; want 3", len(claims[0].manifests))
	}
	if len(duplicates) != 1 || duplicates[0].Package != "pkg:brew/jq" {
		t.Errorf("duplicates = %+v; want one note, on pkg:brew/jq", duplicates)
	}
}

// TestMergeManifests_ABareClaimIsMetByANamespacedOne pins the coordinate rule: a bare `jq` and
// `pkg:winget/jqlang/jq` are one product on winget, and the merged claim carries the namespace, because the bare
// claim asked for a jq and the namespaced one names which (#814, Matrix A's Windows row).
func TestMergeManifests_ABareClaimIsMetByANamespacedOne(t *testing.T) {

	general := writeManifestFile(t, "packages:\n  - name: jq\n")
	windows := writeManifestFile(t, "packages:\n  - name: pkg:winget/jqlang/jq\n")

	claims, duplicates, _, err := mergeManifests(
		testPlatform(t, platform.Windows()), testRegistry(t), []string{general, windows})
	if err != nil {
		t.Fatalf("mergeManifests: %v", err)
	}

	if len(claims) != 1 {
		t.Fatalf("merged %d claims, want 1", len(claims))
	}
	if got := claims[0].identifier(); got != "pkg:winget/jqlang/jq" {
		t.Errorf("identifier = %q; want pkg:winget/jqlang/jq -- the namespaced claim names which jq", got)
	}
	decisions := strings.Join(duplicates[0].Decisions, ";")
	if len(duplicates) != 1 || !strings.Contains(decisions, "met by pkg:winget/jqlang/jq") {
		t.Errorf("duplicates = %+v; want one note saying the bare claim was met by the namespaced one", duplicates)
	}
}

// TestMergeManifests_ADuplicateIsANoteNamingEveryManifest is Requirement 3: a product claimed twice is narrated as a
// note naming every manifest that made a claim, in contribution order.
func TestMergeManifests_ADuplicateIsANoteNamingEveryManifest(t *testing.T) {

	general := writeManifestFile(t, "packages:\n  - name: jq\n")
	team := writeManifestFile(t, "packages:\n  - name: jq\n")
	personal := writeManifestFile(t, "packages:\n  - name: jq@1.7\n")

	_, duplicates := mergeOnDarwin(t, general, team, personal)

	if len(duplicates) != 1 {
		t.Fatalf("duplicates = %+v; want exactly one, for jq", duplicates)
	}
	if want := []string{general, team, personal}; !reflect.DeepEqual(duplicates[0].Manifests, want) {
		t.Errorf("manifests = %v; want %v -- every claim, in contribution order", duplicates[0].Manifests, want)
	}
	if len(duplicates[0].Decisions) != 2 {
		t.Errorf("decisions = %v; want one per later claim", duplicates[0].Decisions)
	}
}

// TestMergeManifests_ARegistryPackageIsDeferred pins the dormant branch: a claim that resolves to a registry package
// is noted and not merged, until the devlore provider (#877) plans it; the rest of the manifest merges as usual.
func TestMergeManifests_ARegistryPackageIsDeferred(t *testing.T) {

	general := writeManifestFile(t, "packages:\n  - name: docker\n  - name: jq\n")
	team := writeManifestFile(t, "packages:\n  - name: docker\n")

	claims, _, deferred, err := mergeManifests(
		testPlatform(t, platform.Darwin()), testRegistry(t, "docker"), []string{general, team})
	if err != nil {
		t.Fatalf("mergeManifests: %v", err)
	}

	if want := []string{"jq"}; !reflect.DeepEqual(names(claims), want) {
		t.Errorf("merged = %v; want %v -- docker is a registry package and is not merged", names(claims), want)
	}
	if len(deferred) != 1 || deferred[0].Package != "docker" || len(deferred[0].Manifests) != 2 {
		t.Errorf("deferred = %+v; want docker, claimed by both manifests", deferred)
	}
}
