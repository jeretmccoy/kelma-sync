from __future__ import annotations

import unittest

from kelma_sync_v2.comparison_cache import count_matching_items_missing_from_source


def key_for(_resource: str, item: dict) -> str:
    return str(item["key"])


def fingerprint_for(_resource: str, item: dict | None):
    return None if item is None else item["checksum"]


class ComparisonCacheTest(unittest.TestCase):
    def test_counts_only_items_where_other_sources_agree(self) -> None:
        manifests = {
            "Client": {"notes": [
                {"key": "same", "checksum": "a"},
                {"key": "disagree", "checksum": "client"},
                {"key": "already-present", "checksum": "x"},
            ]},
            "AnkiWeb": {"notes": [
                {"key": "already-present", "checksum": "x"},
            ]},
            "KelmaSync": {"notes": [
                {"key": "same", "checksum": "a"},
                {"key": "disagree", "checksum": "server"},
                {"key": "already-present", "checksum": "x"},
            ]},
        }

        count = count_matching_items_missing_from_source(
            manifests,
            missing_source="AnkiWeb",
            agreement_sources=("Client", "KelmaSync"),
            resources=("notes",),
            key_for=key_for,
            fingerprint_for=fingerprint_for,
        )

        self.assertEqual(count, 1)


if __name__ == "__main__":
    unittest.main()
