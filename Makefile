# Makefile for raptor-transit.
# Common tasks: make build, make test, make run, make lint.
# Assumes `go` is on PATH (see README for setup on this machine).

.PHONY: build test run lint fetch ingest

# Download the King County Metro GTFS feed (redirects to metro.kingcounty.gov).
fetch:
	mkdir -p data
	curl -sL -o data/google_transit.zip https://www.soundtransit.org/GTFS-KCM/google_transit.zip

# Parse the feed and write data/gtfs.gob.
ingest:
	go run ./cmd/ingest

build:
	go build -o bin/ ./...

test:
	go test -race ./...

run:
	go run ./cmd/server

lint:
	go vet ./...
