package core

import (
	"sync"
	"time"

	"github.com/sockerless/api"
)

// EventBus manages event publishing and subscription for Docker-compatible events.
type EventBus struct {
	mu          sync.Mutex
	subscribers map[string]chan api.Event
	closed      bool
	history     []api.Event  // BUG-520: ring buffer for since/until replay
	maxHistory  int
}

// NewEventBus creates a new EventBus.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[string]chan api.Event),
		maxHistory:  1024,
	}
}

// Publish sends an event to all subscribers (non-blocking).
func (eb *EventBus) Publish(event api.Event) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	if eb.closed {
		return
	}
	// BUG-520: Store event in history for since/until replay
	if len(eb.history) >= eb.maxHistory {
		eb.history = eb.history[1:]
	}
	eb.history = append(eb.history, event)
	for _, ch := range eb.subscribers {
		select {
		case ch <- event:
		default:
			// Drop event for slow subscribers to avoid blocking
		}
	}
}

// History returns events with Time >= since. If until > 0, only events with Time <= until.
// BUG-520: Support replaying past events.
func (eb *EventBus) History(since, until int64) []api.Event {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	var result []api.Event
	for _, ev := range eb.history {
		if ev.Time < since {
			continue
		}
		if until > 0 && ev.Time > until {
			continue
		}
		result = append(result, ev)
	}
	return result
}

// Subscribe creates a new subscription and returns an event channel.
func (eb *EventBus) Subscribe(id string) <-chan api.Event {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	ch := make(chan api.Event, 64)
	eb.subscribers[id] = ch
	return ch
}

// Unsubscribe removes a subscriber and closes its channel.
func (eb *EventBus) Unsubscribe(id string) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	if ch, ok := eb.subscribers[id]; ok {
		close(ch)
		delete(eb.subscribers, id)
	}
}

// Close shuts down the event bus and closes all subscriber channels.
func (eb *EventBus) Close() {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.closed = true
	for id, ch := range eb.subscribers {
		close(ch)
		delete(eb.subscribers, id)
	}
}

// EmitEvent is the exported version of emitEvent for use by cloud backends.
func (s *BaseServer) EmitEvent(eventType, action, actorID string, attrs map[string]string) {
	s.emitEvent(eventType, action, actorID, attrs)
}

// emitEvent is a convenience method on BaseServer to publish a Docker-compatible event.
func (s *BaseServer) emitEvent(eventType, action, actorID string, attrs map[string]string) {
	if s.EventBus == nil {
		return
	}
	now := time.Now()
	s.EventBus.Publish(api.Event{
		Type:   eventType,
		Action: action,
		Scope:  "local",
		Actor: api.EventActor{
			ID:         actorID,
			Attributes: attrs,
		},
		Time:     now.Unix(),
		TimeNano: now.UnixNano(),
	})
}
