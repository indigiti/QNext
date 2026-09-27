package upstox

import (
	"strconv"

	"google.golang.org/protobuf/encoding/protowire"
)

func parseFullFeed(payload []byte) (MarketState, error) {
	var state MarketState
	for len(payload) > 0 {
		num, typ, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return MarketState{}, protowire.ParseError(n)
		}
		payload = payload[n:]
		switch num {
		case 1, 2:
			value, rest, err := consumeBytesField("FullFeed", typ, payload)
			if err != nil {
				return MarketState{}, err
			}
			if num == 1 {
				state, err = parseMarketFullFeed(value)
			} else {
				state, err = parseIndexFullFeed(value)
			}
			if err != nil {
				return MarketState{}, err
			}
			payload = rest
		default:
			rest, err := skipField(num, typ, payload)
			if err != nil {
				return MarketState{}, err
			}
			payload = rest
		}
	}
	return state, nil
}

func parseMarketFullFeed(payload []byte) (MarketState, error) {
	var state MarketState
	for len(payload) > 0 {
		num, typ, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return MarketState{}, protowire.ParseError(n)
		}
		payload = payload[n:]

		switch num {
		case 1:
			value, rest, err := consumeBytesField("MarketFullFeed.ltpc", typ, payload)
			if err != nil {
				return MarketState{}, err
			}
			ltpc, err := parseLTPC(value)
			if err != nil {
				return MarketState{}, err
			}
			state.LTPC = &ltpc
			payload = rest
		case 2:
			value, rest, err := consumeBytesField("MarketFullFeed.marketLevel", typ, payload)
			if err != nil {
				return MarketState{}, err
			}
			state.Depth, err = parseMarketLevel(value)
			if err != nil {
				return MarketState{}, err
			}
			payload = rest
		case 3:
			value, rest, err := consumeBytesField("MarketFullFeed.optionGreeks", typ, payload)
			if err != nil {
				return MarketState{}, err
			}
			greeks, err := parseOptionGreeks(value)
			if err != nil {
				return MarketState{}, err
			}
			state.OptionGreeks = &greeks
			payload = rest
		case 4:
			value, rest, err := consumeBytesField("MarketFullFeed.marketOHLC", typ, payload)
			if err != nil {
				return MarketState{}, err
			}
			_ = value
			payload = rest
		case 5:
			value, rest, err := consumeDoubleField("MarketFullFeed.atp", typ, payload)
			if err != nil {
				return MarketState{}, err
			}
			state.ATP = value
			payload = rest
		case 6:
			value, rest, err := consumeInt64Field("MarketFullFeed.vtt", typ, payload)
			if err != nil {
				return MarketState{}, err
			}
			state.VTT = value
			payload = rest
		case 7:
			value, rest, err := consumeDoubleField("MarketFullFeed.oi", typ, payload)
			if err != nil {
				return MarketState{}, err
			}
			state.OI = value
			payload = rest
		case 8:
			value, rest, err := consumeDoubleField("MarketFullFeed.iv", typ, payload)
			if err != nil {
				return MarketState{}, err
			}
			state.IV = value
			payload = rest
		case 9:
			value, rest, err := consumeDoubleField("MarketFullFeed.tbq", typ, payload)
			if err != nil {
				return MarketState{}, err
			}
			state.TBQ = value
			payload = rest
		case 10:
			value, rest, err := consumeDoubleField("MarketFullFeed.tsq", typ, payload)
			if err != nil {
				return MarketState{}, err
			}
			state.TSQ = value
			payload = rest
		default:
			rest, err := skipField(num, typ, payload)
			if err != nil {
				return MarketState{}, err
			}
			payload = rest
		}
	}
	return state, nil
}

func parseIndexFullFeed(payload []byte) (MarketState, error) {
	var state MarketState
	for len(payload) > 0 {
		num, typ, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return MarketState{}, protowire.ParseError(n)
		}
		payload = payload[n:]
		switch num {
		case 1:
			value, rest, err := consumeBytesField("IndexFullFeed.ltpc", typ, payload)
			if err != nil {
				return MarketState{}, err
			}
			ltpc, err := parseLTPC(value)
			if err != nil {
				return MarketState{}, err
			}
			state.LTPC = &ltpc
			payload = rest
		default:
			rest, err := skipField(num, typ, payload)
			if err != nil {
				return MarketState{}, err
			}
			payload = rest
		}
	}
	return state, nil
}

func parseFirstLevelWithGreeks(payload []byte) (MarketState, error) {
	var state MarketState
	for len(payload) > 0 {
		num, typ, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return MarketState{}, protowire.ParseError(n)
		}
		payload = payload[n:]
		switch num {
		case 1:
			value, rest, err := consumeBytesField("FirstLevelWithGreeks.ltpc", typ, payload)
			if err != nil {
				return MarketState{}, err
			}
			ltpc, err := parseLTPC(value)
			if err != nil {
				return MarketState{}, err
			}
			state.LTPC = &ltpc
			payload = rest
		case 2:
			value, rest, err := consumeBytesField("FirstLevelWithGreeks.firstDepth", typ, payload)
			if err != nil {
				return MarketState{}, err
			}
			quote, err := parseQuote(value)
			if err != nil {
				return MarketState{}, err
			}
			state.Depth = []Quote{quote}
			payload = rest
		case 3:
			value, rest, err := consumeBytesField("FirstLevelWithGreeks.optionGreeks", typ, payload)
			if err != nil {
				return MarketState{}, err
			}
			greeks, err := parseOptionGreeks(value)
			if err != nil {
				return MarketState{}, err
			}
			state.OptionGreeks = &greeks
			payload = rest
		case 4:
			value, rest, err := consumeInt64Field("FirstLevelWithGreeks.vtt", typ, payload)
			if err != nil {
				return MarketState{}, err
			}
			state.VTT = value
			payload = rest
		case 5:
			value, rest, err := consumeDoubleField("FirstLevelWithGreeks.oi", typ, payload)
			if err != nil {
				return MarketState{}, err
			}
			state.OI = value
			payload = rest
		case 6:
			value, rest, err := consumeDoubleField("FirstLevelWithGreeks.iv", typ, payload)
			if err != nil {
				return MarketState{}, err
			}
			state.IV = value
			payload = rest
		default:
			rest, err := skipField(num, typ, payload)
			if err != nil {
				return MarketState{}, err
			}
			payload = rest
		}
	}
	return state, nil
}

func parseMarketLevel(payload []byte) ([]Quote, error) {
	var quotes []Quote
	for len(payload) > 0 {
		num, typ, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		payload = payload[n:]
		if num != 1 {
			rest, err := skipField(num, typ, payload)
			if err != nil {
				return nil, err
			}
			payload = rest
			continue
		}
		value, rest, err := consumeBytesField("MarketLevel.bidAskQuote", typ, payload)
		if err != nil {
			return nil, err
		}
		quote, err := parseQuote(value)
		if err != nil {
			return nil, err
		}
		quotes = append(quotes, quote)
		payload = rest
	}
	return quotes, nil
}

func parseQuote(payload []byte) (Quote, error) {
	var quote Quote
	for len(payload) > 0 {
		num, typ, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return Quote{}, protowire.ParseError(n)
		}
		payload = payload[n:]
		switch num {
		case 1:
			value, rest, err := consumeInt64Field("Quote.bidQ", typ, payload)
			if err != nil {
				return Quote{}, err
			}
			quote.BidQ = value
			payload = rest
		case 2:
			value, rest, err := consumeDoubleField("Quote.bidP", typ, payload)
			if err != nil {
				return Quote{}, err
			}
			quote.BidP = value
			payload = rest
		case 3:
			value, rest, err := consumeInt64Field("Quote.askQ", typ, payload)
			if err != nil {
				return Quote{}, err
			}
			quote.AskQ = value
			payload = rest
		case 4:
			value, rest, err := consumeDoubleField("Quote.askP", typ, payload)
			if err != nil {
				return Quote{}, err
			}
			quote.AskP = value
			payload = rest
		default:
			rest, err := skipField(num, typ, payload)
			if err != nil {
				return Quote{}, err
			}
			payload = rest
		}
	}
	return quote, nil
}

func parseOptionGreeks(payload []byte) (OptionGreeks, error) {
	var greeks OptionGreeks
	for len(payload) > 0 {
		num, typ, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return OptionGreeks{}, protowire.ParseError(n)
		}
		payload = payload[n:]
		switch num {
		case 1, 2, 3, 4, 5:
			value, rest, err := consumeDoubleField("OptionGreeks", typ, payload)
			if err != nil {
				return OptionGreeks{}, err
			}
			switch num {
			case 1:
				greeks.Delta = value
			case 2:
				greeks.Theta = value
			case 3:
				greeks.Gamma = value
			case 4:
				greeks.Vega = value
			case 5:
				greeks.Rho = value
			}
			payload = rest
		default:
			rest, err := skipField(num, typ, payload)
			if err != nil {
				return OptionGreeks{}, err
			}
			payload = rest
		}
	}
	return greeks, nil
}

func parseLTPC(payload []byte) (LTPC, error) {
	var ltpc LTPC

	for len(payload) > 0 {
		num, typ, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return LTPC{}, protowire.ParseError(n)
		}
		payload = payload[n:]

		switch num {
		case 1:
			value, rest, err := consumeDoubleField("LTPC.ltp", typ, payload)
			if err != nil {
				return LTPC{}, err
			}
			ltpc.LTP = value
			payload = rest
		case 2:
			value, rest, err := consumeInt64Field("LTPC.ltt", typ, payload)
			if err != nil {
				return LTPC{}, err
			}
			ltpc.LTT = strconv.FormatInt(value, 10)
			payload = rest
		case 3:
			value, rest, err := consumeInt64Field("LTPC.ltq", typ, payload)
			if err != nil {
				return LTPC{}, err
			}
			ltpc.LTQ = strconv.FormatInt(value, 10)
			payload = rest
		case 4:
			value, rest, err := consumeDoubleField("LTPC.cp", typ, payload)
			if err != nil {
				return LTPC{}, err
			}
			ltpc.CP = value
			payload = rest
		default:
			rest, err := skipField(num, typ, payload)
			if err != nil {
				return LTPC{}, err
			}
			payload = rest
		}
	}

	return ltpc, nil
}
