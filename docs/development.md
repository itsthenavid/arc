# Development Guide

## Toolchain

Arc uses pinned tool versions via mise.

Install all required tools with:

    mise install

---

## Environment files

Arc has two environment examples:

- `.env.example`: use when running services directly on the host machine.
- `.env.devcontainer.example`: use when running inside the devcontainer (service hosts resolve via Docker Compose service names).

Infrastructure environment is defined separately:

- `infra/.env.example`: used by Docker Compose to start PostgreSQL and Redis.

---

## Local workflow

- Start infrastructure: bash tools/scripts/infra-up.sh
- Run quality gates: bash tools/scripts/ci-local.sh

---

## Philosophy

There are no hidden steps. Everything must be explicit, repeatable, and documented.
