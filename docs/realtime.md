<!-- docs/realtime.md -->

# Realtime Communication

Arc uses WebSocket as the primary realtime transport during Alpha.

---

## Goals

- Low latency
- Bidirectional communication
- Explicit contracts

---

## Evolution

- Single-instance realtime
- Redis pub/sub fanout
- Dedicated realtime services if required
