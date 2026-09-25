package resilience

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const transitionRecordSchema = "QNEXT.AUTHORITY.TRANSITION/1"

type TransitionStore struct {
	root string
	mu   sync.Mutex
}

type transitionRecord struct {
	Schema        string `json:"schema"`
	InstrumentID  string `json:"instrument_id"`
	From          string `json:"from"`
	To            string `json:"to"`
	Reason        string `json:"reason"`
	ObservedGapMS int64  `json:"observed_gap_ms"`
	EventTimeMS   int64  `json:"event_time_ms"`
	PolicyVersion string `json:"authority_policy_version"`
}

func NewTransitionStore(root string) *TransitionStore {
	return &TransitionStore{root: root}
}

func (s *TransitionStore) Append(event SwitchEvent) error {
	if s.root == "" {
		return errors.New("transition store root is required")
	}
	if event.InstrumentID == "" || event.To == "" || event.AtMS <= 0 {
		return errors.New("authority transition requires instrument, target provider, and event time")
	}
	record := transitionRecord{
		Schema:        transitionRecordSchema,
		InstrumentID:  event.InstrumentID,
		From:          event.From,
		To:            event.To,
		Reason:        event.Reason,
		ObservedGapMS: event.GapMS,
		EventTimeMS:   event.AtMS,
		PolicyVersion: event.PolicyVersion,
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')

	at := time.UnixMilli(event.AtMS).UTC()
	path := filepath.Join(
		s.root,
		"resilience",
		"authority-transitions",
		at.Format("2006"),
		at.Format("01"),
		at.Format("2006-01-02")+".jsonl",
	)

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return err
	}
	defer file.Close()
	n, err := file.Write(payload)
	if err != nil {
		return err
	}
	if n != len(payload) {
		return io.ErrShortWrite
	}
	return file.Sync()
}
