package research

import "github.com/indigiti/QNext/services/market-core/internal/provider/upstox"

// MultiSink fans one decoded research envelope out to independent consumers.
// It deliberately keeps research features out of the production Market Core path.
type MultiSink []upstox.ResearchSink

func (s MultiSink) Observe(envelope upstox.DecodedEnvelope, plan upstox.ResearchPlan) error {
	for _, sink := range s {
		if sink == nil {
			continue
		}
		if err := sink.Observe(envelope, plan); err != nil {
			return err
		}
	}
	return nil
}
