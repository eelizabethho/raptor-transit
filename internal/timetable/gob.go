package timetable

import (
	"bufio"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
)

// Save writes the timetable to path as a gob file. Same temp-file+rename
// pattern as internal/gtfs: a crash mid-write never leaves a truncated
// file, and the temp file lives in the target directory because rename
// only works within one filesystem.
func (tt *Timetable) Save(path string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".timetable-*.gob.tmp")
	if err != nil {
		return fmt.Errorf("save timetable: %w", err)
	}
	defer os.Remove(tmp.Name())

	w := bufio.NewWriter(tmp)
	if err := gob.NewEncoder(w).Encode(tt); err != nil {
		tmp.Close()
		return fmt.Errorf("encode timetable: %w", err)
	}
	if err := w.Flush(); err != nil {
		tmp.Close()
		return fmt.Errorf("save timetable: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("save timetable: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("save timetable: %w", err)
	}
	return nil
}

// Load reads a timetable previously written by Save and rebuilds the
// derived lookup maps (gob does not persist unexported fields).
func Load(path string) (*Timetable, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("load timetable: %w", err)
	}
	defer file.Close()

	var tt Timetable
	if err := gob.NewDecoder(bufio.NewReader(file)).Decode(&tt); err != nil {
		return nil, fmt.Errorf("decode timetable: %w", err)
	}
	tt.buildLookups()
	return &tt, nil
}
