// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build integration

package star

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// The knowledge indexer updates each domain's index.yaml to track which files exist. It must never
// change anything else: the per-entry metadata is hand-written, and an earlier version that rebuilt
// each file from a directory listing deleted it wholesale (devlore-registry#80). These tests pin
// both halves of the contract — what it preserves, and what it refuses to guess at.

// region HELPERS

// knowledgeRuntime loads the devlore extensions from star/extensions, which is the on-disk tree
// rather than the embedded cmd/star/extensions one. The devlore.* commands live only there.
func knowledgeRuntime(t *testing.T, workdir string) *Application {
	t.Helper()

	root, err := findProjectRoot()
	if err != nil {
		t.Fatalf("locating project root: %v", err)
	}

	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(workdir); err != nil {
		t.Fatalf("chdir %s: %v", workdir, err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	app := NewApplication(&cobra.Command{Use: "star"})
	t.Cleanup(func() { _ = app.Close() })

	if err := app.LoadExtensionsFrom(filepath.Join(root, "star", "extensions")); err != nil {
		t.Fatalf("loading extensions: %v", err)
	}
	return app
}

// runIndex invokes the command the way the workflow does. The target is relative because fsroot
// confinement refuses a path outside the working directory.
func runIndex(t *testing.T, app *Application, target string) error {
	t.Helper()

	cmd, ok := app.Commands()["devlore knowledge index"]
	if !ok {
		t.Fatal(`command "devlore knowledge index" is not registered`)
	}
	return cmd.Run(map[string]string{"target": target, "dry_run": ""})
}

// writeTree materializes files under dir; a path ending in "/" creates an empty directory.
func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()

	for rel, body := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if strings.HasSuffix(rel, "/") {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", full, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
}

// registry builds a minimal target tree and returns the workdir and the relative target.
func registry(t *testing.T, files map[string]string) (workdir, target string) {
	t.Helper()

	workdir = t.TempDir()
	target = "reg"
	writeTree(t, filepath.Join(workdir, target), files)
	return workdir, target
}

func readIndex(t *testing.T, workdir, target, domain string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(workdir, target, "knowledge", domain, "index.yaml"))
	if err != nil {
		t.Fatalf("reading %s index: %v", domain, err)
	}
	return string(data)
}

// refuses asserts the command failed AND that the failure names the problem. Asserting only that an
// error occurred would pass if the command broke for some unrelated reason — the same
// indistinguishable-from-success failure these tests exist to prevent.
func refuses(t *testing.T, app *Application, target, wantSubstring string) {
	t.Helper()

	err := runIndex(t, app, target)
	if err == nil {
		t.Fatalf("expected refusal mentioning %q, got success", wantSubstring)
	}
	if !strings.Contains(err.Error(), wantSubstring) {
		t.Fatalf("refused, but not for the expected reason.\nwant substring: %q\ngot: %v", wantSubstring, err)
	}
}

// region PRESERVATION

func TestKnowledgeIndex_PreservesEntryMetadata(t *testing.T) {
	workdir, target := registry(t, map[string]string{
		"knowledge/packages/slots/homebrew.yaml": "name: homebrew\n",
		"knowledge/packages/index.yaml": "domain: packages\n" +
			"slots:\n" +
			"  - name: homebrew.yaml\n" +
			"    package: homebrew\n" +
			"    description: Package manager for macOS and Linux\n" +
			"    platforms: [darwin, linux]\n",
	})

	app := knowledgeRuntime(t, workdir)
	if err := runIndex(t, app, target); err != nil {
		t.Fatalf("index: %v", err)
	}

	got := readIndex(t, workdir, target, "packages")
	for _, want := range []string{"package: homebrew", "Package manager for macOS and Linux", "darwin"} {
		if !strings.Contains(got, want) {
			t.Errorf("metadata %q was lost:\n%s", want, got)
		}
	}
}

func TestKnowledgeIndex_AddsNewFileAndKeepsNeighbourMetadata(t *testing.T) {
	workdir, target := registry(t, map[string]string{
		"knowledge/packages/slots/homebrew.yaml": "name: homebrew\n",
		"knowledge/packages/slots/macports.yaml": "name: macports\n",
		"knowledge/packages/index.yaml": "domain: packages\n" +
			"slots:\n" +
			"  - name: homebrew.yaml\n" +
			"    package: homebrew\n",
	})

	app := knowledgeRuntime(t, workdir)
	if err := runIndex(t, app, target); err != nil {
		t.Fatalf("index: %v", err)
	}

	got := readIndex(t, workdir, target, "packages")
	if !strings.Contains(got, "macports.yaml") {
		t.Errorf("new file was not indexed:\n%s", got)
	}
	if !strings.Contains(got, "package: homebrew") {
		t.Errorf("adding a file disturbed an existing entry:\n%s", got)
	}
}

func TestKnowledgeIndex_IsIdempotent(t *testing.T) {
	workdir, target := registry(t, map[string]string{
		"knowledge/packages/slots/homebrew.yaml": "name: homebrew\n",
		"knowledge/packages/index.yaml": "domain: packages\n" +
			"slots:\n  - name: homebrew.yaml\n    package: homebrew\n",
	})

	app := knowledgeRuntime(t, workdir)
	if err := runIndex(t, app, target); err != nil {
		t.Fatalf("first run: %v", err)
	}
	first := readIndex(t, workdir, target, "packages")

	if err := runIndex(t, app, target); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second := readIndex(t, workdir, target, "packages"); second != first {
		t.Errorf("second run changed the file:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

// region REFUSAL
//
// Each of these was a silent success before. The indexer's quiet was indistinguishable from a
// correct run, and validate-yaml-schemas stayed green because the output still parsed.

func TestKnowledgeIndex_RefusesUnrecognizedDirectory(t *testing.T) {
	workdir, target := registry(t, map[string]string{
		"knowledge/authoring/bindings/rules.yaml": "x: 1\n",
		"knowledge/authoring/index.yaml":          "domain: authoring\n",
	})

	app := knowledgeRuntime(t, workdir)
	// bindings IS recognized; use a name that is not.
	writeTree(t, filepath.Join(workdir, target), map[string]string{
		"knowledge/authoring/rubbish/thing.yaml": "x: 1\n",
	})

	refuses(t, app, target, "rubbish")
}

func TestKnowledgeIndex_RefusesLooseFile(t *testing.T) {
	workdir, target := registry(t, map[string]string{
		"knowledge/migration/prompts/go.txt": "hello\n",
		"knowledge/migration/README.md":      "loose\n",
		"knowledge/migration/index.yaml":     "domain: migration\nprompts:\n  - name: go.txt\n",
	})

	app := knowledgeRuntime(t, workdir)
	refuses(t, app, target, "README.md")
}

func TestKnowledgeIndex_RefusesIndexedFileThatDoesNotExist(t *testing.T) {
	// The file moved rather than vanished — this is signatures/dotfile-systems.yaml. Dropping the
	// entry silently would discard its metadata and report success.
	workdir, target := registry(t, map[string]string{
		"knowledge/migration/signatures/stow.yaml": "x: 1\n",
		"knowledge/migration/index.yaml": "domain: migration\n" +
			"signatures:\n  - name: stow.yaml\n  - name: moved-away.yaml\n",
	})

	app := knowledgeRuntime(t, workdir)
	refuses(t, app, target, "moved-away.yaml")
}

func TestKnowledgeIndex_RefusesAssetsWithNoSection(t *testing.T) {
	workdir, target := registry(t, map[string]string{
		"knowledge/authoring/bindings/rules.yaml": "x: 1\n",
		"knowledge/authoring/index.yaml":          "domain: authoring\n",
	})

	app := knowledgeRuntime(t, workdir)
	refuses(t, app, target, "bindings")
}

func TestKnowledgeIndex_RefusesUnknownSection(t *testing.T) {
	workdir, target := registry(t, map[string]string{
		"knowledge/migration/prompts/go.txt": "hello\n",
		"knowledge/migration/index.yaml": "domain: migration\n" +
			"prompts:\n  - name: go.txt\n" +
			"invented:\n  - name: nothing.yaml\n",
	})

	app := knowledgeRuntime(t, workdir)
	refuses(t, app, target, "invented")
}

func TestKnowledgeIndex_AcceptsRecognizedNewTypes(t *testing.T) {
	// concepts, providers, and bindings are real directories in the registry and were unknown to
	// the indexer until this change.
	workdir, target := registry(t, map[string]string{
		"knowledge/d/concepts/a.yaml":  "x: 1\n",
		"knowledge/d/providers/b.yaml": "x: 1\n",
		"knowledge/d/bindings/c.yaml":  "x: 1\n",
		"knowledge/d/index.yaml": "domain: d\n" +
			"concepts:\n  - name: a.yaml\n" +
			"providers:\n  - name: b.yaml\n" +
			"bindings:\n  - name: c.yaml\n",
	})

	app := knowledgeRuntime(t, workdir)
	if err := runIndex(t, app, target); err != nil {
		t.Fatalf("the three new asset types should be accepted: %v", err)
	}
}
