// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package status

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/sink"
)

// --- Write ---

// TestNarrator_WriteIsUnaltered pins that Write puts the bytes on the sink exactly as given: no program prefix, no
// symbol, no newline.
func TestNarrator_WriteIsUnaltered(t *testing.T) {

	s, buffer := sink.Capture()
	narrator := NewNarrator("writ", s)

	if n, err := narrator.Write([]byte("10%\r50%")); err != nil || n != 7 {
		t.Fatalf("Write = %d, %v", n, err)
	}
	if got := buffer.String(); got != "10%\r50%" {
		t.Errorf("sink received %q", got)
	}
}

// TestNarrator_WriteUnderSilent pins that a Narrator over Discard, which is what --silent installs, writes nothing.
func TestNarrator_WriteUnderSilent(t *testing.T) {

	narrator := NewNarrator("writ", sink.Discard())
	if n, err := narrator.Write([]byte("anything")); err != nil || n != 8 {
		t.Errorf("Write = %d, %v", n, err)
	}
}
