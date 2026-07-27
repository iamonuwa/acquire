# cail-acquire — build and test targets.

STATICCHECK_VERSION := 2024.1.1
BIN := bin/cail-acquire
GOARCH ?= amd64
VERSION ?= dev

.PHONY: build build-linux test integration vet staticcheck lint regen-fixtures clean

build:
	go build -o $(BIN) ./cmd/cail-acquire

# Static Linux binary for the deploy host; cross-compiles from any machine.
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(GOARCH) go build -trimpath \
		-ldflags "-s -w -X gitlab.com/cail-health/cail-acquire/internal/fetch.Version=$(VERSION)" \
		-o $(BIN)-linux-$(GOARCH) ./cmd/cail-acquire

# Unit tests only; the integration tag is excluded.
test:
	go test ./...

# Integration tests: live fetches, behind the integration build tag.
integration:
	go test -tags=integration ./...

vet:
	go vet ./...

staticcheck:
	go run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION) ./...

lint: vet staticcheck

# Capture the live NL index page into the golden fixture for human review.
# Only refreshes testdata/nlpdp_index_happy.html; the synthetic fixtures are left alone.
regen-fixtures:
	go test -tags=integration -run TestCaptureNLIndex ./internal/fetch

clean:
	rm -rf bin/
