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
