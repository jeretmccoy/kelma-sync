# KelmaSync v2 Python client

This package contains the v2 client implementation for Python-based clients
(Anki plugin / KelmaDesktop), but lives in the v2 repo so it does not pollute the
v1 `kelma_sync` codebase.

Package: `kelma_sync_v2`

Modules:

- `client.py` — raw REST client (`V2Client`, `V2Conflict`)
- `anki_local.py` — converts an Anki collection into v2 record/manifest shapes
- `engine.py` — sync planning and explicit resolution helpers
- `anki_apply.py` — safe local Anki apply helpers for server note records
- `note_sync.py` / `card_sync.py` — content and scheduling reconciliation
- `review_sync.py` — append-only revlog union plus portable daily-limit counters
- `content_sync.py` — full deck/notetype/note/card/review/media orchestration

The Anki plugin and KelmaDesktop vendor this package; changes here must remain
byte-for-byte compatible with their bundled `kelma_sync_v2` copies.

Local install for development:

```bash
cd clients/python
python3 -m pip install -e .
```
