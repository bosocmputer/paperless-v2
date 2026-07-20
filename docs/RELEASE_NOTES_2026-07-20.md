# PaperLess Release Notes - 2026-07-20

## Shared SML Database Readiness

- Each SML database now receives one full schema verification per PaperLess installation.
- A successful result is stored without TTL and shared by every currently authorized SML user; no `user_id` is part of the registry key.
- Every login still verifies SML credentials and reloads database permissions.
- New databases are checked automatically with concurrency capped at two and PostgreSQL advisory locking to prevent duplicate work.
- Failed results are not retried automatically. Users see the stored reason and can retry after the SML issue is repaired.
- Ready results are rechecked only after a structural SML error, a verification-version change, or an explicit superadmin operation.
- `SML_TENANT_READINESS_REGISTRY_ENABLED=false` restores the previous live-check behavior without dropping the additive table.

## Production Deployment

- Commit: `d13867f`
- Release ID: `20260720101905`
- Pui: `http://45.122.49.250:8095`
- Wirat Home Mart: `http://43.240.113.44:8691`
- Both Web and readiness endpoints returned HTTP 200.
- Both PaperLess APIs were healthy and contained `sml_tenant_readiness_registry` after migration.
- Existing PaperLess DB and SML API containers were not restarted.
- Database backups, before/after container evidence, Compose snapshots, and smoke results are stored under `/data/paperless/releases/20260720101905/` on each server.
- Previous API/Web images tagged `2aba190` remain available for rollback.

## Central Container Build Pipeline

- Added GitHub Actions pipelines for PaperLess Web and API images.
- Images are built as `linux/amd64` on GitHub and published to GHCR with immutable Git commit tags.
- Frontend build and backend tests are release gates before image publication.
- Customer servers pull the same published image; source code and Docker build cache are no longer copied to or generated on either production server.
- Production deployment remains manual and service-scoped so an ordinary push cannot restart customer systems automatically.

## Central Pipeline Deployment

- Commit: `c3acecb`
- Release ID: `20260720112943`
- Web image: `ghcr.io/bosocmputer/paperless-web:c3acecb`
- Web digest: `sha256:1ea21b709330d22a0c80a87283e611c5a3f81f9ee602e05815ad2f7db95289f8`
- Deployed the same `linux/amd64` Web image to Pui and Wirat Home Mart by `docker pull`; PaperLess API, DB, and SML API were not restarted.
- Root, live, and ready checks returned HTTP 200 on both installations.
- Browser QA passed on desktop and 390px mobile with no horizontal overflow or console errors.
- Deployment evidence and rollback Compose snapshots are stored under `/data/paperless/releases/20260720112943/` on each server.
