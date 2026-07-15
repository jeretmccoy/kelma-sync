from __future__ import annotations

import copy
import unittest

from kelma_sync_v2.content_sync import _scope_server_manifest_to_decks


class FakeClient:
    def __init__(self, records: dict[int, dict]) -> None:
        self.records = records
        self.pulled_ids: list[int] = []

    def batch_pull(self, *, cards: list[int], **_kwargs) -> dict:
        self.pulled_ids.extend(cards)
        return {"cards": [copy.deepcopy(self.records[card_id]) for card_id in cards]}


class ContentScopeTest(unittest.TestCase):
    def setUp(self) -> None:
        self.records = {
            1: {
                "card_id": 1,
                "note_guid": "g1",
                "ord": 0,
                "deck_name": "Selected",
                "scheduling": {"reps": 1},
            },
            2: {
                "card_id": 2,
                "note_guid": "g2",
                "ord": 0,
                "deck_name": "Other",
                "scheduling": {"reps": 2},
            },
            3: {
                "card_id": 3,
                "note_guid": "g3",
                "ord": 1,
                "deck_name": "Other::Child",
                "scheduling": {"reps": 3},
            },
        }
        self.manifest = {
            "cards": [
                {
                    "card_id": card_id,
                    "note_guid": record["note_guid"],
                    "ord": record["ord"],
                    "deck_name": record["deck_name"],
                }
                for card_id, record in self.records.items()
            ],
            "notes": [{"guid": f"g{i}"} for i in range(1, 4)],
            "decks": [
                {"name": "Selected"},
                {"name": "Other"},
                {"name": "Other::Child"},
            ],
            "notetypes": [],
        }

    def test_current_manifest_pulls_only_selected_cards(self) -> None:
        client = FakeClient(self.records)
        scoped = _scope_server_manifest_to_decks(
            client, copy.deepcopy(self.manifest), ["Selected"]
        )

        self.assertEqual(client.pulled_ids, [1])
        self.assertEqual([card["card_id"] for card in scoped["cards"]], [1])
        self.assertEqual([note["guid"] for note in scoped["notes"]], ["g1"])
        self.assertEqual([deck["name"] for deck in scoped["decks"]], ["Selected"])
        self.assertEqual(scoped["cards"][0]["logical_key"], "g1:0")
        self.assertEqual(scoped["cards"][0]["scheduling"], {"reps": 1})

    def test_legacy_manifest_falls_back_to_assignment_scan(self) -> None:
        manifest = copy.deepcopy(self.manifest)
        for card in manifest["cards"]:
            card.pop("deck_name")
        client = FakeClient(self.records)

        scoped = _scope_server_manifest_to_decks(client, manifest, ["Selected"])

        self.assertEqual(client.pulled_ids, [1, 2, 3])
        self.assertEqual([card["card_id"] for card in scoped["cards"]], [1])


if __name__ == "__main__":
    unittest.main()
