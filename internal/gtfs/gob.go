package gtfs

import (
	"bufio"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
)

// Save writes the feed to path as a gob file. It writes to a temp file
// and renames, so a crash mid-write never leaves a truncated file behind.
// The temp file lives in the target directory because rename only works
// within one filesystem.
func (f *Feed) Save(path string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".gtfs-*.gob.tmp")
	if err != nil {
		return fmt.Errorf("save feed: %w", err)
	}
	defer os.Remove(tmp.Name())

	w := bufio.NewWriter(tmp)
	if err := gob.NewEncoder(w).Encode(f); err != nil {
		tmp.Close()
		return fmt.Errorf("encode feed: %w", err)
	}
	if err := w.Flush(); err != nil {
		tmp.Close()
		return fmt.Errorf("save feed: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("save feed: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("save feed: %w", err)
	}
	return nil
}

// Load reads a feed previously written by Save.
func Load(path string) (*Feed, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("load feed: %w", err)
	}
	defer file.Close()

	var f Feed
	if err := gob.NewDecoder(bufio.NewReader(file)).Decode(&f); err != nil {
		return nil, fmt.Errorf("decode feed: %w", err)
	}
	return &f, nil
}
