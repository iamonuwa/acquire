# cail-acquire — build and test targets.
# Dev toolchain is Go 1.23.x (see go.mod). The release binary is pinned to the
# production toolchain via build-linux (SPEC §9 / CLAUDE toolchain table).

STATICCHECK_VERSION := 2024.1.1
BIN := bin/cail-acquire

.PHONY: build build-linux test integration vet staticcheck lint regen-fixtures clean

build:
	go build -o $(BIN) ./cmd/cail-acquire

# Release build for the droplet (SPEC §9.2): static, linux, pinned toolchain.
# GOTOOLCHAIN forces the production Go version; requires it to be available on
# the build box. See CLAUDE.md toolchain pin (currently flagged for reconcile).
build-linux:
	GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOOS=linux go build -o $(BIN) ./cmd/cail-acquire

# Unit tests only — no network (SPEC §8). The integration tag is excluded.
test:
	go test ./...

# Integration tests (SPEC §8): live fetches, behind the `integration` build tag.
# Manual / weekly CI only, never in the normal test run.
integration:
	go test -tags=integration ./...

vet:
	go vet ./...

# staticcheck via `go run` at a pinned version — no global install, not in go.mod.
staticcheck:
	go run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION) ./...

lint: vet staticcheck

# Capture the NL index page into the golden fixture (SPEC §8). Human-reviewed
# before commit (CLAUDE rule 13). Refreshes only testdata/nlpdp_index_happy.html;
# the synthetic zero/multi/relative fixtures are never overwritten.
regen-fixtures:
	go test -tags=integration -run TestCaptureNLIndex ./internal/fetch

clean:
	rm -rf bin/
