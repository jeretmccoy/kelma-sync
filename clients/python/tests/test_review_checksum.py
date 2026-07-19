from kelma_sync_v2.checksum_rs import review_checksum


def test_review_checksum_matches_mobile_and_server() -> None:
    assert review_checksum("g1", 0, 3, 10, 1, 2500, 4000, 1) == (
        "46a3a58da871440db7ede339f4ccaf508f5283d5060e20a705c1c3b889d7a17a"
    )
