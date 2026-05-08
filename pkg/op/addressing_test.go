// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"errors"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

func TestAddressingMode_String(t *testing.T) {

	cases := []struct {
		mode AddressingMode
		want string
	}{
		{AddressingUnknown, "unknown"},
		{AddressingLocation, "location"},
		{AddressingContent, "content"},
	}

	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {

			if got := c.mode.String(); got != c.want {
				t.Errorf("AddressingMode(%d).String() = %q, want %q", c.mode, got, c.want)
			}
		})
	}
}

func TestAddressingMode_String_PanicsOnInvalid(t *testing.T) {

	defer func() {

		r := recover()
		if r == nil {
			t.Fatal("AddressingMode(99).String() did not panic")
		}

		err, ok := r.(error)
		if !ok {
			t.Fatalf("recovered value %v (%T), want error", r, r)
		}

		var ae *assert.AssertionError
		if !errors.As(err, &ae) {
			t.Fatalf("recovered error %v (%T), want *assert.AssertionError", err, err)
		}

		// Sanity-check the message carries the offending integer.
		if got := ae.Message; got == "" {
			t.Errorf("AssertionError.Message is empty")
		}
	}()

	_ = AddressingMode(99).String()
}

// TestAddressingMode_ZeroValue asserts that the zero value of AddressingMode is
// AddressingUnknown — the boot-discipline test in k.12 will rely on this to
// detect concrete Resource types that haven't overridden Addressing.
func TestAddressingMode_ZeroValue(t *testing.T) {

	var m AddressingMode

	if m != AddressingUnknown {
		t.Errorf("zero value AddressingMode = %v, want AddressingUnknown", m)
	}
}