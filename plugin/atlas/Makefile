DSNPG="postgres://${DB_USER}:${DB_PASS}@localhost:5432/${DB_NAME}?sslmode=disable"

.DEFAULT_GOAL := help

.PHONY: help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: install
install: ## install Ariga Atlas
	@curl -sSf https://atlasgo.sh | sh

.PHONY: docker
docker: ## run docker compose
	@docker compose up -d

.PHONY: generate
generate: ## run go generate
	@go generate ./...

.PHONY: test
test: ## run unit tests
	@go test -v ./... -coverprofile coverage.out
	@go tool cover -html coverage.out -o coverage.html

.PHONY: pg-import
pg-import: ## run postgres zoneimport
	@go run cli/main.go --db.name ${DB_NAME} --db.hostname 127.0.0.1 --db.user ${DB_USER} --db.password ${DB_PASS} zoneImport -f tests/pri.miek.nl -d miek.nl

.PHONY: pg-status
pg-status: ## get Atlas migration status
	@atlas migrate status --url ${DSNPG} --dir "file://migrations"

.PHONY: pg-inspect
pg-inspect: ## inspect postgres db with Ariga Atlas
	@atlas schema inspect -u ${DSNPG}	