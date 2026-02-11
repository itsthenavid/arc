# Tools

This directory contains deterministic developer tooling for Arc.

## Principles

- Explicit over implicit
- Deterministic over convenient
- Documented over assumed
- Local == CI (same entrypoints)

## Common commands

### Infra
- Infrastructure up: `bash tools/scripts/infra-up.sh`
- Infrastructure down: `bash tools/scripts/infra-down.sh`

### Quality gates
- Format: `bash tools/scripts/fmt.sh`
- Lint: `bash tools/scripts/lint.sh`
- Script lint (shellcheck): `bash tools/scripts/shellcheck.sh`
- Script format (shfmt): `bash tools/scripts/shfmt.sh`
- Tests: `bash tools/scripts/test.sh`

### Local CI (what CI runs)
- Full local CI: `bash tools/scripts/ci-local.sh`

### Smoke (end-to-end)
- Full smoke (memory + postgres): `bash tools/scripts/smoke-all.sh`
- WebSocket smoke against a running server:
  - `URL="ws://127.0.0.1:8080/ws" bash tools/scripts/ws-smoke.sh`

## Outputs / Artifacts

- `tools/.state/infra.env` — generated ports for infra
- `tools/.state/smoke.json` — structured smoke report (JSON, stable schema)
