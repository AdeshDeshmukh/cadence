.PHONY: build test test-race fmt vet clean build-universal package package-deb package-rpm

build:
	@if ls ./cmd/cadence/*.go 1> /dev/null 2>&1; then go build ./cmd/cadence; fi
	@if ls ./cmd/cadenced/*.go 1> /dev/null 2>&1; then go build ./cmd/cadenced; fi

build-universal:
	./scripts/build-universal-macos.sh

package:
	./scripts/package-all.sh

package-deb:
	./scripts/package-deb.sh

package-rpm:
	./scripts/package-rpm.sh

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
	rm -rf dist/
