// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import "sync"

// brokerStream is one per-run stream of a broker that lives in this process: its seq and its subscribers. The
// [RecordingBroker] and the [InProcessBroker] each hold one per run per kind, console and events, and guard the seq
// and the subscriber set with their own mutex.
//
// Delivery never drops and never blocks (#1131): each subscriber has a queue of its own, drained into its channel by
// a goroutine of its own, so [brokerStream.publish] returns at once however slow a subscriber is, and the subscriber
// receives every value, in order. The spill to a file and the ring for reconnects are the broker's of iteration 2
// (architecture 2.7 § The two streams).
type brokerStream[T any] struct {

	// seq is the last seq stamped; the next is seq+1.
	seq uint64

	// subscribers is the set of live subscriptions.
	subscribers map[*subscription[T]]struct{}
}

// newBrokerStream returns an empty stream.
//
// Returns:
//   - `*brokerStream[T]`: the stream.
func newBrokerStream[T any]() *brokerStream[T] {
	return &brokerStream[T]{subscribers: make(map[*subscription[T]]struct{})}
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

// publish queues the value for every subscriber and returns; it never blocks and never drops. The caller holds the
// broker's mutex, so values are queued in seq order.
//
// Parameters:
//   - `value`: the value.
func (s *brokerStream[T]) publish(value T) {

	for sub := range s.subscribers {
		sub.enqueue(value)
	}
}

// subscribe adds a subscriber and returns its channel and an idempotent cancel. The cancel takes the broker's mutex,
// which the caller holds now, to leave the subscriber set; it then stops the subscriber's goroutine, discards what
// is still queued, and closes the channel.
//
// Parameters:
//   - `mu`: the broker's mutex, held by the caller now and taken by cancel later.
//
// Returns:
//   - `<-chan T`: the subscription.
//   - `func()`: the cancel.
func (s *brokerStream[T]) subscribe(mu *sync.Mutex) (subscription <-chan T, cancel func()) {

	sub := newSubscription[T]()
	s.subscribers[sub] = struct{}{}

	var once sync.Once
	cancel = func() {
		once.Do(func() {
			mu.Lock()
			delete(s.subscribers, sub)
			mu.Unlock()
			sub.stop()
		})
	}
	return sub.out, cancel
}

// endregion

// endregion

// region SUPPORTING TYPES

// subscription is one subscriber's queue, the goroutine that drains it, and the channel it drains into.
type subscription[T any] struct {

	// mu guards queue and stopped; it is the subscription's own, so a slow channel never holds the broker's mutex.
	mu sync.Mutex

	// queue holds the values not yet handed to out, in order.
	queue []T

	// stopped is set by stop; the drain then ends and discards the queue.
	stopped bool

	// ready signals the drain that the queue gained a value or the subscription stopped; one pending signal is enough.
	ready chan struct{}

	// done is closed by stop, so a drain blocked handing a value to out wakes and ends.
	done chan struct{}

	// out is the subscriber's channel, closed when the drain ends.
	out chan T

	// finished is closed when the drain has ended and out is closed.
	finished chan struct{}
}

// newSubscription starts a subscription's drain.
//
// Returns:
//   - `*subscription[T]`: the subscription, its drain running.
func newSubscription[T any]() *subscription[T] {

	sub := &subscription[T]{
		ready:    make(chan struct{}, 1),
		done:     make(chan struct{}),
		out:      make(chan T),
		finished: make(chan struct{}),
	}
	go sub.drain()
	return sub
}

// region UNEXPORTED METHODS

// region Behaviors

// drain hands queued values to out, in order, until the subscription stops; then it closes out. It runs on the
// subscription's own goroutine.
func (s *subscription[T]) drain() {

	defer close(s.finished)
	defer close(s.out)

	for {
		s.mu.Lock()
		if s.stopped {
			s.queue = nil
			s.mu.Unlock()
			return
		}
		if len(s.queue) == 0 {
			s.mu.Unlock()
			select {
			case <-s.ready:
				continue
			case <-s.done:
				continue
			}
		}
		value := s.queue[0]
		var zero T
		s.queue[0] = zero
		s.queue = s.queue[1:]
		s.mu.Unlock()

		select {
		case s.out <- value:
		case <-s.done:
		}
	}
}

// enqueue appends a value and wakes the drain; it never blocks.
//
// Parameters:
//   - `value`: the value.
func (s *subscription[T]) enqueue(value T) {

	s.mu.Lock()
	if !s.stopped {
		s.queue = append(s.queue, value)
	}
	s.mu.Unlock()

	select {
	case s.ready <- struct{}{}:
	default:
	}
}

// stop ends the drain, discards what is queued, and waits until out is closed.
func (s *subscription[T]) stop() {

	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
	close(s.done)
	<-s.finished
}

// endregion

// endregion

// endregion
