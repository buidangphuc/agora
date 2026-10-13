package testkit

import (
	"context"
	"errors"
	"sync"

	"github.com/twmb/franz-go/pkg/kgo"
)

// ErrInjected is returned by a Recorder for a key registered with FailOn.
var ErrInjected = errors.New("testkit: injected publish failure")

// Record is one message captured by a Recorder.
type Record struct {
	Topic   string
	Key     []byte
	Value   []byte
	Headers []kgo.RecordHeader
}

// Recorder is an in-memory publisher for tests. PublishRecord satisfies both
// outbox.Publisher and consume.DLQProducer; Publish is a convenience for tests
// that record a bare topic, key and value. It is safe for concurrent use.
type Recorder struct {
	mu      sync.Mutex
	records []Record
	fail    map[string]struct{}
}

// NewRecorder returns an empty Recorder.
func NewRecorder() *Recorder { return &Recorder{} }

// FailOn makes every later publish with key return ErrInjected without recording it.
func (r *Recorder) FailOn(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail == nil {
		r.fail = map[string]struct{}{}
	}
	r.fail[key] = struct{}{}
}

// Publish records a message, or returns ErrInjected if key was given to FailOn.
func (r *Recorder) Publish(_ context.Context, topic string, key, value []byte) error {
	return r.add(Record{Topic: topic, Key: key, Value: value})
}

// PublishRecord records a built record, or returns ErrInjected if its key was given to FailOn.
func (r *Recorder) PublishRecord(_ context.Context, rec *kgo.Record) error {
	return r.add(Record{Topic: rec.Topic, Key: rec.Key, Value: rec.Value, Headers: rec.Headers})
}

func (r *Recorder) add(rec Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, bad := r.fail[string(rec.Key)]; bad {
		return ErrInjected
	}
	r.records = append(r.records, rec)
	return nil
}

// Records returns a copy of everything recorded so far, in publish order.
func (r *Recorder) Records() []Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Record(nil), r.records...)
}
