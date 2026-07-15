# Contributing

Issues and pull requests are welcome.

## Development checks

Run these before submitting a change:

```bash
go mod verify
go test ./...
go vet ./...
python3 -m compileall -q clients/python scripts
cargo test --locked --manifest-path clients/rust/kelma-hash/Cargo.toml
```

The handler integration tests require a dedicated PostgreSQL database. The CI
workflow demonstrates the expected `TEST_DATABASE_URL`. Never point tests at a
development or production database because the suite truncates tables.

Keep protocol changes reflected in `API.md`, `DESIGN.md`, and `SCHEMA.md` as
applicable. Never include real collections, media, credentials, tokens, account
identifiers, or production infrastructure details in tests or bug reports.

By contributing, you agree that your contribution is licensed under the
repository's GNU AGPL v3.0-or-later license.
