package integrity

import (
	"encoding/binary"
	"math"
	"sync"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

type TickSink func(domain.Tick) error

type DedupeSink struct {
	mu       sync.Mutex
	capacity int
	seen     map[tickFingerprint]struct{}
	order    []tickFingerprint
	next     TickSink
}

type tickFingerprint struct {
	Provider     string
	InstrumentID string
	EventMS      int64
	PriceBits    uint64
	QuantityBits uint64
}

func NewDedupeSink(capacity int, next TickSink) *DedupeSink {
	if capacity <= 0 {
		capacity = 4096
	}
	return &DedupeSink{
		capacity: capacity,
		seen:     make(map[tickFingerprint]struct{}, capacity),
		next:     next,
	}
}

func (d *DedupeSink) Handle(tick domain.Tick) error {
	fingerprint := tickFingerprint{
		Provider:     tick.Provider,
		InstrumentID: tick.InstrumentID,
		EventMS:      tick.EventTime.UTC().UnixMilli(),
		PriceBits:    math.Float64bits(tick.Price),
		QuantityBits: math.Float64bits(tick.Quantity),
	}

	d.mu.Lock()
	if _, exists := d.seen[fingerprint]; exists {
		d.mu.Unlock()
		return nil
	}
	d.seen[fingerprint] = struct{}{}
	d.order = append(d.order, fingerprint)
	if len(d.order) > d.capacity {
		oldest := d.order[0]
		d.order = d.order[1:]
		delete(d.seen, oldest)
	}
	d.mu.Unlock()

	if d.next == nil {
		return nil
	}
	return d.next(tick)
}

func FingerprintBytes(tick domain.Tick) []byte {
	buffer := make([]byte, 24)
	binary.BigEndian.PutUint64(buffer[:8], uint64(tick.EventTime.UTC().UnixMilli()))
	binary.BigEndian.PutUint64(buffer[8:16], math.Float64bits(tick.Price))
	binary.BigEndian.PutUint64(buffer[16:], math.Float64bits(tick.Quantity))
	return buffer
}
