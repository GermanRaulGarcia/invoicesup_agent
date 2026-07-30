#!/usr/bin/env bash
# Builds the agent binary and bundles it with the .bat installer into a zip that
# an office can download and run (no Inno Setup / Windows build machine needed).
# Output: dist/invoicesup-agent-setup.zip
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
"$here/build.sh" "${1:-dev}"

cd "$here"
rm -f dist/invoicesup-agent-setup.zip
zip -j dist/invoicesup-agent-setup.zip \
  dist/invoicesup-agent.exe \
  installer/instalar.bat \
  installer/desinstalar.bat \
  installer/LEEME.txt

echo "Done: dist/invoicesup-agent-setup.zip"
