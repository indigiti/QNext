package marketcalendar

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type Session string

const (
	SessionRegular  Session = "regular"
	SessionExtended Session = "extended"
)

type Clock struct {
	Hour   int
	Minute int
}

type Definition struct {
	ID            string
	Version       string
	Timezone      string
	RegularOpen   Clock
	RegularClose  Clock
	ExtendedOpen  Clock
	ExtendedClose Clock
	ClosedDates   map[string]string
	SupportedFrom string
	SupportedTo   string
}

type Window struct {
	Start time.Time
	End   time.Time
}

type Registry struct {
	mu          sync.RWMutex
	definitions map[string]Definition
}

func NewRegistry() *Registry {
	return &Registry{definitions: make(map[string]Definition)}
}

func DefaultRegistry() *Registry {
	registry := NewRegistry()
	_ = registry.Register(NSEEquities2026())
	_ = registry.Register(BSEEquities2026())
	return registry
}

func (r *Registry) Register(definition Definition) error {
	if err := validateDefinition(definition); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := strings.ToUpper(strings.TrimSpace(definition.ID))
	if _, exists := r.definitions[key]; exists {
		return errors.New("calendar already registered")
	}
	definition.ID = key
	r.definitions[key] = definition
	return nil
}

func (r *Registry) Definition(id string) (Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	definition, ok := r.definitions[strings.ToUpper(strings.TrimSpace(id))]
	return definition, ok
}

func (r *Registry) Windows(id string, from, to time.Time, session Session) ([]Window, Definition, error) {
	definition, ok := r.Definition(id)
	if !ok {
		return nil, Definition{}, errors.New("market calendar is not registered")
	}
	if from.IsZero() || to.IsZero() || !from.Before(to) {
		return nil, Definition{}, errors.New("invalid calendar range")
	}
	if session == "" {
		session = SessionRegular
	}
	if session != SessionRegular && session != SessionExtended {
		return nil, Definition{}, errors.New("unsupported market session")
	}

	location, err := time.LoadLocation(definition.Timezone)
	if err != nil {
		return nil, Definition{}, fmt.Errorf("load calendar timezone: %w", err)
	}
	supportedFrom, err := time.ParseInLocation("2006-01-02", definition.SupportedFrom, location)
	if err != nil {
		return nil, Definition{}, fmt.Errorf("parse supported_from: %w", err)
	}
	supportedTo, err := time.ParseInLocation("2006-01-02", definition.SupportedTo, location)
	if err != nil {
		return nil, Definition{}, fmt.Errorf("parse supported_to: %w", err)
	}
	if from.Before(supportedFrom) || to.After(supportedTo) {
		return nil, Definition{}, fmt.Errorf(
			"calendar range is outside certified window [%s, %s)",
			definition.SupportedFrom,
			definition.SupportedTo,
		)
	}

	openClock := definition.RegularOpen
	closeClock := definition.RegularClose
	if session == SessionExtended {
		openClock = definition.ExtendedOpen
		closeClock = definition.ExtendedClose
	}

	localFrom := from.In(location)
	localTo := to.In(location)
	cursor := time.Date(localFrom.Year(), localFrom.Month(), localFrom.Day(), 0, 0, 0, 0, location)
	last := time.Date(localTo.Year(), localTo.Month(), localTo.Day(), 0, 0, 0, 0, location)

	windows := make([]Window, 0, 32)
	for !cursor.After(last) {
		weekday := cursor.Weekday()
		dateKey := cursor.Format("2006-01-02")
		_, closed := definition.ClosedDates[dateKey]
		if weekday != time.Saturday && weekday != time.Sunday && !closed {
			start := time.Date(
				cursor.Year(),
				cursor.Month(),
				cursor.Day(),
				openClock.Hour,
				openClock.Minute,
				0,
				0,
				location,
			)
			end := time.Date(
				cursor.Year(),
				cursor.Month(),
				cursor.Day(),
				closeClock.Hour,
				closeClock.Minute,
				0,
				0,
				location,
			)
			if end.After(from) && start.Before(to) {
				if start.Before(from) {
					start = from
				}
				if end.After(to) {
					end = to
				}
				if start.Before(end) {
					windows = append(windows, Window{Start: start.UTC(), End: end.UTC()})
				}
			}
		}
		cursor = cursor.AddDate(0, 0, 1)
	}

	sort.Slice(windows, func(i, j int) bool {
		return windows[i].Start.Before(windows[j].Start)
	})
	return windows, definition, nil
}

func validateDefinition(definition Definition) error {
	if strings.TrimSpace(definition.ID) == "" ||
		strings.TrimSpace(definition.Version) == "" ||
		strings.TrimSpace(definition.Timezone) == "" {
		return errors.New("calendar id, version and timezone are required")
	}
	if definition.SupportedFrom == "" || definition.SupportedTo == "" {
		return errors.New("calendar support window is required")
	}
	if !validClock(definition.RegularOpen) || !validClock(definition.RegularClose) ||
		!validClock(definition.ExtendedOpen) || !validClock(definition.ExtendedClose) {
		return errors.New("calendar session clocks are invalid")
	}
	return nil
}

func validClock(clock Clock) bool {
	return clock.Hour >= 0 && clock.Hour <= 23 && clock.Minute >= 0 && clock.Minute <= 59
}

func NSEEquities2026() Definition {
	return Definition{
		ID:            "NSE_EQ",
		Version:       "nse-equities-2026-v1",
		Timezone:      "Asia/Kolkata",
		RegularOpen:   Clock{Hour: 9, Minute: 15},
		RegularClose:  Clock{Hour: 15, Minute: 30},
		ExtendedOpen:  Clock{Hour: 9, Minute: 15},
		ExtendedClose: Clock{Hour: 15, Minute: 30},
		SupportedFrom: "2026-01-01",
		SupportedTo:   "2027-01-01",
		ClosedDates: map[string]string{
			"2026-01-15": "Municipal Corporation Election - Maharashtra",
			"2026-01-26": "Republic Day",
			"2026-03-03": "Holi",
			"2026-03-26": "Shri Ram Navami",
			"2026-03-31": "Shri Mahavir Jayanti",
			"2026-04-03": "Good Friday",
			"2026-04-14": "Dr. Baba Saheb Ambedkar Jayanti",
			"2026-05-01": "Maharashtra Day",
			"2026-05-28": "Bakri Id",
			"2026-06-26": "Muharram",
			"2026-09-14": "Ganesh Chaturthi",
			"2026-10-02": "Mahatma Gandhi Jayanti",
			"2026-10-20": "Dussehra",
			"2026-11-10": "Diwali-Balipratipada",
			"2026-11-24": "Prakash Gurpurb Sri Guru Nanak Dev",
			"2026-12-25": "Christmas",
		},
	}
}


func BSEEquities2026() Definition {
	definition := NSEEquities2026()
	definition.ID = "BSE_EQ"
	definition.Version = "bse-equities-2026-v1"
	return definition
}
