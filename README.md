# KelmaSync

KelmaSync is an open-source sync server for the Kelma flashcard ecosystem. It
uses an item-oriented v2 protocol: clients exchange manifests and transfer only
changed notes, cards, decks, notetypes, tombstones, and media.

The server is written in Go, stores structured data in PostgreSQL, and stores
media either on the local filesystem or in Cloudflare R2/S3-compatible object
storage. Reference Python clients and the canonical Rust checksum helper are
included.

> **Status:** Beta. Back up collections before testing and review the API and
> schema before operating a public service.

KelmaSync v2 is **not** the Anki wire protocol. Anki-based clients use the
included adapter logic to map collection records to the v2 API.

## Features

- Incremental manifest-based sync
- Explicit conflicts for note, deck, and notetype content
- Newest-wins scheduling synchronization
- Persistent tombstones for deletions
- Batched push, pull, and delete operations
- Local filesystem or R2 media storage
- Local accounts for self-hosting or an optional external account authority
- Bearer tokens stored as hashes, not plaintext

## Repository layout

| Path | Purpose |
|---|---|
| `cmd/server` | Go server entry point |
| `internal` | Authentication, API handlers, database, and media storage |
| `migrations` | PostgreSQL schema migrations |
| `clients/python` | Reference Python/Anki client implementation |
| `clients/rust/kelma-hash` | Canonical cross-language checksum implementation |
| `API.md` | HTTP API reference |
| `DESIGN.md` | Protocol and conflict model |
| `SCHEMA.md` | Database model |

## Quick start with Docker

```bash
cp .env.example .env
docker compose up --build
```

The API listens on `http://127.0.0.1:8080`. The default development setup uses
local authentication with open registration and persistent Docker volumes for
PostgreSQL and media.

Create an account and log in:

```bash
curl -sS -X POST http://127.0.0.1:8080/v2/auth/register \
  -H 'content-type: application/json' \
  -d '{"username":"demo@example.com","password":"change-me"}'

curl -sS -X POST http://127.0.0.1:8080/v2/auth/login \
  -H 'content-type: application/json' \
  -d '{"username":"demo@example.com","password":"change-me","client_label":"laptop"}'
```

## Native development

Requirements: Go 1.26+, PostgreSQL 16+, Python 3, and Rust for the optional
checksum helper.

```bash
make db-up
make test
make vet
make run
```

`make test` uses a dedicated `kelma_sync_test` database and never the development
or production database. See `.github/workflows/ci.yml` for the complete test
setup.

## Authentication modes

- `KELMA_AUTH_MODE=local`: users are stored in PostgreSQL and
  `POST /v2/auth/register` is enabled. This is the self-hosted default in the
  provided Compose files.
- Any other value: login is delegated to `KELMA_IMMERSION_LOGIN_URL` and local
  registration is disabled. This integration is optional and can be replaced
  by ecosystem builders.

**Local mode permits open registration.** Put public deployments behind TLS and
rate-limit authentication endpoints at the reverse proxy. Do not expose
PostgreSQL directly to the internet.

## Production

`docker-compose.prod.yml` is a generic single-host deployment template. Copy
`.env.prod.example` to `.env.prod`, replace every placeholder, then run:

```bash
./deploy.sh
```

The template binds the API to localhost by default. Place Caddy, nginx,
Traefik, or another TLS reverse proxy in front of it. For multi-host or
high-availability deployments, manage PostgreSQL and object storage separately.

## Security

Never commit `.env`, `.env.prod`, database dumps, media, bearer tokens, or object
storage credentials. See [SECURITY.md](SECURITY.md) for private vulnerability
reporting and operational guidance.

## License and trademarks

The software is licensed under the GNU Affero General Public License v3.0 or
later; see [LICENSE](LICENSE). Operators who modify KelmaSync and make it
available over a network must provide the corresponding source as required by
the AGPL.

The software license does not grant rights to the Kelma names or logos. The
project permits descriptive and compatibility uses and offers ecosystem
trademark licenses under [TRADEMARKS.md](TRADEMARKS.md).
