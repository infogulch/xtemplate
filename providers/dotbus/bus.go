package dotbus

import (
	"fmt"
	"sync"
)

// DefaultBuffer is the per-subscriber channel capacity when config Buffer is 0.
const DefaultBuffer = 16

var errClosed = fmt.Errorf("bus closed")

// Bus is a process-local multi-producer multi-consumer topic fan-out.
//
// Publish is best-effort: if a subscriber's buffer is full, that message is
// dropped for that subscriber (never blocks the publisher). Suitable for SSE
// and other live UI fan-out; not a durable queue.
type Bus struct {
	mu      sync.Mutex
	buffer  int
	topics  map[string]map[chan string]struct{}
	reverse map[chan string]map[string]struct{}
	closed  bool
}

// New returns a Bus. buffer is the capacity of each subscriber channel;
// values less than 1 are treated as DefaultBuffer.
func New(buffer int) *Bus {
	if buffer < 1 {
		buffer = DefaultBuffer
	}
	return &Bus{
		buffer:  buffer,
		topics:  make(map[string]map[chan string]struct{}),
		reverse: make(map[chan string]map[string]struct{}),
	}
}

// Publish sends msg to all current subscribers of topic. Slow subscribers
// (full buffer) skip the message. Returns an error if the bus is shut down or
// topic is empty.
func (b *Bus) Publish(topic, msg string) error {
	if topic == "" {
		return fmt.Errorf("bus: topic is required")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return errClosed
	}
	for ch := range b.topics[topic] {
		select {
		case ch <- msg:
		default:
			// drop for this subscriber
		}
	}
	return nil
}

// Subscribe registers a new subscriber on topic and returns its receive
// channel. The channel is closed when Unsubscribe is called or the bus is shut
// down. topic must be non-empty.
func (b *Bus) Subscribe(topic string) (<-chan string, error) {
	if topic == "" {
		return nil, fmt.Errorf("bus: topic is required")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, errClosed
	}
	ch := make(chan string, b.buffer)
	if b.topics[topic] == nil {
		b.topics[topic] = make(map[chan string]struct{})
	}
	b.topics[topic][ch] = struct{}{}
	if b.reverse[ch] == nil {
		b.reverse[ch] = make(map[string]struct{})
	}
	b.reverse[ch][topic] = struct{}{}
	return ch, nil
}

// Unsubscribe removes ch from all topics and closes it. Safe to call more than
// once or after Shutdown; subsequent calls are no-ops.
func (b *Bus) Unsubscribe(ch <-chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.removeLocked(ch)
}

func (b *Bus) removeLocked(ch <-chan string) {
	for c, topics := range b.reverse {
		if (<-chan string)(c) != ch {
			continue
		}
		for t := range topics {
			delete(b.topics[t], c)
			if len(b.topics[t]) == 0 {
				delete(b.topics, t)
			}
		}
		delete(b.reverse, c)
		close(c)
		return
	}
}

// Shutdown closes every subscriber channel and rejects further Publish/Subscribe.
// Safe to call more than once.
func (b *Bus) Shutdown() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for c := range b.reverse {
		close(c)
	}
	b.topics = make(map[string]map[chan string]struct{})
	b.reverse = make(map[chan string]map[string]struct{})
}
