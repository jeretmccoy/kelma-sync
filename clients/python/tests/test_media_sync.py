from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from kelma_sync_v2.media_sync import sync_media_once


class _DB:
    def __init__(self, fields: str) -> None:
        self.fields = fields

    def all(self, _query: str, *_args):
        return [(self.fields,)]


class _Media:
    def __init__(self, path: Path) -> None:
        self.path = path

    def dir(self) -> str:
        return str(self.path)


class _Collection:
    def __init__(self, path: Path, fields: str) -> None:
        self.media = _Media(path)
        self.db = _DB(fields)


class _Client:
    def __init__(self, download: bytes | None = None) -> None:
        self.download = download
        self.gets: list[str] = []
        self.puts: list[str] = []

    def get_media(self, filename: str) -> bytes:
        self.gets.append(filename)
        if self.download is None:
            raise AssertionError("unexpected download")
        return self.download

    def put_media(self, filename: str, _data: bytes, _content_type: str) -> None:
        self.puts.append(filename)


class MediaPlanningTest(unittest.TestCase):
    def test_existing_files_do_not_schedule_network_workers(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / "Audio.MP3").write_bytes(b"present")
            col = _Collection(root, "[sound:audio.mp3]")
            client = _Client()
            progress: list[str] = []
            result = sync_media_once(
                col,  # type: ignore[arg-type]
                client,  # type: ignore[arg-type]
                {"media": [{"filename": "audio.mp3"}]},
                progress.append,
            )
            self.assertEqual((result.uploaded, result.downloaded), (0, 0))
            self.assertEqual(client.gets, [])
            self.assertEqual(client.puts, [])
            self.assertTrue(any("0 missing file(s)" in line for line in progress))

    def test_only_missing_files_are_downloaded(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            col = _Collection(root, "[sound:missing.mp3]")
            client = _Client(b"downloaded")
            result = sync_media_once(
                col,  # type: ignore[arg-type]
                client,  # type: ignore[arg-type]
                {"media": [{"filename": "missing.mp3"}]},
            )
            self.assertEqual(result.downloaded, 1)
            self.assertEqual(client.gets, ["missing.mp3"])
            self.assertEqual((root / "missing.mp3").read_bytes(), b"downloaded")


if __name__ == "__main__":
    unittest.main()
