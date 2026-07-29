package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/GermanRaulGarcia/invoicesup_agent/internal/api"
	"github.com/GermanRaulGarcia/invoicesup_agent/internal/state"
)

// End-to-end wiring test of the file-first lifecycle across two ticks:
// tick 1 writes the file and marks "written"; after Golden "imports" (we delete
// the local file), tick 2 confirms the batch and clears the state.
func TestRunOnceWriteThenConfirm(t *testing.T) {
	var mu sync.Mutex
	pendingActive := true
	var confirmedToken string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/api/v1/connector/pending":
			if pendingActive {
				_, _ = io.WriteString(w, `{"data":[{"business_code":"SPM","filename":"SPM_facturas.txt","content":"R#linea\r\n","batch_token":"t1"}]}`)
			} else {
				_, _ = io.WriteString(w, `{"data":[]}`)
			}
		case "/api/v1/connector/confirm":
			var body struct {
				BatchToken string `json:"batch_token"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			confirmedToken = body.BatchToken
			pendingActive = false // delivered → no longer pending
			_, _ = io.WriteString(w, `{"delivered":1}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	folder := t.TempDir()
	statePath := filepath.Join(folder, stateFileName)
	client := api.New(srv.URL, "tok")
	file := filepath.Join(folder, "SPM_facturas.txt")

	// Tick 1: writes the file, persists "written".
	if err := runOnce(context.Background(), client, folder, statePath); err != nil {
		t.Fatalf("tick 1: %v", err)
	}
	content, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("expected file written: %v", err)
	}
	if string(content) != "R#linea\r\n" {
		t.Fatalf("unexpected content: %q", content)
	}
	st, _ := state.Load(statePath)
	if st["SPM"].State != state.Written || st["SPM"].Token != "t1" {
		t.Fatalf("expected written/t1, got %+v", st["SPM"])
	}

	// While the file exists, another tick must NOT re-write or confirm.
	if err := runOnce(context.Background(), client, folder, statePath); err != nil {
		t.Fatalf("idle tick: %v", err)
	}
	if confirmedToken != "" {
		t.Fatalf("must not confirm while file present, confirmed %q", confirmedToken)
	}

	// Golden imports and deletes the local file.
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}

	// Tick 2: file gone → confirm t1 and clear state.
	if err := runOnce(context.Background(), client, folder, statePath); err != nil {
		t.Fatalf("tick 2: %v", err)
	}
	if confirmedToken != "t1" {
		t.Fatalf("expected confirm of t1, got %q", confirmedToken)
	}
	st, _ = state.Load(statePath)
	if _, still := st["SPM"]; still {
		t.Fatalf("expected state cleared after confirm, got %+v", st)
	}
}

// Critical-fix regression: the agent wrote SPM's file for token t1 and crashed
// before persisting "written" (state left as {t1, writing}). The server now
// re-serves SPM with a SUPERSET token t2 (extra invoices) while the old t1 file
// still sits on disk. Recovery must confirm the ORIGINAL t1 (what Golden
// actually imported), never the superset t2 — confirming t2 would mark the
// never-written extra invoices delivered (silent loss).
func TestRecoveryConfirmsOriginalTokenNotSuperset(t *testing.T) {
	var mu sync.Mutex
	var confirmedToken string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/api/v1/connector/pending":
			// SPM re-served with a superset token while its file is unimported.
			_, _ = io.WriteString(w, `{"data":[{"business_code":"SPM","filename":"SPM_facturas.txt","content":"R#one\r\nR#two\r\n","batch_token":"t2"}]}`)
		case "/api/v1/connector/confirm":
			var body struct {
				BatchToken string `json:"batch_token"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			confirmedToken = body.BatchToken
			_, _ = io.WriteString(w, `{"delivered":1}`)
		}
	}))
	defer srv.Close()

	folder := t.TempDir()
	statePath := filepath.Join(folder, stateFileName)
	file := filepath.Join(folder, "SPM_facturas.txt")

	// Simulate the crash aftermath: the t1 file on disk + a {t1, writing} marker.
	if err := os.WriteFile(file, []byte("R#one\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := state.Save(statePath, state.Store{"SPM": {Token: "t1", State: state.Writing}}); err != nil {
		t.Fatal(err)
	}

	client := api.New(srv.URL, "tok")

	// Tick 1: recovery promotes {t1,writing}+file → {t1,written}; file present,
	// so no confirm and no rewrite.
	if err := runOnce(context.Background(), client, folder, statePath); err != nil {
		t.Fatalf("recovery tick: %v", err)
	}
	st, _ := state.Load(statePath)
	if st["SPM"].State != state.Written || st["SPM"].Token != "t1" {
		t.Fatalf("expected promotion to written/t1, got %+v", st["SPM"])
	}
	if confirmedToken != "" {
		t.Fatalf("must not confirm while file present, confirmed %q", confirmedToken)
	}

	// Golden imports the t1 file (only "R#one") and deletes it.
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}

	// Tick 2: confirm the ORIGINAL t1, not the superset t2.
	if err := runOnce(context.Background(), client, folder, statePath); err != nil {
		t.Fatalf("confirm tick: %v", err)
	}
	if confirmedToken != "t1" {
		t.Fatalf("expected confirm of ORIGINAL t1, got %q (superset t2 would be silent loss)", confirmedToken)
	}
}
