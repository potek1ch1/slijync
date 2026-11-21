.PHONY: up down logs ps migrate sqlc test

up:
	docker compose up --build

down:
	docker compose down

logs:
	docker compose logs -f

ps:
	docker compose ps

migrate:
	docker compose run --rm migrate

sqlc:
	docker run --rm -v "$(CURDIR):/src" -w /src sqlc/sqlc:1.30.0 generate -f db/sqlc.yaml

test:
	cd apps/server && go test ./...
	cd apps/web && npm test

