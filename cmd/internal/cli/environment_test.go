// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/devlore-test/devloretest"
	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/lore/lore"
	"github.com/NobleFactor/devlore-cli/cmd/star/star"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"
	"github.com/NobleFactor/devlore-cli/schema"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// suite is the program name the suite's global settings take: their variables are `DEVLORE_<KEY>`.
const suite = "devlore"

var (
	// laneTwentyFourPairs are the settings that share a variable until lane 24 (#1010) makes the model settings global.
	// lore reads `lore.model.*` through viper, and its `--model-*` flags bind `lore.model-*`. Lane 24 removes both
	// sides and deletes this table; the test fails while an entry no longer holds.
	laneTwentyFourPairs = map[string][]string{
		"LORE_MODEL_API_KEY":  {"lore.model-api-key", "lore.model.api_key"},
		"LORE_MODEL_ENDPOINT": {"lore.model-endpoint", "lore.model.endpoint"},
		"LORE_MODEL_PROVIDER": {"lore.model-provider", "lore.model.provider"},
	}

	// programs are the programs on the shared root, the ones that read their settings through viper.
	programs = []string{"devlore-test", "lore", "star", "writ"}
)

// nameRead is an environment variable the code reads by name.
type nameRead struct {
	name   string // the variable, such as "DEVLORE_PAGER"
	origin string // the file and line that reads it
}

// schemaNode is an object of the configuration schema: a section whose properties are settings, or a setting.
type schemaNode struct {
	Properties map[string]schemaNode `json:"properties"` // the section's settings; none for a setting
}

// setting is a setting the suite knows, and where the test found it.
type setting struct {
	key     string // the key, lower-cased as viper keys it, such as "writ.repo"
	program string // the program whose prefix its variable takes, or [suite] for a global setting
	origin  string // where the test found it: a flag, the schema, or a file and line
}

// variable returns the environment variable the setting is read from.
//
// Returns:
//   - `string`: the variable, as [cli.EnvironmentVariable] names it.
func (s setting) variable() string {
	return cli.EnvironmentVariable(s.program, s.key)
}

// calleeOf returns the package and the function a call names, such as "viper" and "GetString".
//
// Parameters:
//   - `call`: the call.
//
// Returns:
//   - `packageName`: the package's identifier, or "" when the call names no package's function.
//   - `functionName`: the function's name, or "".
func calleeOf(call *ast.CallExpr) (packageName, functionName string) {

	selector, isSelector := call.Fun.(*ast.SelectorExpr)
	if !isSelector {
		return "", ""
	}
	identifier, isIdentifier := selector.X.(*ast.Ident)
	if !isIdentifier {
		return "", ""
	}
	return identifier.Name, selector.Sel.Name
}

// describe lists the keys that share a variable, each with where the test found it.
//
// Parameters:
//   - `keys`: the keys, each mapped to where it was found first.
//
// Returns:
//   - `string`: the keys in order, comma-separated.
func describe(keys map[string]string) string {

	parts := make([]string, 0, len(keys))
	for _, key := range slices.Sorted(maps.Keys(keys)) {
		parts = append(parts, key+" ("+keys[key]+")")
	}
	return strings.Join(parts, ", ")
}

// documentedSettings returns the settings at and under `key`: the key itself when its node has no properties, else
// each property's.
//
// Parameters:
//   - `key`: the node's key, such as "writ" or "writ.vars".
//   - `node`: the node.
//   - `program`: the program the settings belong to, or [suite].
//
// Returns:
//   - `[]setting`: the settings.
func documentedSettings(key string, node schemaNode, program string) []setting {

	if len(node.Properties) == 0 {
		return []setting{{key: strings.ToLower(key), program: program, origin: "the schema"}}
	}

	var settings []setting
	for name, child := range node.Properties {
		settings = append(settings, documentedSettings(key+"."+name, child, program)...)
	}
	return settings
}

// flagSettings returns each program's persistent flags as the settings [cli.BindFlags] binds them to.
//
// The programs' roots are built as devlore-docs builds them, in a sandboxed home, so star loads no extension of the
// developer's.
//
// Parameters:
//   - `t`: the test; its environment is sandboxed for the rest of it.
//
// Returns:
//   - `[]setting`: one setting for each program's persistent flag.
func flagSettings(t *testing.T) []setting {

	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for variable, directory := range map[string]string{
		"XDG_CACHE_HOME": ".cache", "XDG_CONFIG_HOME": ".config", "XDG_DATA_HOME": ".local/share",
		"XDG_STATE_HOME": ".local/state",
	} {
		t.Setenv(variable, filepath.Join(home, filepath.FromSlash(directory)))
	}

	starRoot, starSession := star.NewRootCmd()
	t.Cleanup(func() {
		if err := starSession.Close(); err != nil {
			t.Errorf("close star's session: %v", err)
		}
	})

	roots := map[string]*cobra.Command{
		"devlore-test": devloretest.NewRootCmd(),
		"lore":         lore.NewRootCmd(),
		"star":         starRoot,
		"writ":         writ.NewRootCmd(),
	}

	var settings []setting
	for _, program := range programs {
		roots[program].PersistentFlags().VisitAll(func(persistentFlag *pflag.Flag) {
			settings = append(settings, setting{
				key:     strings.ToLower(program + "." + persistentFlag.Name),
				program: program,
				origin:  "the --" + persistentFlag.Name + " flag",
			})
		})
	}
	return settings
}

// knownSettings returns every setting the suite knows, and the variables its code reads by name.
//
// The settings are each program's persistent flags, the settings the schema documents, and the keys the code reads
// through viper; a setting found in several places is listed once for each.
//
// Parameters:
//   - `t`: the test; it fails on a viper key the source scan cannot read.
//
// Returns:
//   - `[]setting`: the settings.
//   - `[]nameRead`: the variables the code reads by name.
func knownSettings(t *testing.T) ([]setting, []nameRead) {

	t.Helper()

	settings := flagSettings(t)
	settings = append(settings, schemaSettings(t)...)
	keys, names := scanSource(t)
	return append(settings, keys...), names
}

// literalOf resolves `expression` to a string: a literal, a constant of its package, or a local variable holding one.
//
// Parameters:
//   - `expression`: the expression.
//   - `constants`: the package's string constants, by name; may be nil.
//   - `locals`: what the enclosing function assigns to each local variable; may be nil.
//
// Returns:
//   - `string`: the string.
//   - `bool`: true when the expression resolves.
func literalOf(expression ast.Expr, constants map[string]string, locals map[string]ast.Expr) (string, bool) {

	switch node := expression.(type) {
	case *ast.BasicLit:
		if node.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(node.Value)
		return value, err == nil
	case *ast.Ident:
		if value, isConstant := constants[node.Name]; isConstant {
			return value, true
		}
		if assigned, isLocal := locals[node.Name]; isLocal {
			return literalOf(assigned, constants, nil)
		}
	}
	return "", false
}

// localAssignments returns what a function body assigns to each local variable, the last assignment winning.
//
// Parameters:
//   - `body`: the function's body.
//
// Returns:
//   - `map[string]ast.Expr`: the assigned expressions, by variable.
func localAssignments(body *ast.BlockStmt) map[string]ast.Expr {

	locals := map[string]ast.Expr{}
	ast.Inspect(body, func(node ast.Node) bool {
		assignment, isAssignment := node.(*ast.AssignStmt)
		if !isAssignment || len(assignment.Lhs) != len(assignment.Rhs) {
			return true
		}
		for index, target := range assignment.Lhs {
			if identifier, isIdentifier := target.(*ast.Ident); isIdentifier {
				locals[identifier.Name] = assignment.Rhs[index]
			}
		}
		return true
	})
	return locals
}

// moduleRoot returns the module's root, three directories above this package.
//
// Parameters:
//   - `t`: the test; it fails when no go.mod is there.
//
// Returns:
//   - `string`: the root's absolute path.
func moduleRoot(t *testing.T) string {

	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("no go.mod at %s: %v", root, err)
	}
	return root
}

// parseModule parses every non-test Go file under the module's `cmd`, `internal` and `pkg` directories.
//
// Parameters:
//   - `t`: the test; it fails when a file cannot be read or parsed.
//   - `root`: the module's root.
//   - `fileSet`: the file set the positions are recorded in.
//
// Returns:
//   - `map[string][]*ast.File`: the files, by package directory.
func parseModule(t *testing.T, root string, fileSet *token.FileSet) map[string][]*ast.File {

	t.Helper()

	packages := map[string][]*ast.File{}
	for _, top := range []string{"cmd", "internal", "pkg"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(path string, entry fs.DirEntry, err error) error {
			switch {
			case err != nil:
				return err
			case entry.IsDir() && entry.Name() == "testdata":
				return filepath.SkipDir
			case entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go"):
				return nil
			}
			file, err := parser.ParseFile(fileSet, path, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			packages[filepath.Dir(path)] = append(packages[filepath.Dir(path)], file)
			return nil
		})
		if err != nil {
			t.Fatalf("read the source under %s: %v", top, err)
		}
	}
	return packages
}

// positionOf returns where `node` is: its file, relative to the module's root, and its line.
//
// Parameters:
//   - `root`: the module's root.
//   - `fileSet`: the file set the node's position is recorded in.
//   - `node`: the node.
//
// Returns:
//   - `string`: the position, such as "cmd/internal/cli/root.go:195".
func positionOf(root string, fileSet *token.FileSet, node ast.Node) string {

	position := fileSet.Position(node.Pos())
	relative, err := filepath.Rel(root, position.Filename)
	if err != nil {
		relative = position.Filename
	}
	return filepath.ToSlash(relative) + ":" + strconv.Itoa(position.Line)
}

// readsAKey reports whether a viper function takes a setting's key first: each Get function but GetViper, and
// BindEnv, InConfig, IsSet, SetDefault and Sub.
//
// Parameters:
//   - `function`: the function's name.
//
// Returns:
//   - `bool`: true when its first argument is a key.
func readsAKey(function string) bool {

	if strings.HasPrefix(function, "Get") {
		return function != "GetViper"
	}
	return slices.Contains([]string{"BindEnv", "InConfig", "IsSet", "SetDefault", "Sub"}, function)
}

// scanSource reads every non-test Go file in the module for the keys the code reads through viper and the variables
// it reads by name.
//
// A viper key is a literal, a constant of the file's package, a local variable holding one, or a template: an
// expression plus a literal that begins with "." (`name + ".verbose"`), which each program reads under its own name.
// A key in any other form fails the test, which cannot know what it names. A variable read by `os.Getenv` or
// `os.LookupEnv` counts when its name resolves to a literal the same way; a computed name is left alone.
//
// Parameters:
//   - `t`: the test.
//
// Returns:
//   - `[]setting`: the keys the code reads through viper.
//   - `[]nameRead`: the variables the code reads by name.
func scanSource(t *testing.T) ([]setting, []nameRead) {

	t.Helper()

	root := moduleRoot(t)
	fileSet := token.NewFileSet()

	var keys []setting
	var names []nameRead
	for _, files := range parseModule(t, root, fileSet) {
		constants := stringConstants(files)
		for _, file := range files {
			for _, declaration := range file.Decls {
				function, isFunction := declaration.(*ast.FuncDecl)
				if !isFunction || function.Body == nil {
					continue
				}
				locals := localAssignments(function.Body)
				ast.Inspect(function.Body, func(node ast.Node) bool {
					call, isCall := node.(*ast.CallExpr)
					if !isCall || len(call.Args) == 0 {
						return true
					}
					origin := positionOf(root, fileSet, call)
					switch packageName, functionName := calleeOf(call); {
					case packageName == "viper" && readsAKey(functionName):
						keys = append(keys, viperKeys(t, call.Args[0], constants, locals, origin)...)
					case packageName == "os" && (functionName == "Getenv" || functionName == "LookupEnv"):
						if name, resolved := literalOf(call.Args[0], constants, locals); resolved {
							names = append(names, nameRead{name: name, origin: origin})
						}
					}
					return true
				})
			}
		}
	}
	return keys, names
}

// schemaSettings returns the settings the configuration schema documents: a program's section under that program,
// and the rest as the suite's global settings.
//
// Parameters:
//   - `t`: the test; it fails when the schema cannot be decoded.
//
// Returns:
//   - `[]setting`: the documented settings.
func schemaSettings(t *testing.T) []setting {

	t.Helper()

	var document schemaNode
	if err := json.Unmarshal(schema.DevloreSchema, &document); err != nil {
		t.Fatalf("decode the configuration schema: %v", err)
	}

	var settings []setting
	for name, node := range document.Properties {
		program := suite
		if slices.Contains(programs, name) {
			program = name
		}
		settings = append(settings, documentedSettings(name, node, program)...)
	}
	return settings
}

// stringConstants returns a package's package-level string constants, by name.
//
// Parameters:
//   - `files`: the package's files.
//
// Returns:
//   - `map[string]string`: the constants' values.
func stringConstants(files []*ast.File) map[string]string {

	constants := map[string]string{}
	for _, file := range files {
		for _, declaration := range file.Decls {
			general, isGeneral := declaration.(*ast.GenDecl)
			if !isGeneral || general.Tok != token.CONST {
				continue
			}
			for _, specification := range general.Specs {
				value, isValue := specification.(*ast.ValueSpec)
				if !isValue {
					continue
				}
				for index, name := range value.Names {
					if index >= len(value.Values) {
						continue
					}
					if literal, resolved := literalOf(value.Values[index], nil, nil); resolved {
						constants[name.Name] = literal
					}
				}
			}
		}
	}
	return constants
}

// templateOf reports whether `expression` is a program-name template, an expression plus a literal that begins with
// ".", directly or through a local variable, and returns the literal.
//
// Parameters:
//   - `expression`: the expression.
//   - `locals`: what the enclosing function assigns to each local variable; may be nil.
//
// Returns:
//   - `string`: the literal, such as ".verbose".
//   - `bool`: true when the expression is a template.
func templateOf(expression ast.Expr, locals map[string]ast.Expr) (string, bool) {

	if identifier, isIdentifier := expression.(*ast.Ident); isIdentifier {
		if assigned, isLocal := locals[identifier.Name]; isLocal {
			return templateOf(assigned, nil)
		}
		return "", false
	}

	sum, isSum := expression.(*ast.BinaryExpr)
	if !isSum || sum.Op != token.ADD {
		return "", false
	}
	suffix, resolved := literalOf(sum.Y, nil, nil)
	if !resolved || !strings.HasPrefix(suffix, ".") {
		return "", false
	}
	return suffix, true
}

// viperKeys returns the settings a viper call's key argument names.
//
// A template names its key under each program. A key in a program's section belongs to that program; any other key
// is read by the code every program shares, under each program's prefix.
//
// Parameters:
//   - `t`: the test; it fails when the argument cannot be read.
//   - `argument`: the call's key argument.
//   - `constants`: the package's string constants, by name.
//   - `locals`: what the enclosing function assigns to each local variable.
//   - `origin`: where the call is.
//
// Returns:
//   - `[]setting`: the settings the key names.
func viperKeys(
	t *testing.T,
	argument ast.Expr,
	constants map[string]string,
	locals map[string]ast.Expr,
	origin string,
) []setting {

	t.Helper()

	if suffix, isTemplate := templateOf(argument, locals); isTemplate {
		settings := make([]setting, 0, len(programs))
		for _, program := range programs {
			key := strings.ToLower(program + suffix)
			settings = append(settings, setting{key: key, program: program, origin: origin})
		}
		return settings
	}

	key, resolved := literalOf(argument, constants, locals)
	if !resolved {
		t.Errorf("%s: a viper key the test cannot read; make it a literal, a constant, or a program-name template",
			origin)
		return nil
	}

	key = strings.ToLower(key)
	if program, _, isSectioned := strings.Cut(key, "."); isSectioned && slices.Contains(programs, program) {
		return []setting{{key: key, program: program, origin: origin}}
	}

	settings := make([]setting, 0, len(programs))
	for _, program := range programs {
		settings = append(settings, setting{key: key, program: program, origin: origin})
	}
	return settings
}

// --- EnvironmentVariable ---

// TestEnvironmentVariable_NoTwoSettingsShareOne maps every setting the suite knows to its variable, and fails when two
// settings share one, since each would read the other's value.
func TestEnvironmentVariable_NoTwoSettingsShareOne(t *testing.T) {

	settings, _ := knownSettings(t)

	claimants := map[string]map[string]string{}
	for _, known := range settings {
		variable := known.variable()
		if claimants[variable] == nil {
			claimants[variable] = map[string]string{}
		}
		if _, isClaimed := claimants[variable][known.key]; !isClaimed {
			claimants[variable][known.key] = known.origin
		}
	}

	for variable, keys := range claimants {
		if len(keys) > 1 && !slices.Equal(slices.Sorted(maps.Keys(keys)), laneTwentyFourPairs[variable]) {
			t.Errorf("%s is the variable of %d settings: %s", variable, len(keys), describe(keys))
		}
	}

	for variable, pair := range laneTwentyFourPairs {
		if !slices.Equal(slices.Sorted(maps.Keys(claimants[variable])), pair) {
			t.Errorf("%s no longer names %s alone: lane 24 (#1010) deletes its exception", variable,
				strings.Join(pair, " and "))
		}
	}
}

// TestEnvironmentVariable_NoSettingFallsInAReservedFamily fails when a setting's variable lies in a family another
// reader owns, a program's variables family (#1023) or writ's segments, or when the code reads a variables family's
// member by name, which only the variable resolver reads.
func TestEnvironmentVariable_NoSettingFallsInAReservedFamily(t *testing.T) {

	settings, names := knownSettings(t)

	variablesFamilies := map[string]string{}
	for _, program := range programs {
		variablesFamilies[cli.EnvironmentPrefix(program)+"_VARIABLE_"] = program + "'s variables family (#1023)"
	}
	families := maps.Clone(variablesFamilies)
	families[segment.EnvVarPrefix] = "writ's segments"

	for _, known := range settings {
		for family, owner := range families {
			if variable := known.variable(); strings.HasPrefix(variable, family) {
				t.Errorf("%s, the variable of %s (%s), lies in %s", variable, known.key, known.origin, owner)
			}
		}
	}

	for _, read := range names {
		for family, owner := range variablesFamilies {
			if strings.HasPrefix(read.name, family) {
				t.Errorf("%s reads %s by name, a member of %s, which only the variable resolver reads", read.origin,
					read.name, owner)
			}
		}
	}
}
