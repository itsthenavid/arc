#!/usr/bin/env bash
set -Eeuo pipefail

# sc-runner.sh - Run ShellCheck deterministically across repo shell scripts.
# Compatible with Bash 3.2 (macOS default /bin/bash). Avoids `mapfile`.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

if ! command -v shellcheck > /dev/null 2>&1; then
  echo "shellcheck: shellcheck not installed. Install it to enforce shell linting." >&2
  exit 1
fi

# Prefer repo-local config if present.
SHELLCHECK_RC=""
if [[ -f "${ROOT_DIR}/.shellcheckrc" ]]; then
  SHELLCHECK_RC="--shellcheckrc=${ROOT_DIR}/.shellcheckrc"
fi

# Collect shell files tracked by git (deterministic, avoids scanning build artifacts).
# - Include: *.sh and files that start with a bash/sh shebang.
# - Exclude: vendor-ish dirs if they ever get tracked (defensive).
files=()

# 1) *.sh files
while IFS= read -r f; do
  [[ -z "${f}" ]] && continue
  case "${f}" in
    */vendor/* | */node_modules/* | */.git/*) continue ;;
  esac
  files+=("${f}")
done < <(git ls-files '*.sh' 2> /dev/null || true)

# 2) Shebang scripts without .sh extension
# We scan tracked files only, and test first line cheaply.
while IFS= read -r f; do
  [[ -z "${f}" ]] && continue
  case "${f}" in
    */vendor/* | */node_modules/* | */.git/* | *.sh) continue ;;
  esac

  # Read first line safely (ignore binary / unreadable).
  first_line="$(LC_ALL=C sed -n '1p' "${f}" 2> /dev/null || true)"
  case "${first_line}" in
    '#!'*'sh'*)
      files+=("${f}")
      ;;
  esac
done < <(git ls-files 2> /dev/null || true)

if [[ "${#files[@]}" -eq 0 ]]; then
  echo "OK: shellcheck (no shell files found)"
  exit 0
fi

# Deduplicate (Bash 3.2 safe)
# We sort unique via printf/sort/awk and rebuild array.
uniq_files=()
while IFS= read -r f; do
  [[ -z "${f}" ]] && continue
  uniq_files+=("${f}")
done < <(printf "%s\n" "${files[@]}" | LC_ALL=C sort -u)

echo "shellcheck: ${#uniq_files[@]} file(s)"

# Run ShellCheck
# - Use bash by default (most of our scripts are bash), but ShellCheck will infer from shebang too.
# - Keep output stable.
if [[ -n "${SHELLCHECK_RC}" ]]; then
  shellcheck "${SHELLCHECK_RC}" \
    --format=gcc \
    "${uniq_files[@]}"
else
  shellcheck \
    --format=gcc \
    "${uniq_files[@]}"
fi

echo "OK: shellcheck"
