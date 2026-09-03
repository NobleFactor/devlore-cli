// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// CommonSetFlagNames are the four flags the shared root puts on every command of every program
// (10-command-line-interface.md §4): a subcommand inherits all four and defines none.
var CommonSetFlagNames = []string{"filter", "jq", "output", "store"}

// ReservedOutputFlagNames are the names no command may bind for itself: the common set, and the two the
// convention bans outright (§4, §14). Cobra lets a leaf shadow an inherited flag silently, which is how
// `star devlore actions generate -o json` once meant a directory.
var ReservedOutputFlagNames = []string{"filter", "format", "jq", "json", "output", "store"}

// resultImportPath is the package only cmd/internal/cli may import: every rendering goes through
// [BuildPipeline], so a second importer is a second convention.
const resultImportPath = "github.com/NobleFactor/devlore-cli/pkg/result"

// CheckNoOwnOutputFlag walks the command tree beneath root and reports three shapes: a subcommand that
// defines a flag whose long name or shorthand an ancestor's persistent flags already carry -- cobra
// calls the long-name case an override and says nothing, and panics on the shorthand case the first
// time the command runs; neither is protection, this is (ruled 2026-09-03) -- a command that binds one
// of [ReservedOutputFlagNames] on itself, and a subcommand that does not inherit all of
// [CommonSetFlagNames] from the root. An empty result is the invariant holding.
//
// It reads each command's raw flag sets rather than cobra's merged views, because the merged views are
// what panics on a shorthand collision; a flag that is the very ancestor flag, merged in earlier, is told
// from a shadow by identity.
//
// Parameters:
//   - `root`: the program's root command.
//
// Returns:
//   - `[]string`: one line per violation, naming the command path and the flag.
func CheckNoOwnOutputFlag(root *cobra.Command) []string {

	var violations []string
	var walk func(cmd *cobra.Command, inherited *pflag.FlagSet)
	walk = func(cmd *cobra.Command, inherited *pflag.FlagSet) {
		for _, sub := range cmd.Commands() {
			own := ownFlags(sub)
			own.VisitAll(func(local *pflag.Flag) {
				reportShadow(&violations, sub, local, inherited)
			})
			for _, name := range ReservedOutputFlagNames {
				if local := own.Lookup(name); local != nil && inherited.Lookup(name) != local {
					violations = append(violations, fmt.Sprintf("%s binds --%s itself; the common set owns that name", sub.CommandPath(), name))
				}
			}
			if local := own.ShorthandLookup("o"); local != nil && inherited.ShorthandLookup("o") == nil {
				violations = append(violations, fmt.Sprintf("%s binds -o itself, as --%s; -o is --output", sub.CommandPath(), local.Name))
			}
			for _, name := range CommonSetFlagNames {
				if inherited.Lookup(name) == nil {
					violations = append(violations, fmt.Sprintf("%s does not inherit --%s from the root", sub.CommandPath(), name))
				}
			}
			next := pflag.NewFlagSet(sub.CommandPath(), pflag.ContinueOnError)
			next.AddFlagSet(inherited)
			sub.PersistentFlags().VisitAll(func(flag *pflag.Flag) {
				if next.Lookup(flag.Name) == nil && (flag.Shorthand == "" || next.ShorthandLookup(flag.Shorthand) == nil) {
					next.AddFlag(flag)
				}
			})
			walk(sub, next)
		}
	}
	walk(root, root.PersistentFlags())

	return violations
}

// ownFlags returns the flags a command defines itself, local and persistent, read from the raw sets so
// nothing is merged. A flag merged in earlier by cobra is the ancestor's own *pflag.Flag and is not a
// definition of this command's.
//
// Parameters:
//   - `cmd`: the command.
//
// Returns:
//   - `*pflag.FlagSet`: the command's own flags.
func ownFlags(cmd *cobra.Command) *pflag.FlagSet {

	own := pflag.NewFlagSet(cmd.CommandPath(), pflag.ContinueOnError)
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if own.Lookup(flag.Name) == nil {
			own.AddFlag(flag)
		}
	})
	cmd.PersistentFlags().VisitAll(func(flag *pflag.Flag) {
		if own.Lookup(flag.Name) == nil {
			own.AddFlag(flag)
		}
	})

	return own
}

// reportShadow appends a violation when a command's own flag reuses the long name or the shorthand of a
// flag it inherits. The inherited flag merged into the command by cobra is the same *pflag.Flag and is
// not a shadow.
//
// Parameters:
//   - `violations`: the list being built.
//   - `cmd`: the command defining the flag.
//   - `local`: the flag it defines.
//   - `inherited`: every persistent flag its ancestors carry.
func reportShadow(violations *[]string, cmd *cobra.Command, local *pflag.Flag, inherited *pflag.FlagSet) {

	if taken := inherited.Lookup(local.Name); taken != nil && taken != local {
		*violations = append(*violations, fmt.Sprintf("%s redefines --%s, which it inherits; cobra would let the local one win silently", cmd.CommandPath(), local.Name))
	}
	if local.Shorthand != "" {
		if taken := inherited.ShorthandLookup(local.Shorthand); taken != nil && taken != local && taken.Name != local.Name {
			*violations = append(*violations, fmt.Sprintf("%s binds -%s as --%s, but -%s is --%s on an ancestor; cobra would panic when the command runs", cmd.CommandPath(), local.Shorthand, local.Name, local.Shorthand, taken.Name))
		}
	}
}

// CheckSharedSetOnRoot reports whether root carries the common set as the shared root binds it: all four
// flags present as persistent flags, each with the shared usage text, so a root that hand-rolled the set
// with the same names is still caught.
//
// Parameters:
//   - `root`: the program's root command.
//
// Returns:
//   - `[]string`: one line per missing or foreign flag.
func CheckSharedSetOnRoot(root *cobra.Command) []string {

	var violations []string
	for name, usage := range commonSetUsage {
		flag := root.PersistentFlags().Lookup(name)
		switch {
		case flag == nil:
			violations = append(violations, fmt.Sprintf("%s does not register --%s on its root", root.Name(), name))
		case flag.Usage != usage:
			violations = append(violations, fmt.Sprintf("%s registers --%s with its own usage text; the set is the shared root's", root.Name(), name))
		}
	}
	slices.Sort(violations)

	return violations
}

// NoDirectStdout parses every non-test Go file under the directories and reports each write to stdout that
// bypasses the sink: `fmt.Print*`, `fmt.Fprint*` with `os.Stdout` as its writer, `os.Stdout.Write*`, the
// `print` and `println` builtins, and any statement that hands `os.Stdout` to something else -- which is
// how a child process inherits the terminal. Reading `os.Stdout` -- `Fd()`, `Stat()` -- is not a write and
// is not reported. The one place allowed to hand the terminal over is [RunInteractive], the seam every
// interactive child goes through (10-command-line-interface.md §10).
//
// Parameters:
//   - `dirs`: package directories to walk, recursively; `testdata` directories are skipped.
//
// Returns:
//   - `[]string`: one line per write, as `file:line: what`.
//   - `error`: a file that cannot be read or parsed.
func NoDirectStdout(dirs ...string) ([]string, error) {

	var violations []string
	err := walkGoFiles(dirs, func(filename string, file *ast.File, fset *token.FileSet) {
		exempt := map[ast.Node]bool{}
		if file.Name.Name == "cli" {
			for _, decl := range file.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "RunInteractive" && fn.Body != nil {
					exempt[fn.Body] = true
				}
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if exempt[node] {
				return false
			}
			if what := stdoutWrite(node); what != "" {
				violations = append(violations, fmt.Sprintf("%s: %s", fset.Position(node.Pos()), what))
			}
			return true
		})
	})

	return violations, err
}

// NoPrivatePipeline parses every non-test Go file under the directories and reports each import of
// pkg/result outside cmd/internal/cli: [BuildPipeline] is the one way to a rendering, and a second importer
// is a second convention (10-command-line-interface.md §14).
//
// Parameters:
//   - `dirs`: package directories to walk, recursively.
//
// Returns:
//   - `[]string`: one line per importer, as `file:line: imports pkg/result`.
//   - `error`: a file that cannot be read or parsed.
func NoPrivatePipeline(dirs ...string) ([]string, error) {

	var violations []string
	err := walkGoFiles(dirs, func(filename string, file *ast.File, fset *token.FileSet) {
		if file.Name.Name == "cli" && strings.Contains(filepath.ToSlash(filename), "cmd/internal/cli/") {
			return
		}
		for _, imp := range file.Imports {
			if path, err := strconv.Unquote(imp.Path.Value); err == nil && path == resultImportPath {
				violations = append(violations, fmt.Sprintf("%s: imports pkg/result; render through cli.Emit", fset.Position(imp.Pos())))
			}
		}
	})

	return violations, err
}

// stdoutWrite classifies one AST node: the description of the write it is, or "" when it is not one.
//
// Parameters:
//   - `node`: any node.
//
// Returns:
//   - `string`: what was written and how, or "".
func stdoutWrite(node ast.Node) string {

	switch n := node.(type) {
	case *ast.CallExpr:
		switch fun := n.Fun.(type) {
		case *ast.Ident:
			if fun.Name == "print" || fun.Name == "println" {
				return fun.Name + "() writes to stdout"
			}
		case *ast.SelectorExpr:
			if isIdent(fun.X, "fmt") && strings.HasPrefix(fun.Sel.Name, "Print") {
				return "fmt." + fun.Sel.Name + " writes to stdout"
			}
			if isIdent(fun.X, "fmt") && strings.HasPrefix(fun.Sel.Name, "Fprint") && len(n.Args) > 0 && isOSStdout(n.Args[0]) {
				return "fmt." + fun.Sel.Name + "(os.Stdout, ...) writes to stdout"
			}
			if isOSStdout(fun.X) && strings.HasPrefix(fun.Sel.Name, "Write") {
				return "os.Stdout." + fun.Sel.Name + " writes to stdout"
			}
		}
	case *ast.AssignStmt:
		for _, rhs := range n.Rhs {
			if isOSStdout(rhs) {
				return "os.Stdout is handed to something else; only RunInteractive may do that"
			}
		}
	case *ast.KeyValueExpr:
		if isOSStdout(n.Value) {
			return "os.Stdout is handed to something else; only RunInteractive may do that"
		}
	}

	return ""
}

// walkGoFiles parses every non-test, non-testdata Go file under the directories and calls fn on each.
//
// Parameters:
//   - `dirs`: the roots to walk.
//   - `fn`: called with the file name, its parsed form and the file set.
//
// Returns:
//   - `error`: the first read or parse failure.
func walkGoFiles(dirs []string, fn func(filename string, file *ast.File, fset *token.FileSet)) error {

	fset := token.NewFileSet()
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if entry.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			fn(path, file, fset)
			return nil
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// isIdent reports whether the expression is the bare identifier.
func isIdent(expr ast.Expr, name string) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == name
}

// isOSStdout reports whether the expression is the selector `os.Stdout`.
func isOSStdout(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	return ok && isIdent(sel.X, "os") && sel.Sel.Name == "Stdout"
}
