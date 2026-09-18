// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package result

// Group is the kind of reader a rendering serves.
//
// Nine names in one alphabetical list answer "what exists" and never "which one do I want". Five groups answer
// the second question, and the grouping is load-bearing rather than decorative: the pager pages what a person
// reads and nothing else, and it asks the formatter its group rather than keeping a list of names that would
// drift the first time a rendering was added.
//
// The values are the display names, because they are shown as headings in `--output`'s help and in the
// generated man pages.
type Group string

// The groups. Alphabetical, as every set in this suite is presented.
const (

	// GroupComposed is for renderings whose shape the caller chose: `template=BODY` and `value`. The filter
	// stage built the shape and this stage prints it, so paging or decorating it would be this suite second-
	// guessing a shape it was handed.
	GroupComposed Group = "Composed"

	// GroupDocument is for renderings that produce a document: `markdown` and `terminal`. They differ in
	// whether the markup is rendered.
	GroupDocument Group = "Document"

	// GroupNothing is for `none`, which emits nothing at all: the exit code and the side effects are the
	// result.
	GroupNothing Group = "Nothing"

	// GroupRecords is for records laid out for a person: `list` and `table`. They differ in whether the
	// records share one schema -- `table` derives one column set and leaves holes, `list` gives each record
	// its own keys (§8).
	GroupRecords Group = "Records"

	// GroupSerialized is for the lossless renderings a library reads back: `csv`, `json` and `yaml`. That
	// yaml is pleasanter to read than json is a property of yaml, not a different job -- nobody hand-writes a
	// `writ status` result.
	GroupSerialized Group = "Serialized"
)

// region Formatter groups

// Group reports that CSV and value are serialized or composed, by which preset this formatter is.
//
// One type backs two names: `csv` quotes so a parser can round-trip it, and `value` is raw so a shell reads
// exactly what the filter stage composed. [DelimitedFormatter.Raw] is the field that separates them, so it is
// the field that answers here.
//
// Returns:
//   - `Group`: [GroupComposed] for the raw preset, [GroupSerialized] otherwise.
func (f DelimitedFormatter) Group() Group {

	if f.Raw {
		return GroupComposed
	}
	return GroupSerialized
}

// Group reports that JSON is serialized.
//
// Returns:
//   - `Group`: [GroupSerialized].
func (JSONFormatter) Group() Group { return GroupSerialized }

// Group reports that YAML is serialized.
//
// Returns:
//   - `Group`: [GroupSerialized].
func (YAMLFormatter) Group() Group { return GroupSerialized }

// Group reports that a list is records.
//
// Returns:
//   - `Group`: [GroupRecords].
func (ListFormatter) Group() Group { return GroupRecords }

// Group reports that a table is records.
//
// Returns:
//   - `Group`: [GroupRecords].
func (TableFormatter) Group() Group { return GroupRecords }

// Group reports that markdown is a document.
//
// Returns:
//   - `Group`: [GroupDocument].
func (MarkdownFormatter) Group() Group { return GroupDocument }

// Group reports that the terminal rendering is a document.
//
// Returns:
//   - `Group`: [GroupDocument].
func (TerminalFormatter) Group() Group { return GroupDocument }

// Group reports that a template is composed by its caller.
//
// Returns:
//   - `Group`: [GroupComposed].
func (*TemplateFormatter) Group() Group { return GroupComposed }

// Group reports that none emits nothing.
//
// Returns:
//   - `Group`: [GroupNothing].
func (NoneFormatter) Group() Group { return GroupNothing }

// endregion
