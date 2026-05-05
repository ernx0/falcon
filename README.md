# Falcon

Bug bounty recon & findings platform.

- **API** (Go + Chi + Postgres) — programs/targets/scopes/findings/runs/assets CRUD, auth, scheduler.
- **Worker** (Go + Asynq) — recon pipeline: subfinder → httpx → naabu → katana → playwright.
- **Web** (React + Vite + shadcn) — single-user UI for managing programs and findings.

## Quick start

```bash
cp .env.example .env
docker compose up -d --build
# UI:  http://localhost:5173
# API: http://localhost:8080
# Login: admin@falcon.local / admin (change in .env)
```

## Architecture

Worker has no DB access. All persistence flows through API REST endpoints
(`/internal/*` protected by `X-Worker-Token`). Asset bulk upserts apply
out-of-scope matching server-side and flag (not delete) matching rows.

See `bak-soyle-bir-proje-abstract-sutton.md` for the full design.
