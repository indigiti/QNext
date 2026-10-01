package pipeline

// SubminuteTelemetry exposes the canonical 5s/derived integrity snapshot
// without coupling runtime callers to the concrete snapshot type.
func (p *Pipeline) SubminuteTelemetry() any {
	return p.SubminuteSnapshot()
}
