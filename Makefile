DATABASE_URL ?= postgres://pricing:pricing@127.0.0.1:54329/pricing?sslmode=disable
export DATABASE_URL
.PHONY: db-up migrate seed api generate test test-go test-python test-cpp sample smoke

db-up:
	docker compose up -d --wait
migrate:
	go run ./cmd/migrate
seed:
	psql "$(DATABASE_URL)" -v ON_ERROR_STOP=1 -1 -f db/seed.sql
api:
	go run ./cmd/api
generate:
	sqlc generate
sample:
	PYTHONPATH=ingestion/src python3 -m pricing_ingestion.cli
test: test-go test-python test-cpp
test-go:
	go test ./...
test-python:
	PYTHONPATH=ingestion/src python3 -m unittest discover -s ingestion/tests -v
test-cpp:
	cmake -S pricing -B build/pricing -DCMAKE_BUILD_TYPE=Release
	cmake --build build/pricing
	ctest --test-dir build/pricing --output-on-failure
smoke:
	PYTHONPATH=ingestion/src python3 scripts/smoke.py

.PHONY: web web-build test-web
web:
	cd web && npm start
web-build:
	cd web && npm run build
test-web:
	cd web && npm test -- --watch=false
