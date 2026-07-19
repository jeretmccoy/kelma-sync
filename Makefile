PORT ?= 8081
DATABASE_URL ?= postgres://kelma:kelma@localhost:5433/kelma_sync?sslmode=disable
MIGRATIONS_DIR ?= ./migrations

export PORT
export DATABASE_URL
export MIGRATIONS_DIR

.PHONY: db-up db-down db-logs db-reset-content run build test e2e anki-e2e fmt vet curl-register curl-login seed smoke

db-up:
	docker compose up -d postgres

db-down:
	docker compose down

db-logs:
	docker compose logs -f postgres

# Reset only content tables (notes/cards/...), preserving users/clients/tokens
# so logins survive a reset.
db-reset-content:
	docker compose exec -T postgres psql -U kelma -d kelma_sync -c \
	  "TRUNCATE reviews, study_days, notes, cards, notetypes, decks, media, tombstones RESTART IDENTITY;"

run:
	go run ./cmd/server

build:
	go build ./...

test:
	go test ./...

e2e:
	python3 ./scripts/e2e_v2.py

anki-e2e:
	PYTHONPATH=$$HOME/projects/kelma-desktop-public/out/pylib:$$HOME/projects/kelma-desktop-public/pylib \
	$$HOME/projects/kelma-desktop-public/out/pyenv/bin/python ./scripts/e2e_anki_test_deck.py

fmt:
	gofmt -w internal/ cmd/

vet:
	go vet ./...

curl-register:
	curl -sS -X POST http://localhost:$(PORT)/v2/auth/register \
	  -H 'content-type: application/json' \
	  -d '{"username":"demo","password":"demo"}' | jq

curl-login:
	curl -sS -X POST http://localhost:$(PORT)/v2/auth/login \
	  -H 'content-type: application/json' \
	  -d '{"username":"demo","password":"demo","client_label":"MacBook"}' | jq

seed:
	bash ./scripts/seed.sh

smoke:
	bash ./scripts/smoke.sh
