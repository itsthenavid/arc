<!-- docs/protocol-events.md -->

# Protocol and Events

## Naming scheme

arc.<domain>.<action>.v1

---

## Alpha events

- arc.messaging.message.created.v1
- arc.messaging.message.delivered.v1
- arc.messaging.message.read.v1
- arc.realtime.typing.started.v1
- arc.realtime.typing.stopped.v1
- arc.presence.updated.v1

---

## Rules

- JSON payloads in Alpha
- No breaking changes within the same version
- Events must be documented before implementation
