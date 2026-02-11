#!/usr/bin/env bash
set -Eeuo pipefail

# ci-local.sh - Run the same checks CI runs (local == CI).

bash tools/scripts/doctor.sh
bash tools/scripts/fmt.sh
bash tools/scripts/shfmt.sh
bash tools/scripts/shellcheck.sh
bash tools/scripts/lint.sh
bash tools/scripts/test.sh

echo "OK: Local CI passed."
