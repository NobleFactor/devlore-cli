// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package result

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/muesli/termenv"
)

// TerminalFormatter renders a value as a document laid out for a terminal: bold, italic, color, word wrap, and
// box-drawn tables.
//
// It is the second link of a chain. [MarkdownFormatter] turns the normalized JSON into a markdown document, and
// this formatter hands that document to glamour: goldmark parses it, glamour's renderer walks the tree, and
// [terminalStyle] decides what each element becomes.
//
// Every marker is consumed and re-emitted as an attribute -- a heading loses its `#`, `**` becomes bold, `*`
// becomes italic, a code span loses its backticks. That is what separates this from glamour's own `notty`
// style, which keeps every marker, and from `markdown`, which is the source.
//
// Nothing is probed. glamour's color profile defaults to TrueColor rather than asking the terminal, and the
// style is fixed rather than glamour's `auto`, which reads the terminal's background. So the bytes are the same
// piped, redirected, or on a TTY -- §7's rule that a rendering does not change when it is observed. A caller who
// wants no escape codes asks for `markdown`.
//
// `NO_COLOR` is honored, because it is an input rather than a probe. Set and not empty, it drops every color and
// keeps every attribute: no-color.org's rule suppresses color, not bold, italic, or underline. §10 of the
// specification records why the suite honors a convention no standards body stands behind.
type TerminalFormatter struct {

	// WordWrap is the column text wraps at. Zero means [terminalWordWrap]. Fixed rather than read from the
	// terminal, since reading it would be a probe.
	WordWrap int

	// NoColor drops every color and keeps every attribute. [NewTerminalFormatter] sets it from `NO_COLOR`.
	NoColor bool

	// document is the first link of the chain.
	document MarkdownFormatter
}

// terminalWordWrap is glamour's own default width.
const terminalWordWrap = 80

// Compile-time interface guard.
var _ Formatter = TerminalFormatter{}

// NewTerminalFormatter returns the terminal document renderer.
//
// Returns:
//   - `TerminalFormatter`: the formatter.
func NewTerminalFormatter() TerminalFormatter {
	return TerminalFormatter{
		WordWrap: terminalWordWrap,
		NoColor:  os.Getenv("NO_COLOR") != "",
		document: NewMarkdownFormatter(),
	}
}

// region Formatter

// Format renders value as markdown, then renders that markdown for a terminal, to w.
//
// Parameters:
//   - `value`: any normalized value; a string is the document itself.
//   - `w`: the destination.
//
// Returns:
//   - `error`: when the renderer cannot be built or the document cannot be rendered, or any write error.
func (f TerminalFormatter) Format(value any, w io.Writer) error {

	var document bytes.Buffer
	if err := f.document.Format(value, &document); err != nil {
		return err
	}

	if document.Len() == 0 {
		return nil
	}

	wordWrap := f.WordWrap
	if wordWrap <= 0 {
		wordWrap = terminalWordWrap
	}

	// TrueColor is glamour's own default, named here so that nothing downstream is left to guess. Ascii is
	// termenv's profile without color: it answers every color with none and leaves the attributes alone.
	profile := termenv.TrueColor
	if f.NoColor {
		profile = termenv.Ascii
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(terminalStyle()),
		glamour.WithColorProfile(profile),
		glamour.WithWordWrap(wordWrap),
	)
	if err != nil {
		return fmt.Errorf("result.TerminalFormatter: build the renderer: %w", err)
	}

	rendered, err := renderer.RenderBytes(document.Bytes())
	if err != nil {
		return fmt.Errorf("result.TerminalFormatter: render the document: %w", err)
	}

	_, err = w.Write(trimPadding(rendered))
	return err
}

// endregion

// region Helpers

// linePadding matches the run of spaces and style sequences glamour leaves at the end of a line.
var linePadding = regexp.MustCompile(`(?:\x1b\[[0-9;]*m|[ \t])+$`)

// styleSequence matches one SGR escape sequence.
var styleSequence = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// reset closes any style still open.
const reset = "\x1b[0m"

// trimPadding removes the padding glamour adds to the end of every line.
//
// glamour pads each line out to the wrap width through `reflow/padding` ([ansi/margin.go]) and offers no option
// to stop it, so a reader copying a report out of the terminal collects trailing spaces, and a diff of two
// renderings is noise. The padding is also most of the byte count, since each padded space is wrapped in its
// own style sequence.
//
// A trailing run may carry style sequences as well as spaces. Dropping those could leave a style open and bleed
// it onto the next line, so when the trimmed run held any sequence and the line still carries one, a reset is
// put back.
//
// Parameters:
//   - `rendered`: glamour's output.
//
// Returns:
//   - `[]byte`: the same output with no line ending in padding.
func trimPadding(rendered []byte) []byte {

	lines := bytes.Split(rendered, []byte("\n"))

	for i, line := range lines {
		trimmed := linePadding.ReplaceAll(line, nil)
		if len(trimmed) == len(line) {
			continue
		}
		if styleSequence.Match(line[len(trimmed):]) && styleSequence.Match(trimmed) {
			trimmed = append(trimmed, reset...)
		}
		lines[i] = trimmed
	}

	return bytes.Join(lines, []byte("\n"))
}

// terminalStyle returns the style sheet for [TerminalFormatter].
//
// It starts from glamour's `notty` sheet for its layout -- list indentation, task boxes, the rule -- and replaces
// every element whose marker survives there. Each element is assigned whole rather than edited through a
// pointer, because the bundled sheet is a package variable and its pointers are shared.
//
// | Element       | Renders as                                   |
// | ------------- | -------------------------------------------- |
// | h1            | bold, italic, underlined; no `#`             |
// | h2 to h6      | bold; no `#`                                 |
// | strong        | bold; no `**`                                |
// | emph          | italic; no `*`                               |
// | strikethrough | crossed out; no `~~`                         |
// | code span     | colored; no backticks                        |
// | code block    | syntax-highlighted, glamour's dark palette   |
// | block quote   | indented under `│ `, italic                  |
// | bullet        | `- `                                         |
// | table         | box-drawn; links collected under it          |
// | link          | its text, then the URL, underlined           |
//
// A code block needs nothing special under `NO_COLOR`. Chroma writes its own escape codes, but glamour only
// hands the block to chroma when the color profile is not Ascii, and otherwise renders it plain.
//
// Returns:
//   - `ansi.StyleConfig`: the style sheet.
func terminalStyle() ansi.StyleConfig {

	style := styles.NoTTYStyleConfig

	style.Document = ansi.StyleBlock{}

	style.Heading = ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{BlockSuffix: "\n", Bold: new(true)}}
	style.H1 = ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Italic: new(true), Underline: new(true)}}
	style.H2 = ansi.StyleBlock{}
	style.H3 = ansi.StyleBlock{}
	style.H4 = ansi.StyleBlock{}
	style.H5 = ansi.StyleBlock{}
	style.H6 = ansi.StyleBlock{}

	style.Strong = ansi.StylePrimitive{Bold: new(true)}
	style.Emph = ansi.StylePrimitive{Italic: new(true)}
	style.Strikethrough = ansi.StylePrimitive{CrossedOut: new(true)}

	style.Code = ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: new("63")}}
	style.CodeBlock = styles.DarkStyleConfig.CodeBlock

	style.BlockQuote = ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{Italic: new(true)},
		Indent:         new(uint(1)),
		IndentToken:    new("│ "),
	}

	style.Item = ansi.StylePrimitive{BlockPrefix: "- "}

	// The zero separators are what make glamour draw the table with lipgloss's box characters; `notty` sets
	// them to `|` and `-`.
	style.Table = ansi.StyleTable{}

	style.Link = ansi.StylePrimitive{Color: new("39"), Underline: new(true)}

	return style
}

// endregion
