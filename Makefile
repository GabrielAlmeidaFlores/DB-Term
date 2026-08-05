.PHONY: build run test test-integration lint fmt fmt-check clean

build:
	go build -o bin/dbterm ./cmd/main.go

run:
	go run ./cmd/main.go

test:
	go test ./... -race -timeout 30s

test-integration:
	go test ./internal/db/... -tags integration -race

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .
	goimports -w .

# fmt-check verifies formatting without modifying files (used in CI)
fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "Run 'make fmt' to fix formatting:" && gofmt -l . && exit 1)

clean:
	rm -rf bin/
