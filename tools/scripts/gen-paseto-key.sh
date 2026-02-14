#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

tmp_go="$(mktemp "${TMPDIR:-/tmp}/arc-paseto-key.XXXXXX.go")"
cat > "${tmp_go}" << 'GOEOF'
package main

import (
	"fmt"

	paseto "aidanwoods.dev/go-paseto"
)

func main() {
	fmt.Print(paseto.NewV4AsymmetricSecretKey().ExportHex())
}
GOEOF

(
  cd "${ROOT_DIR}/server/go"
  go run "${tmp_go}"
)

rm -f "${tmp_go}"
