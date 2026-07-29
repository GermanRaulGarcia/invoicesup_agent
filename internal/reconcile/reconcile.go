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

// Action is a side effect for the loop to apply. The on-disk filename is
// derived deterministically from Code by the loop ({code}_facturas.txt) — the
// same rule the presence check uses — so writes and existence checks can never
// disagree on the path.
type Action struct {
	Kind    string
	Code    string
	Content string // write only
	Token   string // write and confirm
}

// Recover reconciles "writing" markers with the disk on startup (and harmlessly
// every tick). A "writing" entry was persisted WITH its token before the atomic
// file write, so recovery never has to guess which batch a file holds:
//
//   - file present → the write landed → promote to "written" (same token).
//   - file absent  → the write never completed → drop it, so the current
//     pending batch is written fresh next.
//
// This is why the token is persisted before the file: binding a recovered file
// to the *current* pending token instead (an earlier approach) could confirm a
// superset batch whose extra invoices were never in the file — a silent loss.
// Returns whether the store changed (so the caller can persist it).
func Recover(fileExists func(code string) bool, store state.Store) bool {
	changed := false
	for code, e := range store {
		if e.State != state.Writing {
			continue
		}
		if fileExists(code) {
			store[code] = state.Entry{Token: e.Token, State: state.Written}
		} else {
			delete(store, code)
		}
		changed = true
	}
	return changed
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
			Kind:    Write,
			Code:    b.BusinessCode,
			Content: b.Content,
			Token:   b.BatchToken,
		})
	}

	return actions
}
