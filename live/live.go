// Package live tells users' open pages when group data changed.
package live

import (
	"sync"

	"github.com/mgnsk/webkit/snowflake"
)

// Event is a change in a group.
type Event struct {
	GroupID snowflake.ID
}

// Broker fans out events to the open pages of users. It is in memory, so
// it only reaches pages connected to this process.
type Broker struct {
	mu     sync.Mutex
	subs   map[snowflake.ID]map[chan Event]struct{}
	done   chan struct{}
	closed bool
}

// NewBroker creates a broker.
func NewBroker() *Broker {
	return &Broker{
		subs: map[snowflake.ID]map[chan Event]struct{}{},
		done: make(chan struct{}),
	}
}

// Subscribe returns a channel of the user's events and a function that must
// be called to unsubscribe.
func (b *Broker) Subscribe(userID snowflake.ID) (<-chan Event, func()) {
	ch := make(chan Event, 16)

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.subs[userID] == nil {
		b.subs[userID] = map[chan Event]struct{}{}
	}
	b.subs[userID][ch] = struct{}{}

	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(b.subs[userID], ch)
		if len(b.subs[userID]) == 0 {
			delete(b.subs, userID)
		}
	}
}

// Publish sends e to the open pages of users. A page that is not keeping up
// misses the event: it already has one pending, which refreshes it anyway.
func (b *Broker) Publish(userIDs []snowflake.ID, e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, id := range userIDs {
		for ch := range b.subs[id] {
			select {
			case ch <- e:
			default:
			}
		}
	}
}

// Done is closed when the broker is closed.
func (b *Broker) Done() <-chan struct{} {
	return b.done
}

// Close ends all subscriptions, e.g. on server shutdown.
func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.closed {
		b.closed = true
		close(b.done)
	}
}
