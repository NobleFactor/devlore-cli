// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package server_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/server"

	// Register the flow provider so the graph root ("flow.subgraph") resolves.
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/flow/gen"
)

// gateReached signals that gate.Wait has begun; gateRelease unblocks it. Package-level so the announced provider and
// the single integration test share them.
var (
	gateReached = make(chan struct{}, 4)
	gateRelease = make(chan struct{})
)

// gate is a test provider whose Wait action blocks a running node until released, so a command can be issued while
// the run is deterministically in-flight.
type gate struct{ op.ProviderBase }

// Wait signals it has begun, then blocks until gateRelease is closed.
func (g *gate) Wait() error {
	gateReached <- struct{}{}
	<-gateRelease
	return nil
}

func init() {
	op.AnnounceProvider(reflect.TypeFor[gate](), op.RoleAction,
		func(runtimeEnvironment *op.RuntimeEnvironment) (any, error) {
			return &gate{ProviderBase: op.NewProviderBase(runtimeEnvironment)}, nil
		},
		map[string]op.MethodMetadata{"Wait": {}})
}

// TestServer_CommandAndEvents drives the whole wire end to end against a real run: a client subscribes to the SSE
// event stream, issues a pause over the command endpoint while the run is in-flight, and observes both the
// request/response ack (phase=paused) and the pushed phase-change event on the stream.
func TestServer_CommandAndEvents(t *testing.T) {

	gateRelease = make(chan struct{}) // fresh per run

	gateAction, err := op.ReceiverRegistry().BuildAction("gate.wait")
	if err != nil {
		t.Fatalf("BuildAction(gate.wait): %v", err)
	}
	node1, err := op.NewNode(op.NewNodeSpec().WithID("gate-1").WithAction(gateAction))
	if err != nil {
		t.Fatalf("NewNode(gate-1): %v", err)
	}
	node2, err := op.NewNode(op.NewNodeSpec().WithID("gate-2").WithAction(gateAction))
	if err != nil {
		t.Fatalf("NewNode(gate-2): %v", err)
	}
	graph, err := op.NewGraph(op.NewGraphSpec().WithOrigin(op.OriginBase{}).WithUnits(node1, node2))
	if err != nil {
		t.Fatalf("NewGraph: %v", err)
	}

	spec := op.NewRuntimeEnvironmentSpec("test").WithApplication(&application.Application{Name: "test"})
	executor := op.NewGraphExecutor(graph, spec)

	router := server.NewRouter()
	router.Register("run-1", executor.Control(), executor.RunStatus)
	ts := httptest.NewServer(router.Handler())
	defer ts.Close()

	// Subscribe first: http.Get returns once the handler has flushed the SSE response headers, which it does after
	// subscribing to the plane — so the subscription is live before the run emits anything.
	eventsReq, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ts.URL+"/v1/runs/run-1/events", http.NoBody)
	if err != nil {
		t.Fatalf("build events request: %v", err)
	}
	eventsResp, err := http.DefaultClient.Do(eventsReq)
	if err != nil {
		t.Fatalf("GET events: %v", err)
	}
	defer func() { _ = eventsResp.Body.Close() }()

	runDone := make(chan error, 1)
	go func() { _, runErr := executor.Run(context.Background(), nil); runDone <- runErr }()

	// gate-1 is running: the run is in-flight, blocked inside Wait.
	select {
	case <-gateReached:
	case <-time.After(2 * time.Second):
		t.Fatal("gate-1 never reached — the run did not start")
	}

	// Release gate-1 shortly after the pause POST below has reached the server and enqueued its command, so gate-2's
	// control-point drains the pause instead of dispatching gate-2.
	go func() {
		time.Sleep(150 * time.Millisecond)
		close(gateRelease)
	}()

	// The pause POST blocks until the run serves it at gate-2's control-point; the JSON body is the ack.
	pauseReq, err := http.NewRequestWithContext(t.Context(), http.MethodPost, ts.URL+"/v1/runs/run-1/commands",
		strings.NewReader(`{"command":"pause","request_id":"c-1"}`))
	if err != nil {
		t.Fatalf("build pause request: %v", err)
	}
	pauseReq.Header.Set("Content-Type", "application/json")
	postResp, err := http.DefaultClient.Do(pauseReq)
	if err != nil {
		t.Fatalf("POST pause: %v", err)
	}
	defer func() { _ = postResp.Body.Close() }()

	if postResp.StatusCode != http.StatusOK {
		t.Fatalf("POST pause: status = %d, want 200", postResp.StatusCode)
	}
	var ack struct {
		Status    *op.RunStatus `json:"status"`
		RequestID string        `json:"request_id"`
	}
	if err := json.NewDecoder(postResp.Body).Decode(&ack); err != nil {
		t.Fatalf("decode ack: %v", err)
	}
	if ack.Status == nil || ack.Status.Phase != op.PhasePaused {
		t.Errorf("ack = %+v, want status.phase = paused", ack)
	}
	if ack.RequestID != "c-1" {
		t.Errorf("ack request_id = %q, want c-1 (echoed)", ack.RequestID)
	}

	// The run parked at paused.
	if runErr := <-runDone; !errors.Is(runErr, op.ErrPaused) {
		t.Errorf("Run: err = %v, want ErrPaused", runErr)
	}

	// The SSE stream carried the paused phase-change event.
	if !readSSEUntil(eventsResp.Body, 2*time.Second, func(kind string, data map[string]any) bool {
		return kind == "phase_changed" && data["phase"] == "paused"
	}) {
		t.Error("SSE stream did not carry the paused phase-change event")
	}
}

// readSSEUntil reads SSE frames from `body` until `match(kind, data)` returns true or `timeout` elapses.
func readSSEUntil(body io.Reader, timeout time.Duration, match func(kind string, data map[string]any) bool) bool {

	found := make(chan bool, 1)

	go func() {
		scanner := bufio.NewScanner(body)
		var kind string
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				kind = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				var data map[string]any
				if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &data) == nil && match(kind, data) {
					found <- true
					return
				}
			}
		}
		found <- false
	}()

	select {
	case ok := <-found:
		return ok
	case <-time.After(timeout):
		return false
	}
}
