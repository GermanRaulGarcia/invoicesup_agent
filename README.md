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

## Running as a Windows service (later)

For unattended operation, wrap the binary as a Windows service so it starts with
the machine, e.g. with [NSSM](https://nssm.cc/):

```
nssm install InvoicesUpAgent "C:\InvoicesUp\invoicesup-agent.exe" -config "C:\InvoicesUp\config.json"
nssm start InvoicesUpAgent
```

Run it under the account that has the Golden import folder mapped so Golden and
the agent share the same `folder`.

## Not yet included

Code signing, an installer, and auto-update are out of scope for this MVP —
they are packaging concerns to add once the flow is validated in the field.
