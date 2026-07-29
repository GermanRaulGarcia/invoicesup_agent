# InvoicesUp Connector Agent — MVP Implementation Plan

**Goal:** A cross-platform Go binary that pulls an accounting office's pending
invoice TXT from InvoicesUp over HTTPS, writes one `{code}_facturas.txt` per
business into a local folder Golden watches, and confirms delivery once Golden
imports and deletes the local file.

**Contract source:** `invoicesup/docs/superpowers/specs/2026-07-29-office-connector-design.md`
(the "Slice 2 — The agent (contract)" section) and the shipped backend API:
`GET /api/v1/connector/pending`, `POST /api/v1/connector/confirm`.

**MVP scope (this plan):** config, HTTP client, durable per-business state, the
reconcile state machine (pure + unit-tested), the poll loop, foreground run.
Builds and tests on macOS/Linux/Windows.

**Deferred (separate follow-up):** Windows service wrapper (NSSM), code signing,
auto-update, installer, tray UI. None are needed to prove the flow end-to-end.

## Module layout

```
invoicesup_agent/
  go.mod
  cmd/agent/main.go          # wiring + poll loop + signal handling
  internal/config/config.go  # load + validate config
  internal/api/client.go     # Pending(), Confirm() against the backend
  internal/state/store.go    # durable per-business state (atomic JSON file)
  internal/reconcile/reconcile.go  # PURE decision function (the crux)
  config.example.json
  README.md
```

Keeping `reconcile` pure (no I/O) is deliberate — it is where all the
correctness lives, so it must be unit-testable without a server or a disk.

## Core types

```go
// api.Batch — one business's pending export, from GET /connector/pending.
type Batch struct {
    BusinessCode string `json:"business_code"`
    Filename     string `json:"filename"`
    Content      string `json:"content"`
    BatchToken   string `json:"batch_token"`
}

// state.Entry — durable per-business state.
type Entry struct {
    Token string `json:"token"`
    State string `json:"state"` // "written" | "awaiting_confirm"
}
// state.Store is map[businessCode]Entry, persisted as one JSON file.

// reconcile.Action — what the loop must do (applied by the caller).
type Action struct {
    Kind    string // "write" | "confirm" | "clear"
    Code    string
    // write:
    Filename string
    Content  string
    // write/confirm:
    Token   string
}
```

## The state machine (reconcile — the crux)

`Reconcile(pending []Batch, fileExists func(code string) bool, store map[string]Entry) []Action`

Rules, evaluated per business code:

1. **A written file that Golden has now removed → confirm.**
   For each code in `store` with `State=="written"` where `!fileExists(code)`:
   emit `confirm{code, token=entry.Token}` (the loop, on a successful confirm,
   sets the entry to `awaiting_confirm` first then clears it — see ordering).

2. **A confirm that never completed → retry.**
   For each code in `store` with `State=="awaiting_confirm"`:
   emit `confirm{code, token=entry.Token}`.

3. **Idle business with pending content → write.**
   For each pending batch where the code has **no** `store` entry **and**
   `!fileExists(code)`: emit `write{code, filename, content, token}`.

4. **Otherwise do nothing:** a batch whose file still exists (Golden hasn't
   imported), or a code mid-cycle (`store` entry present) — the newest invoices
   wait until the current file clears. Confirm always uses the **persisted**
   token, never the latest pending token, so the batch Golden actually imported
   is the batch that gets marked delivered.

## Crash-safety ordering (the decision to confirm before coding)

Persisting state and the file write cannot be atomic across a crash, so one
ordering must be chosen:

- **Chosen: two-phase write (persist token first), then recover on startup.**
  The loop persists `{token, State=writing}` **before** the atomic file write,
  then persists `State=written`. On startup `Recover` reconciles each `writing`
  marker with the disk: file present → promote to `written` (keeping its token);
  file absent → drop it and rewrite the current pending batch fresh. Because the
  token is persisted *before* the file, a recovered file is always bound to the
  token that produced it — we never confirm a later superset batch whose extra
  invoices were never written (which would be a silent loss). If the `writing`
  record can't be persisted, the file is not written at all.

- **Residual window (documented, accepted):** file written → Golden imports and
  deletes it → agent crashes *before* persisting `written` (state still
  `writing`, file gone). On restart `Recover` drops the marker and the batch
  re-serves → Golden re-imports — a **detectable duplicate** (Golden rejects
  repeated invoice numbers), never a silent omission. This requires Golden to
  import within the ~millisecond gap between the file write and the next line;
  practically it does not happen.

- **Why this ordering:** the alternative (persist `written` first) turns the
  same window into a *silent missing invoice* instead of a duplicate. For
  accounting, a duplicate is detectable (Golden rejects repeated invoice
  numbers) and correctable; a silent omission is not. We prefer the detectable
  failure. A future hardening (server-side idempotency by invoice number, or a
  two-phase `writing`→`written` marker reconciled against the server) can close
  it entirely; out of scope for the MVP.

Confirm is idempotent server-side (`insertOrIgnore` on the unique index), so
retries in rules 1–2 are always safe.

## Tasks (TDD, commit after each)

Each task: write the Go test first, run it red, implement, run green, commit
(conventional commits, no AI-attribution trailer).

- **Task 1 — config.** `config.Load(path) (Config, error)`: parse JSON
  `{base_url, token, folder, poll_seconds}`, validate non-empty base_url/token/
  folder and poll_seconds ≥ 5 (default 30 when omitted). Test: valid load,
  missing field → error, poll default.
- **Task 2 — api client.** `client.New(baseURL, token)`, `Pending(ctx) ([]Batch,
  error)`, `Confirm(ctx, token) (delivered int, err error)`. Bearer auth header;
  JSON decode; non-2xx → error. Test with `httptest.Server`: pending happy path,
  401 → error, confirm posts the token and returns count.
- **Task 3 — state store.** `store.Load(path)`, `store.Save(path, m)` writing
  atomically (temp file + `os.Rename`). Test: round-trip, corrupt/missing file →
  empty store (not error).
- **Task 4 — reconcile (the crux).** Pure `Reconcile(...)` per the rules above.
  Tests: (a) idle+pending → write; (b) file still present → no action;
  (c) written+file-gone → confirm; (d) awaiting_confirm → confirm retry;
  (e) mid-cycle code with new pending → no second write; (f) orphan adoption
  helper (file present, no entry → written). This is the most-tested unit.
- **Task 5 — loop + wiring (`cmd/agent/main.go`).** Load config; every
  `poll_seconds`: `Pending` → adopt orphans → `Reconcile` → apply actions
  (write files, call `Confirm`, update+save store with the file-first ordering);
  structured logging; graceful shutdown on SIGINT/SIGTERM. Manual smoke test
  against a stub server documented in the README.
- **Task 6 — README + config.example.json.** How to configure, run in the
  foreground, and (notes only) later wrap as a Windows service with NSSM.

## Testing

`go test ./...` must pass. `go vet ./...` clean. The reconcile and config
packages carry the real coverage; the client uses `httptest`; the loop is
smoke-tested manually against a stub since its logic is thin over the tested
units.
