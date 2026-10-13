package httpapi

import "time"

const maxHistoryBars = 50_000

func maxHistoryWindow(timeframe string) (time.Duration, bool) {
	day := 24 * time.Hour
	switch timeframe {
	case "1s":
		return 2 * day, true
	case "5s":
		return 10 * day, true
	case "10s":
		return 20 * day, true
	case "15s":
		return 31 * day, true
	case "30s":
		return 62 * day, true
	case "45s":
		return 90 * day, true
	case "1m":
		return 120 * day, true
	case "2m":
		return 240 * day, true
	case "3m", "5m", "10m", "15m", "30m", "45m", "1h":
		return 370 * day, true
	case "2h", "3h", "4h":
		return 2 * 370 * day, true
	case "1D":
		return 10 * 370 * day, true
	case "1W":
		return 20 * 370 * day, true
	case "1M", "3M", "6M", "12M":
		return 30 * 370 * day, true
	default:
		return 0, false
	}
}
