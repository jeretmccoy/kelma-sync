#!/usr/bin/env python3
"""End-to-end smoke test for KelmaSync v2.

This script starts the Go server locally in KELMA_AUTH_MODE=local, exercises the
Python v2 client against it, and verifies:

- health
- register/login
- note create/get
- manifest
- conflict on stale base_checksum
- Force-Override resolution

It assumes local Postgres is available via docker compose (`make db-up`).
"""
from __future__ import annotations

import json
import os
import socket
import subprocess
import sys
import time
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
PY_CLIENT = ROOT / "clients" / "python"
PORT = int(os.environ.get("E2E_PORT", "18081"))
BASE_URL = f"http://localhost:{PORT}"
DATABASE_URL = os.environ.get(
    "DATABASE_URL",
    "postgres://kelma:kelma@localhost:5433/kelma_sync_test?sslmode=disable",
)

sys.path.insert(0, str(PY_CLIENT))

from kelma_sync_v2.client import V2Client, V2Conflict  # noqa: E402


def wait_port(port: int, timeout: float = 20.0) -> None:
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            with socket.create_connection(("127.0.0.1", port), timeout=0.5):
                return
        except OSError:
            time.sleep(0.2)
    raise RuntimeError(f"server did not open port {port}")


def health() -> None:
    with urllib.request.urlopen(f"{BASE_URL}/health", timeout=5) as r:
        data = json.loads(r.read())
    assert data == {"ok": True}, data


def start_server() -> subprocess.Popen[str]:
    env = os.environ.copy()
    env.update(
        {
            "PORT": str(PORT),
            "DATABASE_URL": DATABASE_URL,
            "MIGRATIONS_DIR": str(ROOT / "migrations"),
            "KELMA_AUTH_MODE": "local",
        }
    )
    bin_path = ROOT / ".tmp" / "kelma-sync2-e2e"
    bin_path.parent.mkdir(exist_ok=True)
    subprocess.run(["go", "build", "-o", str(bin_path), "./cmd/server"], cwd=ROOT, check=True, timeout=30)
    return subprocess.Popen(
        [str(bin_path)],
        cwd=ROOT,
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
    )


def main() -> int:
    print("== ensuring postgres ==")
    subprocess.run(["docker", "compose", "up", "-d", "postgres"], cwd=ROOT, check=True, timeout=30)

    print("== starting server ==")
    proc = start_server()
    try:
        wait_port(PORT, timeout=8)
        if proc.poll() is not None:
            out = proc.stdout.read() if proc.stdout else ""
            raise RuntimeError(f"server exited early:\n{out}")
        health()
        print("server ok")

        suffix = str(int(time.time() * 1000))
        username = f"e2e-{suffix}@example.com"
        password = "test-password"

        mac = V2Client(BASE_URL)
        phone = V2Client(BASE_URL)

        print("== register/login ==")
        reg = mac.register(username, password)
        assert reg["user_id"], reg
        mac_auth = mac.login(username, password, "MacBook")
        phone_auth = phone.login(username, password, "iPhone")
        assert mac_auth.token and phone_auth.token

        print("== notetype create/get/conflict/force ==")
        nt_created = mac.put_notetype(
            1,
            name="Basic",
            definition={"fields": ["Front", "Back"], "templates": [{"name": "Card 1"}]},
            client_modified_at="2026-07-10T00:00:00Z",
            base_checksum="",
        )
        nt_base = nt_created["checksum"]
        assert phone.get_notetype(1)["name"] == "Basic"
        phone.put_notetype(
            1,
            name="Basic phone",
            definition={"fields": ["Front", "Back"], "templates": [{"name": "Card 1"}]},
            client_modified_at="2026-07-10T01:00:00Z",
            base_checksum=nt_base,
        )
        try:
            mac.put_notetype(
                1,
                name="Basic mac",
                definition={"fields": ["Front", "Back"], "templates": [{"name": "Card 1"}]},
                client_modified_at="2026-07-10T02:00:00Z",
                base_checksum=nt_base,
            )
        except V2Conflict:
            pass
        else:
            raise AssertionError("expected notetype conflict")
        mac.put_notetype(
            1,
            name="Basic mac",
            definition={"fields": ["Front", "Back"], "templates": [{"name": "Card 1"}]},
            client_modified_at="2026-07-10T02:00:00Z",
            base_checksum=nt_base,
            force=True,
        )

        print("== create note ==")
        created = mac.put_note(
            "guid-e2e-1",
            notetype_id=1,
            fields=["front", "back"],
            tags=["e2e"],
            client_modified_at="2026-07-10T00:00:00Z",
            base_checksum="",
        )
        base = created["checksum"]
        assert created["guid"] == "guid-e2e-1"

        print("== get/manifest ==")
        fetched = mac.get_note("guid-e2e-1")
        assert fetched["fields"] == ["front", "back"], fetched
        manifest = mac.manifest()
        assert any(n["guid"] == "guid-e2e-1" for n in manifest["notes"]), manifest

        print("== concurrent edit / conflict ==")
        phone.put_note(
            "guid-e2e-1",
            notetype_id=1,
            fields=["front phone", "back"],
            tags=["e2e"],
            client_modified_at="2026-07-10T01:00:00Z",
            base_checksum=base,
        )
        try:
            mac.put_note(
                "guid-e2e-1",
                notetype_id=1,
                fields=["front mac", "back"],
                tags=["e2e"],
                client_modified_at="2026-07-10T02:00:00Z",
                base_checksum=base,
            )
        except V2Conflict as c:
            assert c.server["fields"][0] == "front phone", c.payload
        else:
            raise AssertionError("expected conflict")

        print("== force override ==")
        forced = mac.put_note(
            "guid-e2e-1",
            notetype_id=1,
            fields=["front mac", "back"],
            tags=["e2e"],
            client_modified_at="2026-07-10T02:00:00Z",
            base_checksum=base,
            force=True,
        )
        assert forced["fields"][0] == "front mac", forced

        print("E2E OK")
        return 0
    finally:
        if proc.poll() is None:
            proc.terminate()
            try:
                proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                proc.kill()
        if proc.stdout:
            out = proc.stdout.read()
            if out:
                print("\n== server log ==")
                print(out)


if __name__ == "__main__":
    raise SystemExit(main())
