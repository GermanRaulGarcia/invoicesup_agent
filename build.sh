#!/usr/bin/env bash
# Cross-compiles the Windows agent binary into ./dist.
# Optional code signing: set SIGN_PFX (path to .pfx) and SIGN_PASS to sign the
# binary (requires osslsigncode); otherwise an unsigned binary is produced.
set -euo pipefail

version="${1:-dev}"
out="dist"
mkdir -p "$out"

echo "Building invoicesup-agent.exe (version: $version)..."
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -ldflags "-s -w" -o "$out/invoicesup-agent.exe" ./cmd/agent

if [[ -n "${SIGN_PFX:-}" ]]; then
  echo "Signing with $SIGN_PFX..."
  osslsigncode sign \
    -pkcs12 "$SIGN_PFX" -pass "${SIGN_PASS:-}" \
    -n "InvoicesUp Connector Agent" \
    -t http://timestamp.digicert.com \
    -in "$out/invoicesup-agent.exe" -out "$out/invoicesup-agent-signed.exe"
  mv "$out/invoicesup-agent-signed.exe" "$out/invoicesup-agent.exe"
  echo "Signed."
else
  echo "SIGN_PFX not set — producing an UNSIGNED binary (fine for testing)."
fi

echo "Done: $out/invoicesup-agent.exe"
