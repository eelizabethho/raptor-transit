// Command ingest parses a GTFS zip feed into the internal Feed structure
// and gob-serializes it for later use by the routing server.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"raptor-transit/internal/gtfs"
)

func main() {
	in := flag.String("in", "data/google_transit.zip", "path to GTFS zip file")
	out := flag.String("out", "data/gtfs.gob", "path to write the gob file")
	flag.Parse()

	start := time.Now()
	feed, err := gtfs.ParseZip(*in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest: %v\n", err)
		os.Exit(1)
	}
	parseDur := time.Since(start)

	if err := feed.Save(*out); err != nil {
		fmt.Fprintf(os.Stderr, "ingest: %v\n", err)
		os.Exit(1)
	}

	info, err := os.Stat(*out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ingest: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("parsed %s in %s\n", *in, parseDur.Round(time.Millisecond))
	fmt.Printf("  stops:      %d\n", len(feed.Stops))
	fmt.Printf("  routes:     %d\n", len(feed.Routes))
	fmt.Printf("  trips:      %d\n", len(feed.Trips))
	fmt.Printf("  stop_times: %d\n", feed.NumStopTimes())
	fmt.Printf("  services:   %d\n", len(feed.Services))
	fmt.Printf("wrote %s (%.1f MB) in %s total\n",
		*out, float64(info.Size())/(1<<20), time.Since(start).Round(time.Millisecond))
}
