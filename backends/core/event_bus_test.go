package core

import (
	"testing"
	"time"

	"github.com/sockerless/api"
)

func TestEventBus_PublishSubscribe(t *testing.T) {
	eb := NewEventBus()
	defer eb.Close()

	ch := eb.Subscribe("sub1")
	eb.Publish(api.Event{Type: "container", Action: "create"})

	select {
	case ev := <-ch:
		if ev.Type != "container" || ev.Action != "create" {
			t.Fatalf("unexpected event: %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestEventBus_MultipleSubscribers(t *testing.T) {
	eb := NewEventBus()
	defer eb.Close()

	ch1 := eb.Subscribe("sub1")
	ch2 := eb.Subscribe("sub2")
	eb.Publish(api.Event{Type: "network", Action: "destroy"})

	for _, ch := range []<-chan api.Event{ch1, ch2} {
		select {
		case ev := <-ch:
			if ev.Type != "network" || ev.Action != "destroy" {
				t.Fatalf("unexpected event: %+v", ev)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for event")
		}
	}
}

func TestEventBus_Unsubscribe(t *testing.T) {
	eb := NewEventBus()
	defer eb.Close()

	ch := eb.Subscribe("sub1")
	eb.Unsubscribe("sub1")

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Fatal("channel should be closed after unsubscribe")
	}
}
