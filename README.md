# PaperLess V2

PaperLess / E-Signature starter stack for the customer pilot.

## Stack

- Frontend: Vue 3, Vite, PrimeVue 4, Sakai-inspired layout
- Backend: Go HTTP API
- Database: PostgreSQL 16
- Runtime: Docker Compose with 3 containers

## Default Login

- Username: `superadmin`
- Password: `superadmin`
- Role: `admin`

## Local Source Flow

This repository is meant to be pushed to GitHub and built on the dev server.
Local machines do not need Docker or dependency installs.

## Server Deploy

```bash
git pull
cp .env.example .env
docker compose up -d --build
```

The frontend is exposed on port `3070`, backend on `8080`, and Postgres on `54320`.

