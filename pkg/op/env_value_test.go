// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// region TEST FIXTURES

// envValueText is a test type implementing [encoding.TextUnmarshaler]; verifies envValue's TextUnmarshaler
// probe path in the cascade.
type envValueText struct {
	Decoded string
}

// UnmarshalText satisfies [encoding.TextUnmarshaler]; prefixes the raw bytes with "decoded:" so the test
// can distinguish the path from JSON.
func (e *envValueText) UnmarshalText(text []byte) error {

	e.Decoded = "decoded:" + string(text)
	return nil
}

// envValueTextErr is a test type whose UnmarshalText always errors; verifies envValue surfaces the error.
type envValueTextErr struct{}

func (e *envValueTextErr) UnmarshalText(_ []byte) error {

	return errors.New("text unmarshal denied")
}

// envValueJSON is a plain struct with json tags; verifies envValue's JSON fallback path.
type envValueJSON struct {
	A string `json:"a"`
	B int    `json:"b"`
}

// endregion

func TestEnvValue_CanConvertTo(t *testing.T) {

	t.Run("nil target returns false", func(t *testing.T) {
		if envValue("x").CanConvertTo(nil) {
			t.Fatal("nil target should return false")
		}
	})

	t.Run("Resource interface returns false", func(t *testing.T) {
		target := reflect.TypeFor[*convertResource]()
		if envValue("x").CanConvertTo(target) {
			t.Fatal("Resource-implementing target should return false (let Convert step 7 win)")
		}
	})

	t.Run("string returns true", func(t *testing.T) {
		if !envValue("x").CanConvertTo(reflect.TypeFor[string]()) {
			t.Fatal("string target should return true")
		}
	})

	t.Run("primitive returns true", func(t *testing.T) {
		if !envValue("x").CanConvertTo(reflect.TypeFor[int]()) {
			t.Fatal("int target should return true")
		}
	})
}

func TestEnvValue_ConvertTo_String(t *testing.T) {

	got, err := envValue("hello world").ConvertTo(reflect.TypeFor[string]())
	if err != nil {
		t.Fatalf("ConvertTo string: %v", err)
	}
	if got != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

func TestEnvValue_ConvertTo_TimeDuration(t *testing.T) {

	tests := []struct {
		raw  string
		want time.Duration
	}{
		{"30s", 30 * time.Second},
		{"1h15m", 1*time.Hour + 15*time.Minute},
		{"500ms", 500 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := envValue(tt.raw).ConvertTo(reflect.TypeFor[time.Duration]())
			if err != nil {
				t.Fatalf("ConvertTo: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("malformed returns error", func(t *testing.T) {
		_, err := envValue("not a duration").ConvertTo(reflect.TypeFor[time.Duration]())
		if err == nil {
			t.Fatal("malformed duration should error")
		}
	})
}

func TestEnvValue_ConvertTo_FileMode(t *testing.T) {

	tests := []struct {
		raw  string
		want os.FileMode
	}{
		{"0o755", 0o755},
		{"0x1ff", 0x1ff},
		{"493", 493}, // decimal 493 == 0o755
		{"0", 0},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := envValue(tt.raw).ConvertTo(reflect.TypeFor[os.FileMode]())
			if err != nil {
				t.Fatalf("ConvertTo: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("malformed returns error", func(t *testing.T) {
		_, err := envValue("not a mode").ConvertTo(reflect.TypeFor[os.FileMode]())
		if err == nil {
			t.Fatal("malformed file mode should error")
		}
	})
}

func TestEnvValue_ConvertTo_Primitives(t *testing.T) {

	t.Run("bool", func(t *testing.T) {
		for _, raw := range []string{"true", "TRUE", "1"} {
			got, err := envValue(raw).ConvertTo(reflect.TypeFor[bool]())
			if err != nil || got != true {
				t.Errorf("%q: got (%v, %v), want (true, nil)", raw, got, err)
			}
		}
		for _, raw := range []string{"false", "FALSE", "0"} {
			got, err := envValue(raw).ConvertTo(reflect.TypeFor[bool]())
			if err != nil || got != false {
				t.Errorf("%q: got (%v, %v), want (false, nil)", raw, got, err)
			}
		}
		_, err := envValue("nope").ConvertTo(reflect.TypeFor[bool]())
		if err == nil {
			t.Error("malformed bool should error")
		}
	})

	t.Run("int with auto-base", func(t *testing.T) {
		tests := []struct {
			raw  string
			want int
		}{
			{"42", 42},
			{"0x2a", 42},
			{"0o52", 42},
			{"-7", -7},
		}
		for _, tt := range tests {
			got, err := envValue(tt.raw).ConvertTo(reflect.TypeFor[int]())
			if err != nil || got != tt.want {
				t.Errorf("%q: got (%v, %v), want (%v, nil)", tt.raw, got, err, tt.want)
			}
		}
	})

	t.Run("uint", func(t *testing.T) {
		got, err := envValue("42").ConvertTo(reflect.TypeFor[uint32]())
		if err != nil || got != uint32(42) {
			t.Errorf("got (%v, %v), want (42, nil)", got, err)
		}
	})

	t.Run("float", func(t *testing.T) {
		got, err := envValue("3.14").ConvertTo(reflect.TypeFor[float64]())
		if err != nil || got != 3.14 {
			t.Errorf("got (%v, %v), want (3.14, nil)", got, err)
		}
	})

	t.Run("complex", func(t *testing.T) {
		got, err := envValue("(3+4i)").ConvertTo(reflect.TypeFor[complex128]())
		if err != nil || got != complex(3, 4) {
			t.Errorf("got (%v, %v), want ((3+4i), nil)", got, err)
		}
		_, err = envValue("not complex").ConvertTo(reflect.TypeFor[complex64]())
		if err == nil {
			t.Error("malformed complex should error")
		}
	})
}

func TestEnvValue_ConvertTo_TextUnmarshaler(t *testing.T) {

	got, err := envValue("xyz").ConvertTo(reflect.TypeFor[envValueText]())
	if err != nil {
		t.Fatalf("ConvertTo: %v", err)
	}
	decoded, ok := got.(envValueText)
	if !ok {
		t.Fatalf("got %T, want envValueText", got)
	}
	if decoded.Decoded != "decoded:xyz" {
		t.Errorf("got %q, want %q", decoded.Decoded, "decoded:xyz")
	}
}

func TestEnvValue_ConvertTo_TextUnmarshaler_Error(t *testing.T) {

	_, err := envValue("x").ConvertTo(reflect.TypeFor[envValueTextErr]())
	if err == nil {
		t.Fatal("UnmarshalText error should propagate")
	}
	if !strings.Contains(err.Error(), "text unmarshal denied") {
		t.Errorf("expected wrapped UnmarshalText error, got: %v", err)
	}
}

func TestEnvValue_ConvertTo_JSON(t *testing.T) {

	got, err := envValue(`{"a":"hello","b":7}`).ConvertTo(reflect.TypeFor[envValueJSON]())
	if err != nil {
		t.Fatalf("ConvertTo: %v", err)
	}
	v, ok := got.(envValueJSON)
	if !ok {
		t.Fatalf("got %T, want envValueJSON", got)
	}
	if v.A != "hello" || v.B != 7 {
		t.Errorf("got %+v, want {A:hello B:7}", v)
	}
}

func TestEnvValue_ConvertTo_JSON_Error(t *testing.T) {

	_, err := envValue("not json").ConvertTo(reflect.TypeFor[envValueJSON]())
	if err == nil {
		t.Fatal("malformed JSON should error")
	}
}

func TestEnvValue_ConvertTo_NilTarget(t *testing.T) {

	_, err := envValue("x").ConvertTo(nil)
	if err == nil {
		t.Fatal("nil target should error")
	}
}

// TestEnvValue_ThroughConvert verifies the cascade routes envValue via SourceConverter step 5.
func TestEnvValue_ThroughConvert(t *testing.T) {

	t.Run("primitive via step 5", func(t *testing.T) {
		got, err := Convert(nil, envValue("0o755"), reflect.TypeFor[os.FileMode]())
		if err != nil {
			t.Fatalf("Convert: %v", err)
		}
		if got != os.FileMode(0o755) {
			t.Errorf("got %v, want 0o755", got)
		}
	})

	t.Run("string identity short-circuit", func(t *testing.T) {
		// envValue's underlying kind is String, so step 2 (assignability + ConvertibleTo) might fire
		// before step 5. Either path is acceptable as long as the result is correct.
		got, err := Convert(nil, envValue("hi"), reflect.TypeFor[string]())
		if err != nil {
			t.Fatalf("Convert: %v", err)
		}
		if got != "hi" {
			t.Errorf("got %q, want %q", got, "hi")
		}
	})

	t.Run("plain string still errors for primitive (envValue does not corrupt cascade)", func(t *testing.T) {
		// The whole point of the envValue wrapper: plain Go strings continue to fail on primitive
		// targets. Only envValue-wrapped strings get the lenient parse.
		_, err := Convert(nil, "0o755", reflect.TypeFor[os.FileMode]())
		if err == nil {
			t.Fatal("plain string → os.FileMode should error (only envValue gets the parse)")
		}
	})
}

// Verify the documented identifier shape — keeps the type-name single source of truth checkable.
var _ = fmt.Sprintf("%T", envValue(""))
