package shared

import (
	"time"

	json "github.com/bytedance/sonic"
)

type Event interface {
	EventType() string
	OccurredAt() time.Time
}

type Envelope struct {
	Type    string    `json:"type"`
	Seq     int64     `json:"seq"`
	Payload RawJSON   `json:"payload"`
	WroteAt time.Time `json:"wroteAt"`
}

func EncodeEvent(seq int64, e Event) ([]byte, error) {
	payload, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}

	return json.Marshal(Envelope{
		Type:    e.EventType(),
		Seq:     seq,
		Payload: payload,
		WroteAt: e.OccurredAt(),
	})
}

func DecodeEnvelope(line []byte) (Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return Envelope{}, err
	}
	return env, nil
}
