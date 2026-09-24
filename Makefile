.PHONY: test fmt vet market-core

test:
	cd services/market-core && go test ./...

fmt:
	cd services/market-core && test -z "$$(gofmt -l .)"

vet:
	cd services/market-core && go vet ./...

market-core:
	cd services/market-core && go build ./cmd/qnext-market-core
