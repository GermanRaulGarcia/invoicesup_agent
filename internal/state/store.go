// Package state persists the agent's per-business delivery state across runs.
package state

import (
	"encoding/json"
	"os"
)

// Per-business lifecycle states.
const (
	// Writing is the intent recorded (with the batch token) BEFORE the local
	// file is written, so a file recovered after a crash is always bound to the
	// token that actually produced it. On restart: file present → the write
	// landed (promote to Written); file absent → the write never completed
	// (drop it, rewrite the current pending batch fresh).
	Writing = "writing"
	// Written means the local file was created and is waiting for Golden to
	// import and delete it.
	Written = "written"
	// AwaitingConfirm means Golden removed the file (imported) and the server
	// confirmation is pending/being retried.
	AwaitingConfirm = "awaiting_confirm"
)

// Entry is the durable state for one business code.
type Entry struct {
	Token string `json:"token"`
	State string `json:"state"`
}

// Store maps a business code to its current Entry.
type Store map[string]Entry

// Load reads the state file. A missing or corrupt file yields an empty store
// rather than an error, so a lost/garbled state never wedges the agent.
func Load(path string) (Store, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Store{}, nil
		}
		return nil, err
	}

	var s Store
	if err := json.Unmarshal(b, &s); err != nil || s == nil {
		return Store{}, nil
	}
	return s, nil
}

// Save atomically writes the state file (temp file + rename) so a crash mid-write
// never leaves a truncated state.
func Save(path string, s Store) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
