.PHONY: q0-check q0-fixtures proto-lint proto-generate

q0-check:
	@test -f schemas/protobuf/qnext_market.proto
	@test -f schemas/protobuf/qnext_instrument.proto
	@test -f schemas/protobuf/qnext_synthetic.proto
	@test -f schemas/protobuf/qnext_intelligence.proto
	@test -f schemas/protobuf/qnext_strategy.proto
	@test -f schemas/openapi/qnext.yaml
	@test -f schemas/asyncapi/qnext-stream.yaml
	@test -f tests/certification/README.md
	@test -f fixtures/market/nifty_ticks.jsonl
	@test -f fixtures/synthetic/nifty_syn.json
	@! grep -R "QSYN.MARKET" schemas
	@python3 tools/q0_validate.py
	@echo "Q0 contract checks: PASS"

q0-fixtures:
	@python3 tools/q0_validate.py

proto-lint:
	@buf lint

proto-generate:
	@buf generate
