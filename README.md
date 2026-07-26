# raptor-transit

A Go implementation of the RAPTOR (Round-bAsed Public Transit Optimized Router)
transit routing algorithm over King County Metro GTFS data. Phase 1 focuses on
GTFS ingestion: downloading and parsing the feed into an internal format that
the routing engine and HTTP API will build on. Standard library only — no
third-party dependencies.

## Setup

Go 1.23 is installed at `~/.local/go` on this machine. Add it to your PATH:

```sh
export PATH=$HOME/.local/go/bin:$PATH
```

## Build, test, run

```sh
make build   # compile everything into bin/
make test    # run tests with the race detector
make run     # start the HTTP server on :8080
make lint    # run go vet
```

Once running, check health with:

```sh
curl localhost:8080/healthz
```
