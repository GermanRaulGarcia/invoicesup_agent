// Command agent runs the InvoicesUp connector agent: it polls the connector API
// and mirrors pending invoice TXT into a local folder Golden watches, confirming
// each batch once Golden imports and deletes the local file.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/GermanRaulGarcia/invoicesup_agent/internal/api"
	"github.com/GermanRaulGarcia/invoicesup_agent/internal/config"
	"github.com/GermanRaulGarcia/invoicesup_agent/internal/reconcile"
	"github.com/GermanRaulGarcia/invoicesup_agent/internal/state"
)

const stateFileName = ".invoicesup-agent-state.json"

func main() {
	configPath := flag.String("config", "config.json", "path to the config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := os.MkdirAll(cfg.Folder, 0o755); err != nil {
		log.Fatalf("cannot create folder %s: %v", cfg.Folder, err)
	}

	client := api.New(cfg.BaseURL, cfg.Token)
	statePath := filepath.Join(cfg.Folder, stateFileName)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("agent started: base=%s folder=%s poll=%ds", cfg.BaseURL, cfg.Folder, cfg.PollSeconds)

	ticker := time.NewTicker(time.Duration(cfg.PollSeconds) * time.Second)
	defer ticker.Stop()

	for {
		if err := runOnce(ctx, client, cfg.Folder, statePath); err != nil {
			log.Printf("tick: %v", err)
		}
		select {
		case <-ctx.Done():
			log.Println("shutting down")
			return
		case <-ticker.C:
		}
	}
}

// runOnce performs a single poll → reconcile → apply cycle. Per-action failures
// are logged and skipped (retried next tick); it only returns an error when the
// cycle cannot start (state load / server unreachable).
func runOnce(ctx context.Context, client *api.Client, folder, statePath string) error {
	store, err := state.Load(statePath)
	if err != nil {
		return err
	}

	pending, err := client.Pending(ctx)
	if err != nil {
		return err
	}

	fileExists := func(code string) bool {
		_, err := os.Stat(filepath.Join(folder, code+"_facturas.txt"))
		return err == nil
	}

	// Re-adopt any file we wrote but crashed before persisting (file-first
	// ordering), and persist the adoption so its eventual deletion is confirmed
	// rather than re-served (which would double-import).
	before := len(store)
	store = reconcile.AdoptOrphans(pending, fileExists, store)
	if len(store) != before {
		if err := state.Save(statePath, store); err != nil {
			log.Printf("save (adopt): %v", err)
		}
	}

	for _, a := range reconcile.Reconcile(pending, fileExists, store) {
		switch a.Kind {
		case reconcile.Write:
			// File FIRST, then persist "written": a crash in between leaves an
			// orphan file that AdoptOrphans re-adopts, never a "written" marker
			// for a file that was never created.
			if err := os.WriteFile(filepath.Join(folder, a.Filename), []byte(a.Content), 0o644); err != nil {
				log.Printf("write %s: %v", a.Code, err)
				continue
			}
			store[a.Code] = state.Entry{Token: a.Token, State: state.Written}
			if err := state.Save(statePath, store); err != nil {
				log.Printf("save (write %s): %v", a.Code, err)
			}
			log.Printf("wrote %s", a.Filename)

		case reconcile.Confirm:
			// Persist "awaiting_confirm" BEFORE the network call so a crash mid
			// confirm retries next tick instead of losing the pending confirm.
			store[a.Code] = state.Entry{Token: a.Token, State: state.AwaitingConfirm}
			if err := state.Save(statePath, store); err != nil {
				log.Printf("save (confirm %s): %v", a.Code, err)
			}
			n, err := client.Confirm(ctx, a.Token)
			if err != nil {
				log.Printf("confirm %s: %v", a.Code, err)
				continue // stays awaiting_confirm, retried next tick
			}
			delete(store, a.Code)
			if err := state.Save(statePath, store); err != nil {
				log.Printf("save (clear %s): %v", a.Code, err)
			}
			log.Printf("confirmed %s (%d delivered)", a.Code, n)
		}
	}

	return nil
}
