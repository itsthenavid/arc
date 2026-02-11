## PR-013 — Docs, Runbooks, and Productionization Pass

### Objective
Finalize ADR-0002 delivery to “portfolio-grade” with documentation, scripts,
and end-to-end smoke coverage.

### Scope
- Update docs:
  - `docs/security.md` (auth threat model + mitigations)
  - `docs/development.md` (local auth flows, env vars)
  - `docs/realtime.md` (auth binding notes)
- Expand smoke scripts:
  - `smoke-all.sh` to include:
    - invite issue (optional)
    - signup/login/refresh
    - ws connect with auth token
- Add troubleshooting section + common CI pitfalls

### Quality Gates
- CI must pass on clean runner (no implicit local installs).
- One-command local verification: `./tools/scripts/smoke-all.sh`.
