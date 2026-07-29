# InvoicesUp Connector Agent

A small Go service that runs on an accounting office's PC. It pulls that office's
pending invoice TXT from InvoicesUp over HTTPS and writes one
`{code}_facturas.txt` file per business into a local folder that **Golden .Net**
watches. When Golden imports and deletes the local file, the agent confirms the
delivery back to InvoicesUp.

It replaces the SFTP-drive setup (rclone/RaiDrive): no SFTP server, no
third-party mount tool. The agent authenticates with a **connector token** an
admin generates for the office's user in InvoicesUp, so it only ever sees the
businesses that user is assigned (per-business visibility is enforced
server-side).

## Configure

Copy `config.example.json` to `config.json` and fill it in:

```json
{
  "base_url": "https://invoicesup.kordino.com",
  "token": "<connector token from InvoicesUp → Usuarios → editar → Generar token>",
  "folder": "C:\\InvoicesUp\\exports",
  "poll_seconds": 30
}
```

- `folder` is where Golden reads its import files (and where the agent keeps its
  small `.invoicesup-agent-state.json`).
- `poll_seconds` must be ≥ 5 (defaults to 30).

## Build & run

```bash
go build -o invoicesup-agent ./cmd/agent      # Linux/macOS
GOOS=windows GOARCH=amd64 go build -o invoicesup-agent.exe ./cmd/agent   # Windows

./invoicesup-agent -config config.json
```

The agent runs in the foreground and logs each write/confirm. Stop it with
Ctrl-C (it shuts down gracefully).

## How it works

Each poll:

1. `GET /api/v1/connector/pending` → the pending batches for this office.
2. For each business with new invoices and no local file → write
   `{code}_facturas.txt`.
3. When Golden imports and deletes a file the agent had written →
   `POST /api/v1/connector/confirm` marks that batch delivered.

State is tracked per business in `.invoicesup-agent-state.json` and is
**crash-safe**: the file is written before the state is marked, and an orphan
file left by a crash is re-adopted on restart, so a crash never causes a lost or
silently-missing invoice. In the (millisecond) window where a crash coincides
exactly with Golden's delete, a batch may be re-served and re-imported — a
**detectable duplicate** (Golden rejects repeated invoice numbers), never a
silent omission. This is a deliberate trade-off.

## Windows service

The binary is its own Windows service — no third-party wrapper. On Windows it
defaults its config to `C:\ProgramData\InvoicesUp\config.json` and, when the
Service Control Manager launches it, runs as a service (logging to
`C:\ProgramData\InvoicesUp\agent.log`); run from a console it stays in the
foreground.

Service commands (run an elevated prompt):

```
invoicesup-agent.exe install     # register + start the service (auto-start)
invoicesup-agent.exe stop
invoicesup-agent.exe start
invoicesup-agent.exe uninstall
invoicesup-agent.exe run         # run in the foreground (debugging)
```

## Packaging (installer)

1. Cross-compile the Windows binary (from macOS/Linux/Windows with Go):

   ```bash
   ./build.sh 0.1.0
   # → dist/invoicesup-agent.exe
   ```

   To ship a **signed** binary, set `SIGN_PFX` (path to your code-signing
   `.pfx`) and `SIGN_PASS` before running `build.sh` — signing needs your own
   certificate and is skipped otherwise.

2. On Windows, compile `installer/installer.iss` with
   [Inno Setup 6+](https://jrsoftware.org/isinfo.php) → `invoicesup-agent-setup.exe`.

The installer asks the office admin for the InvoicesUp URL, the connector token,
and the local folder, writes `C:\ProgramData\InvoicesUp\config.json`, installs
the binary, and registers + starts the service. Uninstall stops and removes it.

To ship signed binaries, see [`docs/code-signing.md`](docs/code-signing.md).

## Documentation

- [`docs/despliegue-oficina.md`](docs/despliegue-oficina.md) — short deployment
  guide for the accounting office (Spanish).
- [`docs/code-signing.md`](docs/code-signing.md) — signing the binary/installer
  with a `.pfx`.
- [`docs/plan.md`](docs/plan.md) — MVP design and the crash-safety model.

## Not yet included

Code signing certificate (yours to procure — the build wires the signing step)
and auto-update are out of scope for this MVP; add them once the flow is
validated in the field.
