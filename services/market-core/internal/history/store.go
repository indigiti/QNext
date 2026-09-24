package history

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

const barRecordSchema = "QNEXT.HISTORY.BAR/1"

type Store struct {
	root string
	mu   sync.Mutex
}

type barRecord struct {
	Schema              string         `json:"schema"`
	InstrumentID        string         `json:"instrument_id"`
	Timeframe           string         `json:"timeframe"`
	OpenTimeMS          int64          `json:"open_time_ms"`
	CloseTimeMS         int64          `json:"close_time_ms"`
	Open                float64        `json:"open"`
	High                float64        `json:"high"`
	Low                 float64        `json:"low"`
	Close               float64        `json:"close"`
	Volume              float64        `json:"volume"`
	Final               bool           `json:"final"`
	Revision            uint32         `json:"revision"`
	AuthorityProvider   string         `json:"authority_provider"`
	Quality             domain.Quality `json:"quality"`
	Recovered           bool           `json:"recovered"`
	Corrected           bool           `json:"corrected"`
	CandleEngineVersion string         `json:"candle_engine_version"`
	SyntheticVersion    string         `json:"synthetic_version,omitempty"`
}

func New(root string) *Store {
	return &Store{root: root}
}

func (s *Store) AppendBar(bar domain.Bar) error {
	if s.root == "" {
		return errors.New("history root is required")
	}
	if bar.InstrumentID == "" || bar.Timeframe == "" || bar.OpenTime.IsZero() {
		return errors.New("bar requires instrument, timeframe, and open time")
	}
	if !bar.Final {
		return errors.New("only finalized bars may be persisted to canonical history")
	}

	record := fromBar(bar)
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	path := s.dayPath(bar.InstrumentID, bar.Timeframe, bar.OpenTime)

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

	n, err := file.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	return file.Sync()
}

func (s *Store) LoadDay(instrumentID, timeframe string, day time.Time) ([]domain.Bar, error) {
	if s.root == "" {
		return nil, errors.New("history root is required")
	}
	if instrumentID == "" || timeframe == "" {
		return nil, errors.New("instrument and timeframe are required")
	}

	path := s.dayPath(instrumentID, timeframe, day)
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	latest := make(map[string]domain.Bar)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var record barRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, fmt.Errorf("decode history record: %w", err)
		}
		if record.Schema != barRecordSchema {
			return nil, fmt.Errorf("unsupported history schema %q", record.Schema)
		}
		bar := record.toBar()
		if previous, ok := latest[bar.Key()]; !ok || bar.Revision >= previous.Revision {
			latest[bar.Key()] = bar
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	bars := make([]domain.Bar, 0, len(latest))
	for _, bar := range latest {
		bars = append(bars, bar)
	}
	sort.Slice(bars, func(i, j int) bool {
		return bars[i].OpenTime.Before(bars[j].OpenTime)
	})
	return bars, nil
}

func (s *Store) dayPath(instrumentID, timeframe string, at time.Time) string {
	day := at.UTC()
	return filepath.Join(
		s.root,
		"market",
		safeComponent(instrumentID),
		safeComponent(timeframe),
		fmt.Sprintf("%04d", day.Year()),
		fmt.Sprintf("%02d", int(day.Month())),
		day.Format("2006-01-02")+".jsonl",
	)
}

func safeComponent(value string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_")
	return replacer.Replace(value)
}

func fromBar(bar domain.Bar) barRecord {
	return barRecord{
		Schema:              barRecordSchema,
		InstrumentID:        bar.InstrumentID,
		Timeframe:           bar.Timeframe,
		OpenTimeMS:          bar.OpenTime.UTC().UnixMilli(),
		CloseTimeMS:         bar.CloseTime.UTC().UnixMilli(),
		Open:                bar.Open,
		High:                bar.High,
		Low:                 bar.Low,
		Close:               bar.Close,
		Volume:              bar.Volume,
		Final:               bar.Final,
		Revision:            bar.Revision,
		AuthorityProvider:   bar.AuthorityProvider,
		Quality:             bar.Quality,
		Recovered:           bar.Recovered,
		Corrected:           bar.Corrected,
		CandleEngineVersion: bar.CandleEngineVersion,
		SyntheticVersion:    bar.SyntheticVersion,
	}
}

func (r barRecord) toBar() domain.Bar {
	return domain.Bar{
		InstrumentID:        r.InstrumentID,
		Timeframe:           r.Timeframe,
		OpenTime:            time.UnixMilli(r.OpenTimeMS).UTC(),
		CloseTime:           time.UnixMilli(r.CloseTimeMS).UTC(),
		Open:                r.Open,
		High:                r.High,
		Low:                 r.Low,
		Close:               r.Close,
		Volume:              r.Volume,
		Final:               r.Final,
		Revision:            r.Revision,
		AuthorityProvider:   r.AuthorityProvider,
		Quality:             r.Quality,
		Recovered:           r.Recovered,
		Corrected:           r.Corrected,
		CandleEngineVersion: r.CandleEngineVersion,
		SyntheticVersion:    r.SyntheticVersion,
	}
}
