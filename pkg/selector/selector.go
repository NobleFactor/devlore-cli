// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package selector detects the host and selects directory names by platform: the one selector API writ and lore share
// (#944).
//
// A name reads, in order, `[<project>][.<os>][.<arch>][.<extra>...]`. The OS part is one word of the host's chain
// (Unix, Linux, a distribution in its lineage), the architecture part one spelling of its architecture (arm64 or
// aarch64), and each extra one value of a segment declared in configuration, in configured order. Selection answers
// which of a list of names this machine includes, in the order to apply them, and reports every name that breaks the
// grammar. docs/guides/selectors.md is the design.
package selector

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// Builtins are the segment names detection supplies. No declared segment may use one.
var Builtins = []string{"OS", "DISTRO", "ARCH"}

// Selector selects directory names for one host.
type Selector struct {

	// Host is the machine selected for.
	Host Host

	// Projects are the projects a name may begin with, in the order they're applied: writ's names carry one. Empty when
	// names carry no project, as lore's don't.
	Projects []string

	// Base is the name with no selector words, when names carry no project: lore's Common.
	Base string

	// Segments are the extras, in configured order.
	Segments []Segment
}

// region EXPORTED METHODS

// region Behaviors

// Select chooses the names this machine includes, in the order to apply them, and finds every name that breaks the
// grammar.
//
// A name is included when each of its words names this machine: its OS word is in the host's chain, its
// architecture is the host's, and each extra's value is the segment's current value. A well-formed name for another
// machine is excluded silently. The order is the projects' order, then the platform ranking within each project: the
// OS word's depth in the chain, then the architecture, then each extra in configured order, each part narrowing the
// one to its left; then the name, so the order never depends on the order names were listed in.
//
// Parameters:
//   - `names`: directory names.
//
// Returns:
//   - `[]Selected`: the names included, in the order to apply them; the last applied wins.
//   - `[]*GrammarError`: every name that breaks the grammar. A caller refuses the run when there is one.
func (s Selector) Select(names []string) ([]Selected, []*GrammarError) {

	var selected []Selected
	var grammarErrors []*GrammarError

	for _, name := range names {
		project, words := s.split(name)

		rank, matches, grammarError := s.judge(name, words)
		if grammarError != nil {
			grammarErrors = append(grammarErrors, grammarError)
			continue
		}

		index := s.projectIndex(project)
		if !matches || index < 0 {
			continue
		}

		rank.Project = index
		selected = append(selected, Selected{Name: name, Project: project, Rank: rank})
	}

	sort.SliceStable(selected, func(i, j int) bool {
		if c := selected[i].Rank.Compare(selected[j].Rank); c != 0 {
			return c < 0
		}
		return selected[i].Name < selected[j].Name
	})

	return selected, grammarErrors
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// judge attributes a name's words to their parts and decides whether they name this machine.
//
// Each word is taken by the part whose vocabulary holds it; the vocabularies don't overlap, because segment values are
// unique. The parts must come in order, and each takes at most one word.
//
// Parameters:
//   - `name`: the directory name, for the error.
//   - `words`: its selector words, in order.
//
// Returns:
//   - `Rank`: the name's platform ranking; its Project is left for the caller.
//   - `bool`: true when every word names this machine.
//   - `*GrammarError`: non-nil when the words break the grammar.
func (s Selector) judge(name string, words []string) (Rank, bool, *GrammarError) {

	rank := Rank{Extras: make([]bool, len(s.Segments))}
	matches := true
	last := -1

	for _, word := range words {
		part, value, known := s.partOf(word)
		switch {
		case !known:
			return Rank{}, false, &GrammarError{Name: name, Word: word, Violation: UnknownWord}
		case part == last:
			return Rank{}, false, &GrammarError{Name: name, Word: word, Violation: RepeatedPart, Part: s.partName(part)}
		case part < last:
			return Rank{}, false, &GrammarError{Name: name, Word: word, Violation: OutOfOrder, Part: s.partName(part)}
		}
		last = part

		switch part {
		case osPart:
			rank.Link = s.Host.depth(word)
			matches = matches && rank.Link > 0
		case archPart:
			rank.Arch = true
			matches = matches && value == s.Host.Arch
		default:
			segment := s.Segments[part-extraParts]
			rank.Extras[part-extraParts] = true
			matches = matches && segment.Value != "" && value == segment.Value
		}
	}

	return rank, matches, nil
}

// partName names a part for an error.
//
// Parameters:
//   - `part`: the part's position in the grammar.
//
// Returns:
//   - `string`: OS, architecture, or the segment's name.
func (s Selector) partName(part int) string {

	switch part {
	case osPart:
		return "OS"
	case archPart:
		return "architecture"
	default:
		return s.Segments[part-extraParts].Name
	}
}

// partOf finds the part of the grammar a word belongs to.
//
// Parameters:
//   - `word`: a selector word.
//
// Returns:
//   - `int`: the part's position in the grammar.
//   - `string`: the word's value in that part: an architecture as runtime.GOARCH names it, otherwise the word.
//   - `bool`: false when no part's vocabulary holds the word.
func (s Selector) partOf(word string) (part int, value string, known bool) {

	if isPlatformWord(word) || s.Host.depth(word) > 0 {
		return osPart, word, true
	}
	if arch, ok := archOf(word); ok {
		return archPart, arch, true
	}
	if word == s.Host.Arch {
		return archPart, word, true
	}
	for i, segment := range s.Segments {
		if slices.Contains(segment.Values, word) {
			return extraParts + i, word, true
		}
	}
	return 0, "", false
}

// projectIndex gives a project's position in the order projects are applied.
//
// Parameters:
//   - `project`: the project a name begins with; empty when names carry no project.
//
// Returns:
//   - `int`: the position; 0 when names carry no project; -1 when the project isn't one of Projects.
func (s Selector) projectIndex(project string) int {

	if len(s.Projects) == 0 {
		return 0
	}
	return slices.Index(s.Projects, project)
}

// split separates a name's project from its selector words.
//
// Parameters:
//   - `name`: the directory name.
//
// Returns:
//   - `string`: the project; empty when names carry no project.
//   - `[]string`: the selector words, in order.
func (s Selector) split(name string) (project string, words []string) {

	if len(s.Projects) > 0 {
		project, rest, found := strings.Cut(name, ".")
		if !found {
			return project, nil
		}
		return project, strings.Split(rest, ".")
	}

	if name == s.Base {
		return "", nil
	}
	return "", strings.Split(name, ".")
}

// endregion

// endregion

// region EXPORTED FUNCTIONS

// ValidateSegments checks declared segments: names unique and none a built-in; values unique across every segment
// and none a word of the OS or architecture parts; each current value one of its segment's values.
//
// Values must be unique because a name carries values, never segment names, so a value is attributed to the only part
// whose vocabulary holds it.
//
// Parameters:
//   - `segments`: the declared segments, in configured order.
//
// Returns:
//   - `error`: every problem found, joined; nil when there is none.
func ValidateSegments(segments []Segment) error {

	var problems []error
	names := make(map[string]bool, len(segments))
	owner := make(map[string]string)

	for _, segment := range segments {
		switch {
		case segment.Name == "":
			problems = append(problems, errors.New("a segment has no name"))
		case slices.Contains(Builtins, segment.Name):
			problems = append(problems, fmt.Errorf("segment %s: %s is built in, and cannot be declared", segment.Name,
				segment.Name))
		case names[segment.Name]:
			problems = append(problems, fmt.Errorf("segment %s is declared twice", segment.Name))
		}
		names[segment.Name] = true

		for _, value := range segment.Values {
			_, isArch := archOf(value)
			switch {
			case value == "":
				problems = append(problems, fmt.Errorf("segment %s declares an empty value", segment.Name))
			case isPlatformWord(value):
				problems = append(problems, fmt.Errorf("segment %s: %q is an OS or distribution word", segment.Name, value))
			case isArch:
				problems = append(problems, fmt.Errorf("segment %s: %q is an architecture word", segment.Name, value))
			case owner[value] != "":
				problems = append(problems, fmt.Errorf("segments %s and %s both declare %q; values must be unique",
					owner[value], segment.Name, value))
			default:
				owner[value] = segment.Name
			}
		}

		if segment.Value != "" && !slices.Contains(segment.Values, segment.Value) {
			problems = append(problems, fmt.Errorf("segment %s: %q is not one of its values %v", segment.Name,
				segment.Value, segment.Values))
		}
	}

	return errors.Join(problems...)
}

// endregion

// region SUPPORTING TYPES

// The parts of the grammar, in order. An extra's part is extraParts plus its position among the segments.
const (
	osPart = iota
	archPart
	extraParts
)

// GrammarError is a name that breaks the grammar. The caller refuses the run, naming every one.
type GrammarError struct {

	// Name is the directory name.
	Name string

	// Word is the word that breaks the grammar.
	Word string

	// Violation is the rule the word breaks.
	Violation Violation

	// Part is the part the word belongs to; empty for an unknown word.
	Part string
}

// Error describes the name, the word, and the rule it breaks.
//
// Returns:
//   - `string`: the description.
func (e *GrammarError) Error() string {

	switch e.Violation {
	case OutOfOrder:
		return fmt.Sprintf("%s: %q (%s) is out of order; a name reads project, OS, architecture, then each segment in "+
			"configured order", e.Name, e.Word, e.Part)
	case RepeatedPart:
		return fmt.Sprintf("%s: %q is a second %s word; a name has at most one", e.Name, e.Word, e.Part)
	default:
		return fmt.Sprintf("%s: %q is not a known OS, distribution or architecture, nor a declared segment value",
			e.Name, e.Word)
	}
}

// Rank orders the names a selection includes. Compare it field by field, in field order.
type Rank struct {

	// Project is the project's position in the order projects are applied.
	Project int

	// Link is the OS word's position in the host's chain, most general first; 0 when the name has no OS word.
	Link int

	// Arch reports whether the name names the architecture.
	Arch bool

	// Extras reports, for each segment in configured order, whether the name names it.
	Extras []bool
}

// Compare orders two ranks: project, then the OS word's depth, then the architecture, then each extra in configured
// order.
//
// Parameters:
//   - `other`: the rank to compare with.
//
// Returns:
//   - `int`: negative when r is applied before other, positive when after, 0 when they rank alike.
func (r Rank) Compare(other Rank) int {

	if c := r.Project - other.Project; c != 0 {
		return c
	}
	if c := r.Link - other.Link; c != 0 {
		return c
	}
	if c := compareBool(r.Arch, other.Arch); c != 0 {
		return c
	}
	for i := 0; i < len(r.Extras) && i < len(other.Extras); i++ {
		if c := compareBool(r.Extras[i], other.Extras[i]); c != 0 {
			return c
		}
	}
	return len(r.Extras) - len(other.Extras)
}

// Segment is an extra segment, declared in configuration with the values it may take.
type Segment struct {

	// Name is the segment's name: ROLE.
	Name string

	// Values are the values it may take: desktop, server. A directory name carries one of them.
	Values []string

	// Value is this machine's value; empty when unset, and then the segment matches no name.
	Value string
}

// Selected is a name a selection includes.
type Selected struct {

	// Name is the directory name.
	Name string

	// Project is the project it begins with; empty when names carry no project.
	Project string

	// Rank is its place in the order of application.
	Rank Rank
}

// Violation is a rule of the grammar a name breaks.
type Violation int

const (

	// UnknownWord is a word no part's vocabulary holds: a misspelling, or a distribution the table doesn't know.
	UnknownWord Violation = iota + 1

	// OutOfOrder is a word whose part comes before a part the name has already named.
	OutOfOrder

	// RepeatedPart is a second word for one part: two OS words, two architectures, or two values of one segment.
	RepeatedPart
)

// endregion

// region HELPER FUNCTIONS

// compareBool orders false before true.
//
// Parameters:
//   - `a`: the first value.
//   - `b`: the second value.
//
// Returns:
//   - `int`: -1 when only b is true, 1 when only a is, 0 when they're equal.
func compareBool(a, b bool) int {

	switch {
	case a == b:
		return 0
	case b:
		return -1
	default:
		return 1
	}
}

// endregion
