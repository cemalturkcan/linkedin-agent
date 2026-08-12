package events

import (
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"api/internal/app/clock"
)

type Type string

const (
	TypeSetup    Type = "setup"
	TypeIndex    Type = "index"
	TypeCycle    Type = "cycle"
	TypeJobs     Type = "jobs"
	TypeScreen   Type = "screen"
	TypePlugin   Type = "plugin"
	TypeSettings Type = "settings"
	TypeTrace    Type = "trace"
	TypeError    Type = "error"
)

type DeltaType string

const TypeTraceDelta DeltaType = "trace-delta"

const (
	RetryHint      = 3 * time.Second
	Heartbeat      = 20 * time.Second
	RingCapacity   = 200
	MaxSubscribers = 8

	stateQueueCapacity = RingCapacity + 56
	deltaQueueCapacity = 64
)

type Frame struct {
	ID   int64
	Name string
	Data json.RawMessage
}

type Subscriber struct {
	state     chan Frame
	delta     chan Frame
	done      chan struct{}
	closeOnce sync.Once
}

func newSubscriber() *Subscriber {
	return &Subscriber{
		state: make(chan Frame, stateQueueCapacity),
		delta: make(chan Frame, deltaQueueCapacity),
		done:  make(chan struct{}),
	}
}

func (s *Subscriber) State() <-chan Frame {
	return s.state
}

func (s *Subscriber) Delta() <-chan Frame {
	return s.delta
}

func (s *Subscriber) Done() <-chan struct{} {
	return s.done
}

func (s *Subscriber) close() {
	s.closeOnce.Do(func() { close(s.done) })
}

type Hub struct {
	clock clock.Clock

	mu          sync.Mutex
	nextID      int64
	ring        []Frame
	subscribers map[*Subscriber]struct{}
}

func NewHub(source clock.Clock) *Hub {
	return &Hub{
		clock:       source,
		nextID:      1,
		ring:        make([]Frame, 0, RingCapacity),
		subscribers: make(map[*Subscriber]struct{}, MaxSubscribers),
	}
}

func (h *Hub) Emit(eventType Type, data any) int64 {
	body, err := h.body(string(eventType), data)
	if err != nil {
		slog.Error("event dropped", "type", string(eventType), "err", err)
		return 0
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	frame := Frame{ID: h.nextID, Name: string(eventType), Data: body}
	h.nextID++
	h.ring = append(h.ring, frame)
	if len(h.ring) > RingCapacity {
		h.ring = append(h.ring[:0], h.ring[1:]...)
	}
	for subscriber := range h.subscribers {
		select {
		case subscriber.state <- frame:
		default:
			h.dropLocked(subscriber)
		}
	}
	return frame.ID
}

func (h *Hub) EmitDelta(deltaType DeltaType, data any) {
	body, err := h.body(string(deltaType), data)
	if err != nil {
		slog.Error("delta dropped", "type", string(deltaType), "err", err)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	frame := Frame{Name: string(deltaType), Data: body}
	for subscriber := range h.subscribers {
		select {
		case subscriber.delta <- frame:
		default:
		}
	}
}

func (h *Hub) body(name string, data any) (json.RawMessage, error) {
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return nil, err
	}
	fields["type"], err = json.Marshal(name)
	if err != nil {
		return nil, err
	}
	fields["at"], err = json.Marshal(clock.Stamp(h.clock))
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func (h *Hub) Subscribe(lastEventID int64) (*Subscriber, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.subscribers) >= MaxSubscribers {
		return nil, false
	}
	subscriber := newSubscriber()
	for _, frame := range h.ring {
		if frame.ID <= lastEventID {
			continue
		}
		select {
		case subscriber.state <- frame:
		default:
		}
	}
	h.subscribers[subscriber] = struct{}{}
	return subscriber, true
}

func (h *Hub) Unsubscribe(subscriber *Subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dropLocked(subscriber)
}

func (h *Hub) dropLocked(subscriber *Subscriber) {
	if _, present := h.subscribers[subscriber]; !present {
		return
	}
	delete(h.subscribers, subscriber)
	subscriber.close()
}

func (h *Hub) Subscribers() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subscribers)
}

func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for subscriber := range h.subscribers {
		h.dropLocked(subscriber)
	}
}
