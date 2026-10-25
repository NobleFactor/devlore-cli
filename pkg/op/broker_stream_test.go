// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"sync"
	"testing"
	"time"
)

// --- brokerStream ---

// TestBrokerStream_OrderAcrossSubscribers pins that every subscriber receives every value, in publish order, however
// differently they read.
func TestBrokerStream_OrderAcrossSubscribers(t *testing.T) {

	var mu sync.Mutex
	stream := newBrokerStream[int]()

	mu.Lock()
	fast, cancelFast := stream.subscribe(&mu)
	slow, cancelSlow := stream.subscribe(&mu)
	for i := range 1000 {
		stream.publish(i)
	}
	mu.Unlock()
	defer cancelFast()
	defer cancelSlow()

	for want := range 1000 {
		if got := <-fast; got != want {
			t.Fatalf("fast subscriber: got %d, want %d", got, want)
		}
	}
	for want := range 1000 {
		if got := <-slow; got != want {
			t.Fatalf("slow subscriber: got %d, want %d", got, want)
		}
	}
}

// TestBrokerStream_CancelDiscardsAndCloses pins cancel: the subscription leaves the stream, what it still had queued
// is discarded, its channel is closed, its drain has ended, and a second cancel is harmless.
func TestBrokerStream_CancelDiscardsAndCloses(t *testing.T) {

	var mu sync.Mutex
	stream := newBrokerStream[int]()

	mu.Lock()
	values, cancel := stream.subscribe(&mu)
	var sub *subscription[int]
	for s := range stream.subscribers {
		sub = s
	}
	for i := range 100 {
		stream.publish(i)
	}
	mu.Unlock()

	cancel()
	cancel()

	mu.Lock()
	remaining := len(stream.subscribers)
	mu.Unlock()
	if remaining != 0 {
		t.Errorf("after cancel: %d subscribers remain", remaining)
	}

	select {
	case <-sub.finished:
	case <-time.After(5 * time.Second):
		t.Fatal("the drain did not end")
	}

	// The channel is closed; at most the one value the drain held when cancel came may still be read.
	read := 0
	for range values {
		read++
	}
	if read > 1 {
		t.Errorf("after cancel: read %d values, want the queue discarded", read)
	}
}

// TestBrokerStream_PublishAfterCancelIsHarmless pins that a value published after a subscription is canceled is
// neither delivered nor a panic.
func TestBrokerStream_PublishAfterCancelIsHarmless(t *testing.T) {

	var mu sync.Mutex
	stream := newBrokerStream[int]()

	mu.Lock()
	values, cancel := stream.subscribe(&mu)
	mu.Unlock()
	cancel()

	mu.Lock()
	stream.publish(1)
	mu.Unlock()

	if _, open := <-values; open {
		t.Error("a value reached a canceled subscription")
	}
}
