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
}

// NewEventBus creates a new EventBus.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[string]chan api.Event),
	}
}

// Publish sends an event to all subscribers (non-blocking).
func (eb *EventBus) Publish(event api.Event) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	if eb.closed {
		return
	}
	for _, ch := range eb.subscribers {
		select {
		case ch <- event:
		default:
			// Drop event for slow subscribers to avoid blocking
		}
	}
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

// emitEvent is a convenience method on BaseServer to publish a Docker-compatible event.
func (s *BaseServer) emitEvent(eventType, action, actorID string, attrs map[string]string) {
	if s.EventBus == nil {
		return
	}
	s.EventBus.Publish(api.Event{
		Type:   eventType,
		Action: action,
		Actor: api.EventActor{
			ID:         actorID,
			Attributes: attrs,
		},
		Time:     time.Now().Unix(),
		TimeNano: time.Now().UnixNano(),
	})
}
