#!/bin/sh
set -euo pipefail

go install ./cmd/morningweave

if [ -n "${GOBIN:-}" ]; then
  BIN_DIR="$GOBIN"
else
  BIN_DIR="$(go env GOPATH)/bin"
fi

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *)
    printf "\nAdded binary to %s, but it's not on PATH.\n" "$BIN_DIR"
    printf "Add it to your shell profile, e.g.:\n"
    printf "  export PATH=\"%s:\\$PATH\"\n\n" "$BIN_DIR"
    ;;
esac
