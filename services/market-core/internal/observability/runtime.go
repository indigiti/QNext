package observability

import "sync"

type Snapshot struct {
	Capture any `json:"capture,omitempty"`
	Broker  any `json:"broker,omitempty"`
}

var (
	mu            sync.RWMutex
	captureSource func() any
	brokerSource  func() any
)

func SetCaptureSource(source func() any) {
	mu.Lock()
	captureSource = source
	mu.Unlock()
}

func SetBrokerSource(source func() any) {
	mu.Lock()
	brokerSource = source
	mu.Unlock()
}

func Current() Snapshot {
	mu.RLock()
	capture := captureSource
	broker := brokerSource
	mu.RUnlock()

	var snapshot Snapshot
	if capture != nil {
		snapshot.Capture = capture()
	}
	if broker != nil {
		snapshot.Broker = broker()
	}
	return snapshot
}
