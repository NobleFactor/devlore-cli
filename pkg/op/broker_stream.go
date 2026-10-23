// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import "sync"

// brokerStream is one per-run stream of a broker that lives in this process: its seq and its subscribers. The
// [RecordingBroker] and the [InProcessBroker] each hold one per run per kind, console and events, and guard it with
// their own mutex; nothing here locks.
type brokerStream[T any] struct {

	// seq is the last seq stamped; the next is seq+1.
	seq uint64

	// subscribers is the set of live channels.
	subscribers map[chan T]struct{}
}

// newBrokerStream returns an empty stream.
//
// Returns:
//   - `*brokerStream[T]`: the stream.
func newBrokerStream[T any]() *brokerStream[T] {
	return &brokerStream[T]{subscribers: make(map[chan T]struct{})}
}

// region UNEXPORTED METHODS

// region Behaviors

// next returns the next seq.
//
// Returns:
//   - `uint64`: the seq, starting at 1.
func (s *brokerStream[T]) next() uint64 {

	s.seq++
	return s.seq
}

// publish sends the value to every subscriber without blocking; a full subscriber misses it, and the gap in seq
// shows. The spool that never drops is the runner's, in iteration 3 (6.2 § 4.1).
//
// Parameters:
//   - `value`: the value.
func (s *brokerStream[T]) publish(value T) {

	for ch := range s.subscribers {
		select {
		case ch <- value:
		default:
		}
	}
}

// subscribe adds a subscriber and returns its channel and an idempotent cancel that closes it. The cancel takes the
// lock the caller passes, since the subscriber set is the broker's to guard.
//
// Parameters:
//   - `mu`: the broker's mutex, held by the caller now and taken by cancel later.
//
// Returns:
//   - `<-chan T`: the subscription.
//   - `func()`: the cancel.
func (s *brokerStream[T]) subscribe(mu *sync.Mutex) (subscription <-chan T, cancel func()) {

	ch := make(chan T, brokerStreamBuffer)
	s.subscribers[ch] = struct{}{}

	var once sync.Once
	cancel = func() {
		once.Do(func() {
			mu.Lock()
			defer mu.Unlock()
			delete(s.subscribers, ch)
			close(ch)
		})
	}
	return ch, cancel
}

// endregion

// endregion

// brokerStreamBuffer is how many frames a subscriber may fall behind before it misses one.
const brokerStreamBuffer = 256
