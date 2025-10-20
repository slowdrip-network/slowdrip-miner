SHELL := /bin/bash
APP := miner
COMPOSE := docker compose
PKG := ./...
BINARY := miner

# ---------- Docker / Compose ----------
.PHONY: up down restart logs rebuild build-image push-image

up:
	$(COMPOSE) up -d --build

down:
	$(COMPOSE) down -v

restart:
	$(COMPOSE) restart

logs:
	$(COMPOSE) logs -f --tail=200

rebuild:
	$(COMPOSE) build $(APP) && $(COMPOSE) up -d $(APP)

# Build the Docker image directly (without compose)
build-image:
	docker build -f deploy/Dockerfile.miner -t slowdrip/miner:dev .

push-image: ## example tag; adjust registry as needed
	docker push slowdrip/miner:dev

# ---------- Local Go (optional) ----------
.PHONY: build run test lint fmt tidy

build:
	go build -o bin/$(BINARY) ./cmd/$(APP)

run:
	go run ./cmd/$(APP)

test:
	go test -v $(PKG)

lint:
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "golangci-lint not found. Install: https://golangci-lint.run/usage/install/"; \
		exit 1; \
	fi
	golangci-lint run

fmt:
	go fmt $(PKG)

tidy:
	go mod tidy

# ---------- Utilities ----------
.PHONY: health
health:
	curl -sS http://localhost:8080/healthz && echo
