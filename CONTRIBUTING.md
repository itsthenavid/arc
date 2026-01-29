# Contributing to Arc

Arc is intentionally deterministic and documented. Please keep it that way.

---

## Required checks

Before pushing any changes, run:

    bash tools/scripts/ci-local.sh

---

## Commit convention

Arc follows Conventional Commits:

- feat: new feature
- fix: bug fix
- docs: documentation changes
- chore: tooling or maintenance
- refactor: refactoring without behavior change
- test: test-related changes
- ci: CI or workflow changes

Examples:

- chore: scaffold phase-0 foundation
- docs: add alpha specification
- ci: add PR title gate

---

## Code style

- All code comments must be written in English.
- Avoid magic or implicit behavior.
- Keep changes small and focused.

---

## Review process

- Use the PR template.
- Update documentation when changing architecture, conventions, or contracts.
