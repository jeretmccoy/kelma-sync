from __future__ import annotations

import unittest

from kelma_sync_v2.full_diff import _diff_keyed, card_comparison_fingerprint


class CardComparisonTest(unittest.TestCase):
    def card(self, deck: str, *, due: int = 5, crt_day: int = 100, reps: int = 3) -> dict:
        return {
            "logical_key": "guid:0",
            "note_guid": "guid",
            "ord": 0,
            "deck_name": deck,
            "checksum": "raw-checksum-" + deck,
            "modified_at": "different timestamps are not state",
            "scheduling": {
                "_crt": crt_day * 86400,
                "type": 2,
                "queue": 2,
                "due": due,
                "ivl": 30,
                "factor": 2500,
                "reps": reps,
                "lapses": 0,
                "left": 0,
                "odue": 0,
                "odid": 0,
                "flags": 0,
            },
        }

    def test_parent_path_alias_is_not_a_card_diff(self) -> None:
        local = self.card("German Irregular Verbs::Mes Mots")
        server = self.card("Mes Mots")

        diff = _diff_keyed([local], [server], "logical_key")

        self.assertEqual(diff[0].status, "in-sync")

    def test_relative_due_values_are_normalized_by_collection_creation(self) -> None:
        local = self.card("Mes Mots", due=5, crt_day=100)
        server = self.card("Mes Mots", due=15, crt_day=90)

        self.assertEqual(
            card_comparison_fingerprint(local),
            card_comparison_fingerprint(server),
        )

    def test_confirmed_namespace_group_suppresses_per_card_path_diff(self) -> None:
        local = self.card("German Irregular Verbs::Mes Mots")
        server = self.card("Mes Mots")
        local["comparison_namespace_group"] = "same-membership"
        server["comparison_namespace_group"] = "same-membership"

        self.assertEqual(
            card_comparison_fingerprint(local),
            card_comparison_fingerprint(server),
        )

    def test_real_leaf_or_scheduling_change_remains_visible(self) -> None:
        local = self.card("Mes Mots")
        moved = self.card("Different Deck")
        reviewed = self.card("Mes Mots", reps=4)

        self.assertEqual(
            _diff_keyed([local], [moved], "logical_key")[0].status,
            "changed",
        )
        self.assertEqual(
            _diff_keyed([local], [reviewed], "logical_key")[0].status,
            "changed",
        )


if __name__ == "__main__":
    unittest.main()
