package httpapi

import (
	"context"
	stdjson "encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	json "github.com/bytedance/sonic"
	"github.com/google/uuid"

	"github.com/masteryyh/agenty-core/pkg/infra/modelcall"
)

const (
	maxReplayEvents         = 2048
	maxReplayBytes          = 16 << 20
	maxLiveProjectionBytes  = 16 << 20
	maxLiveProjectionEvents = 4096
	maxSubscriberQueue      = 128
	maxStreamSubscribers    = 256
	maxStreamCommandBytes   = 1 << 20
)

type EventCursor struct {
	StreamID string `json:"streamId"`
	Sequence uint64 `json:"sequence"`
}

type StreamFrame struct {
	Type     string             `json:"type"`
	Topic    string             `json:"topic"`
	StreamID string             `json:"streamId,omitempty"`
	Sequence uint64             `json:"sequence,omitempty"`
	Cursor   *EventCursor       `json:"cursor,omitempty"`
	Reason   string             `json:"reason,omitempty"`
	Event    stdjson.RawMessage `json:"event,omitempty"`
	Snapshot any                `json:"snapshot,omitempty"`
	Code     string             `json:"code,omitempty"`
	Message  string             `json:"message,omitempty"`
}

type StreamCommand struct {
	Type  string       `json:"type"`
	Topic string       `json:"topic"`
	After *EventCursor `json:"after,omitempty"`
}

type EventSnapshot struct {
	State              any           `json:"state"`
	RecentEvents       []StreamFrame `json:"recentEvents"`
	ProjectionComplete bool          `json:"projectionComplete"`
}

type replayEntry struct {
	frame StreamFrame
	bytes int
}

type topicLog struct {
	sequence           uint64
	entries            []replayEntry
	activeRoundID      uuid.UUID
	liveEvents         []StreamFrame
	liveBytes          int
	projectionComplete bool
}

type streamSubscriber struct {
	id      uint64
	topic   string
	frames  chan StreamFrame
	dropped chan struct{}
}

type StreamSubscription struct {
	Frames  <-chan StreamFrame
	Dropped <-chan struct{}
	close   func()
}

type BrokerError struct {
	Status  int
	Code    string
	Message string
}

func (err *BrokerError) Error() string { return err.Message }

func (subscription *StreamSubscription) Close() {
	if subscription != nil && subscription.close != nil {
		subscription.close()
	}
}

type SnapshotFunc func(context.Context, string) (any, error)

type StreamBroker struct {
	mu              sync.Mutex
	barrierMu       sync.Mutex
	barriers        map[string]*sync.Mutex
	streamID        string
	topics          map[string]*topicLog
	order           []replayEntryRef
	bytes           int
	subscribers     map[string]map[uint64]*streamSubscriber
	subscriberCount int
	nextID          atomic.Uint64
	snapshot        SnapshotFunc
	closed          bool
}

type replayEntryRef struct {
	topic string
	seq   uint64
	bytes int
}

func NewStreamBroker(snapshot SnapshotFunc) *StreamBroker {
	return &StreamBroker{
		streamID:    uuid.NewString(),
		topics:      make(map[string]*topicLog),
		barriers:    make(map[string]*sync.Mutex),
		subscribers: make(map[string]map[uint64]*streamSubscriber),
		snapshot:    snapshot,
	}
}

type topicBarrierContextKey struct{}

type heldTopicBarrier struct {
	broker *StreamBroker
	topic  string
}

// TopicBarrier serializes a state mutation with the corresponding stream
// snapshot. The context marker makes synchronous nested event publication
// reentrant without making the underlying mutex reentrant.
func (broker *StreamBroker) TopicBarrier(ctx context.Context, topic string) (context.Context, func()) {
	if ctx == nil {
		ctx = context.Background()
	}
	if held, ok := ctx.Value(topicBarrierContextKey{}).(heldTopicBarrier); ok && held.broker == broker && held.topic == topic {
		return ctx, func() {}
	}
	barrier := broker.barrier(topic)
	barrier.Lock()
	return context.WithValue(ctx, topicBarrierContextKey{}, heldTopicBarrier{broker: broker, topic: topic}), barrier.Unlock
}

func (broker *StreamBroker) barrier(topic string) *sync.Mutex {
	broker.barrierMu.Lock()
	defer broker.barrierMu.Unlock()
	barrier := broker.barriers[topic]
	if barrier == nil {
		barrier = &sync.Mutex{}
		broker.barriers[topic] = barrier
	}
	return barrier
}

func (broker *StreamBroker) StreamID() string {
	return broker.streamID
}

func (broker *StreamBroker) Close() {
	broker.mu.Lock()
	defer broker.mu.Unlock()
	if broker.closed {
		return
	}
	broker.closed = true
	for topic, subscribers := range broker.subscribers {
		for id, subscriber := range subscribers {
			delete(subscribers, id)
			broker.subscriberCount--
			close(subscriber.dropped)
		}
		delete(broker.subscribers, topic)
	}
}

func (broker *StreamBroker) Publish(ctx context.Context, topic string, event any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validTopic(topic) {
		return fmt.Errorf("invalid event topic %q", topic)
	}
	ctx, release := broker.TopicBarrier(ctx, topic)
	defer release()
	eventBytes, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode event for topic %s: %w", topic, err)
	}
	broker.mu.Lock()
	defer broker.mu.Unlock()
	if broker.closed {
		return fmt.Errorf("event broker is shutting down")
	}
	log := broker.topic(topic)
	log.sequence++
	if value, ok := event.(SessionEvent); ok {
		value.Sequence = log.sequence
		eventBytes, err = json.Marshal(value)
		if err != nil {
			return fmt.Errorf("encode session event: %w", err)
		}
	} else if value, ok := event.(*SessionEvent); ok && value != nil {
		copyValue := *value
		copyValue.Sequence = log.sequence
		eventBytes, err = json.Marshal(copyValue)
		if err != nil {
			return fmt.Errorf("encode session event: %w", err)
		}
	}
	frame := StreamFrame{
		Type:     "event",
		Topic:    topic,
		StreamID: broker.streamID,
		Sequence: log.sequence,
		Cursor:   &EventCursor{StreamID: broker.streamID, Sequence: log.sequence},
		Event:    eventBytes,
	}
	broker.updateLiveProjection(log, frame)
	frameBytes, err := json.Marshal(frame)
	if err != nil {
		return fmt.Errorf("encode event frame: %w", err)
	}
	entry := replayEntry{frame: frame, bytes: len(frameBytes)}
	log.entries = append(log.entries, entry)
	broker.order = append(broker.order, replayEntryRef{topic: topic, seq: log.sequence, bytes: entry.bytes})
	broker.bytes += entry.bytes
	broker.trimReplay()
	for _, subscriber := range broker.subscribers[topic] {
		select {
		case subscriber.frames <- frame:
		default:
			delete(broker.subscribers[topic], subscriber.id)
			broker.subscriberCount--
			close(subscriber.dropped)
		}
	}
	return nil
}

func (broker *StreamBroker) topic(name string) *topicLog {
	log := broker.topics[name]
	if log == nil {
		log = &topicLog{projectionComplete: true}
		broker.topics[name] = log
	}
	return log
}

func (broker *StreamBroker) trimReplay() {
	for len(broker.order) > maxReplayEvents || broker.bytes > maxReplayBytes {
		oldest := broker.order[0]
		broker.order = broker.order[1:]
		broker.bytes -= oldest.bytes
		log := broker.topics[oldest.topic]
		if log == nil || len(log.entries) == 0 || log.entries[0].frame.Sequence != oldest.seq {
			continue
		}
		log.entries = log.entries[1:]
	}
}

func (broker *StreamBroker) updateLiveProjection(log *topicLog, frame StreamFrame) {
	var sessionEvent SessionEvent
	if err := json.Unmarshal(frame.Event, &sessionEvent); err != nil || sessionEvent.Kind != "session" {
		return
	}

	switch sessionEvent.Type {
	case SessionEventRoundStarted:
		log.activeRoundID = sessionEvent.RoundID
		log.liveEvents = []StreamFrame{frame}
		log.liveBytes = len(frame.Event)
		log.projectionComplete = true
		return
	case SessionEventRoundEnded:
		if log.activeRoundID == sessionEvent.RoundID {
			log.activeRoundID = uuid.Nil
			log.liveEvents = nil
			log.liveBytes = 0
			log.projectionComplete = true
		}
		return
	case SessionEventMessageAppended:
		if log.activeRoundID == sessionEvent.RoundID && len(log.liveEvents) > 0 {
			start := log.liveEvents[0]
			log.liveEvents = []StreamFrame{start}
			log.liveBytes = len(start.Event)
			log.projectionComplete = true
		}
		return
	case SessionEventModelStream:
		if log.activeRoundID != sessionEvent.RoundID || sessionEvent.Stream == nil {
			return
		}
		if sessionEvent.Stream.Type == modelcall.ModelCallStreamEventCompleted {
			return
		}
		appendLiveProjection(log, frame, sessionEvent)
	case "tool_review_started", "tool_review_resolved":
		if log.activeRoundID == sessionEvent.RoundID {
			appendLiveProjection(log, frame, sessionEvent)
		}
	}
}

func appendLiveProjection(log *topicLog, frame StreamFrame, event SessionEvent) {
	if len(log.liveEvents) > 0 {
		lastIndex := len(log.liveEvents) - 1
		var previous SessionEvent
		if json.Unmarshal(log.liveEvents[lastIndex].Event, &previous) == nil && canMergeStreamDeltas(previous, event) {
			previous.Stream.Delta += event.Stream.Delta
			previous.Sequence = event.Sequence
			encoded, err := json.Marshal(previous)
			if err == nil {
				log.liveBytes -= len(log.liveEvents[lastIndex].Event)
				log.liveEvents[lastIndex] = StreamFrame{
					Type: "event", Topic: frame.Topic, StreamID: frame.StreamID,
					Sequence: frame.Sequence, Cursor: frame.Cursor, Event: encoded,
				}
				log.liveBytes += len(encoded)
				return
			}
		}
	}
	log.liveEvents = append(log.liveEvents, frame)
	log.liveBytes += len(frame.Event)
	if len(log.liveEvents) > maxLiveProjectionEvents || log.liveBytes > maxLiveProjectionBytes {
		log.projectionComplete = false
		for len(log.liveEvents) > 1 && log.liveBytes > maxLiveProjectionBytes {
			log.liveBytes -= len(log.liveEvents[1].Event)
			log.liveEvents = append(log.liveEvents[:1], log.liveEvents[2:]...)
		}
		if len(log.liveEvents) > maxLiveProjectionEvents {
			log.liveBytes = len(log.liveEvents[0].Event)
			log.liveEvents = log.liveEvents[:1]
		}
	}
}

func canMergeStreamDeltas(previous, next SessionEvent) bool {
	if previous.Type != SessionEventModelStream || next.Type != SessionEventModelStream || previous.Stream == nil || next.Stream == nil {
		return false
	}
	if previous.RoundID != next.RoundID || previous.Iteration != next.Iteration {
		return false
	}
	if previous.Stream.Type != next.Stream.Type || previous.Stream.Index != next.Stream.Index || previous.Stream.ToolUseID != next.Stream.ToolUseID {
		return false
	}
	switch next.Stream.Type {
	case modelcall.ModelCallStreamEventTextDelta, modelcall.ModelCallStreamEventReasoningDelta, modelcall.ModelCallStreamEventToolInputDelta:
		return true
	default:
		return false
	}
}

func (broker *StreamBroker) subscribe(ctx context.Context, topic string, after *EventCursor) ([]StreamFrame, *StreamSubscription, error) {
	if !validTopic(topic) {
		return nil, nil, fmt.Errorf("topic must be mcp or session:<sessionId>")
	}
	barrier := broker.barrier(topic)
	barrier.Lock()
	defer barrier.Unlock()
	broker.mu.Lock()
	if broker.closed {
		broker.mu.Unlock()
		return nil, nil, &BrokerError{Status: http.StatusServiceUnavailable, Code: "shutting_down", Message: "event stream is shutting down"}
	}
	log := broker.topic(topic)
	if broker.subscriberCount >= maxStreamSubscribers {
		broker.mu.Unlock()
		return nil, nil, &BrokerError{Status: http.StatusTooManyRequests, Code: "too_many_subscribers", Message: "core event subscriber limit reached"}
	}
	latest := log.sequence
	first := latest + 1
	if len(log.entries) > 0 {
		first = log.entries[0].frame.Sequence
	}
	initial := make([]StreamFrame, 0, len(log.entries)+1)
	resyncReason := ""
	if after == nil {
		resyncReason = "snapshot_requested"
	} else if after.StreamID != broker.streamID {
		resyncReason = "stream_changed"
	} else if after.Sequence > latest {
		resyncReason = "cursor_ahead"
	} else if after.Sequence+1 < first {
		resyncReason = "cursor_expired"
	}
	if resyncReason != "" {
		broker.mu.Unlock()
		var snapshot any
		var err error
		if broker.snapshot != nil {
			snapshot, err = broker.snapshot(ctx, topic)
			if err != nil {
				return nil, nil, err
			}
		}
		broker.mu.Lock()
		if broker.closed {
			broker.mu.Unlock()
			return nil, nil, &BrokerError{Status: http.StatusServiceUnavailable, Code: "shutting_down", Message: "event stream is shutting down"}
		}
		log = broker.topic(topic)
		latest = log.sequence
		recentEvents := append([]StreamFrame(nil), log.liveEvents...)
		initial = append(initial, StreamFrame{
			Type:     "snapshot",
			Topic:    topic,
			StreamID: broker.streamID,
			Sequence: latest,
			Cursor:   &EventCursor{StreamID: broker.streamID, Sequence: latest},
			Reason:   resyncReason,
			Snapshot: EventSnapshot{State: snapshot, RecentEvents: recentEvents, ProjectionComplete: log.projectionComplete},
		})
	} else {
		for _, entry := range log.entries {
			if entry.frame.Sequence > after.Sequence {
				initial = append(initial, entry.frame)
			}
		}
		initial = append(initial, StreamFrame{
			Type:     "ready",
			Topic:    topic,
			StreamID: broker.streamID,
			Sequence: latest,
			Cursor:   &EventCursor{StreamID: broker.streamID, Sequence: latest},
		})
	}
	subscriber := &streamSubscriber{
		id:      broker.nextID.Add(1),
		topic:   topic,
		frames:  make(chan StreamFrame, maxSubscriberQueue),
		dropped: make(chan struct{}),
	}
	if broker.subscribers[topic] == nil {
		broker.subscribers[topic] = make(map[uint64]*streamSubscriber)
	}
	broker.subscribers[topic][subscriber.id] = subscriber
	broker.subscriberCount++
	broker.mu.Unlock()
	subscription := &StreamSubscription{Frames: subscriber.frames, Dropped: subscriber.dropped}
	subscription.close = func() {
		broker.mu.Lock()
		defer broker.mu.Unlock()
		if current, ok := broker.subscribers[topic][subscriber.id]; ok && current == subscriber {
			delete(broker.subscribers[topic], subscriber.id)
			broker.subscriberCount--
		}
	}
	return initial, subscription, nil
}

// Subscribe atomically returns the initial snapshot or replay frames together
// with a live subscription for the same topic. Callers must apply initial
// frames before consuming live frames to preserve the topic cursor order.
func (broker *StreamBroker) Subscribe(ctx context.Context, topic string, after *EventCursor) ([]StreamFrame, *StreamSubscription, error) {
	return broker.subscribe(ctx, topic, after)
}

func (broker *StreamBroker) SubscriberCount() int {
	broker.mu.Lock()
	defer broker.mu.Unlock()
	return broker.subscriberCount
}

func validTopic(topic string) bool {
	if topic == "mcp" {
		return true
	}
	if !strings.HasPrefix(topic, "session:") {
		return false
	}
	_, err := uuid.Parse(strings.TrimPrefix(topic, "session:"))
	return err == nil
}
