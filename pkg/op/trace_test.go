// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoadTrace_RoundTrip(t *testing.T) {

	trace := &Trace{GraphChecksum: "sha256:0123"}
	if err := trace.StampChecksum(); err != nil {
		t.Fatalf("StampChecksum: %v", err)
	}

	data, err := yaml.Marshal(trace)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	loaded, err := LoadTrace(data)
	if err != nil {
		t.Fatalf("LoadTrace: %v", err)
	}
	if loaded.GraphChecksum != trace.GraphChecksum {
		t.Errorf("GraphChecksum = %q, want %q", loaded.GraphChecksum, trace.GraphChecksum)
	}
	if loaded.Checksum != trace.Checksum {
		t.Errorf("Checksum = %q, want %q", loaded.Checksum, trace.Checksum)
	}
}

func TestLoadTrace_StampIsIdempotent(t *testing.T) {

	trace := &Trace{GraphChecksum: "sha256:0123"}
	if err := trace.StampChecksum(); err != nil {
		t.Fatalf("StampChecksum: %v", err)
	}
	first := trace.Checksum

	if err := trace.StampChecksum(); err != nil {
		t.Fatalf("StampChecksum (second): %v", err)
	}
	if trace.Checksum != first {
		t.Errorf("restamp changed checksum: %q -> %q", first, trace.Checksum)
	}
}

func TestLoadTrace_RefusesMissingChecksum(t *testing.T) {

	data, err := yaml.Marshal(&Trace{GraphChecksum: "sha256:0123"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	_, err = LoadTrace(data)
	if err == nil || !strings.Contains(err.Error(), "no checksum") {
		t.Fatalf("LoadTrace = %v, want no-checksum error", err)
	}
}

func TestLoadTrace_RefusesTamperedDocument(t *testing.T) {

	trace := &Trace{GraphChecksum: "sha256:0123"}
	if err := trace.StampChecksum(); err != nil {
		t.Fatalf("StampChecksum: %v", err)
	}

	data, err := yaml.Marshal(trace)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	tampered := bytes.Replace(data, []byte("sha256:0123"), []byte("sha256:4567"), 1)
	if bytes.Equal(tampered, data) {
		t.Fatal("tamper had no effect on the document bytes")
	}

	_, err = LoadTrace(tampered)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("LoadTrace = %v, want checksum-mismatch error", err)
	}
}

// TestLoadTrace_VariablesReloadTyped pins #712 phase 3, item 4: a paused run's variables are read back from the
// trace by ResumeExecutor, so each value carries its type -- an integer returns as an int64, a list as the document's
// shape, and a resource as the recorded id that graph dispatch resolves against the run catalog.
func TestLoadTrace_VariablesReloadTyped(t *testing.T) {

	catalog := NewResourceCatalog()
	held := newLifecycle("test:///held", AddressingLocation)
	_, id := catalog.Resolve(held)
	trace := &Trace{GraphChecksum: "sha256:0123", Variables: map[string]Variable{
		"count": {Name: "count", Value: int64(7)},
		"name":  {Name: "name", Field: "first", Value: "x"},
		"items": {Name: "items", Value: []any{int64(1), "two"}},
		"held":  {Name: "held", Value: held},
	}}
	if err := trace.StampChecksum(); err != nil {
		t.Fatalf("StampChecksum: %v", err)
	}
	data, err := yaml.Marshal(trace)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	loaded, err := LoadTrace(data)
	if err != nil {
		t.Fatalf("LoadTrace: %v", err)
	}
	for name, want := range map[string]any{
		"count": int64(7),
		"name":  "x",
		"items": []any{int64(1), "two"},
		"held":  recordedResourceID(id),
	} {
		got := loaded.Variables[name].Value
		if !reflect.DeepEqual(got, want) {
			t.Errorf("variable %q = %#v (%T); want %#v (%T)", name, got, got, want, want)
		}
	}
	if loaded.Variables["name"].Field != "first" {
		t.Errorf("variable name's Field = %q; want first", loaded.Variables["name"].Field)
	}
}

// TestVariable_JSONRoundTripKeepsTheType pins the JSON half of item 4 on the Variable itself: the document carries
// the envelope, and a bare value in it is refused.
func TestVariable_JSONRoundTripKeepsTheType(t *testing.T) {

	data, err := json.Marshal(Variable{Name: "count", Value: int64(9)})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Contains(data, []byte(`"value":{"$int64":"9"}`)) {
		t.Fatalf("document does not carry the envelope: %s", data)
	}
	var loaded Variable
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if loaded.Value != int64(9) {
		t.Errorf("Value = %#v (%T); want int64 9", loaded.Value, loaded.Value)
	}
	if err := json.Unmarshal([]byte(`{"name":"count","value":9}`), &loaded); err == nil {
		t.Fatal("a bare value in a variable document was accepted; want a refusal")
	}
}
