// Package reconcile is the agent's pure decision logic: given the server's
// pending batches, which local files exist, and the persisted state, it decides
// what the loop must do. It performs no I/O so the correctness-critical rules
// are unit-testable without a server or a disk.
package reconcile

import (
	"github.com/GermanRaulGarcia/invoicesup_agent/internal/api"
	"github.com/GermanRaulGarcia/invoicesup_agent/internal/state"
)

// Action kinds.
const (
	Write   = "write"
	Confirm = "confirm"
)

// Action is a side effect for the loop to apply.
type Action struct {
	Kind     string
	Code     string
	Filename string // write only
	Content  string // write only
	Token    string // write and confirm
}

// AdoptOrphans reconciles the store with the disk before deciding actions: a
// local file that exists with no store entry is a file we wrote but crashed
// before persisting "written" (the file-first ordering). Re-adopt it as
// written, using its pending batch's token, so it is not rewritten and its
// eventual deletion by Golden is still confirmed. Returns the updated store.
//
// Only pending batches can be adopted — if the file has no matching pending
// batch, the server already considers it delivered, so there is nothing to
// confirm and the lingering file is left alone.
func AdoptOrphans(pending []api.Batch, fileExists func(code string) bool, store state.Store) state.Store {
	for _, b := range pending {
		if _, known := store[b.BusinessCode]; known {
			continue
		}
		if fileExists(b.BusinessCode) {
			store[b.BusinessCode] = state.Entry{Token: b.BatchToken, State: state.Written}
		}
	}
	return store
}

// Reconcile decides the actions for one tick. Pure.
func Reconcile(pending []api.Batch, fileExists func(code string) bool, store state.Store) []Action {
	var actions []Action

	// Rules 1 & 2: outstanding confirmations. A "written" file that Golden has
	// now removed means the import happened; an "awaiting_confirm" entry is a
	// confirm that never completed and must be retried. Both confirm the
	// PERSISTED token — the batch Golden actually imported — never the latest
	// pending token.
	for code, entry := range store {
		switch entry.State {
		case state.Written:
			if !fileExists(code) {
				actions = append(actions, Action{Kind: Confirm, Code: code, Token: entry.Token})
			}
		case state.AwaitingConfirm:
			actions = append(actions, Action{Kind: Confirm, Code: code, Token: entry.Token})
		}
	}

	// Rule 3: an idle business (no store entry) with pending content and no
	// local file → write it. A business mid-cycle, or whose file still exists
	// (Golden has not imported yet), waits.
	for _, b := range pending {
		if _, mid := store[b.BusinessCode]; mid {
			continue
		}
		if fileExists(b.BusinessCode) {
			continue
		}
		actions = append(actions, Action{
			Kind:     Write,
			Code:     b.BusinessCode,
			Filename: b.Filename,
			Content:  b.Content,
			Token:    b.BatchToken,
		})
	}

	return actions
}
