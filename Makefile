.PHONY: build test test-race fmt vet clean

build:
	@if ls ./cmd/cadence/*.go 1> /dev/null 2>&1; then go build ./cmd/cadence; fi
	@if ls ./cmd/cadenced/*.go 1> /dev/null 2>&1; then go build ./cmd/cadenced; fi

test:
	go test ./...

test-race:
	go test -race ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

clean:
	rm -f cadence cadenced
