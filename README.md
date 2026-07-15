# KelmaSync

KelmaSync is open sync infrastructure for the Kelma flashcard ecosystem. Its
purpose is not merely to provide another place to upload an Anki collection. It
is intended to make the **server, protocol, provider architecture, and client
integration points** part of the open ecosystem so that users and builders can
run independent services, inspect sync behavior, and develop new clients
without depending on a private hosted backend.

The server uses an item-oriented v2 protocol: clients exchange manifests and
transfer only changed notes, cards, decks, notetypes, tombstones, and media. It
is written in Go, stores structured data in PostgreSQL, and stores media either
on the local filesystem or in Cloudflare R2 through its S3-compatible API.
Reference Python clients and the canonical Rust checksum helper are included.

> **Status:** Beta. Back up collections before testing and review the API and
> schema before operating a public service.

## Why this exists

Anki demonstrated the value of an open-source flashcard client, but the sync
experience still centers on AnkiWeb, a service controlled and operated by one
provider. That creates a practical boundary around an otherwise open ecosystem:
independent builders can modify the client, but cannot extend or participate in
the production AnkiWeb service architecture on equal terms.

There is an important distinction:

- The [Anki desktop client](https://github.com/ankitects/anki) is open source.
- Anki has included an official self-hosted server in desktop releases since
  2.1.57 and a standalone Rust server since 2.1.66. The
  [official manual](https://docs.ankiweb.net/sync-server.html) documents the
  bundled server, the official Python package, and the `anki-sync-server` Rust
  binary; their source lives in the Anki repository. Unofficial third-party
  servers also exist, but they are no longer the only self-hosting option.
- Anki explicitly targets its bundled server at individual/family use and notes
  that additions such as a REST API or external databases are unlikely to be
  accepted because simplicity is a design goal. It is an official way to
  reproduce the Anki sync model, not an extensible provider platform.
- The hosted AnkiWeb service—including its production account system, storage,
  deployment, operations, and product policies—is not an open, federated
  provider platform. Running the standalone server does not make AnkiWeb itself
  community-operated or turn sync providers into interchangeable ecosystem
  participants.

KelmaSync addresses that final layer. The complete service architecture is
published under a network copyleft license, local accounts work without a Kelma
service, the account authority is replaceable, storage is replaceable, and the
HTTP API is documented for non-Anki clients as well as Anki adapters.

The goal is an ecosystem in which:

- users can choose and change their sync provider;
- clients can expose a configurable sync endpoint instead of hard-coding one
  hosted service;
- organizations can self-host without depending on a project-operated account
  database;
- ecosystem builders can offer compatible sync services and integrations;
- conflict behavior is visible and controllable rather than hidden inside a
  collection merge; and
- changes to the deployed server can remain available to the community under
  the AGPL.

## Why not use the traditional Anki sync design?

The Anki protocol is effective at keeping Anki collections synchronized and has
years of compatibility work behind it. Its constraints follow from its original
purpose: synchronize Anki's SQLite collection model faithfully, including
scheduler state and legacy clients.

That model has tradeoffs for a broader multi-client ecosystem:

- **The collection is the main synchronization boundary.** Notes, cards,
  notetypes, decks, and collection metadata participate in a coordinated sync
  process rather than acting as independently addressable API resources.
- **The protocol is specialized and stateful.** Incremental sync proceeds
  through a multi-request transaction, while collection and media sync use
  separate protocol surfaces. Correct implementations are closely tied to
  Anki's internal schema and merge engine.
- **Some divergence becomes a full-sync decision.** Schema or consistency
  conditions can require replacing the whole collection with an upload or a
  download instead of resolving only the affected records.
- **Conflicts have limited item-level visibility.** The protocol is optimized
  for automatic convergence, not for showing a user two versions of one note
  and asking which should become canonical.
- **Routing is not a first-class protocol concept.** The normal model is one
  collection account and one configured sync service, rather than per-deck or
  multi-provider routing controlled by a client.
- **Extending the model is difficult for independent clients.** A new client
  must reproduce Anki collection semantics and wire behavior even if it only
  needs a subset of the flashcard domain.

These are reasonable choices for Anki compatibility, not defects in every use
case. KelmaSync v2 chooses different tradeoffs because its target is an open
provider and client ecosystem, not drop-in AnkiWeb compatibility.

## Design

KelmaSync stores a normalized canonical record for each resource. A client
fetches a lightweight manifest, compares checksums with local state, and then
pulls, pushes, or deletes only the records that differ.

```text
Kelma / Anki adapter / independent client
                 │
                 │  authenticated HTTP + JSON manifests/batches
                 ▼
          KelmaSync Go API
             │         │
             │         └── media: local filesystem or Cloudflare R2
             ▼
       PostgreSQL canonical records
       + clients, tokens, checksums, tombstones
```

Content writes include the checksum the client previously observed. If the
server has changed since then, it returns a conflict instead of silently
replacing either copy. The client can accept the server copy, explicitly force
the local copy, or present both versions for a user-authored merge. Scheduling
state uses newest-wins because review state changes continuously and is not
well-suited to manual field-by-field conflict resolution.

| Concern | Traditional Anki sync model | KelmaSync v2 |
|---|---|---|
| Primary boundary | Collection and protocol tables | Individual domain resources |
| Transport | Specialized stateful sync protocol | Documented HTTP/JSON API |
| Change discovery | Collection metadata and update sequence | Per-resource manifests and checksums |
| Content conflicts | Protocol merge or collection-level choice | Explicit record-level conflict response |
| Full replacement | Required in some divergence cases | Not part of the normal item-sync flow |
| Client scope | Reproduce Anki collection semantics | Use adapters or implement the resource API |
| Provider model | Anki-compatible server endpoint | Replaceable auth, storage, server, and clients |
| Source obligation | Depends on the implementation/service | AGPL applies to network-modified KelmaSync |

KelmaSync v2 is therefore **not** the Anki wire protocol and is not a drop-in
AnkiWeb replacement. Anki-based clients use the included adapter logic to map
collection records to the v2 API.

## Current limitations and non-goals

Openness does not make this design complete or automatically better for every
installation. Today:

- existing Anki clients require an adapter and cannot point their stock Anki
  sync setting directly at this API;
- this is beta software with a young protocol and migration history;
- it is not a CRDT and does not merge concurrent text edits automatically;
- the server keeps current canonical state and tombstones, not a complete
  revision history that users can browse;
- tombstones are retained indefinitely;
- scheduling conflicts use newest-wins rather than manual resolution;
- batch operations reduce round trips but are not a single global collection
  transaction;
- the included deployment is single-host oriented and does not provide built-in
  multi-region replication or high availability;
- TLS, abuse prevention, authentication rate limits, email verification,
  password recovery, and an operator administration UI must be supplied by the
  deployment or surrounding services; and
- data is not end-to-end encrypted: a sync operator can access the stored
  collection and media.

These limitations are documented so builders can improve them in public rather
than relying on an opaque production service.

## Features

- Incremental manifest-based sync
- Explicit conflicts for note, deck, and notetype content
- Newest-wins scheduling synchronization
- Persistent tombstones for deletions
- Batched push, pull, and delete operations
- Local filesystem or Cloudflare R2 media storage
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
