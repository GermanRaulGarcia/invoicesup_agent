# Code signing

The agent binary and installer should be signed with a code-signing certificate.
Unsigned, Windows SmartScreen and most antivirus flag them ("Unknown publisher"),
which scares office staff and can block the install.

## The certificate

You need an **OV or EV code-signing certificate** from a CA (DigiCert, Sectigo,
GlobalSign, SSL.com, …). Notes:

- **OV** works fine here; **EV** additionally clears SmartScreen reputation
  immediately (OV builds reputation over time/downloads).
- The CA gives you (or you export) a **`.pfx` / `.p12`** file — the certificate
  **plus its private key** — protected by a password. That `.pfx` + password is
  what signs the binary.
- EV certs are increasingly issued on hardware tokens/HSM; if yours is
  token-bound you can't export a `.pfx` and must sign on the machine with the
  token attached (see "Signing on Windows" with `signtool` + the token's CSP).

## Signing via build.sh (cross-platform)

`build.sh` has an opt-in signing step using [`osslsigncode`](https://github.com/mtrojnar/osslsigncode)
(works on macOS/Linux too — `brew install osslsigncode`). Set two env vars:

```bash
export SIGN_PFX="/secure/path/invoicesup-codesign.pfx"
export SIGN_PASS="the-pfx-password"
./build.sh 0.1.0
```

When `SIGN_PFX` is set, it runs:

```
osslsigncode sign \
  -pkcs12 "$SIGN_PFX" -pass "$SIGN_PASS" \
  -n "InvoicesUp Connector Agent" \
  -t http://timestamp.digicert.com \
  -in dist/invoicesup-agent.exe -out dist/invoicesup-agent.exe
```

Without `SIGN_PFX` it produces an unsigned binary (fine for testing).

## Signing on Windows (alternative: signtool)

`signtool` ships with the Windows SDK. From a Developer Command Prompt:

```bat
signtool sign ^
  /f invoicesup-codesign.pfx /p the-pfx-password ^
  /d "InvoicesUp Connector Agent" ^
  /tr http://timestamp.digicert.com /td sha256 /fd sha256 ^
  dist\invoicesup-agent.exe
```

For a hardware-token EV cert, drop `/f`/`/p` and use `/sha1 <thumbprint>` (or
`/n "<subject>"`) so it selects the cert from the token's store.

## Timestamping (do not skip)

The `-t` / `/tr` timestamp URL is important: it lets the signature stay **valid
after the certificate expires**. Without it, everything you signed stops
verifying the day the cert lapses. Any RFC 3161 server works
(`http://timestamp.digicert.com`, `http://timestamp.sectigo.com`, …).

## Sign both artifacts

Sign the **binary** (`build.sh`, above) **and** the **installer** that Inno Setup
produces (`invoicesup-agent-setup.exe`) — sign the setup .exe after compiling it,
the same way. Signing only one leaves the other flagged. (Inno Setup can also be
configured with a `SignTool` directive to sign automatically at compile time.)

## Verify

```bash
osslsigncode verify dist/invoicesup-agent.exe     # cross-platform
```
```bat
signtool verify /pa /v dist\invoicesup-agent.exe   :: Windows
```

Look for a valid chain and a present timestamp.

## Protect the .pfx

The `.pfx` is your signing identity — anyone with it (and the password) can sign
software as you. It's git-ignored (`*.pfx`, `*.p12`), but keep it out of the repo
regardless: store it in a secrets manager, and never pass `SIGN_PASS` on a shared
shell history (prefer a CI secret or an untracked env file).
