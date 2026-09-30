// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package starlint

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.starlark.net/syntax"

	"github.com/NobleFactor/devlore-cli/cmd/internal/lorepackage"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// phaseEntryArity is the parameter count lore calls a phase entry point with: (package, phase).
const phaseEntryArity = 2

// region Types

// Finding is one problem in one file.
type Finding struct {

	// Rule is a stable kebab-case identifier, and the contract testdata/docker-package.want.tsv holds.
	Rule string

	// Path is the file, as it was given to the checker.
	Path string

	// Line is 1-based.
	Line int

	// Call is the call or construct at fault, for the reader to find on that line.
	Call string

	// Message says what is wrong, and where a near miss exists, what to write instead.
	Message string
}

// String renders a finding the way a linter prints one.
//
// Returns:
//   - `string`: "path:line: message (rule)".
func (f Finding) String() string {
	return fmt.Sprintf("%s:%d: %s (%s)", f.Path, f.Line, f.Message, f.Rule)
}

// Checker reports dead `plan.*` references and phase-script mistakes.
//
// Build one with [NewChecker].
//
// It holds both resolvers because a `.star` file is not one kind of thing. A package phase script runs in
// lore's hermetic planning runtime with the lifecycle verbs denied; a `star` extension command or a
// devlore-test data script runs in an ambient one where neither applies. The file's location decides which,
// by the same test that drives Requirements 3a and 3b.
type Checker struct {
	phaseScript *Resolver
	ambient     *Resolver
}

// endregion

// region Constructors

// NewChecker builds a Checker holding a resolver for each runtime.
//
// Returns:
//   - `*Checker`: ready to check.
func NewChecker() *Checker {
	return &Checker{phaseScript: NewPhaseScriptResolver(), ambient: NewAmbientResolver()}
}

// endregion

// region Behaviors

// CheckFile parses one `.star` file and reports its findings, sorted by line.
//
// A parse error is itself a finding rather than an error return: a corpus is checked as a whole, and one
// unparseable file must not stop the other 168.
//
// Parameters:
//   - `path`: the file to read, and the path findings carry.
//
// Returns:
//   - `[]Finding`: what is wrong, sorted by line then rule.
//   - `error`: non-nil only when the file cannot be read.
func (c *Checker) CheckFile(path string) ([]Finding, error) {

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("starlint: reading %s: %w", path, err)
	}

	return c.Check(path, data), nil
}

// Check reports the findings in already-read source.
//
// Parameters:
//   - `path`: the path findings carry, and which Requirements 3a and 3b are resolved against.
//   - `source`: the file's bytes.
//
// Returns:
//   - `[]Finding`: what is wrong, sorted by line then rule.
func (c *Checker) Check(path string, source []byte) []Finding {

	options := syntax.FileOptions{}

	file, err := options.Parse(path, source, 0)
	if err != nil {
		return []Finding{{
			Rule:    "parse-error",
			Path:    path,
			Line:    parseErrorLine(err),
			Call:    "",
			Message: err.Error(),
		}}
	}

	// A file inside an action directory is a package phase script and gets lore's runtime; anything else
	// gets an ambient one. Getting this wrong is not theoretical -- see [NewAmbientResolver].
	resolver := c.ambient
	if _, isPhaseScript := actionOf(path); isPhaseScript {
		resolver = c.phaseScript
	}

	findings := c.checkCalls(resolver, path, file)
	findings = append(findings, c.checkPhaseScript(path, file)...)

	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Line != findings[j].Line {
			return findings[i].Line < findings[j].Line
		}
		return findings[i].Rule < findings[j].Rule
	})

	return findings
}

// endregion

// region Private functions -- calls

// checkCalls reports every `plan.*` call that does not resolve, and every bad keyword on one that does.
//
// # Scope
//
// A `plan` bound in an enclosing function is not the builtin, and calls through it are not resolved. This is
// not hypothetical: `validate.star` and `extract.star` each bind `plan = {}` as a local dictionary and then
// write `plan[namespace].append(entry)`. A checker matching on the name alone reports them and fails its own
// zero-false-positive requirement on the day it lands.
//
// Starlark scopes as Python does -- a name assigned anywhere in a function is local to the whole function --
// so a binding is collected per function before its body is walked, never by line position. Note that
// `plan[k] = v` is an [syntax.IndexExpr] on the left and does NOT bind `plan`; it mutates it. Treating that
// as a binding would silently exempt every file that mutates the real plan.
//
// Parameters:
//   - `path`: the path findings carry.
//   - `file`: the parsed file.
//
// Returns:
//   - `[]Finding`: unresolved calls and rejected keywords.
func (c *Checker) checkCalls(resolver *Resolver, path string, file *syntax.File) []Finding {

	var findings []Finding

	// shadowed is a stack, one entry per enclosing function scope, true when that scope binds `plan`.
	shadowed := []bool{bindsPlan(file.Stmts)}

	// pushedScope parallels every node Walk descends into, so the matching exit can pop what it pushed.
	// Walk calls f(nil) after each node whose f returned true, and that nil is the only exit signal.
	var pushedScope []bool

	isShadowed := func() bool {
		for _, bound := range shadowed {
			if bound {
				return true
			}
		}
		return false
	}

	syntax.Walk(file, func(node syntax.Node) bool {

		if node == nil {

			last := len(pushedScope) - 1
			if last < 0 {
				return true
			}

			if pushedScope[last] {
				shadowed = shadowed[:len(shadowed)-1]
			}
			pushedScope = pushedScope[:last]

			return true
		}

		pushed := false

		switch typed := node.(type) {

		case *syntax.DefStmt:
			shadowed = append(shadowed, bindsPlan(typed.Body) || paramsBindPlan(typed.Params))
			pushed = true

		case *syntax.LambdaExpr:
			shadowed = append(shadowed, paramsBindPlan(typed.Params))
			pushed = true

		case *syntax.CallExpr:
			if !isShadowed() {
				findings = append(findings, c.checkCall(resolver, path, typed)...)
			}
		}

		pushedScope = append(pushedScope, pushed)

		return true
	})

	return findings
}

// checkCall resolves one call expression, when it is a `plan.*` shape at all.
//
// Parameters:
//   - `path`: the path findings carry.
//   - `call`: the call expression.
//
// Returns:
//   - `[]Finding`: zero, one resolution finding, or keyword findings.
func (c *Checker) checkCall(resolver *Resolver, path string, call *syntax.CallExpr) []Finding {

	namespace, method, ok := planCallShape(call.Fn)
	if !ok {
		return nil
	}

	line := int(call.Lparen.Line)

	written := "plan." + method
	if namespace != "" {
		written = "plan." + namespace + "." + method
	}

	resolution, resolved := resolver.Resolve(namespace, method)

	switch resolution {

	case UnknownNamespace:
		return []Finding{{
			Rule:    resolution.String(),
			Path:    path,
			Line:    line,
			Call:    written,
			Message: fmt.Sprintf("%s: plan has no %q namespace%s", written, namespace, nearest(namespace, resolver.Namespaces())),
		}}

	case UnknownMethod:
		var candidates []string
		if namespace == "" {
			candidates = resolver.Bare()
		} else if methods, found := resolver.namespaces[namespace]; found {
			candidates = sortedKeys(methods)
		}

		where := "plan"
		if namespace != "" {
			where = "plan." + namespace
		}

		return []Finding{{
			Rule:    resolution.String(),
			Path:    path,
			Line:    line,
			Call:    written,
			Message: fmt.Sprintf("%s: %s has no %q%s", written, where, method, nearest(method, candidates)),
		}}

	case Denied:
		return []Finding{{
			Rule:    resolution.String(),
			Path:    path,
			Line:    line,
			Call:    written,
			Message: fmt.Sprintf("%s: reserved for lore; a phase script contributes invocations and does not %s", written, method),
		}}

	case NotDeterministic:
		return []Finding{{
			Rule:    resolution.String(),
			Path:    path,
			Line:    line,
			Call:    written,
			Message: fmt.Sprintf("%s: unavailable while planning, which must produce the same graph on any machine", written),
		}}
	}

	return c.checkKeywords(resolver, path, call, written, resolved)
}

// checkKeywords reports keyword arguments a resolved method does not accept.
//
// Parameters:
//   - `path`: the path findings carry.
//   - `call`: the call expression.
//   - `written`: the call as the author wrote it, for the message.
//   - `method`: the resolved method.
//
// Returns:
//   - `[]Finding`: one per rejected keyword.
func (c *Checker) checkKeywords(resolver *Resolver, path string, call *syntax.CallExpr, written string, method *op.Method) []Finding {

	var findings []Finding

	for _, argument := range call.Args {

		binary, ok := argument.(*syntax.BinaryExpr)
		if !ok || binary.Op != syntax.EQ {
			continue
		}

		keyword, ok := binary.X.(*syntax.Ident)
		if !ok {
			continue
		}

		if resolver.AcceptsKeyword(method, keyword.Name) {
			continue
		}

		accepted := make([]string, 0, len(method.Parameters()))
		for _, parameter := range method.Parameters() {
			accepted = append(accepted, parameter.Name)
		}

		findings = append(findings, Finding{
			Rule:    "unknown-keyword",
			Path:    path,
			Line:    int(keyword.NamePos.Line),
			Call:    written,
			Message: fmt.Sprintf("%s: no parameter %q%s", written, keyword.Name, nearest(keyword.Name, accepted)),
		})
	}

	return findings
}

// endregion

// region Private functions -- phase scripts

// checkPhaseScript reports Requirements 3a and 3b, and does nothing for a file outside an action directory.
//
// 3b first, because it is the one nothing else can tell you. `cmd/lore/lore/builder.go` skips a phase with no
// actions without a word, so a file named for a phase its lifecycle does not have is never opened: nothing is
// logged and the step does not happen. 3a's mistake already fails at run time naming the function and the
// file, so the checker only moves that message earlier.
//
// Parameters:
//   - `path`: the file's path, whose parent directories carry the action and phase.
//   - `file`: the parsed file.
//
// Returns:
//   - `[]Finding`: zero, one, or both.
func (c *Checker) checkPhaseScript(path string, file *syntax.File) []Finding {

	action, ok := actionOf(path)
	if !ok {
		return nil
	}

	phase := strings.TrimSuffix(filepath.Base(path), ".star")
	order := lorepackage.PhaseOrder(action)

	var findings []Finding

	entry, found := topLevelFunction(file, phase)

	if !contains(order, phase) {

		// The defect is the file's NAME, which has no line of its own. The phase name does appear in the
		// source though -- at the entry point declared for it -- and that is where a reader has something to
		// look at. Line 1 only when the file never names the phase at all.
		line := 1
		if found {
			line = entry.Line
		}

		findings = append(findings, Finding{
			Rule: "phase-not-in-order",
			Path: path,
			Line: line,
			Call: phase,
			Message: fmt.Sprintf("%s is not %s %s phase, so this file is never opened; %s runs %s",
				phase, article(string(action)), action, action, strings.Join(order, ", ")),
		})
	}

	switch {
	case !found:
		findings = append(findings, Finding{
			Rule:    "missing-phase-entry-point",
			Path:    path,
			Line:    1,
			Call:    "def " + phase,
			Message: fmt.Sprintf("no top-level def %s(package, phase); lore looks the entry point up by the phase name", phase),
		})

	case entry.Parameters != phaseEntryArity:
		findings = append(findings, Finding{
			Rule:    "phase-entry-point-arity",
			Path:    path,
			Line:    entry.Line,
			Call:    "def " + phase,
			Message: fmt.Sprintf("def %s takes %d parameters; lore calls it with (package, phase)", phase, entry.Parameters),
		})
	}

	return findings
}

// endregion

// region Private functions -- syntax helpers

// planCallShape decomposes a call's function expression into the `plan.*` shape it is, if any.
//
// Two shapes reach a provider, and a deeper chain is neither:
//
//	plan.method(...)            -> ("", "method", true)
//	plan.namespace.method(...)  -> ("namespace", "method", true)
//
// Parameters:
//   - `fn`: the call expression's function.
//
// Returns:
//   - `string`: the namespace, or "" for a call directly on `plan`.
//   - `string`: the method.
//   - `bool`: true when this is a `plan.*` call at all.
func planCallShape(fn syntax.Expr) (namespace, method string, isPlanCall bool) {

	dot, ok := fn.(*syntax.DotExpr)
	if !ok {
		return "", "", false
	}

	switch receiver := dot.X.(type) {

	case *syntax.Ident:
		if receiver.Name != planReceiver {
			return "", "", false
		}
		return "", dot.Name.Name, true

	case *syntax.DotExpr:
		root, ok := receiver.X.(*syntax.Ident)
		if !ok || root.Name != planReceiver {
			return "", "", false
		}
		return receiver.Name.Name, dot.Name.Name, true
	}

	return "", "", false
}

// bindsPlan reports whether any statement binds the name `plan` in the scope those statements belong to.
//
// It does not descend into a nested [syntax.DefStmt] or [syntax.LambdaExpr] body: those are their own scopes,
// and a binding there does not shadow `plan` for the statements around them.
//
// Parameters:
//   - `stmts`: the statements of one scope.
//
// Returns:
//   - `bool`: true when `plan` is bound here.
func bindsPlan(stmts []syntax.Stmt) bool {

	for _, stmt := range stmts {
		if statementBindsPlan(stmt) {
			return true
		}
	}

	return false
}

// statementBindsPlan reports whether one statement binds `plan` in its own scope.
//
// Split out of [bindsPlan] so each statement kind reads on its own; the combined switch and loop measured 35
// cognitive complexity against a limit of 20.
//
// Parameters:
//   - `stmt`: the statement to examine.
//
// Returns:
//   - `bool`: true when this statement binds `plan`.
func statementBindsPlan(stmt syntax.Stmt) bool {

	switch typed := stmt.(type) {

	case *syntax.AssignStmt:
		// An IndexExpr or DotExpr on the left mutates its receiver and binds nothing, which is exactly what
		// `plan[namespace] = []` does. Only a bare name, or a name inside an unpacking target, binds.
		return targetBindsPlan(typed.LHS)

	case *syntax.ForStmt:
		return targetBindsPlan(typed.Vars) || bindsPlan(typed.Body)

	case *syntax.WhileStmt:
		return bindsPlan(typed.Body)

	case *syntax.IfStmt:
		return bindsPlan(typed.True) || bindsPlan(typed.False)

	case *syntax.DefStmt:
		// The nested function's NAME is bound here, though its body is not this scope's business.
		return typed.Name != nil && typed.Name.Name == planReceiver

	case *syntax.LoadStmt:
		for _, ident := range typed.To {
			if ident != nil && ident.Name == planReceiver {
				return true
			}
		}
	}

	return false
}

// paramsBindPlan reports whether a parameter list binds `plan`.
//
// A parameter is `ident`, `ident=expr`, `*`, `*ident` or `**ident`, so the name sits at a different depth in
// each form.
//
// Parameters:
//   - `params`: the parameter list.
//
// Returns:
//   - `bool`: true when a parameter is named `plan`.
func paramsBindPlan(params []syntax.Expr) bool {

	for _, param := range params {

		switch typed := param.(type) {

		case *syntax.Ident:
			if typed.Name == planReceiver {
				return true
			}

		case *syntax.BinaryExpr: // ident=expr
			if ident, ok := typed.X.(*syntax.Ident); ok && ident.Name == planReceiver {
				return true
			}

		case *syntax.UnaryExpr: // *ident or **ident
			if ident, ok := typed.X.(*syntax.Ident); ok && ident.Name == planReceiver {
				return true
			}
		}
	}

	return false
}

// targetBindsPlan reports whether an assignment target binds `plan`, descending through unpacking.
//
// Parameters:
//   - `target`: the assignment target.
//
// Returns:
//   - `bool`: true when `plan` is one of the names bound.
func targetBindsPlan(target syntax.Expr) bool {

	switch typed := target.(type) {

	case *syntax.Ident:
		return typed.Name == planReceiver

	case *syntax.TupleExpr:
		for _, element := range typed.List {
			if targetBindsPlan(element) {
				return true
			}
		}

	case *syntax.ListExpr:
		for _, element := range typed.List {
			if targetBindsPlan(element) {
				return true
			}
		}

	case *syntax.ParenExpr:
		return targetBindsPlan(typed.X)
	}

	return false
}

// entryPoint is a top-level function's shape, enough to judge it by.
type entryPoint struct {
	Line       int
	Parameters int
}

// topLevelFunction finds a top-level `def` by name.
//
// Parameters:
//   - `file`: the parsed file.
//   - `name`: the function name to find.
//
// Returns:
//   - `entryPoint`: its line and parameter count, zero when absent.
//   - `bool`: true when a top-level def of that name exists.
func topLevelFunction(file *syntax.File, name string) (entryPoint, bool) {

	for _, stmt := range file.Stmts {
		if def, ok := stmt.(*syntax.DefStmt); ok && def.Name != nil && def.Name.Name == name {
			return entryPoint{Line: int(def.Def.Line), Parameters: len(def.Params)}, true
		}
	}

	return entryPoint{}, false
}

// actionOf reads the lifecycle action from a phase script's parent directory.
//
// A package phase script is `<platform>/<Action>/<phase>.star`, so the action is the parent directory's name.
// A file anywhere else is not a phase script and neither requirement applies to it.
//
// Parameters:
//   - `path`: the script's path.
//
// Returns:
//   - `lorepackage.Action`: the action.
//   - `bool`: true when the path is inside a recognized action directory.
func actionOf(path string) (lorepackage.Action, bool) {

	parent := filepath.Base(filepath.Dir(filepath.ToSlash(path)))

	for _, action := range []lorepackage.Action{
		lorepackage.Deploy, lorepackage.Upgrade, lorepackage.Decommission, lorepackage.Reconcile,
	} {
		if string(action) == parent {
			return action, true
		}
	}

	return "", false
}

// article returns "a" or "an" for a word, so a message reads as English.
//
// The action names are a closed set -- Deploy, Upgrade, Decommission, Reconcile -- so a vowel test is
// sufficient and there is no need for the exceptions a general rule would want.
//
// Parameters:
//   - `word`: the word the article precedes.
//
// Returns:
//   - `string`: "an" before a vowel, else "a".
func article(word string) string {

	if word == "" {
		return "a"
	}

	if strings.ContainsRune("AEIOUaeiou", rune(word[0])) {
		return "an"
	}

	return "a"
}

// contains reports whether a slice holds a value.
//
// Parameters:
//   - `values`: the slice to search.
//   - `want`: the value to find.
//
// Returns:
//   - `bool`: true when found.
func contains(values []string, want string) bool {

	for _, value := range values {
		if value == want {
			return true
		}
	}

	return false
}

// nearest suggests the closest candidate to a misspelled name, as " (did you mean %q?)" or "".
//
// A near miss is most of the value in a finding: `plan.file.write` against `write_text`, and
// `plan.package.install` against `pkg`, are both one suggestion away from being fixed without a doc search.
// Only a genuinely close candidate is offered -- a bad suggestion is worse than none.
//
// Parameters:
//   - `name`: the name as written.
//   - `candidates`: the names it could have been.
//
// Returns:
//   - `string`: a parenthetical suggestion, or "".
func nearest(name string, candidates []string) string {

	best, bestDistance := "", 1<<31-1

	limit := len(name) / 2
	if limit < 2 {
		limit = 2
	}

	for _, candidate := range candidates {

		distance := editDistance(name, candidate)
		if distance < bestDistance {
			best, bestDistance = candidate, distance
		}
	}

	if best == "" || bestDistance > limit {
		return ""
	}

	return fmt.Sprintf(" (did you mean %q?)", best)
}

// editDistance is Levenshtein distance between two short identifiers.
//
// Parameters:
//   - `a`: one identifier.
//   - `b`: the other.
//
// Returns:
//   - `int`: the number of single-character edits between them.
func editDistance(a, b string) int {

	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)

	for j := range previous {
		previous[j] = j
	}

	for i := 1; i <= len(a); i++ {

		current[0] = i

		for j := 1; j <= len(b); j++ {

			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}

			current[j] = min(previous[j]+1, min(current[j-1]+1, previous[j-1]+cost))
		}

		previous, current = current, previous
	}

	return previous[len(b)]
}

// parseErrorLine digs the line out of a [syntax.Error], falling back to 1.
//
// Parameters:
//   - `err`: the parse error.
//
// Returns:
//   - `int`: the 1-based line the parser objected to.
func parseErrorLine(err error) int {

	var syntaxError syntax.Error
	if errors.As(err, &syntaxError) {
		return int(syntaxError.Pos.Line)
	}

	return 1
}

// endregion
