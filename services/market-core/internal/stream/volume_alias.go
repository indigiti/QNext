package stream

import (
	"sort"
	"time"

	"github.com/indigiti/QNext/services/market-core/internal/domain"
)

// ConfigureSyntheticVolume ties production SYN price candles to the exact
// configured cash-index futures proxy. SYN+ has its own shadow price bridge.
// All aliases must be configured before the broker begins publishing.
func (b *Broker) ConfigureSyntheticVolume(aliases map[string]string) {
	if b == nil { return }
	b.mu.Lock()
	defer b.mu.Unlock()
	b.syntheticVolume = make(map[string]string, len(aliases))
	b.volumeTargets = make(map[string][]string)
	b.volumeSources = make(map[string][]domain.Bar)
	b.syntheticBars = make(map[string][]domain.Bar)
	for target, source := range aliases {
		if target == "" || source == "" || target == source { continue }
		b.syntheticVolume[target] = source
		b.volumeTargets[source] = append(b.volumeTargets[source], target)
	}
	for source := range b.volumeTargets {
		sort.Strings(b.volumeTargets[source])
	}
}

func volumeState(source domain.Bar) (string, string) {
	if source.Volume <= 0 { return "", "UNKNOWN" }
	if source.VolumeSource == "FUTURES_PROXY" { return "FUTURES_PROXY", "OBSERVED" }
	return "", "LEGACY_UNVERIFIED"
}

// alignVolumeLocked runs on the same mutex as stream publish and replay,
// guaranteeing that a price event and its future volume correction are ordered.
// It only keeps the last three candle buckets for each stream.
func (b *Broker) alignVolumeLocked(bar domain.Bar) (domain.Bar, []domain.Bar) {
	if sourceID := b.syntheticVolume[bar.InstrumentID]; sourceID != "" {
		key := streamKey(bar.InstrumentID, bar.Timeframe)
		for _, previous := range b.syntheticBars[key] {
			if previous.OpenTime.Equal(bar.OpenTime) && previous.Revision > bar.Revision {
				bar.Revision = previous.Revision
				break
			}
		}
		bar.Volume = 0
		bar.VolumeSource, bar.VolumeStatus = "", "UNKNOWN"
		for _, source := range b.volumeSources[streamKey(sourceID, bar.Timeframe)] {
			if source.OpenTime.Equal(bar.OpenTime) {
				bar.Volume = source.Volume
				bar.VolumeSource, bar.VolumeStatus = volumeState(source)
				break
			}
		}
		b.syntheticBars[key] = rememberRecentBar(b.syntheticBars[key], bar)
		return bar, nil
	}
	targets := b.volumeTargets[bar.InstrumentID]
	if len(targets) == 0 { return bar, nil }
	sourceKey := streamKey(bar.InstrumentID, bar.Timeframe)
	for _, old := range b.volumeSources[sourceKey] {
		if !old.OpenTime.Equal(bar.OpenTime) { continue }
		if old.Revision > bar.Revision ||
			(old.Revision == bar.Revision && old.Volume > bar.Volume) {
			// Stale replay must not erase already verified traded volume.
			bar.Volume = old.Volume
			bar.VolumeSource = old.VolumeSource
			bar.VolumeStatus = old.VolumeStatus
		}
		break
	}
	b.volumeSources[sourceKey] = rememberRecentBar(b.volumeSources[sourceKey], bar)
	var patches []domain.Bar
	for _, target := range targets {
		key := streamKey(target, bar.Timeframe)
		for _, syn := range b.syntheticBars[key] {
			if !syn.OpenTime.Equal(bar.OpenTime) { continue }
			sourceName, status := volumeState(bar)
			if syn.Volume == bar.Volume && syn.VolumeSource == sourceName &&
				syn.VolumeStatus == status { break }
			patch := syn
			patch.Volume = bar.Volume
			patch.VolumeSource, patch.VolumeStatus = sourceName, status
			patch.Revision++
			if patch.Final {
				patch.Corrected = true
				patch.CorrectedAt = time.Now().UTC()
			}
			patches = append(patches, patch)
			break
		}
	}
	return bar, patches
}

func rememberRecentBar(bars []domain.Bar, next domain.Bar) []domain.Bar {
	for i, bar := range bars {
		if bar.OpenTime.Equal(next.OpenTime) {
			bars[i] = next
			return bars
		}
	}
	if len(bars) < 3 { return append(bars, next) }
	oldest := 0
	for i := 1; i < len(bars); i++ {
		if bars[i].OpenTime.Before(bars[oldest].OpenTime) { oldest = i }
	}
	if next.OpenTime.After(bars[oldest].OpenTime) {
		bars[oldest] = next
	}
	return bars
}
