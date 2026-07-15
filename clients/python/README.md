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
- `note_sync.py` — first usable note-only sync orchestration

This code is not stored in the v1 plugin repo. When the v2 server is ready, the
plugin/desktop app should vendor or package this client from `kelma-sync`.

Local install for development:

```bash
cd clients/python
python3 -m pip install -e .
```
