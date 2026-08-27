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
	"os"
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
	Via      string // the in-module hop that led there, empty when the claiming method reaches it directly
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

	if v.Via != "" {
		return fmt.Sprintf("method %s: claims %s but reaches %s at %s, through %s",
			v.Method, v.Claim, v.Reach, v.Position, v.Via)
	}

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

	return CheckGOOS("", patterns...)
}

// CheckGOOS is [Check] against a chosen target platform.
//
// A claim can hold on one platform and fail on another: build tags select different bodies, so the call graph a
// method reaches is not the same everywhere. CI covers the spread across its matrix — each leg checks its own
// host, and the legs span darwin, linux, and windows between them. Passing an explicit goos is for sweeping the
// others from one machine; see platformsUnderTest in the test.
//
// Parameters:
//   - `goos`: the target platform, e.g. "linux"; empty loads for the host.
//   - `patterns`: go package patterns.
//
// Returns:
//   - `[]Violation`: the violations found for that platform.
//   - `error`: non-nil when loading fails or a package carries type errors.
func CheckGOOS(goos string, patterns ...string) ([]Violation, error) {

	mode := packages.NeedName | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo |
		packages.NeedDeps | packages.NeedImports | packages.NeedFiles

	config := &packages.Config{Mode: mode, Tests: false}
	if goos != "" {
		config.Env = append(os.Environ(), "GOOS="+goos)
	}

	loaded, err := packages.Load(config, patterns...)
	if err != nil {
		return nil, fmt.Errorf("claimcheck: load %v for GOOS=%q: %w", patterns, goos, err)
	}

	var violations []Violation

	bodies := map[*types.Func]declaration{}

	for _, pkg := range loaded {
		if len(pkg.Errors) > 0 {
			return nil, fmt.Errorf("claimcheck: %s: %w", pkg.PkgPath, pkg.Errors[0])
		}

		indexBodies(pkg, bodies)
	}

	for _, pkg := range loaded {
		violations = append(violations, inspectPackage(pkg, bodies)...)
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

// trustBoundaries are the packages propagation stops at.
//
// Each is asserted by hand, once, in the package itself. Analyzing through either would have the check contradict
// the design rather than test it.
//
//   - `pkg/fsroot` IS the sandbox. Confinement is the kernel's, via os.OpenRoot, so its own body necessarily
//     calls the capability it provides; walking through it reports every sandboxed method as escaping itself.
//   - `pkg/assert` reaches runtime.Callers only to render a stack frame while raising. That call sits on the
//     abort path of a failed assertion — a program-construction error that panics — never on a path a caller's
//     result depends on. Statically it is reachable from anything that asserts, which is nearly everything;
//     dynamically it runs only as the process is dying.
var trustBoundaries = map[string]bool{
	"github.com/NobleFactor/devlore-cli/pkg/assert": true,
	"github.com/NobleFactor/devlore-cli/pkg/fsroot": true,
}

// inspectPackage reports the violations carried by one loaded package.
//
// Parameters:
//   - `pkg`: a package loaded with type information.
//
// Returns:
//   - `[]Violation`: violations found in this package.
func inspectPackage(pkg *packages.Package, bodies map[*types.Func]declaration) []Violation {

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
			violations = append(violations, inspectBody(pkg, fn, method, claims, bodies)...)
		}
	}

	return violations
}

// inspectBody reports every capability call `fn` reaches that one of `claims` forbids.
//
// Parameters:
//   - `pkg`: the loaded package, for its type information and file set.
//   - `fn`: the claiming method.
//   - `method`: the method's display name.
//   - `claims`: the claims it carries.
//   - `bodies`: the in-module body index, for following direct calls.
//
// Returns:
//   - `[]Violation`: violations found from this method, directly or through an in-module hop.
func inspectBody(
	pkg *packages.Package,
	fn *ast.FuncDecl,
	method string,
	claims []string,
	bodies map[*types.Func]declaration,
) []Violation {

	w := &walker{
		method: method,
		claims: claims,
		bodies: bodies,
		seen:   map[string]bool{},
		walked: map[*types.Func]bool{},
	}

	w.walk(declaration{body: fn.Body, info: pkg.TypesInfo, fset: pkg.Fset}, "")

	return w.found
}

// walker carries the state of one claiming method's traversal.
type walker struct {
	method string
	claims []string
	bodies map[*types.Func]declaration
	seen   map[string]bool
	walked map[*types.Func]bool
	found  []Violation
}

// walk inspects one body, recording forbidden reaches and following direct calls into in-module callees.
//
// Interface dispatch is NOT followed and cannot be: `action.Do(...)` resolves to an interface method whose
// implementation dispatches by reflection, so no static walk reaches what it runs. Propagation therefore catches
// a helper that reaches a capability, and never a capability reached through dispatch — which is why a claim
// stays an assertion the author is accountable for rather than a proof.
//
// Parameters:
//   - `from`: the body to inspect, with its type information and file set.
//   - `via`: the hop chain that led here, empty at the claiming method itself.
func (w *walker) walk(from declaration, via string) {

	// Identifiers, not selectors. A qualified call reaches its function through a selector whose .Sel is an
	// identifier, and a same-package call is a bare identifier with no selector at all -- so inspecting selectors
	// saw `p.lstat(...)` and was blind to `helper()`, which is how most of a provider's own helpers are called.
	// Resolving identifiers covers both, and the type check discards the rest: a package qualifier resolves to a
	// PkgName, a field to a Var, a type to a TypeName.
	ast.Inspect(from.body, func(node ast.Node) bool {

		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}

		function, ok := from.info.Uses[identifier].(*types.Func)
		if !ok || function.Pkg() == nil {
			// Not a function at all: a type, a constant, a sentinel, a field, or a package qualifier.
			return true
		}

		// Recording and following ask different questions, so the method test gates only one of them. A method on
		// a foreign value is not an acquisition -- f.Stat() on an *os.File obtained through the root is already
		// confined -- but a method of OURS is exactly what propagation must walk, since p.lstat(...) and
		// p.newTracker(...) are how a provider reaches most of what it reaches.
		if !isMethod(function) {
			w.record(function, identifier, from.fset, via)
		}

		w.follow(function, via)

		return true
	})
}

// record notes every claim that `function` contradicts.
//
// Parameters:
//   - `function`: the resolved callee.
//   - `at`: the identifier naming it, for the position.
//   - `fset`: the file set the call belongs to.
//   - `via`: the hop chain that led here.
func (w *walker) record(function *types.Func, at *ast.Ident, fset *token.FileSet, via string) {

	reachedPkg := function.Pkg().Path()
	reach := reachedPkg + "." + function.Name()

	for _, claim := range w.claims {

		key := claim + " " + reach + " " + via
		if w.seen[key] || !reaches(claim, reachedPkg, function.Name()) {
			continue
		}

		w.seen[key] = true
		w.found = append(w.found, Violation{
			Method:   w.method,
			Claim:    claim,
			Reach:    reach,
			Position: position(fset, at.Pos()),
			Via:      via,
		})
	}
}

// follow walks into `function` when its body is ours to read and has not been walked already.
//
// Parameters:
//   - `function`: the resolved callee.
//   - `via`: the hop chain that led here.
func (w *walker) follow(function *types.Func, via string) {

	// A declared trust boundary is never walked through; [trustBoundaries] records why each one is there.
	if trustBoundaries[function.Pkg().Path()] || w.walked[function] {
		return
	}

	callee, known := w.bodies[function]
	if !known {
		return
	}

	w.walked[function] = true

	hop := function.Pkg().Name() + "." + function.Name()
	if via != "" {
		hop = via + " -> " + hop
	}

	w.walk(callee, hop)
}

// isMethod reports whether `function` is a method on a value rather than a package-level function.
//
// A method on a value is not an acquisition. `f.Stat()` on an *os.File obtained through the root is confined
// already -- what would escape is OBTAINING the handle unsandboxed, which is a package-level call like os.Open.
//
// Parameters:
//   - `function`: the resolved callee.
//
// Returns:
//   - `bool`: true when the callee declares a receiver.
func isMethod(function *types.Func) bool {

	signature, ok := function.Type().(*types.Signature)

	return ok && signature.Recv() != nil
}

// declaration is an in-module function body, with the type information and file set needed to read it.
type declaration struct {
	body *ast.BlockStmt
	info *types.Info
	fset *token.FileSet
}

// indexBodies records every in-module function body against its type object, so a claiming method's callees can
// be walked.
//
// Parameters:
//   - `pkg`: a loaded package.
//   - `into`: the index to populate.
func indexBodies(pkg *packages.Package, into map[*types.Func]declaration) {

	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {

			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}

			if object, ok := pkg.TypesInfo.Defs[fn.Name].(*types.Func); ok {
				into[object] = declaration{body: fn.Body, info: pkg.TypesInfo, fset: pkg.Fset}
			}
		}
	}
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
