// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"strings"
	"testing"
)

// region Tests

// TestTidyManRoff_GivesAQuotedBlockGitsShape is the rule the man pages are held to.
//
// md2man renders a block quote as `.PP` `.RS` and then the quoted text as a further paragraph, so a name and
// its description arrive three blank lines apart. git's pages put the description in an `.RS 4` block directly
// under the name, and that is the shape required of ours.
func TestTidyManRoff_GivesAQuotedBlockGitsShape(t *testing.T) {

	generated := ".PP\n\\fBterminal\\fP\n\n.PP\n.RS\n\n.PP\nthe document rendered, with bold and italic\n\n.RE\n"
	want := ".PP\n\\fBterminal\\fP\n.RS 4\nthe document rendered, with bold and italic\n.RE\n"

	if got := string(tidyManRoff([]byte(generated))); got != want {
		t.Errorf("tidyManRoff produced:\n%q\nwant:\n%q", got, want)
	}
}

// TestTidyManRoff_ClosesUpParagraphBreaks pins the spacing.
//
// md2man writes a blank line before every `.PP`, which roff renders as a line of its own, so entries stand two
// lines apart where git's stand one. The macro needs no blank line before it.
func TestTidyManRoff_ClosesUpParagraphBreaks(t *testing.T) {

	got := string(tidyManRoff([]byte(".PP\nfirst\n\n.PP\nsecond\n")))

	if strings.Contains(got, "\n\n.PP\n") {
		t.Errorf("a blank line survives before a paragraph macro:\n%q", got)
	}
	for _, want := range []string{"first", "second"} {
		if !strings.Contains(got, want) {
			t.Errorf("tidyManRoff lost %q:\n%q", want, got)
		}
	}
}

// TestTidyManRoff_LeavesAnUnquotedPageAlone guards the pages that carry no structured usage.
//
// Every command's page passes through this, and a page with no block quote in it must come out byte-identical.
func TestTidyManRoff_LeavesAnUnquotedPageAlone(t *testing.T) {

	page := ".TH WRIT 1\n.SH NAME\nwrit \\- a thing\n.SH DESCRIPTION\n.PP\nIt does things.\n.RE\n"

	if got := string(tidyManRoff([]byte(page))); got != page {
		t.Errorf("an unquoted page changed:\n%q\nwant:\n%q", got, page)
	}
}

// endregion
