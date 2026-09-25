package dhan

import (
	"errors"
	"strconv"
	"strings"
)

const ProviderName = "dhan"

type InstrumentKey struct {
	ExchangeSegment string
	SecurityID      int64
	Instrument      string
}

func ParseInstrumentKey(value string) (InstrumentKey, error) {
	parts := strings.Split(value, "|")
	if len(parts) != 3 {
		return InstrumentKey{}, errors.New("Dhan provider key must be EXCHANGE_SEGMENT|SECURITY_ID|INSTRUMENT")
	}
	segment := strings.ToUpper(strings.TrimSpace(parts[0]))
	instrument := strings.ToUpper(strings.TrimSpace(parts[2]))
	id, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil || id <= 0 {
		return InstrumentKey{}, errors.New("Dhan security id must be positive integer")
	}
	if !validSegment(segment) {
		return InstrumentKey{}, errors.New("unsupported Dhan exchange segment")
	}
	if instrument == "" {
		return InstrumentKey{}, errors.New("Dhan instrument type is required")
	}
	return InstrumentKey{ExchangeSegment: segment, SecurityID: id, Instrument: instrument}, nil
}
func (k InstrumentKey) String() string {
	return k.ExchangeSegment + "|" + strconv.FormatInt(k.SecurityID, 10) + "|" + k.Instrument
}
func validSegment(v string) bool {
	switch v {
	case "IDX_I", "NSE_EQ", "NSE_FNO", "NSE_CURRENCY", "BSE_EQ", "MCX_COMM", "BSE_CURRENCY", "BSE_FNO":
		return true
	default:
		return false
	}
}
