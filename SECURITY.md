# Security Policy

## Reporting a vulnerability

Please do not open a public issue for a suspected vulnerability. Use GitHub's
private vulnerability reporting for this repository:

https://github.com/jeretmccoy/kelma-sync/security/advisories/new

Include affected versions, impact, reproduction steps, and any suggested fix.
Reports involving cross-account data access, authentication bypass, token or
credential disclosure, destructive sync behavior, or storage-key isolation are
especially important.

## Operational security

- Terminate TLS before exposing the API to a network.
- Keep PostgreSQL and local media storage private.
- Use long random database and internal-API secrets.
- Rate-limit `/v2/auth/*` and request bodies at the reverse proxy.
- Keep `.env*`, database backups, media, and tokens out of Git and Docker build
  contexts.
- Back up PostgreSQL and media together; they form one logical data set.
- The `/v2/internal/*` endpoints are disabled when `KELMA_INTERNAL_SECRET` is
  empty. Do not expose them with a weak or reused secret.

Only the latest `main` branch receives security fixes during beta.
