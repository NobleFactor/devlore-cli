// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package claimcheck verifies that a provider method does what its `+devlore:claim=` directive says.
//
// A claim is an assertion by the author, not a fact derived from the signature ([op.MethodClaims]). This package
// is what holds the author to it: it loads the module with full type information and reports every place a
// claiming method reaches a capability the claim forbids.
//
// The check needs types, not syntax. `os.Getenv` and `os.FileMode` are the same shape to a parser — both a
// selector on `os` — and `os.FileMode(mode)` is a *call expression* despite being a type conversion. Only a
// type-checked program can say that the first is a function, the second a type, and the third a conversion. That
// is why this loads through [packages] rather than walking an AST.
package claimcheck

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// region EXPORTED TYPES

// Violation is one claim contradicted by the code that carries it.
type Violation struct {
	Method   string // the claiming method, as `provider.Method`
	Claim    string // the claim the reach contradicts
	Reach    string // the capability reached, as `package.Function`
	Position string // where, as `file.go:line`
}

// String renders the violation as the build error an author reads.
//
// The shape names the reach and never renders a verdict: an author who reads "calls os.Getenv at provider.go:110"
// knows what to change, where "not deterministic" sends them hunting. It mirrors the codegen-side message that
// quotes an offending signature back.
//
// Returns:
//   - `string`: the one-line failure.
func (v Violation) String() string {
	return fmt.Sprintf("method %s: claims %s but calls %s at %s", v.Method, v.Claim, v.Reach, v.Position)
}

// endregion

// region EXPORTED FUNCTIONS

// Check loads `patterns` with type information and reports every claim its own code contradicts.
//
// Loading is the expensive part — it runs the compiler front end — so callers pass every pattern at once rather
// than calling per package.
//
// Parameters:
//   - `patterns`: go package patterns, e.g. "./pkg/op/provider/...".
//
// Returns:
//   - `[]Violation`: the violations found, ordered by method then reach; empty when every claim holds.
//   - `error`: non-nil when loading fails or a package carries type errors.
func Check(patterns ...string) ([]Violation, error) {

	mode := packages.NeedName | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo |
		packages.NeedDeps | packages.NeedImports | packages.NeedFiles

	loaded, err := packages.Load(&packages.Config{Mode: mode, Tests: false}, patterns...)
	if err != nil {
		return nil, fmt.Errorf("claimcheck: load %v: %w", patterns, err)
	}

	var violations []Violation

	for _, pkg := range loaded {

		if len(pkg.Errors) > 0 {
			return nil, fmt.Errorf("claimcheck: %s: %w", pkg.PkgPath, pkg.Errors[0])
		}

		violations = append(violations, inspectPackage(pkg)...)
	}

	sort.Slice(violations, func(i, j int) bool {
		if violations[i].Method != violations[j].Method {
			return violations[i].Method < violations[j].Method
		}

		return violations[i].Reach < violations[j].Reach
	})

	return violations, nil
}

// endregion

// region HELPER FUNCTIONS

// forbidden maps a claim to the packages a method carrying it may not call into.
//
// The lists enumerate the ways a Go program reaches outside its own process, which the operating-system interface
// fixes — they are not a curated set of "unsafe" packages that grows as the codebase does. `deterministic` forbids
// all of them. `sandboxed` forbids only the ones that touch the filesystem, since a sandboxed method may still
// read a clock or spawn nothing.
var forbidden = map[string][]string{
	"deterministic": {
		"crypto/rand", "math/rand", "net", "net/http", "os", "os/exec",
		"os/signal", "os/user", "runtime", "syscall", "time",
	},
	"sandboxed": {"os", "os/exec", "syscall"},
}

// sandboxedTraversal are the path-walking functions a sandboxed method may not call.
//
// They live in `path/filepath`, which is otherwise pure path algebra: `filepath.Join` computes, while
// `filepath.Glob` and `filepath.WalkDir` resolve against the real filesystem from wherever the process happens to
// stand. The distinction is per function, so the package cannot simply be forbidden.
var sandboxedTraversal = map[string]bool{"Glob": true, "Walk": true, "WalkDir": true}

// inspectPackage reports the violations carried by one loaded package.
//
// Parameters:
//   - `pkg`: a package loaded with type information.
//
// Returns:
//   - `[]Violation`: violations found in this package.
func inspectPackage(pkg *packages.Package) []Violation {

	var violations []Violation

	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {

			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Doc == nil {
				continue
			}

			claims := parseClaims(fn.Doc.Text())
			if len(claims) == 0 {
				continue
			}

			method := pkg.Name + "." + fn.Name.Name
			violations = append(violations, inspectBody(pkg, fn, method, claims)...)
		}
	}

	return violations
}

// inspectBody reports every capability call in `fn` that one of `claims` forbids.
//
// Every selector is resolved through the package's type information, so a type conversion (`os.FileMode(mode)`),
// a constant (`os.O_CREATE`), and a sentinel (`os.ErrNotExist`) are all recognized as what they are and pass. Only
// an object that is genuinely a function counts as a reach.
//
// Parameters:
//   - `pkg`: the loaded package, for its type information and file set.
//   - `fn`: the claiming method.
//   - `method`: the method's display name.
//   - `claims`: the claims it carries.
//
// Returns:
//   - `[]Violation`: violations found in this body.
func inspectBody(pkg *packages.Package, fn *ast.FuncDecl, method string, claims []string) []Violation {

	var violations []Violation
	seen := map[string]bool{}

	ast.Inspect(fn.Body, func(node ast.Node) bool {

		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		function, ok := pkg.TypesInfo.Uses[selector.Sel].(*types.Func)
		if !ok || function.Pkg() == nil {
			// Not a function: a type, a constant, a sentinel, or a field. None of those reach anything.
			return true
		}

		reachedPkg := function.Pkg().Path()
		reach := reachedPkg + "." + function.Name()

		for _, claim := range claims {
			if !reaches(claim, reachedPkg, function.Name()) {
				continue
			}

			key := claim + " " + reach
			if seen[key] {
				continue
			}

			seen[key] = true
			violations = append(violations, Violation{
				Method:   method,
				Claim:    claim,
				Reach:    reach,
				Position: position(pkg.Fset, selector.Pos()),
			})
		}

		return true
	})

	return violations
}

// parseClaims extracts the claim names from a doc comment's `+devlore:claim=` directive.
//
// Parameters:
//   - `doc`: the method's doc comment text.
//
// Returns:
//   - `[]string`: the claim names, empty when the directive is absent.
func parseClaims(doc string) []string {

	for _, line := range strings.Split(doc, "\n") {

		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "+devlore:claim=") {
			continue
		}

		var claims []string
		for _, claim := range strings.Split(strings.TrimPrefix(line, "+devlore:claim="), ",") {
			if trimmed := strings.TrimSpace(claim); trimmed != "" {
				claims = append(claims, trimmed)
			}
		}

		return claims
	}

	return nil
}

// position renders a source position as the clickable `file.go:line` an author follows.
//
// Parameters:
//   - `fset`: the file set the position belongs to.
//   - `pos`: the position.
//
// Returns:
//   - `string`: the rendered position.
func position(fset *token.FileSet, pos token.Pos) string {

	p := fset.Position(pos)
	parts := strings.Split(p.Filename, "/")

	return fmt.Sprintf("%s:%d", parts[len(parts)-1], p.Line)
}

// reaches reports whether a call into `pkgPath`.`name` contradicts `claim`.
//
// Parameters:
//   - `claim`: the claim being tested.
//   - `pkgPath`: the called function's package path.
//   - `name`: the called function's name.
//
// Returns:
//   - `bool`: true when the call is forbidden to a method carrying this claim.
func reaches(claim, pkgPath, name string) bool {

	if claim == "sandboxed" && pkgPath == "path/filepath" && sandboxedTraversal[name] {
		return true
	}

	for _, forbiddenPkg := range forbidden[claim] {
		if pkgPath == forbiddenPkg {
			return true
		}
	}

	return false
}

// endregion
