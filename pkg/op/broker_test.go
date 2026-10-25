// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

// marshal renders v as compact JSON or fails the test.
//
// Parameters:
//   - `t`: the test.
//   - `v`: the value.
//
// Returns:
//   - `string`: the JSON.
func marshal(t *testing.T, v any) string {

	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(data)
}

// --- ExecRequest ---

// TestExecRequest_WireArgv pins the argv form of POST /v1/exec to 2.7 § Message reference, byte for byte.
func TestExecRequest_WireArgv(t *testing.T) {

	request := ExecRequest{
		Run: "run-7f3a", Unit: "u-12", Account: "root",
		Argv:  []string{"apt-get", "install", "-y", "openssh-server"},
		Env:   map[string]string{"DEBIAN_FRONTEND": "noninteractive"},
		Cwd:   "/",
		Stdin: &Stdin{AutoConfirm: true},
	}

	want := `{"run":"run-7f3a","unit":"u-12","account":"root","argv":["apt-get","install","-y","openssh-server"],` +
		`"env":{"DEBIAN_FRONTEND":"noninteractive"},"cwd":"/","stdin":"auto_confirm"}`
	if got := marshal(t, request); got != want {
		t.Errorf("argv form:\n got %s\nwant %s", got, want)
	}
}

// TestExecRequest_WireScript pins the script form of POST /v1/exec to 2.7 § Message reference, byte for byte.
func TestExecRequest_WireScript(t *testing.T) {

	request := ExecRequest{
		Run: "run-7f3a", Unit: "u-13", Account: "caller",
		Script: &Script{Dialect: "sh", Body: "set -eu\njournalctl -u \"$1\" --since \"$2\" | tail -20"},
		Args:   []string{"sshd", "-1h"},
	}

	want := `{"run":"run-7f3a","unit":"u-13","account":"caller",` +
		`"script":{"dialect":"sh","body":"set -eu\njournalctl -u \"$1\" --since \"$2\" | tail -20"},"args":["sshd","-1h"]}`
	if got := marshal(t, request); got != want {
		t.Errorf("script form:\n got %s\nwant %s", got, want)
	}
}

// TestExecRequest_Validate pins what a broker refuses before a runner sees it: a missing run, unit or account,
// neither or both of argv and script, a script with no dialect.
func TestExecRequest_Validate(t *testing.T) {

	good := ExecRequest{Run: "r", Unit: "u", Account: "caller", Argv: []string{"true"}}
	if err := good.Validate(); err != nil {
		t.Fatalf("a good request: %v", err)
	}

	cases := map[string]ExecRequest{
		"no run":             {Unit: "u", Account: "caller", Argv: []string{"true"}},
		"no unit":            {Run: "r", Account: "caller", Argv: []string{"true"}},
		"no account":         {Run: "r", Unit: "u", Argv: []string{"true"}},
		"neither":            {Run: "r", Unit: "u", Account: "caller"},
		"both":               {Run: "r", Unit: "u", Account: "caller", Argv: []string{"true"}, Script: &Script{Dialect: "sh"}},
		"script, no dialect": {Run: "r", Unit: "u", Account: "caller", Script: &Script{Body: "true"}},
	}
	for name, request := range cases {
		if err := request.Validate(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

// --- Stdin ---

// TestStdin_RoundTrip pins the two wire shapes: the word auto_confirm, and {"bytes": "<base64>"}.
func TestStdin_RoundTrip(t *testing.T) {

	for name, stdin := range map[string]Stdin{"auto_confirm": {AutoConfirm: true}, "bytes": {Bytes: []byte("y\n")}} {
		data := marshal(t, stdin)
		var back Stdin
		if err := json.Unmarshal([]byte(data), &back); err != nil {
			t.Fatalf("%s: unmarshal %s: %v", name, data, err)
		}
		if back.AutoConfirm != stdin.AutoConfirm || !bytes.Equal(back.Bytes, stdin.Bytes) {
			t.Errorf("%s: %s came back as %+v", name, data, back)
		}
	}

	if got := marshal(t, Stdin{Bytes: []byte("y\n")}); got != `{"bytes":"eQo="}` {
		t.Errorf("bytes form = %s", got)
	}

	var stdin Stdin
	if err := json.Unmarshal([]byte(`"interactive"`), &stdin); err == nil {
		t.Error("an unknown word: want an error")
	}
}

// --- Refusal ---

// TestRefusal_Wire pins the named policy failure to 2.7 § Message reference and proves a caller can errors.As it.
func TestRefusal_Wire(t *testing.T) {

	var err error = &Refusal{Reason: "account root is not offered to uid 501 by this broker", Offer: "off_host_root"}

	want := `{"error":"account root is not offered to uid 501 by this broker","offer":"off_host_root"}`
	if got := marshal(t, err); got != want {
		t.Errorf("refusal:\n got %s\nwant %s", got, want)
	}

	var refusal *Refusal
	if !errors.As(err, &refusal) || refusal.Offer != "off_host_root" {
		t.Errorf("errors.As: got %v", refusal)
	}
	if err.Error() != "account root is not offered to uid 501 by this broker" {
		t.Errorf("Error() = %q", err.Error())
	}
}

// --- PostRequest ---

// TestPostRequest_Validate pins what a broker refuses: a missing run, unit, account or graph.
func TestPostRequest_Validate(t *testing.T) {

	good := PostRequest{Run: "r", Unit: "u", Account: "root", Graph: []byte("{}")}
	if err := good.Validate(); err != nil {
		t.Fatalf("a good request: %v", err)
	}

	cases := map[string]PostRequest{
		"no run":     {Unit: "u", Account: "root", Graph: []byte("{}")},
		"no unit":    {Run: "r", Account: "root", Graph: []byte("{}")},
		"no account": {Run: "r", Unit: "u", Graph: []byte("{}")},
		"no graph":   {Run: "r", Unit: "u", Account: "root"},
	}
	for name, request := range cases {
		if err := request.Validate(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

// --- ConsoleFrame ---

// TestConsoleFrame_Wire pins the three payloads to 2.7 § The wire, in detail: the data: field of each SSE event.
func TestConsoleFrame_Wire(t *testing.T) {

	cases := []struct {
		frame ConsoleFrame
		want  string
	}{
		{NarrationFrame("e-41", "u-12", []byte("Installing openssh-server\n")),
			`{"exec":"e-41","unit":"u-12","len":26,"bytes":"SW5zdGFsbGluZyBvcGVuc3NoLXNlcnZlcgo="}`},
		{OutputFrame("e-42", []byte("abc")), `{"exec":"e-42","len":3,"bytes":"YWJj"}`},
		{ControlFrame("e-41", 0, nil), `{"exec":"e-41","exit_status":0}`},
		{ControlFrame("e-43", -1, errors.New("canceled")), `{"exec":"e-43","exit_status":-1,"error":"canceled"}`},
	}
	for _, c := range cases {
		data, err := c.frame.Data()
		if err != nil {
			t.Fatalf("%s: %v", c.frame.Channel, err)
		}
		if string(data) != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.frame.Channel, data, c.want)
		}
		if c.frame.Exec() == "" {
			t.Errorf("%s: Exec() is empty", c.frame.Channel)
		}
	}
}

// TestConsoleFrame_NoPayload pins that a frame whose channel names no payload is an error, not an empty object.
func TestConsoleFrame_NoPayload(t *testing.T) {

	if _, err := (ConsoleFrame{Channel: ChannelOutput}).Data(); err == nil {
		t.Error("want an error")
	}
	if got := (ConsoleFrame{}).Exec(); got != "" {
		t.Errorf("Exec() = %q", got)
	}
}

// TestOutput_RoundTrip pins that an output payload reads back from the wire, and that a length that disagrees with
// the bytes is refused.
func TestOutput_RoundTrip(t *testing.T) {

	var output Output
	if err := json.Unmarshal([]byte(`{"exec":"e-42","len":3,"bytes":"YWJj"}`), &output); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if output.Exec != "e-42" || string(output.Bytes) != "abc" {
		t.Errorf("came back as %+v", output)
	}

	if err := json.Unmarshal([]byte(`{"exec":"e-42","len":2,"bytes":"YWJj"}`), &output); err == nil {
		t.Error("a wrong len: want an error")
	}
}

// TestNarration_RoundTrip pins that a narration payload reads back from the wire byte for byte, carriage returns and
// invalid UTF-8 included, and that a length that disagrees with the bytes is refused.
func TestNarration_RoundTrip(t *testing.T) {

	raw := []byte("10%\r50%\r100%\n\xff\xfe")
	data, err := json.Marshal(Narration{Exec: "e-1", Unit: "u", Bytes: raw})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var back Narration
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal %s: %v", data, err)
	}
	if !bytes.Equal(back.Bytes, raw) || back.Exec != "e-1" || back.Unit != "u" {
		t.Errorf("came back as %+v", back)
	}

	if err := json.Unmarshal([]byte(`{"exec":"e-1","len":2,"bytes":"YWJj"}`), &back); err == nil {
		t.Error("a wrong len: want an error")
	}
}
