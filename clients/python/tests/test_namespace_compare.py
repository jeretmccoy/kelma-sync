from __future__ import annotations

import copy
import unittest

from kelma_sync_v2.full_diff import card_comparison_fingerprint
from kelma_sync_v2.namespace_compare import prepare_deck_namespace_comparison


class NamespaceComparisonTest(unittest.TestCase):
    def manifest(self, populated_path: str, *, empty_path: str | None = None) -> dict:
        decks = [{"name": populated_path, "checksum": "same-deck-config"}]
        if empty_path:
            decks.append({"name": empty_path, "checksum": "same-deck-config"})
        cards = [
            {
                "logical_key": f"guid-{index}:0",
                "note_guid": f"guid-{index}",
                "ord": 0,
                "deck_name": populated_path,
                "checksum": f"raw-{populated_path}-{index}",
                "scheduling": {"queue": 2, "due": index, "reps": 1},
            }
            for index in range(3)
        ]
        return {"decks": decks, "cards": cards, "notes": [], "notetypes": []}

    def test_same_membership_becomes_one_namespace_row(self) -> None:
        client = self.manifest(
            "German Irregular Verbs::Mes Mots", empty_path="Mes Mots"
        )
        ankiweb = copy.deepcopy(client)
        kelma = self.manifest("Mes Mots")

        count = prepare_deck_namespace_comparison({
            "Client": client,
            "AnkiWeb": ankiweb,
            "KelmaSync": kelma,
        })

        self.assertEqual(count, 1)
        for manifest in (client, ankiweb, kelma):
            synthetic = [deck for deck in manifest["decks"] if deck.get("namespace_group")]
            self.assertEqual(len(synthetic), 1)
            self.assertEqual(synthetic[0]["namespace_card_count"], 3)
        self.assertEqual(len(client["decks"]), 1)  # empty flat shell was hidden
        for index in range(3):
            self.assertEqual(
                card_comparison_fingerprint(client["cards"][index]),
                card_comparison_fingerprint(kelma["cards"][index]),
            )

    def test_different_membership_is_not_collapsed(self) -> None:
        client = self.manifest("Parent::Mes Mots")
        kelma = self.manifest("Mes Mots")
        kelma["cards"].pop()

        count = prepare_deck_namespace_comparison({
            "Client": client,
            "KelmaSync": kelma,
        })

        self.assertEqual(count, 0)
        self.assertFalse(any(deck.get("namespace_group") for deck in client["decks"]))


if __name__ == "__main__":
    unittest.main()
