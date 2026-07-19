from __future__ import annotations

from typing import Any

from kelma_sync_v2.client import V2Client


def test_batch_pull_only_sends_reviews_after_capability_detection() -> None:
    client = V2Client("https://sync.invalid", "token")
    calls: list[tuple[str, str, Any]] = []

    def fake_json(method: str, path: str, payload: Any, **_: Any) -> dict[str, Any]:
        calls.append((method, path, payload))
        return {}

    client._json = fake_json  # type: ignore[method-assign]

    client.batch_pull(cards=[7])
    assert calls[-1][2] == {
        "notes": [],
        "cards": [7],
        "notetypes": [],
        "decks": [],
    }

    client.batch_pull(reviews=[1001])
    assert calls[-1][2] == {
        "notes": [],
        "cards": [],
        "notetypes": [],
        "decks": [],
        "reviews": [1001],
    }
