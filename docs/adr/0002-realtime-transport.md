# ADR-0002: Realtime Transport for Arc Messaging

## Status
Accepted (Phase 1)

## Context
Arc requires real-time bidirectional communication between Flutter clients and the Go backend for messaging.
Low latency, server push, and cross-platform support are required.

## Decision
Use WebSocket (WS) with a JSON-based, contract-first protocol (Arc Realtime Protocol v1).

## Alternatives Considered
1. HTTP Polling — High latency, poor UX.
2. Server-Sent Events — One-way only.
3. gRPC Streaming — Heavy for cross-platform clients.

## Consequences

### Positive
- True bidirectional communication.
- Works across mobile, desktop, and web.
- Easy debugging and iteration.
- Clear protocol contract.

### Negative / Risks
- Requires careful connection lifecycle management.
- Scaling requires fan-out strategy (future Redis pubsub).
- Security must be enforced at handshake and per-event validation.

## Follow-ups
- Implement WS handshake and messaging loop (PR-001).
- Add persistence and history fetch (PR-002).
- Introduce full authentication (PR-003).
