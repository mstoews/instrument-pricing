DATABASE_URL ?= postgres://pricing:pricing@127.0.0.1:54329/pricing?sslmode=disable
export DATABASE_URL
.PHONY: db-up migrate seed api api-noauth generate test test-go test-python test-cpp sample smoke

db-up:
	docker compose up -d --wait
migrate:
	go run ./cmd/migrate
seed:
	psql "$(DATABASE_URL)" -v ON_ERROR_STOP=1 -1 -f db/seed.sql
api:
	go run ./cmd/api
# Open API for local exploration with curl; refused off loopback. The web app needs `make api`.
api-noauth:
	AUTH_DISABLED=true go run ./cmd/api
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
# Provisions a local admin and an ingest key, so it refuses non-local databases.
SMOKE_EMAIL ?= smoke-admin@example.test
SMOKE_PASSWORD ?= local-smoke-password
smoke:
	@case "$(DATABASE_URL)" in *@127.0.0.1:*|*@localhost:*) ;; *) echo "smoke provisions credentials; refusing non-local DATABASE_URL" >&2; exit 1;; esac
	USERADMIN_PASSWORD='$(SMOKE_PASSWORD)' go run ./cmd/useradmin user -email $(SMOKE_EMAIL) -role admin
	PRICING_API_KEY=$$(go run ./cmd/useradmin api-key -email $(SMOKE_EMAIL) -name smoke -scope ingest) \
	SMOKE_EMAIL=$(SMOKE_EMAIL) SMOKE_PASSWORD='$(SMOKE_PASSWORD)' PYTHONPATH=ingestion/src python3 scripts/smoke.py

.PHONY: ingest-massive ingest-treasury
# Vendor ingestion; needs PRICING_API_KEY (ingest scope) and, for Massive, MASSIVE_API_KEY.
API_URL ?= http://127.0.0.1:8080
YEAR ?= $(shell date +%Y)
ingest-massive:
	@test -n "$(TICKERS)" || { echo "usage: make ingest-massive TICKERS=AAPL,MSFT [FROM=YYYY-MM-DD] [TO=YYYY-MM-DD]" >&2; exit 2; }
	PYTHONPATH=ingestion/src python3 -m pricing_ingestion.cli --base-url $(API_URL) massive-bars --tickers $(TICKERS) $(if $(FROM),--from $(FROM)) $(if $(TO),--to $(TO))
ingest-treasury:
	PYTHONPATH=ingestion/src python3 -m pricing_ingestion.cli --base-url $(API_URL) treasury-curve --year $(YEAR) $(if $(LATEST),--latest)

.PHONY: web web-build test-web
web:
	cd web && npm start
web-build:
	cd web && npm run build
test-web:
	cd web && npm test -- --watch=false
