DB_USER     ?= postgres
DB_PASSWORD ?= secret
DB_HOST     ?= localhost
DB_PORT     ?= 5432
DB_NAME     ?= search_service
DB_URL      ?= postgresql://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=disable

MIGRATE_PATH = migrations

.PHONY: postgres createdb dropdb \
	migrateup migrateup1 migratedown migratedown1 \
	build run test test-integration tidy \
	docker-up docker-down

# --- local postgres (standalone container) ---

postgres:
	docker run --name postgres_db -p $(DB_PORT):5432 \
		-e POSTGRES_USER=$(DB_USER) \
		-e POSTGRES_PASSWORD=$(DB_PASSWORD) \
		-d postgres:16-alpine

createdb:
	docker exec -it postgres_db createdb --username=$(DB_USER) --owner=$(DB_USER) $(DB_NAME)

dropdb:
	docker exec -it postgres_db dropdb --username=$(DB_USER) $(DB_NAME)

# --- migrations ---

migrateup:
	migrate -path $(MIGRATE_PATH) -database "$(DB_URL)" -verbose up

migrateup1:
	migrate -path $(MIGRATE_PATH) -database "$(DB_URL)" -verbose up 1

migratedown:
	migrate -path $(MIGRATE_PATH) -database "$(DB_URL)" -verbose down

migratedown1:
	migrate -path $(MIGRATE_PATH) -database "$(DB_URL)" -verbose down 1

# --- app ---

build:
	CGO_ENABLED=0 go build -o bin/search-api ./cmd/search

run:
	go run ./cmd/search

# --- tests ---

test:
	go test ./...

test-integration:
	TEST_DATABASE_URL="$(DB_URL)" go test ./... -run Integration -v

tidy:
	go mod tidy

# --- docker compose ---

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down
