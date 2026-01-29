# Infrastructure

The file infra/compose.yml is the single source of truth for local development infrastructure.

---

## Services

- PostgreSQL
- Redis
- Development container (optional)

---

## Usage

    docker compose --env-file infra/.env.example -f infra/compose.yml up -d
