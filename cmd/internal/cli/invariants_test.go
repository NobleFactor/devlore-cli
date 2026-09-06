// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// inScopeDirs are the packages the convention governs: the shared package and the four programs
// (10-command-line-interface.md §2). The developer tools -- devlore-docs, devlore-index,
// devlore-inventory -- are not yet in scope and join the walk when they join the suite.
func inScopeDirs(t *testing.T) []string {
	t.Helper()
	root := filepath.Join("..", "..", "..")
	var dirs []string
	for _, dir := range []string{"cmd/internal/cli", "cmd/devlore-test", "cmd/lore", "cmd/star", "cmd/writ"} {
		dirs = append(dirs, filepath.Join(root, filepath.FromSlash(dir)))
	}
	return dirs
}

// TestNoDirectStdout_IsRedOnTheFixture shows the walk can fail: the fixture has six writes and two reads,
// and the walk reports the six by line.
func TestNoDirectStdout_IsRedOnTheFixture(t *testing.T) {

	violations, err := NoDirectStdout(filepath.Join("testdata", "direct_stdout"))
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 6 {
		t.Fatalf("expected 6 writes reported, got %d:\n%s", len(violations), strings.Join(violations, "\n"))
	}
	for _, unwanted := range []string{"Fd()", "Stat()", "Stderr"} {
		for _, v := range violations {
			if strings.Contains(v, unwanted) {
				t.Errorf("a read or a stderr write was reported: %s", v)
			}
		}
	}
}

// TestNoDirectStdout_InScope is invariant 1 over the real tree. It is committed red: on 2026-09-03 the shared
// package itself has thirteen writes -- config get, list, path, schema and validate print their results,
// man prints its install notes, and self install prints its prompt. Phase 2 of 776-output-enforcement.md
// turns it green.
func TestNoDirectStdout_InScope(t *testing.T) {

	violations, err := NoDirectStdout(inScopeDirs(t)...)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Errorf("%d direct writes to stdout in in-scope packages:\n%s", len(violations), strings.Join(violations, "\n"))
	}
}

// TestNoPrivatePipeline_IsRedOnTheFixture shows the walk can fail.
func TestNoPrivatePipeline_IsRedOnTheFixture(t *testing.T) {

	violations, err := NoPrivatePipeline(filepath.Join("testdata", "private_pipeline"))
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 1 {
		t.Fatalf("expected the one import reported, got %d:\n%s", len(violations), strings.Join(violations, "\n"))
	}
}

// TestNoPrivatePipeline_InScope is invariant 3's source half over the real tree: only cmd/internal/cli
// imports pkg/result.
func TestNoPrivatePipeline_InScope(t *testing.T) {

	violations, err := NoPrivatePipeline(inScopeDirs(t)...)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Errorf("pkg/result imported outside cmd/internal/cli:\n%s", strings.Join(violations, "\n"))
	}
}

// TestCheckNoOwnOutputFlag_IsRedOnASyntheticTree shows the tree checker can fail, on each shape it guards.
func TestCheckNoOwnOutputFlag_IsRedOnASyntheticTree(t *testing.T) {

	// A private --format under the shared root: the leaf shadows nothing, so cobra lets it through and the
	// checker is what catches it.
	root := sharedRoot("probe")
	own := &cobra.Command{Use: "own", Run: func(*cobra.Command, []string) {}}
	own.Flags().String("format", "", "a private rendering")
	own.Flags().Bool("verbose", false, "a shadow of the root's --verbose; cobra would let it win silently")
	root.AddCommand(own)
	violations := CheckNoOwnOutputFlag(root)
	if !hasPrefix(violations, "probe own binds --format itself") {
		t.Errorf("a private --format passed; got:\n%s", strings.Join(violations, "\n"))
	}
	if !hasPrefix(violations, "probe own redefines --verbose") {
		t.Errorf("a shadowed --verbose passed; got:\n%s", strings.Join(violations, "\n"))
	}

	// A shorthand collision under the shared root would panic in cobra's own merge before any test saw
	// it; the checker reads the flag sets without merging, so it reports the collision instead.
	collide := &cobra.Command{Use: "collide", Run: func(*cobra.Command, []string) {}}
	collide.Flags().StringP("out", "o", "", "a private shorthand")
	root.AddCommand(collide)
	if !hasPrefix(CheckNoOwnOutputFlag(root), "probe collide binds -o as --out, but -o is --output on an ancestor") {
		t.Errorf("a shorthand collision passed; got:\n%s", strings.Join(CheckNoOwnOutputFlag(root), "\n"))
	}

	// A private -o cannot even be built under the shared root: pflag panics on the redefined shorthand when
	// cobra merges the parent's flags, which is a louder guard than this one. Under a bare root the
	// checker reports it, along with the four the leaf does not inherit.
	bare := &cobra.Command{Use: "bare"}
	leaf := &cobra.Command{Use: "leaf", Run: func(*cobra.Command, []string) {}}
	leaf.Flags().StringP("out", "o", "", "a private shorthand")
	bare.AddCommand(leaf)
	violations = CheckNoOwnOutputFlag(bare)
	if !hasPrefix(violations, "bare leaf binds -o itself") {
		t.Errorf("a private -o passed; got:\n%s", strings.Join(violations, "\n"))
	}
	if len(violations) != 5 {
		t.Errorf("expected -o plus the four missing inherits, 5 violations; got %d:\n%s", len(violations), strings.Join(violations, "\n"))
	}
}

// TestCheckSharedSetOnRoot_IsRedOnAHandRolledRoot shows the root checker can fail: a root that binds the
// four names with usage text of its own is not on the shared set.
func TestCheckSharedSetOnRoot_IsRedOnAHandRolledRoot(t *testing.T) {

	if v := CheckSharedSetOnRoot(sharedRoot("probe")); len(v) != 0 {
		t.Errorf("the shared root fails its own check:\n%s", strings.Join(v, "\n"))
	}

	hand := &cobra.Command{Use: "hand"}
	for _, name := range CommonSetFlagNames {
		hand.PersistentFlags().String(name, "", "my own "+name)
	}
	if v := CheckSharedSetOnRoot(hand); len(v) != 4 {
		t.Errorf("a hand-rolled set passed; expected 4 violations, got %d:\n%s", len(v), strings.Join(v, "\n"))
	}
	if v := CheckSharedSetOnRoot(&cobra.Command{Use: "none"}); len(v) != 4 {
		t.Errorf("a root with no set passed; expected 4 violations, got %d", len(v))
	}
}

// sharedRoot builds a root the way every program does: the shared constructor, which carries the common
// set by construction.
func sharedRoot(name string) *cobra.Command {
	return NewRootCmd(RootConfig{Name: name, Short: "a probe"})
}

// TestRunInteractive_RefusesWithoutATerminal pins the seam's refusal: no terminal, no launch, and the
// message names the alternative.
func TestRunInteractive_RefusesWithoutATerminal(t *testing.T) {

	previous := isTerminal
	isTerminal = func(*os.File) bool { return false }
	t.Cleanup(func() { isTerminal = previous })

	err := RunInteractive(exec.CommandContext(context.Background(), "vi"), "run `probe config path` and open the file yourself")
	if err == nil {
		t.Fatal("RunInteractive launched an editor with no terminal")
	}
	if !strings.Contains(err.Error(), "config path") {
		t.Errorf("the refusal does not name the alternative: %v", err)
	}
}

// TestRunInteractive_HandsOverWithATerminal pins the handoff: with a terminal, the child runs with the
// process's streams. A no-op child stands in for the editor.
func TestRunInteractive_HandsOverWithATerminal(t *testing.T) {

	previous := isTerminal
	isTerminal = func(*os.File) bool { return true }
	t.Cleanup(func() { isTerminal = previous })

	child := exec.CommandContext(context.Background(), "true")
	if runtime.GOOS == "windows" {
		child = exec.CommandContext(context.Background(), "cmd", "/c", "exit", "0")
	}
	if err := RunInteractive(child, "irrelevant"); err != nil {
		t.Fatalf("the handoff failed: %v", err)
	}
	if child.Stdout != os.Stdout || child.Stdin != os.Stdin {
		t.Error("the child did not receive the process's terminal")
	}
}

// hasPrefix reports whether any line starts with the prefix.
func hasPrefix(lines []string, prefix string) bool {
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}
