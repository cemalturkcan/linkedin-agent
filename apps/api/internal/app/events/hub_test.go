package events

import (
	"encoding/json"
	"strings"
	"testing"

	"api/internal/app/clock"
)

type payload struct {
	Note string `json:"note"`
}

func drain(t *testing.T, subscriber *Subscriber) []Frame {
	t.Helper()
	frames := make([]Frame, 0, RingCapacity)
	for {
		select {
		case frame := <-subscriber.State():
			frames = append(frames, frame)
		default:
			return frames
		}
	}
}

func TestEmitAssignsMonotonicIdsAndStampsTheFrame(t *testing.T) {
	hub := NewHub(clock.System{})
	subscriber, admitted := hub.Subscribe(0)
	if !admitted {
		t.Fatal("first subscriber was refused")
	}

	first := hub.Emit(TypeTrace, payload{Note: "one"})
	second := hub.Emit(TypeTrace, payload{Note: "two"})
	if first != 1 || second != 2 {
		t.Fatalf("ids = %d, %d", first, second)
	}

	frames := drain(t, subscriber)
	if len(frames) != 2 {
		t.Fatalf("received %d frames", len(frames))
	}
	var body map[string]any
	if err := json.Unmarshal(frames[0].Data, &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["type"] != string(TypeTrace) || body["note"] != "one" || body["at"] == nil {
		t.Fatalf("frame body = %v", body)
	}
}

func TestReplayStartsAfterTheLastSeenId(t *testing.T) {
	hub := NewHub(clock.System{})
	for index := range 5 {
		hub.Emit(TypeTrace, payload{Note: string(rune('a' + index))})
	}
	subscriber, _ := hub.Subscribe(3)
	frames := drain(t, subscriber)
	if len(frames) != 2 {
		t.Fatalf("replayed %d frames, want 2", len(frames))
	}
	if frames[0].ID != 4 || frames[1].ID != 5 {
		t.Fatalf("replayed ids %d and %d", frames[0].ID, frames[1].ID)
	}
}

func TestRingIsBoundedAndKeepsTheNewestEvents(t *testing.T) {
	hub := NewHub(clock.System{})
	for range RingCapacity + 20 {
		hub.Emit(TypeTrace, payload{Note: "x"})
	}
	subscriber, _ := hub.Subscribe(0)
	frames := drain(t, subscriber)
	if len(frames) != RingCapacity {
		t.Fatalf("replayed %d frames, want %d", len(frames), RingCapacity)
	}
	if frames[0].ID != 21 {
		t.Fatalf("oldest replayed id = %d, want 21", frames[0].ID)
	}
}

func TestSubscribersAreCapped(t *testing.T) {
	hub := NewHub(clock.System{})
	for index := range MaxSubscribers {
		if _, admitted := hub.Subscribe(0); !admitted {
			t.Fatalf("subscriber %d was refused below the cap", index)
		}
	}
	if _, admitted := hub.Subscribe(0); admitted {
		t.Fatal("the cap admitted one subscriber too many")
	}
}

func TestUnsubscribeFreesASlotAndClosesTheSubscriber(t *testing.T) {
	hub := NewHub(clock.System{})
	subscriber, _ := hub.Subscribe(0)
	hub.Unsubscribe(subscriber)
	select {
	case <-subscriber.Done():
	default:
		t.Fatal("unsubscribe left the subscriber open")
	}
	if hub.Subscribers() != 0 {
		t.Fatalf("hub still counts %d subscribers", hub.Subscribers())
	}
	hub.Unsubscribe(subscriber)
}

func TestDeltasCarryNoIdAndStayOutOfTheRing(t *testing.T) {
	hub := NewHub(clock.System{})
	subscriber, _ := hub.Subscribe(0)

	hub.EmitDelta(TypeTraceDelta, payload{Note: "fragment"})
	if hub.Emit(TypeTrace, payload{Note: "state"}) != 1 {
		t.Fatal("a delta consumed an event id")
	}

	select {
	case frame := <-subscriber.Delta():
		if frame.ID != 0 {
			t.Fatalf("delta carries id %d", frame.ID)
		}
		if !strings.Contains(string(frame.Data), string(TypeTraceDelta)) {
			t.Fatalf("delta body = %s", frame.Data)
		}
	default:
		t.Fatal("delta was not delivered")
	}

	replay, _ := hub.Subscribe(0)
	frames := drain(t, replay)
	if len(frames) != 1 {
		t.Fatalf("ring holds %d frames, want only the state event", len(frames))
	}
}

func TestDeltaFloodNeverEvictsAStateEvent(t *testing.T) {
	hub := NewHub(clock.System{})
	subscriber, _ := hub.Subscribe(0)
	for range deltaQueueCapacity * 4 {
		hub.EmitDelta(TypeTraceDelta, payload{Note: "fragment"})
	}
	if hub.Subscribers() != 1 {
		t.Fatal("a delta flood dropped the subscriber")
	}
	hub.Emit(TypeTrace, payload{Note: "state"})
	frames := drain(t, subscriber)
	if len(frames) != 1 {
		t.Fatalf("state queue holds %d frames, want 1", len(frames))
	}
}

func TestASubscriberThatStopsReadingStateIsDropped(t *testing.T) {
	hub := NewHub(clock.System{})
	subscriber, _ := hub.Subscribe(0)
	for range stateQueueCapacity + 1 {
		hub.Emit(TypeTrace, payload{Note: "state"})
	}
	if hub.Subscribers() != 0 {
		t.Fatal("a stalled subscriber was kept")
	}
	select {
	case <-subscriber.Done():
	default:
		t.Fatal("the dropped subscriber was not closed")
	}
}
