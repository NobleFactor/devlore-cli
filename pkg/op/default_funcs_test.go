// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"os"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

// region defaultUmask

func TestDefaultUmask_MasksBaseAgainstProcessUmask(t *testing.T) {

	// Snapshot current umask without changing it.
	mask := syscall.Umask(0)
	syscall.Umask(mask)

	cases := []struct {
		name string
		base os.FileMode
	}{
		{"file convention", 0o666},
		{"dir convention", 0o777},
		{"executable convention", 0o755},
		{"zero base", 0o000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := defaultUmask(nil, nil, []reflect.Value{reflect.ValueOf(tc.base)})
			if err != nil {
				t.Fatalf("defaultUmask: %v", err)
			}
			got, ok := result.Interface().(os.FileMode)
			if !ok {
				t.Fatalf("got %T, want os.FileMode", result.Interface())
			}
			want := tc.base &^ os.FileMode(mask)
			if got != want {
				t.Errorf("defaultUmask(%o) = %o, want %o (umask %o)", tc.base, got, want, mask)
			}
		})
	}
}

func TestDefaultUmask_AcceptsIntAndFileMode(t *testing.T) {

	// int64 (text/template/parse's natural literal type).
	if _, err := defaultUmask(nil, nil, []reflect.Value{reflect.ValueOf(int64(0o666))}); err != nil {
		t.Errorf("int64 base: %v", err)
	}

	// uint64 (alternative numeric kind).
	if _, err := defaultUmask(nil, nil, []reflect.Value{reflect.ValueOf(uint64(0o666))}); err != nil {
		t.Errorf("uint64 base: %v", err)
	}

	// os.FileMode (sibling-slot reference holding an already-typed mode).
	if _, err := defaultUmask(nil, nil, []reflect.Value{reflect.ValueOf(os.FileMode(0o666))}); err != nil {
		t.Errorf("os.FileMode base: %v", err)
	}
}

func TestDefaultUmask_RejectsArgErrors(t *testing.T) {

	cases := []struct {
		name string
		args []reflect.Value
		want string
	}{
		{"zero args", []reflect.Value{}, "expected 1 argument"},
		{"two args", []reflect.Value{reflect.ValueOf(0o666), reflect.ValueOf(0o755)}, "expected 1 argument"},
		{"string arg", []reflect.Value{reflect.ValueOf("0o666")}, "expected file mode"},
		{"negative int", []reflect.Value{reflect.ValueOf(-1)}, "negative file mode"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := defaultUmask(nil, nil, tc.args)
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want containing %q", err.Error(), tc.want)
			}
		})
	}
}

// endregion

// region defaultMode

func TestDefaultMode_RoundTrip(t *testing.T) {

	cases := []struct {
		input string
		want  os.FileMode
	}{
		{"---------", 0o000},
		{"r--------", 0o400},
		{"rw-------", 0o600},
		{"rwx------", 0o700},
		{"rwxr-x---", 0o750},
		{"rwxrwxrwx", 0o777},
		{"rw-r--r--", 0o644},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			result, err := defaultMode(nil, nil, []reflect.Value{reflect.ValueOf(tc.input)})
			if err != nil {
				t.Fatalf("defaultMode(%q): %v", tc.input, err)
			}
			got, ok := result.Interface().(os.FileMode)
			if !ok {
				t.Fatalf("got %T, want os.FileMode", result.Interface())
			}
			if got != tc.want {
				t.Errorf("defaultMode(%q) = %o, want %o", tc.input, got, tc.want)
			}
		})
	}
}

func TestDefaultMode_RejectsMalformed(t *testing.T) {

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"too short", "rwx", "9-character"},
		{"too long", "rwxrwxrwxrwx", "9-character"},
		{"wrong letter at position 0", "Xwxrwxrwx", `expected "r"`},
		{"wrong letter at position 4", "rwxrXxrwx", `expected "w"`},
		{"unicode", "rwx🔒rwxrw", "9-character"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := defaultMode(nil, nil, []reflect.Value{reflect.ValueOf(tc.input)})
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want containing %q", err.Error(), tc.want)
			}
		})
	}
}

// endregion

// region defaultEnv

func TestDefaultEnv_Lookup(t *testing.T) {

	t.Setenv("DEVLORE_TEST_DEFAULT_ENV", "value-for-test")

	result, err := defaultEnv(nil, nil, []reflect.Value{reflect.ValueOf("DEVLORE_TEST_DEFAULT_ENV")})
	if err != nil {
		t.Fatalf("defaultEnv: %v", err)
	}
	got, ok := result.Interface().(string)
	if !ok {
		t.Fatalf("got %T, want string", result.Interface())
	}
	if got != "value-for-test" {
		t.Errorf("got %q, want %q", got, "value-for-test")
	}
}

func TestDefaultEnv_MissingResolvesToEmptyString(t *testing.T) {

	result, err := defaultEnv(nil, nil, []reflect.Value{reflect.ValueOf("DEVLORE_TEST_GUARANTEED_UNSET_VAR_XYZQ")})
	if err != nil {
		t.Fatalf("defaultEnv: %v", err)
	}
	got, ok := result.Interface().(string)
	if !ok {
		t.Fatalf("got %T, want string", result.Interface())
	}
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestDefaultEnv_RejectsArgErrors(t *testing.T) {

	if _, err := defaultEnv(nil, nil, []reflect.Value{}); err == nil {
		t.Error("zero args: want error")
	}
	if _, err := defaultEnv(nil, nil, []reflect.Value{reflect.ValueOf(42)}); err == nil {
		t.Error("int arg: want error")
	}
}

// endregion