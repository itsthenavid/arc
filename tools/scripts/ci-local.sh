#!/usr/bin/env bash
set -euo pipefail

bash tools/scripts/doctor.sh
bash tools/scripts/fmt.sh
bash tools/scripts/lint.sh
bash tools/scripts/test.sh

echo "Local CI passed."
