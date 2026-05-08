// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"strings"
	"testing"
)

func TestDigest_String_RoundTrip(t *testing.T) {

	cases := []struct {
		name string
		hex  string
	}{
		{"all zero", strings.Repeat("0", 64)},
		{"all f", strings.Repeat("f", 64)},
		{"mixed", "0000000000000000000000000000000000000000000000000000000000000001"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {

			input := "sha256:" + c.hex

			d, err := ParseDigest(input)
			if err != nil {
				t.Fatalf("ParseDigest(%q): %v", input, err)
			}

			if got := d.String(); got != input {
				t.Errorf("String() = %q, want %q", got, input)
			}
		})
	}
}

func TestDigest_Equal(t *testing.T) {

	a := Digest{Algorithm: "sha256", Bytes: []byte{0x01, 0x02, 0x03}}
	b := Digest{Algorithm: "sha256", Bytes: []byte{0x01, 0x02, 0x03}}
	c := Digest{Algorithm: "sha256", Bytes: []byte{0x01, 0x02, 0x04}}
	d := Digest{Algorithm: "sha512", Bytes: []byte{0x01, 0x02, 0x03}}
	e := Digest{Algorithm: "sha256", Bytes: []byte{0x01, 0x02}}

	cases := []struct {
		name string
		x, y Digest
		want bool
	}{
		{"identical", a, b, true},
		{"reflexive", a, a, true},
		{"different bytes", a, c, false},
		{"different algorithm", a, d, false},
		{"different length", a, e, false},
		{"both empty", Digest{}, Digest{}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {

			if got := tc.x.Equal(tc.y); got != tc.want {
				t.Errorf("%v.Equal(%v) = %v, want %v", tc.x, tc.y, got, tc.want)
			}

			// Symmetry.
			if got := tc.y.Equal(tc.x); got != tc.want {
				t.Errorf("symmetry: %v.Equal(%v) = %v, want %v", tc.y, tc.x, got, tc.want)
			}
		})
	}
}

func TestParseDigest_Valid(t *testing.T) {

	input := "sha256:" + strings.Repeat("ab", 32)

	d, err := ParseDigest(input)
	if err != nil {
		t.Fatalf("ParseDigest: %v", err)
	}

	if d.Algorithm != "sha256" {
		t.Errorf("Algorithm = %q, want %q", d.Algorithm, "sha256")
	}

	if len(d.Bytes) != 32 {
		t.Errorf("len(Bytes) = %d, want 32", len(d.Bytes))
	}
}

func TestParseDigest_Reject(t *testing.T) {

	cases := []struct {
		name    string
		input   string
		wantSub string
	}{
		{"empty", "", "malformed"},
		{"no separator", "sha256" + strings.Repeat("a", 64), "malformed"},
		{"no algorithm", ":" + strings.Repeat("a", 64), "malformed"},
		{"no payload", "sha256:", "malformed"},
		{"uppercase hex", "sha256:" + strings.Repeat("A", 64), "malformed"},
		{"non-hex char", "sha256:" + strings.Repeat("g", 64), "malformed"},
		{"sha256 wrong length", "sha256:" + strings.Repeat("a", 62), "32 bytes"},
		{"unknown algorithm", "blake3:" + strings.Repeat("a", 64), "unsupported algorithm"},
		{"leading whitespace", " sha256:" + strings.Repeat("a", 64), "malformed"},
		{"trailing whitespace", "sha256:" + strings.Repeat("a", 64) + " ", "malformed"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {

			_, err := ParseDigest(c.input)
			if err == nil {
				t.Fatalf("ParseDigest(%q) succeeded, want error containing %q", c.input, c.wantSub)
			}

			if !strings.Contains(err.Error(), c.wantSub) {
				t.Errorf("error = %q, want substring %q", err.Error(), c.wantSub)
			}
		})
	}
}