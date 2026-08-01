// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// newTestServer registers one run under "run-1" with `status` and returns the started httptest server.
func newTestServer(t *testing.T, status func() op.RunStatus) *httptest.Server {
	t.Helper()
	s := NewRouter()
	s.Register("run-1", op.NewControlPlane(), status)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// TestServer_Status serves the current RunStatus as JSON with the design's string field names.
func TestServer_Status(t *testing.T) {

	ts := newTestServer(t, func() op.RunStatus {
		return op.RunStatus{Phase: op.PhasePaused, Condition: op.ConditionHealthy, Reason: op.ReasonPaused}
	})

	resp, err := http.Get(ts.URL + "/v1/runs/run-1")
	if err != nil {
		t.Fatalf("GET status: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["phase"] != "paused" || body["condition"] != "healthy" || body["reason"] != "paused" {
		t.Errorf("status body = %v, want phase=paused condition=healthy reason=paused", body)
	}
}

// TestServer_UnknownRun rejects every endpoint for an unregistered run.
func TestServer_UnknownRun(t *testing.T) {

	ts := newTestServer(t, func() op.RunStatus { return op.RunStatus{Phase: op.PhaseRunning} })

	for _, tc := range []struct {
		method, path string
	}{
		{http.MethodGet, "/v1/runs/nope"},
		{http.MethodGet, "/v1/runs/nope/events"},
		{http.MethodPost, "/v1/runs/nope/commands"},
	} {
		req, _ := http.NewRequest(tc.method, ts.URL+tc.path, strings.NewReader(`{"command":"pause"}`))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", tc.method, tc.path, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s %s: status = %d, want 404", tc.method, tc.path, resp.StatusCode)
		}
	}
}

// TestServer_Command_Rejections covers the command handler's guard paths (no plane service required): an unknown verb
// is 400, and a command on a non-running run is 409 — never a silent no-op.
func TestServer_Command_Rejections(t *testing.T) {

	t.Run("unknown verb", func(t *testing.T) {
		ts := newTestServer(t, func() op.RunStatus { return op.RunStatus{Phase: op.PhaseRunning} })
		resp := post(t, ts, `{"command":"levitate","request_id":"c-9"}`)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", resp.StatusCode)
		}
		if body := decode(t, resp); body.Error == "" || body.RequestID != "c-9" {
			t.Errorf("body = %+v, want an error naming the verb and echoing request_id", body)
		}
	})

	t.Run("run not running", func(t *testing.T) {
		ts := newTestServer(t, func() op.RunStatus { return op.RunStatus{Phase: op.PhaseCompleted} })
		resp := post(t, ts, `{"command":"pause"}`)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusConflict {
			t.Errorf("status = %d, want 409", resp.StatusCode)
		}
	})

	t.Run("bad body", func(t *testing.T) {
		ts := newTestServer(t, func() op.RunStatus { return op.RunStatus{Phase: op.PhaseRunning} })
		resp := post(t, ts, `{not json`)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", resp.StatusCode)
		}
	})
}

// TestServer_Unregister removes the run so subsequent requests 404.
func TestServer_Unregister(t *testing.T) {

	s := NewRouter()
	unregister := s.Register("run-1", op.NewControlPlane(), func() op.RunStatus { return op.RunStatus{} })
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	unregister()

	resp, err := http.Get(ts.URL + "/v1/runs/run-1")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("after unregister: status = %d, want 404", resp.StatusCode)
	}
}

// TestWriteSSE pins the SSE frame shape against the architecture-doc message reference: `event: <kind>` then a
// `data:` line whose JSON carries seq + the flattened RunStatus + the unit.
func TestWriteSSE(t *testing.T) {

	var b strings.Builder
	writeSSE(&sseRecorder{&b}, op.ControlEvent{
		Seq:    42,
		Kind:   op.EventPhaseChanged,
		Status: op.RunStatus{Phase: op.PhaseRunning, Condition: op.ConditionHealthy},
		UnitID: "file.mkdir-1",
	})

	got := b.String()
	if !strings.HasPrefix(got, "event: phase_changed\ndata: ") || !strings.HasSuffix(got, "\n\n") {
		t.Fatalf("frame shape = %q", got)
	}
	data := strings.TrimSuffix(strings.TrimPrefix(got, "event: phase_changed\ndata: "), "\n\n")
	var payload map[string]any
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		t.Fatalf("data not JSON: %v (%q)", err, data)
	}
	if payload["seq"].(float64) != 42 || payload["phase"] != "running" || payload["unit"] != "file.mkdir-1" {
		t.Errorf("payload = %v, want seq=42 phase=running unit=file.mkdir-1", payload)
	}
}

// --- helpers ---

func post(t *testing.T, ts *httptest.Server, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(ts.URL+"/v1/runs/run-1/commands", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func decode(t *testing.T, resp *http.Response) commandResponse {
	t.Helper()
	var body commandResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

// sseRecorder adapts a strings.Builder to the http.ResponseWriter shape writeSSE needs (only Write is used).
type sseRecorder struct{ b *strings.Builder }

func (r *sseRecorder) Header() http.Header         { return http.Header{} }
func (r *sseRecorder) Write(p []byte) (int, error) { return r.b.Write(p) }
func (r *sseRecorder) WriteHeader(int)             {}
