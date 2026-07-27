// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package server is the HTTP/2 wire listener for op runs (architecture 2.7) — it bridges a remote consumer to a run's
// in-process [op.ControlPlane] over REST commands and Server-Sent Events, so the whole bidirectional channel is
// drivable with curl.
//
// A run registers its plane under a run id ([Router.Register]); the [Router] is a stateless run-id → plane lookup, so
// commands and the event stream are independent across the same or different connections. Three endpoints:
//
//   - POST /v1/runs/{runID}/commands — body {"command": "pause"|"stop"|"step", "request_id"?}; the JSON response body
//     is the response frame (the resulting [op.RunStatus], or an error).
//   - GET  /v1/runs/{runID}/events   — a text/event-stream the run pushes onto (the REGISTER → EVENT axis).
//   - GET  /v1/runs/{runID}          — the current [op.RunStatus] (a plain poll).
//
// The handler is served over cleartext HTTP/2 (h2c) so a single connection multiplexes the SSE stream and command
// POSTs as independent streams; HTTP/1.1 clients work too (the endpoints are transport-agnostic).
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Router routes HTTP requests to registered runs' control planes.
//
// One router can front many concurrent runs; each registers its plane under a run id. The zero value is not usable —
// construct via [NewRouter].
type Router struct {
	mu   sync.RWMutex
	runs map[string]registration
}

// registration is a run's control surface as the router sees it: the plane commands and events flow through, and a
// status accessor for the poll endpoint and the terminal-phase guard.
type registration struct {
	plane  *op.ControlPlane
	status func() op.RunStatus
}

// NewRouter returns an empty control-plane HTTP router.
//
// Returns:
//   - *Router: the constructed router, ready for [Router.Register] and [Router.Handler].
func NewRouter() *Router {
	return &Router{runs: make(map[string]registration)}
}

// region EXPORTED METHODS

// region Behaviors

// Register makes `plane` reachable under `runID` and returns an unregister func.
//
// The caller (a run's driver) registers when a run starts and calls the returned func when it ends. `status` is the
// run's current-status accessor — typically [GraphExecutor.RunStatus] as a method value — read by the status endpoint
// and to reject a command on a run that has already reached a terminal phase.
//
// Parameters:
//   - `runID`: the id the run is addressed by on the wire.
//   - `plane`: the run's control plane.
//   - `status`: the run's current-[op.RunStatus] accessor.
//
// Returns:
//   - `func()`: the unregister; idempotent.
func (s *Router) Register(runID string, plane *op.ControlPlane, status func() op.RunStatus) func() {

	s.mu.Lock()
	s.runs[runID] = registration{plane: plane, status: status}
	s.mu.Unlock()

	return func() {
		s.mu.Lock()
		delete(s.runs, runID)
		s.mu.Unlock()
	}
}

// Handler returns the HTTP handler serving the control endpoints, wrapped for cleartext HTTP/2 (h2c).
//
// Mount it on an [*http.Server] (or an [httptest.Server]); it also handles HTTP/1.1 transparently.
//
// Returns:
//   - `http.Handler`: the h2c-wrapped router.
func (s *Router) Handler() http.Handler {

	mux := http.NewServeMux()

	mux.HandleFunc("POST /v1/runs/{runID}/commands", s.handleCommand)
	mux.HandleFunc("GET /v1/runs/{runID}/events", s.handleEvents)
	mux.HandleFunc("GET /v1/runs/{runID}", s.handleStatus)

	return h2c.NewHandler(mux, &http2.Server{})
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// lookup returns the registration for `runID`.
//
// Parameters:
//   - `runID`: the run to resolve.
//
// Returns:
//   - `registration`: the run's surface; zero value when absent.
//   - `bool`: true when the run is registered.
func (s *Router) lookup(runID string) (registration, bool) {

	s.mu.RLock()
	defer s.mu.RUnlock()

	run, ok := s.runs[runID]
	return run, ok
}

// handleCommand serves POST /v1/runs/{runID}/commands — the request/response half.
//
// It decodes the command, rejects it on an unknown run / bad body / unknown verb / terminal run, else issues it on
// the plane and blocks (interruptibly) for the one response the executor sends at its next control-point.
func (s *Router) handleCommand(w http.ResponseWriter, r *http.Request) {

	run, ok := s.lookup(r.PathValue("runID"))
	if !ok {
		writeJSON(w, http.StatusNotFound, commandResponse{Error: "unknown run"})
		return
	}

	var request commandRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, commandResponse{Error: "invalid command body"})
		return
	}

	command, known := parseCommand(request.Command)
	if !known {
		writeJSON(w, http.StatusBadRequest,
			commandResponse{Error: fmt.Sprintf("unknown command %q", request.Command), RequestID: request.RequestID})
		return
	}

	// A run past its control-points (terminal, or paused) will never serve the command; reject rather than hang the
	// request until the client gives up. Best-effort: the status may change under us, and the interruptible receive
	// below is the backstop.
	if phase := run.status().Phase; phase != op.PhaseRunning {
		writeJSON(w, http.StatusConflict,
			commandResponse{Error: fmt.Sprintf("run is %s, not running", phase), RequestID: request.RequestID})
		return
	}

	select {
	case response := <-run.plane.Request(command):
		if response.Err != nil {
			writeJSON(w, http.StatusConflict,
				commandResponse{Error: response.Err.Error(), RequestID: request.RequestID})
			return
		}
		status := response.Status
		writeJSON(w, http.StatusOK, commandResponse{Status: &status, RequestID: request.RequestID})

	case <-r.Context().Done():
		writeJSON(w, http.StatusServiceUnavailable,
			commandResponse{Error: "canceled before the run served the command", RequestID: request.RequestID})
	}
}

// handleEvents serves GET /v1/runs/{runID}/events — the Server-Sent Events push stream.
//
// It subscribes to the run's plane and streams each event as an SSE frame until the client disconnects (request
// context done) or the subscription closes.
func (s *Router) handleEvents(w http.ResponseWriter, r *http.Request) {

	run, ok := s.lookup(r.PathValue("runID"))
	if !ok {
		http.Error(w, "unknown run", http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	events, cancel := run.plane.Subscribe()
	defer cancel()

	for {
		select {
		case event, open := <-events:
			if !open {
				return
			}
			writeSSE(w, event)
			flusher.Flush()

		case <-r.Context().Done():
			return
		}
	}
}

// handleStatus serves GET /v1/runs/{runID} — the current-status poll.
func (s *Router) handleStatus(w http.ResponseWriter, r *http.Request) {

	run, ok := s.lookup(r.PathValue("runID"))
	if !ok {
		http.Error(w, "unknown run", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, run.status())
}

// endregion

// endregion

// region HELPER FUNCTIONS

// writeJSON writes `body` as a JSON response with `code`.
//
// Parameters:
//   - `w`: the response writer.
//   - `code`: the HTTP status code.
//   - `body`: the value to encode.
func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	//nolint:errcheck // diagnose-ignored-error: response write; see docs/architecture/2.8-eventing-infrastructure.md
	_ = json.NewEncoder(w).Encode(body)
}

// writeSSE writes one [op.ControlEvent] as an SSE frame (`event:` = the kind, `data:` = the JSON payload).
//
// Parameters:
//   - `w`: the response writer.
//   - `event`: the event to frame.
func writeSSE(w http.ResponseWriter, event op.ControlEvent) {

	payload := eventPayload{Seq: event.Seq, RunStatus: event.Status, UnitID: event.UnitID}
	if event.Err != nil {
		payload.Error = event.Err.Error()
	}

	data := assert.Must(json.Marshal(payload))
	//nolint:errcheck // diagnose-ignored-error: SSE write; see docs/architecture/2.8-eventing-infrastructure.md
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventKindName(event.Kind), data)
}

// parseCommand maps a wire command string to an [op.ControlCommand].
//
// Parameters:
//   - `name`: the wire verb.
//
// Returns:
//   - `op.ControlCommand`: the mapped command.
//   - `bool`: true when `name` is a known verb.
func parseCommand(name string) (op.ControlCommand, bool) {
	switch name {
	case "pause":
		return op.ControlPause, true
	case "stop":
		return op.ControlStop, true
	case "step":
		return op.ControlStep, true
	default:
		return 0, false
	}
}

// eventKindName maps an [op.ControlEventKind] to its SSE event-name.
//
// Parameters:
//   - `kind`: the event kind.
//
// Returns:
//   - `string`: the SSE `event:` name.
func eventKindName(kind op.ControlEventKind) string {
	switch kind {
	case op.EventPhaseChanged:
		return "phase_changed"
	case op.EventError:
		return "error"
	case op.EventResult:
		return "result"
	default:
		return "event"
	}
}

// endregion

// region SUPPORTING TYPES

// commandRequest is the JSON body of a POST to the commands endpoint.
type commandRequest struct {

	// Command is the verb: "pause" / "stop" / "step".
	Command string `json:"command"`

	// RequestID is an optional client correlation token, echoed in the response.
	RequestID string `json:"request_id,omitempty"`

	// Count is the optional step count (default 1); reserved for "step".
	Count int `json:"count,omitempty"`
}

// commandResponse is the JSON body of the commands endpoint's response — the response frame.
type commandResponse struct {

	// Status is the run status the command produced; nil on rejection.
	Status *op.RunStatus `json:"status,omitempty"`

	// Error is the rejection detail; empty on success.
	Error string `json:"error,omitempty"`

	// RequestID echoes the request's correlation token.
	RequestID string `json:"request_id,omitempty"`
}

// eventPayload is the JSON `data:` of an SSE event frame — the run status at the event, its sequence, and the
// event-specific unit / error. The embedded [op.RunStatus] inlines phase / condition / reason / message.
type eventPayload struct {
	Seq uint64 `json:"seq"`
	op.RunStatus
	UnitID string `json:"unit,omitempty"`
	Error  string `json:"error,omitempty"`
}

// endregion
