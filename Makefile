.PHONY: build run dev-backend dev-frontend test build-web docker-dev clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

## Sestaví Go binárku (bez CGO)
build:
	CGO_ENABLED=0 go build -ldflags "-X github.com/qwerin/zahrada/internal/api.Version=$(VERSION)" -o bin/zahrada ./cmd/server/

## Sestaví frontend i backend a spustí na :8080
run: build-web build
	ZAHRADA_STATIC_DIR=web/dist ./bin/zahrada

## Backend přes go run (:8080)
dev-backend:
	go run ./cmd/server/

## Vite dev server na :5173 (proxuje /api → :8080)
dev-frontend:
	cd web && npm run dev

## go vet + Go testy + typecheck frontendu (pokud jsou nainstalované node_modules)
test:
	go vet ./...
	go test ./... -count=1
	@if [ -d web/node_modules ]; then cd web && npm run typecheck; fi

## Produkční build frontendu
build-web:
	cd web && npm ci && npm run build

## Celý stack lokálně s vlastním buildem image (Postgres + aplikace na 127.0.0.1:8080)
docker-dev:
	docker compose -f docker-compose.yml -f docker-compose.dev.yml up --build

clean:
	rm -rf bin/ web/dist/
	rm -f zahrada.db zahrada.db-wal zahrada.db-shm
