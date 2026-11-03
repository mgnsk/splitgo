.DEFAULT_GOAL := list

DEV_ENV := DATABASE_DIR=./data BASE_URL=http://localhost:8080

# List all commands in this file.
.PHONY: list
list:
	@grep '^[^#[:space:].].*:' Makefile

# Run all tests.
.PHONY: test
test:
	go test ./...

# Fuzz the money logic for a short while.
.PHONY: fuzz
fuzz:
	go test ./money -run='^$$' -fuzz=FuzzParseMinor -fuzztime=30s
	go test ./split -run='^$$' -fuzz=FuzzCompute -fuzztime=30s
	go test ./balance -run='^$$' -fuzz=FuzzSimplify -fuzztime=30s

# Lint application code.
.PHONY: lint
lint:
	go vet ./...
	go tool revive -formatter=stylish -config=revive.toml ./...

# Run the application with automatic rebuild.
.PHONY: dev
dev:
	$(DEV_ENV) go tool air -c .air.toml

# Run the application.
.PHONY: run
run:
	$(DEV_ENV) go run ./cmd/splitgo

# Build the single static binary.
.PHONY: build
build:
	CGO_ENABLED=0 go build -trimpath -tags timetzdata -o splitgo ./cmd/splitgo

# Build the docker image.
.PHONY: docker
docker:
	docker build -t splitgo:edge .
