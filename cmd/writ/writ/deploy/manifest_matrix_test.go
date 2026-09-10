// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package deploy_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/internal/lorepackage"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/deploy"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/tree"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/platform"
)

// layerFixture is one layer of a Matrix A tree: its name and the manifests it holds, keyed by project directory.
type layerFixture struct {
	name      string
	manifests map[string]string // directory ("common", "common.Darwin") → manifest YAML
}

// matrixConfig writes the layers under one temporary root and returns a layered deploy configuration over them,
// segments detected from the host, and a registry holding the named packages.
//
// Parameters:
//   - `t`: the test.
//   - `layers`: the layers, in contribution order.
//   - `registryPackages`: the registry packages to create, each with an empty lifecycle.
//
// Returns:
//   - `*deploy.Config`: the configuration; `BuildGraphs` with an empty pin plans it without git.
func matrixConfig(t *testing.T, layers []layerFixture, registryPackages ...string) *deploy.Config {

	t.Helper()

	root := t.TempDir()
	for _, name := range []string{"state", "config", "data", "cache"} {
		t.Setenv("XDG_"+strings.ToUpper(name)+"_HOME", filepath.Join(root, name))
	}

	targetRoot := filepath.Join(root, "home")
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	var sources []tree.LayerSource
	for order, layer := range layers {
		layerDir := filepath.Join(root, layer.name)
		for dir, body := range layer.manifests {
			if err := os.MkdirAll(filepath.Join(layerDir, dir), 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(layerDir, dir, "packages-manifest.yaml")
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		sources = append(sources, tree.LayerSource{
			Layer: layer.name, Path: layerDir, Order: order, SourceRoot: layerDir, OriginRoot: layerDir,
			TargetRoot: targetRoot, TargetName: "Home",
		})
	}

	registryDir := filepath.Join(root, "registry")
	for _, name := range registryPackages {
		pkgDir := filepath.Join(registryDir, "packages", name)
		if err := os.MkdirAll(pkgDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pkgDir, "lifecycle.yaml"), []byte("name: "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return &deploy.Config{
		SourceRoot:   root,
		TargetRoot:   targetRoot,
		LayerSources: sources,
		Projects:     []string{"common"},
		Segments:     segment.DetectSegments(),
		Registry:     lorepackage.New("test", nil, registryDir),
	}
}

// hostDefaultPurlType is the purl type of the host's default package manager: brew on Darwin, winget on Windows,
// and deb, rpm or alpm on Linux by distribution.
//
// Parameters:
//   - `t`: the test.
//
// Returns:
//   - `string`: the default purl type.
func hostDefaultPurlType(t *testing.T) string {

	t.Helper()

	spec, err := platform.Detect()
	if err != nil {
		t.Fatalf("platform.Detect: %v", err)
	}
	plat, err := platform.New(spec)
	if err != nil {
		t.Fatalf("platform.New: %v", err)
	}
	return plat.DefaultPurlType()
}

// plannedPackages returns the identifiers of the `pkg.install` units in the graph's packages subgraph, in order:
// each resource's versionless URI with its requested version appended.
//
// Parameters:
//   - `t`: the test.
//   - `graph`: a planned scope graph.
//
// Returns:
//   - `[]string`: the identifiers, in contribution order.
func plannedPackages(t *testing.T, graph *op.Graph) []string {

	t.Helper()

	var packages *op.Subgraph
	for _, sub := range graph.Subgraphs() {
		if sub.Name == "packages" {
			packages = sub
		}
	}
	if packages == nil {
		t.Fatalf("the scope graph has no packages subgraph; subgraphs: %d, nodes: %d",
			len(graph.Subgraphs()), len(graph.Nodes()))
	}

	var identifiers []string
	for _, child := range packages.Children() {
		node, ok := child.(*op.Node)
		if !ok {
			t.Fatalf("packages child %s is a %T, want a node", child.ID(), child)
		}
		if node.Action() == nil || node.Action().Name() != "pkg.install" {
			t.Fatalf("packages child %s does not plan pkg.install", node.ID())
		}
		binding, ok := node.Slots()["packages"].(op.ImmediateBinding)
		if !ok {
			t.Fatalf("node %s: packages slot is %T, want an immediate binding", node.ID(), node.Slots()["packages"])
		}
		values := reflect.ValueOf(binding.Resolve(nil, nil))
		for i := range values.Len() {
			identifiers = append(identifiers, identifierOf(t, values.Index(i).Interface()))
		}
	}

	return identifiers
}

// identifierOf renders one planned package as its purl with the requested version.
//
// Parameters:
//   - `t`: the test.
//   - `value`: a converted package resource, or the identifier string if conversion did not run.
//
// Returns:
//   - `string`: the identifier.
func identifierOf(t *testing.T, value any) string {

	t.Helper()

	switch v := value.(type) {
	case string:
		return v
	case interface {
		ReachabilityURI() string
		Version() string
	}:
		if v.Version() == "" {
			return v.ReachabilityURI()
		}
		return v.ReachabilityURI() + "@" + v.Version()
	default:
		t.Fatalf("planned package is a %T, want a pkg resource", value)
		return ""
	}
}

// TestBuildGraphs_MatrixA_TheUnionOnThisPlatform is row 6 of #814's test plan and Matrix A of #872: one sparsely
// populated three-layer tree yields a different union on Darwin, Linux and Windows, because the suffix chain differs
// per OS. Each cell exercises one merge rule; the expectation is the host platform's row, so CI proves all three.
func TestBuildGraphs_MatrixA_TheUnionOnThisPlatform(t *testing.T) {

	defaultType := hostDefaultPurlType(t)

	layers := []layerFixture{
		{"base", map[string]string{
			"common":      "packages:\n  - name: jq\n  - name: git\n",
			"common.Unix": "packages:\n  - name: shellcheck\n",
		}},
		{"team", map[string]string{
			"common":         "packages:\n  - name: jq\n  - name: ripgrep\n",
			"common.Darwin":  "packages:\n  - name: brew:jq\n",
			"common.Windows": "packages:\n  - name: pkg:winget/jqlang/jq\n",
		}},
		{"personal", map[string]string{
			"common":        "packages:\n  - name: jq@1.7\n    with: [feature]\n",
			"common.Darwin": "packages:\n  - name: port:jq\n",
			"common.Linux":  "packages:\n  - name: pkg:" + defaultType + "/curl\n",
		}},
	}

	var want []string
	var wantClaims int
	switch runtime.GOOS {
	case "darwin":
		want = []string{"pkg:brew/jq@1.7", "pkg:brew/git", "pkg:brew/shellcheck", "pkg:brew/ripgrep", "pkg:port/jq"}
		wantClaims = 4 // base, team, team.Darwin (brew:jq), personal (the pin)
	case "linux":
		want = []string{
			"pkg:" + defaultType + "/jq@1.7", "pkg:" + defaultType + "/git", "pkg:" + defaultType + "/shellcheck",
			"pkg:" + defaultType + "/ripgrep", "pkg:" + defaultType + "/curl",
		}
		wantClaims = 3 // base, team, personal (the pin)
	case "windows":
		want = []string{"pkg:winget/jqlang/jq@1.7", "pkg:winget/git", "pkg:winget/ripgrep"}
		wantClaims = 4 // base, team, team.Windows (the namespaced purl), personal (the pin)
	default:
		t.Skipf("Matrix A has no row for %s", runtime.GOOS)
	}

	build, err := deploy.BuildGraphs(context.Background(), matrixConfig(t, layers), &deploy.PinInfo{})
	if err != nil {
		t.Fatalf("BuildGraphs: %v", err)
	}
	if len(build.Graphs) != 1 {
		t.Fatalf("planned %d graphs, want 1 -- one Home scope", len(build.Graphs))
	}

	if got := plannedPackages(t, build.Graphs[0]); !reflect.DeepEqual(got, want) {
		t.Errorf("planned = %v\nwant      %v", got, want)
	}

	if len(build.Deferred) != 0 {
		t.Errorf("deferred = %+v; want none -- no claim names a registry package", build.Deferred)
	}
	if len(build.Duplicates) != 1 || !strings.HasPrefix(build.Duplicates[0].Package, "pkg:") ||
		!strings.Contains(build.Duplicates[0].Package, "/jq") {
		t.Fatalf("duplicates = %+v; want exactly one note, on jq", build.Duplicates)
	}
	if got := len(build.Duplicates[0].Manifests); got != wantClaims {
		t.Errorf("jq claimed by %d manifests, want %d: %v", got, wantClaims, build.Duplicates[0].Manifests)
	}
}

// TestBuildGraphs_TwoDifferentPinsRefuseTheDeploy is the second Matrix A tree: two manifests pinning different
// versions of one package refuse the deploy naming the package, both versions and both manifests. The interim
// answer, until the broker can put the question to the manager (#872).
func TestBuildGraphs_TwoDifferentPinsRefuseTheDeploy(t *testing.T) {

	osDir := "common." + segment.DetectSegments()[0].Value
	layers := []layerFixture{
		{"base", map[string]string{"common": "packages:\n  - name: jq@1.6\n", osDir: "packages:\n  - name: jq@1.7\n"}},
	}

	_, err := deploy.BuildGraphs(context.Background(), matrixConfig(t, layers), &deploy.PinInfo{})
	if err == nil {
		t.Fatal("BuildGraphs planned two different pins of jq; want a refusal")
	}
	for _, want := range []string{"jq", "1.6", "1.7", "common", osDir} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal %q does not name %s", err, want)
		}
	}
}

// TestBuildGraphs_AManifestOnlyDirectoryPlansAScope is the third Matrix A tree, delta 9 of the #814 review: a layer
// holding a manifest and no files still yields a scope graph, whose only unit is the packages subgraph; and a claim
// that names a registry package is deferred with a note rather than planned.
func TestBuildGraphs_AManifestOnlyDirectoryPlansAScope(t *testing.T) {

	layers := []layerFixture{
		{"base", map[string]string{"common": "packages:\n  - name: docker\n  - name: jq\n"}},
	}

	build, err := deploy.BuildGraphs(context.Background(), matrixConfig(t, layers, "docker"), &deploy.PinInfo{})
	if err != nil {
		t.Fatalf("BuildGraphs: %v", err)
	}
	if len(build.Graphs) != 1 {
		t.Fatalf("planned %d graphs, want 1 -- a manifest-only scope still gets a graph", len(build.Graphs))
	}

	graph := build.Graphs[0]
	if want := []string{"pkg:" + hostDefaultPurlType(t) + "/jq"}; !reflect.DeepEqual(plannedPackages(t, graph), want) {
		t.Errorf("planned = %v; want %v", plannedPackages(t, graph), want)
	}
	if len(graph.Nodes()) != 1 || len(graph.Subgraphs()) != 1 {
		t.Errorf("graph holds %d nodes and %d subgraphs; want 1 and 1 -- the packages subgraph and its one install",
			len(graph.Nodes()), len(graph.Subgraphs()))
	}
	if len(build.Deferred) != 1 || build.Deferred[0].Package != "docker" {
		t.Errorf("deferred = %+v; want docker, noted and not planned", build.Deferred)
	}
}
