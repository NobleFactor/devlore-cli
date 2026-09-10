// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package deploy

import (
	"fmt"
	"maps"
	"path/filepath"
	"strings"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/internal/lorepackage"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/tree"
	"github.com/NobleFactor/devlore-cli/internal/manifest"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/flow"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/pkg"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/plan"
	"github.com/NobleFactor/devlore-cli/pkg/platform"
)

// region SUPPORTING TYPES

// Duplicate records a product more than one manifest claimed, and what the merge did with the later claims. A
// duplicate is ordinary -- a general and a platform manifest naming one package is the design working -- and
// noteworthy: whether the later claim is redundant depends on which layers this machine composes, so the record
// reports the fact and does not recommend (#814, Requirement 3).
type Duplicate struct {
	Package   string   // the product, as the merged claim spells it
	Manifests []string // every manifest that claimed it, in contribution order
	Decisions []string // what the merge did with each later claim, in order
}

// Deferred records a claim that resolves to a registry package. writ does not plan those until the devlore provider
// (#877) exists; the claim is noted and the rest of the scope deploys.
type Deferred struct {
	Package   string   // the registry package's name
	Manifests []string // every manifest that claimed it, in contribution order
}

// claim is one merged product: the coordinates the catalog will intern, the content the claims contributed, and the
// manifests that made them.
type claim struct {
	purl      platform.PURL // the identity, versionless; namespace and qualifiers are coordinates (see mergeClaim)
	version   string        // the requested version; "" for any
	pinnedBy  string        // the manifest whose pin `version` is; "" when unpinned
	with      []string      // the union of the claims' features
	manifests []string      // the manifests that claimed the product, in contribution order
	decisions []string      // what the merge did with each later claim
}

// endregion

// region HELPER FUNCTIONS

// planManifests plans a scope's packages from the union of its manifests: one `pkg.install` per native product, the
// invocations gathered into one subgraph named for the scope's packages (#814).
//
// A claim that resolves to a registry package is deferred, not planned: its lifecycle is a pipeline the devlore
// provider (#877) will construct, and until then writ notes it and deploys the rest of the scope.
//
// Parameters:
//   - `cfg`: the deploy configuration; a nil `Registry` skips manifest planning with a note.
//   - `provider`: the scope's plan provider; the invocations register into it and are gathered from it.
//   - `environment`: the planning runtime environment; supplies the platform the identifiers parse against.
//   - `scope`: the scope's name, for the subgraph's identity; "" in single-source mode.
//   - `manifests`: the scope's manifests, in contribution order.
//
// Returns:
//   - `op.ExecutableUnit`: the packages subgraph, or nil when nothing was planned.
//   - `[]Duplicate`: the products more than one manifest claimed.
//   - `[]Deferred`: the claims that resolve to registry packages.
//   - `error`: a manifest that cannot be loaded, an identifier that cannot be parsed, two pins that disagree, or a
//     unit that cannot be planned.
func planManifests(
	cfg *Config, provider *plan.Provider, environment *op.RuntimeEnvironment, scope string,
	manifests []*tree.FileEntry,
) (op.ExecutableUnit, []Duplicate, []Deferred, error) {

	if len(manifests) == 0 {
		return nil, nil, nil, nil
	}

	if cfg.Registry == nil {
		cli.Note("Skipping %d packages-manifest file(s): no registry configured", len(manifests))
		return nil, nil, nil, nil
	}

	sources := make([]string, 0, len(manifests))
	for _, m := range manifests {
		sources = append(sources, m.Source)
	}

	claims, duplicates, deferred, err := mergeManifests(environment.Platform, cfg.Registry, sources)
	if err != nil {
		return nil, nil, nil, err
	}

	for i := range claims {
		identifier := claims[i].identifier()
		if _, err := provider.Plan(pkg.Install, nil, map[string]any{"packages": []any{identifier}}); err != nil {
			return nil, nil, nil, fmt.Errorf("planning %s: %w", identifier, err)
		}
	}

	children := parentlessUnits(provider)
	if len(children) == 0 {
		return nil, duplicates, deferred, nil
	}

	subgraph, err := packagesSubgraph(scope, children)
	if err != nil {
		return nil, nil, nil, err
	}

	return subgraph, duplicates, deferred, nil
}

// mergeManifests merges the claims of every manifest, in contribution order, into one claim per product.
//
// Identity is what the catalog interns, by the pkg provider's own grammar ([platform.ParseIdentifier]): the manager
// type plus the name. Namespace, qualifiers and version are claim content, resolved by satisfaction:
//
//   - a claim without coordinates (no namespace, no qualifiers) is met by the product's first claim, and a claim with
//     coordinates upgrades a coordinate-less one; two claims with different coordinates on one manager are two
//     products, both planned, with a note;
//   - a bare name asks for any version, so a pin satisfies both claims whichever order they arrive in; two different
//     pins are refused naming both manifests, the interim answer until the broker can ask the manager (#872);
//   - features union.
//
// A claim that resolves to a registry package is deferred rather than merged (see [Deferred]).
//
// Parameters:
//   - `plat`: the target platform the identifiers parse against.
//   - `registry`: the registry client, asked one question: is this name a registry package.
//   - `paths`: the manifest files, in contribution order.
//
// Returns:
//   - `[]claim`: the merged claims, in first-claim order.
//   - `[]Duplicate`: the products more than one manifest claimed, or that another claim touched.
//   - `[]Deferred`: the registry packages, noted and not merged.
//   - `error`: a manifest that cannot be loaded, an identifier that cannot be parsed, or two pins that disagree.
func mergeManifests(
	plat platform.Platform, registry *lorepackage.Registry, paths []string,
) ([]claim, []Duplicate, []Deferred, error) {

	var claims []claim
	var deferred []Deferred

	for _, path := range paths {
		loaded, err := manifest.Load(path)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("parsing manifest %s: %w", path, err)
		}

		for _, entry := range loaded.Packages {
			if isRegistryPackage(registry, entry.Name) {
				deferred = deferClaim(deferred, packageName(entry.Name), path)
				continue
			}

			parsed, err := platform.ParseIdentifier(plat, entry.Name)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("manifest %s: package %q: %w", path, entry.Name, err)
			}

			claims, err = mergeClaim(claims, parsed, entry.With, path)
			if err != nil {
				return nil, nil, nil, err
			}
		}
	}

	return claims, duplicatesOf(claims), deferred, nil
}

// deferClaim records a claim on a registry package: a new [Deferred], or one more manifest on the existing record.
//
// Parameters:
//   - `deferred`: the records so far, in first-claim order.
//   - `name`: the registry package's name.
//   - `path`: the manifest that made the claim.
//
// Returns:
//   - `[]Deferred`: the records, with this claim folded in.
func deferClaim(deferred []Deferred, name, path string) []Deferred {

	for i := range deferred {
		if deferred[i].Package == name {
			deferred[i].Manifests = append(deferred[i].Manifests, path)
			return deferred
		}
	}

	return append(deferred, Deferred{Package: name, Manifests: []string{path}})
}

// duplicatesOf reports every product more than one manifest claimed, or that another claim touched.
//
// Parameters:
//   - `claims`: the merged claims.
//
// Returns:
//   - `[]Duplicate`: one record per such product, in claim order.
func duplicatesOf(claims []claim) []Duplicate {

	var duplicates []Duplicate
	for i := range claims {
		c := &claims[i]
		if len(c.manifests) > 1 || len(c.decisions) > 0 {
			duplicates = append(duplicates, Duplicate{Package: c.identifier(), Manifests: c.manifests, Decisions: c.decisions})
		}
	}

	return duplicates
}

// mergeClaim folds one parsed claim into `claims`: the product it belongs to gains the claim's content, or a new
// product is appended.
//
// Parameters:
//   - `claims`: the merged claims so far, in first-claim order.
//   - `parsed`: the claim's coordinates, version included.
//   - `with`: the claim's features.
//   - `path`: the manifest that made the claim.
//
// Returns:
//   - `[]claim`: the claims, with this one folded in.
//   - `error`: two pins that disagree, naming both manifests.
func mergeClaim(claims []claim, parsed platform.PURL, with []string, path string) ([]claim, error) {

	version := parsed.Version
	parsed.Version = ""

	at := matchingClaim(claims, parsed)
	if at < 0 {
		if sibling := sameProduct(claims, parsed); sibling >= 0 {
			claims[sibling].decisions = append(claims[sibling].decisions,
				fmt.Sprintf("%s (%s) is a second product on %s", parsed.String(), path, parsed.Type))
		}
		pinnedBy := ""
		if version != "" {
			pinnedBy = path
		}
		return append(claims, claim{
			purl: parsed, version: version, pinnedBy: pinnedBy, with: unionFeatures(nil, with), manifests: []string{path},
		}), nil
	}

	existing := &claims[at]
	existing.manifests = append(existing.manifests, path)
	existing.with = unionFeatures(existing.with, with)

	if hasCoordinates(parsed) && !hasCoordinates(existing.purl) {
		existing.decisions = append(existing.decisions, fmt.Sprintf("met by %s (%s)", parsed.String(), path))
		existing.purl = parsed
	}

	switch {
	case version == "" || version == existing.version:
		existing.decisions = append(existing.decisions, fmt.Sprintf("claimed again by %s", path))
	case existing.version == "":
		existing.decisions = append(existing.decisions, fmt.Sprintf("pin %s (%s) satisfies", version, path))
		existing.version, existing.pinnedBy = version, path
	default:
		return nil, fmt.Errorf(
			"package %q is claimed at version %q by %s and at version %q by %s: one package interns one catalog "+
				"entry, so no version satisfies both claims",
			existing.purl.Name, existing.version, existing.pinnedBy, version, path)
	}

	return claims, nil
}

// matchingClaim finds the claim `parsed` is a claim on: same manager and name, and coordinates that satisfy one
// another -- equal, or absent on either side.
//
// Parameters:
//   - `claims`: the merged claims so far.
//   - `parsed`: the versionless coordinates of the new claim.
//
// Returns:
//   - `int`: the index of the matching claim, or -1.
func matchingClaim(claims []claim, parsed platform.PURL) int {

	for i := range claims {
		c := &claims[i]
		if c.purl.Type != parsed.Type || c.purl.Name != parsed.Name {
			continue
		}
		if sameCoordinates(c.purl, parsed) || !hasCoordinates(parsed) || !hasCoordinates(c.purl) {
			return i
		}
	}

	return -1
}

// sameProduct finds a claim with the same manager and name as `parsed`, whatever its coordinates.
//
// Parameters:
//   - `claims`: the merged claims so far.
//   - `parsed`: the versionless coordinates of the new claim.
//
// Returns:
//   - `int`: the index of the first such claim, or -1.
func sameProduct(claims []claim, parsed platform.PURL) int {

	for i := range claims {
		if claims[i].purl.Type == parsed.Type && claims[i].purl.Name == parsed.Name {
			return i
		}
	}

	return -1
}

// hasCoordinates reports whether a purl carries a namespace or qualifiers.
//
// Parameters:
//   - `p`: the purl.
//
// Returns:
//   - `bool`: true when a namespace or any qualifier is present.
func hasCoordinates(p platform.PURL) bool {
	return p.Namespace != "" || len(p.Qualifiers) > 0
}

// sameCoordinates reports whether two purls carry the same namespace and qualifiers.
//
// Parameters:
//   - `a`: one purl.
//   - `b`: the other.
//
// Returns:
//   - `bool`: true when the namespaces and the qualifier maps are equal.
func sameCoordinates(a, b platform.PURL) bool {
	return a.Namespace == b.Namespace && maps.Equal(a.Qualifiers, b.Qualifiers)
}

// isRegistryPackage reports whether a manifest entry names a registry package: a bare name (no scheme, no manager
// prefix) with a lifecycle in the registry's package directory. A purl or a prefixed name is native by construction.
//
// Parameters:
//   - `registry`: the registry client.
//   - `raw`: the manifest entry's name.
//
// Returns:
//   - `bool`: true when the registry holds a package of that name.
func isRegistryPackage(registry *lorepackage.Registry, raw string) bool {

	if strings.ContainsAny(raw, ":/") {
		return false
	}

	return registry.FileExists(filepath.Join("packages", packageName(raw), "lifecycle.yaml"))
}

// packageName strips the `@version` tail from a bare manifest entry.
//
// Parameters:
//   - `raw`: the manifest entry's name.
//
// Returns:
//   - `string`: the name without its requested version.
func packageName(raw string) string {

	name, _, _ := strings.Cut(raw, "@")
	return name
}

// unionFeatures returns the features of both claims, in first-seen order and without duplicates.
//
// Parameters:
//   - `first`: the features of the earlier claim.
//   - `second`: the features of the later one.
//
// Returns:
//   - `[]string`: the union, in first-seen order; nil when empty.
func unionFeatures(first, second []string) []string {

	seen := make(map[string]bool, len(first)+len(second))
	union := make([]string, 0, len(first)+len(second))
	for _, feature := range append(append([]string{}, first...), second...) {
		if seen[feature] {
			continue
		}
		seen[feature] = true
		union = append(union, feature)
	}
	if len(union) == 0 {
		return nil
	}
	return union
}

// packagesSubgraph seals a scope's package invocations into one subgraph named for the scope's packages, a unit of
// the scope graph beside the file chains. Its children are peers in the graph's topological sort; the devlore
// sibling and the edge that orders native packages first are #877's.
//
// Parameters:
//   - `scope`: the scope's name; "" in single-source mode.
//   - `children`: the scope's `pkg.install` invocations, in contribution order.
//
// Returns:
//   - `*op.Subgraph`: the sealed subgraph.
//   - `error`: non-nil when the subgraph cannot be built.
func packagesSubgraph(scope string, children []op.ExecutableUnit) (*op.Subgraph, error) {

	subgraphAction, err := op.ReceiverRegistry().BuildAction(flow.Subgraph)
	if err != nil {
		return nil, fmt.Errorf("packages subgraph: %w", err)
	}

	id := "subgraph.packages"
	if scope != "" {
		id = "subgraph." + scope + ".packages"
	}

	spec := op.NewSubgraphSpec().
		WithID(id).
		WithName("packages").
		WithAction(subgraphAction).
		WithAnnotations(map[string]any{"scope": scope}).
		WithChildren(children...)

	subgraph, err := op.NewSubgraph(spec)
	if err != nil {
		return nil, fmt.Errorf("packages subgraph: %w", err)
	}

	return subgraph, nil
}

// identifier renders the claim as the identifier the pkg provider parses back: the purl with the requested version.
//
// Returns:
//   - `string`: the purl string, version included when requested.
func (c claim) identifier() string {

	p := c.purl
	p.Version = c.version
	return p.String()
}

// endregion
