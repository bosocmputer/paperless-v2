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
