// Command agent runs the InvoicesUp connector agent: it polls the connector API
// and mirrors pending invoice TXT into a local folder Golden watches, confirming
// each batch once Golden imports and deletes the local file.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GermanRaulGarcia/invoicesup_agent/internal/api"
	"github.com/GermanRaulGarcia/invoicesup_agent/internal/config"
	"github.com/GermanRaulGarcia/invoicesup_agent/internal/reconcile"
	"github.com/GermanRaulGarcia/invoicesup_agent/internal/state"
)

const stateFileName = ".invoicesup-agent-state.json"

func main() {
	// An optional leading subcommand, then flags: `agent [cmd] [-config path]`.
	cmd := ""
	args := os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet("invoicesup-agent", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath(), "path to the config file")
	_ = fs.Parse(args)

	switch cmd {
	case "", "run":
		runAgent(*configPath)
	case "install", "uninstall", "start", "stop":
		if err := controlService(cmd, *configPath); err != nil {
			log.Fatalf("%s: %v", cmd, err)
		}
	default:
		log.Fatalf("unknown command %q (use: install | uninstall | start | stop | run)", cmd)
	}
}

// runtime holds the wired dependencies a serve loop needs.
type runtime struct {
	client    *api.Client
	folder    string
	statePath string
	poll      time.Duration
}

// prepare loads config and wires the runtime — shared by the service and the
// foreground runner.
func prepare(configPath string) (*runtime, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.Folder, 0o755); err != nil {
		return nil, fmt.Errorf("cannot create folder %s: %w", cfg.Folder, err)
	}
	return &runtime{
		client:    api.New(cfg.BaseURL, cfg.Token),
		folder:    cfg.Folder,
		statePath: filepath.Join(cfg.Folder, stateFileName),
		poll:      time.Duration(cfg.PollSeconds) * time.Second,
	}, nil
}

// serve runs the poll loop until ctx is cancelled. Cross-platform: the Windows
// service handler and the foreground runner both drive it.
func serve(ctx context.Context, rt *runtime) {
	log.Printf("agent started: folder=%s poll=%s", rt.folder, rt.poll)
	ticker := time.NewTicker(rt.poll)
	defer ticker.Stop()

	for {
		if err := runOnce(ctx, rt.client, rt.folder, rt.statePath); err != nil {
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
		_, err := os.Stat(filepath.Join(folder, filenameFor(code)))
		return err == nil
	}

	// Reconcile "writing" markers with the disk: a file present means the write
	// landed (→ written), absent means it never completed (→ dropped, rewritten).
	if reconcile.Recover(fileExists, store) {
		if err := state.Save(statePath, store); err != nil {
			log.Printf("save (recover): %v", err)
		}
	}

	for _, a := range reconcile.Reconcile(pending, fileExists, store) {
		if ctx.Err() != nil {
			return nil // shutting down; unfinished actions resume next run
		}
		switch a.Kind {
		case reconcile.Write:
			name := filenameFor(a.Code)
			if name != filepath.Base(name) {
				log.Printf("write %s: unsafe business code, skipped", a.Code)
				continue
			}
			// Two-phase: persist intent+token BEFORE writing, so a file recovered
			// after a crash is always bound to the token that produced it (never a
			// later superset batch → no silent loss). If the intent can't be
			// recorded, do not write the file at all.
			store[a.Code] = state.Entry{Token: a.Token, State: state.Writing}
			if err := state.Save(statePath, store); err != nil {
				log.Printf("save (writing %s): %v", a.Code, err)
				delete(store, a.Code)
				continue
			}
			if err := writeFileAtomic(filepath.Join(folder, name), a.Content); err != nil {
				log.Printf("write %s: %v", a.Code, err) // stays "writing"; Recover retries next tick
				continue
			}
			store[a.Code] = state.Entry{Token: a.Token, State: state.Written}
			if err := state.Save(statePath, store); err != nil {
				log.Printf("save (written %s): %v", a.Code, err)
			}
			log.Printf("wrote %s", name)

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

// filenameFor is the single source of truth for a business's on-disk file name.
// Both the write path and the presence check derive it from the code, so they
// can never disagree on the path.
func filenameFor(code string) string {
	return code + "_facturas.txt"
}

// writeFileAtomic writes content to a temp file and renames it into place, so a
// reader (Golden) never observes a half-written file and a crash never leaves a
// truncated one.
func writeFileAtomic(path, content string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
