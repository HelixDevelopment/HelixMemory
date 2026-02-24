# HelixMemory - Unified Cognitive Memory Engine
# Module: digital.vasic.helixmemory

.PHONY: build test test-race test-short test-integration test-security test-bench test-stress
.PHONY: test-e2e test-coverage fmt vet lint clean help
.PHONY: infra-start infra-stop infra-status challenge challenge-internal

MODULE := digital.vasic.helixmemory
GOMAXPROCS ?= 2

build:
	go build ./...

test:
	GOMAXPROCS=$(GOMAXPROCS) go test -count=1 -race -p 1 ./...

test-race:
	GOMAXPROCS=$(GOMAXPROCS) go test -count=1 -race -p 1 ./...

test-short:
	GOMAXPROCS=$(GOMAXPROCS) go test -count=1 -short -p 1 ./...

test-integration:
	GOMAXPROCS=$(GOMAXPROCS) go test -count=1 -race -p 1 ./tests/integration/...

test-security:
	GOMAXPROCS=$(GOMAXPROCS) go test -count=1 -race -p 1 ./tests/security/...

test-stress:
	GOMAXPROCS=$(GOMAXPROCS) go test -count=1 -race -p 1 -timeout 5m ./tests/stress/...

test-bench:
	GOMAXPROCS=$(GOMAXPROCS) go test -bench=. -benchmem ./tests/benchmark/...

test-e2e:
	GOMAXPROCS=$(GOMAXPROCS) go test -count=1 -race -p 1 ./tests/e2e/...

test-coverage:
	GOMAXPROCS=$(GOMAXPROCS) go test -count=1 -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

fmt:
	gofmt -w .
	goimports -w .

vet:
	go vet ./...

lint:
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed"; exit 1; }
	golangci-lint run ./...

clean:
	rm -f coverage.out coverage.html
	go clean -cache

# Infrastructure
infra-start:
	docker compose -f docker/docker-compose.yml up -d

infra-stop:
	docker compose -f docker/docker-compose.yml down

infra-status:
	docker compose -f docker/docker-compose.yml ps

# Challenges (run from parent HelixAgent project)
challenge:
	../challenges/scripts/helixmemory_challenge.sh

challenge-internal:
	./challenges/scripts/helixmemory_structure_challenge.sh
	./challenges/scripts/helixmemory_tests_challenge.sh
	./challenges/scripts/helixmemory_interfaces_challenge.sh

help:
	@echo "HelixMemory - Unified Cognitive Memory Engine"
	@echo ""
	@echo "Build & Test:"
	@echo "  make build         Build all packages"
	@echo "  make test          Run all tests with race detection"
	@echo "  make test-short    Run unit tests only"
	@echo "  make test-bench    Run benchmarks"
	@echo "  make test-coverage Generate coverage report"
	@echo ""
	@echo "Quality:"
	@echo "  make fmt           Format code"
	@echo "  make vet           Run go vet"
	@echo "  make lint          Run golangci-lint"
	@echo ""
	@echo "Infrastructure:"
	@echo "  make infra-start   Start backend services"
	@echo "  make infra-stop    Stop backend services"
	@echo "  make infra-status  Show service status"
