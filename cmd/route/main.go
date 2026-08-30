// Command route answers point-to-point transit queries from the terminal.
//
//	go run ./cmd/route -from 10911 -to 1120 -at 08:00:00 -date 20260805
//	go run ./cmd/route -from "U District Station" -to Westlake -at 08:00:00 -date 20260805
//
// It loads the compact timetable (data/timetable.gob, written by
// cmd/ingest), generates walking footpaths from stop coordinates, and
// runs RAPTOR.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"raptor-transit/internal/gtfs"
	"raptor-transit/internal/raptor"
	"raptor-transit/internal/stopsearch"
	"raptor-transit/internal/timetable"
	"raptor-transit/internal/transfers"
)

func main() {
	ttPath := flag.String("timetable", "data/timetable.gob", "path to timetable gob (from cmd/ingest)")
	from := flag.String("from", "", "origin stop_id or stop name")
	to := flag.String("to", "", "destination stop_id or stop name")
	at := flag.String("at", "", "departure time HH:MM:SS")
	date := flag.String("date", "", "service date YYYYMMDD")
	flag.Parse()

	if *from == "" || *to == "" || *at == "" || *date == "" {
		flag.Usage()
		os.Exit(2)
	}
	dep, err := gtfs.ParseTime(*at)
	if err != nil || dep < 0 {
		fmt.Fprintf(os.Stderr, "route: bad -at %q (want HH:MM:SS)\n", *at)
		os.Exit(2)
	}

	tt, err := timetable.Load(*ttPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "route: %v (run `go run ./cmd/ingest` first)\n", err)
		os.Exit(1)
	}
	paths, skipped := transfers.Generate(tt.StopLats, tt.StopLons, 200, 1.2)
	if skipped > 0 {
		fmt.Fprintf(os.Stderr, "note: %d stops skipped for bad coordinates\n", skipped)
	}
	eng := raptor.New(tt, paths)

	// Accept stop names as well as stop_ids on both ends.
	index := stopsearch.New(tt)
	fromID, err := resolve(index, "from", *from)
	if err != nil {
		fmt.Fprintf(os.Stderr, "route: %v\n", err)
		os.Exit(2)
	}
	toID, err := resolve(index, "to", *to)
	if err != nil {
		fmt.Fprintf(os.Stderr, "route: %v\n", err)
		os.Exit(2)
	}

	journeys, err := eng.Query(fromID, toID, int32(dep), *date)
	if err != nil {
		fmt.Fprintf(os.Stderr, "route: %v\n", err)
		os.Exit(1)
	}
	if len(journeys) == 0 {
		fmt.Printf("No journey found from %s to %s departing %s on %s.\n",
			stopLabel(tt, fromID), stopLabel(tt, toID), *at, *date)
		return
	}

	for i, j := range journeys {
		fmt.Printf("Option %d — arrive %s, %d transfer(s)\n", i+1, clock(j.Arrival), j.Transfers)
		for _, leg := range j.Legs {
			fromName := stopName(tt, leg.FromStop)
			toName := stopName(tt, leg.ToStop)
			if leg.Trip < 0 {
				fmt.Printf("  %s  walk    %s -> %s (arrive %s)\n",
					clock(leg.Departure), fromName, toName, clock(leg.Arrival))
			} else {
				fmt.Printf("  %s  route %-6s %s -> %s (arrive %s)\n",
					clock(leg.Departure), leg.Route, fromName, toName, clock(leg.Arrival))
			}
		}
		fmt.Println()
	}
}

// clock renders seconds-since-midnight as HH:MM:SS; times past 86400 wrap
// to next-day clock with a marker.
func clock(sec int32) string {
	suffix := ""
	if sec >= 86400 {
		sec -= 86400
		suffix = "+1d"
	}
	return fmt.Sprintf("%02d:%02d:%02d%s", sec/3600, sec%3600/60, sec%60, suffix)
}

func stopName(tt *timetable.Timetable, idx int32) string {
	name := tt.StopNames[idx]
	if name == "" {
		return tt.StopIDs[idx]
	}
	return name
}

func stopLabel(tt *timetable.Timetable, id string) string {
	if idx, ok := tt.StopIdx(id); ok {
		return fmt.Sprintf("%s (%s)", strings.TrimSpace(tt.StopNames[idx]), id)
	}
	return id
}

// resolve turns a -from/-to value (stop_id or name) into a stop_id. An
// ambiguous name lists the candidates rather than guessing, since picking
// one arbitrarily would silently plan a trip from the wrong stop.
func resolve(index *stopsearch.Index, flagName, input string) (string, error) {
	id, candidates, err := index.Resolve(input)
	if err == nil {
		return id, nil
	}
	if errors.Is(err, stopsearch.ErrAmbiguous) {
		var b strings.Builder
		fmt.Fprintf(&b, "-%s %q matches several stops:\n", flagName, input)
		for _, m := range candidates {
			fmt.Fprintf(&b, "    %-8s %s\n", m.StopID, m.Name)
		}
		b.WriteString("  retry with one of these stop IDs")
		return "", errors.New(b.String())
	}
	return "", fmt.Errorf("-%s %q: no matching stop", flagName, input)
}
