package stopsearch

import "errors"

// Resolve failure modes. Callers map these to HTTP status codes: ErrEmpty
// and ErrAmbiguous are the caller's fault (400), ErrNotFound is a miss (404).
var (
	// ErrEmpty means the input was blank or whitespace only.
	ErrEmpty = errors.New("stopsearch: empty query")
	// ErrNotFound means no stop_id or stop name matched.
	ErrNotFound = errors.New("stopsearch: no matching stop")
	// ErrAmbiguous means several stops matched equally well. The returned
	// candidates let the caller present a choice.
	ErrAmbiguous = errors.New("stopsearch: ambiguous stop name")
)
