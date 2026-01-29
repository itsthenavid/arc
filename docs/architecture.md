<!-- docs/architecture.md -->

# Architecture

Arc begins as a modular monolith to maximize clarity and speed of iteration.

---

## Core components

- HTTP API for authentication and data access
- WebSocket gateway for realtime communication
- PostgreSQL as the system of record
- Redis for ephemeral state and coordination

---

## Design principles

- Explicit and versioned contracts
- Append-only messaging model
- Clear module boundaries

---

## Scaling path

- Alpha: single binary deployment
- Next: Redis-based fanout for multi-instance realtime
- Later: service separation only when justified
