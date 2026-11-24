.PHONY: up down logs test send bench psql db-dev dev-api dev-ui

up:            ## Build and start everything (Postgres + PostDream) in Docker
	docker compose up -d --build

down:          ## Stop the containers (data stays in the volume)
	docker compose down

logs:          ## Follow PostDream logs
	docker compose logs -f postdream

test:          ## Run the Go tests inside Docker (no local Go needed)
	docker run --rm -v "$(PWD)/backend":/src -w /src golang:1.26-alpine go test ./...

bench:         ## SMTP load test against the running stack (2000 emails, 20 connections)
	docker run --rm --network postdream_default -v "$(PWD)/backend":/src -w /src golang:1.26-alpine \
		go run ./cmd/pdbench -addr postdream:1025 -n 2000 -c 20 -size 5000

send:          ## Send 5 test emails to the local SMTP server
	python3 scripts/send-test-mail.py -n 5

psql:          ## Open a Postgres shell
	docker compose exec db psql -U postdream

db-dev:        ## Start only a dev Postgres on localhost:5433 (for running the API from source)
	docker compose -f docker-compose.dev.yml up -d

dev-api:       ## Run the backend from source (needs Go + `make db-dev`)
	cd backend && DATABASE_URL=postgres://postdream:postdream@localhost:5433/postdream?sslmode=disable \
		ADMIN_PASSWORD=admin12345 LOG_LEVEL=debug go run ./cmd/postdream

dev-ui:        ## Frontend with hot reload on :5173 (proxies /api to :8025)
	cd frontend && npm install && npm run dev
