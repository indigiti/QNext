.PHONY: q0-check

q0-check:
	@test -f schemas/protobuf/qnext_market.proto
	@test -f schemas/openapi/qnext.yaml
	@test -f schemas/asyncapi/qnext-stream.yaml
	@test -f tests/certification/README.md
	@! grep -R "QSYN.MARKET" schemas
	@echo "Q0 contract checks: PASS"
