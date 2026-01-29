# Arc

Arc is a modern, open-source, real-time messaging system focused on clarity, performance, calm UX, and professional engineering. Built as an educational, fully documented reference architecture, Arc is smooth and fluid by design and is written with modern Go and Flutter. Currently in **Alpha**.

---

## Why Arc Exists

The core idea of Arc was born during the 2026 internet blackout imposed by the Islamic Republic of Iran. During that period, access to the international internet was cut off, and a strange emptiness settled in.

For me, computers and the internet were not merely tools of convenience; they were tools of survival. I used them to escape the harshness of everyday reality—immersing myself for hours in technology: colorful, fascinating, alive.

When that connection disappeared, it felt as if all of it was taken away at once. Worse, it reawakened a deeper darkness inside me: a return of hopelessness, and even self-disgust—disgust at myself for thinking about my own state while people were dying.

Yet beyond all of this, one truth remains: freedom does not die. Arc exists as a remembrance. For everyone.

Just when you hold peak dominance in darkness and control; just when you have imposed limitations more than ever before; just when you have darkened everything, right there, freedom is born.

May the Force be with us all.

---

## Status

Arc is in **Alpha**. Alpha is intentionally small: we build the core messaging loop with professional architecture and calm UX, then evolve.

---

## Repository Layout

- client/flutter — Flutter client (UX + UI)
- server/go — Go backend (HTTP + WebSocket)
- infra — Local development infrastructure (Postgres + Redis)
- docs — Specifications, architecture, ADRs
- tools — Deterministic developer scripts
- shared — Shared contracts and definitions

---

## Learning & Education

Arc is designed to be a learning resource:

- Engineering decisions are documented (ADRs, architecture, conventions)
- Tooling is deterministic (no hidden steps)
- CI enforces quality gates (format, lint, test)

---

## Quickstart

### Install toolchain

Install mise, then run:

    mise install

### Start infrastructure

    bash tools/scripts/infra-up.sh

### Run local CI

    bash tools/scripts/ci-local.sh

---

## License

MIT.
