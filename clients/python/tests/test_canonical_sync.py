from __future__ import annotations

import unittest
from unittest.mock import patch

from kelma_sync_v2 import canonical_sync


class FakeDB:
    def list(self, sql: str, *_args):
        if "SELECT id FROM cards" in sql:
            return list(range(1, 2502))
        raise AssertionError(sql)


class FakeDecks:
    def by_name(self, name: str):
        return {"id": 42, "name": name}


class FakeCollection:
    def __init__(self) -> None:
        self.db = FakeDB()
        self.decks = FakeDecks()


class FakeClient:
    def __init__(self) -> None:
        self.payloads: list[dict] = []
        self.deleted: list[str] = []

    def batch_push(self, payload: dict, *, force: bool = False) -> dict:
        self.assert_force = force
        self.payloads.append(payload)
        return {"accepted": {key: len(value) for key, value in payload.items()}}

    def delete_deck(self, name: str) -> None:
        self.deleted.append(name)


class NamespacePublishTest(unittest.TestCase):
    def test_one_namespace_decision_publishes_cards_in_batches(self) -> None:
        col = FakeCollection()
        client = FakeClient()

        def deck_record(_col, name: str):
            return {
                "name": name,
                "config": {"dyn": 0},
                "client_modified_at": "2026-07-13T00:00:00Z",
            }

        def card_record(_col, card_id: int):
            return {
                "card_id": card_id,
                "note_guid": f"guid-{card_id}",
                "deck_name": "Parent::Mes Mots",
                "ord": 0,
                "scheduling": {},
                "client_modified_at": "2026-07-13T00:00:00Z",
            }

        with (
            patch.object(canonical_sync.anki_local, "deck_record", side_effect=deck_record),
            patch.object(canonical_sync.anki_local, "card_record", side_effect=card_record),
        ):
            totals = canonical_sync.push_selected_client_state(
                col,
                client,
                changes=[{
                    "resource": "decks",
                    "key": "namespace:membership",
                    "namespace": True,
                    "selected_namespace": "Parent::Mes Mots",
                    "target_namespace": "Mes Mots",
                    "server_item": {"namespace_name": "Mes Mots"},
                }],
                deck_names=["Mes Mots", "Parent::Mes Mots"],
            )

        self.assertEqual(totals["decks"], 1)
        self.assertEqual(totals["cards"], 2501)
        self.assertEqual(client.deleted, ["Mes Mots"])
        self.assertEqual(
            [(len(payload["decks"]), len(payload["cards"])) for payload in client.payloads],
            [(1, 0), (0, 1000), (0, 1000), (0, 501)],
        )


if __name__ == "__main__":
    unittest.main()
