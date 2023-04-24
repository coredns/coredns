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
pg-import: ## run postgres zoneimport from tests/pri.miek.nl for domain miek.nl 
	@go run cli/main.go -u ${DSN0} zoneImport -f tests/pri.miek.nl -d miek.nl

.PHONY: pg-bulk-import
pg-bulk-import: ## run postgres zoneimport from tests/pri.miek.nl for domain miek.nl 
	@go run cli/main.go -u ${DSN0} zoneImport -b --dir import

.PHONY: pg-status
pg-status: ## get Atlas migration status for postgres db
	@atlas migrate status --url ${DSN0} --dir "file://migrations/postgres"

.PHONY: pg-inspect
pg-inspect: ## inspect postgres db with Ariga Atlas
	@atlas schema inspect -u ${DSN0}

.PHONY: pg-apply
pg-apply: ## apply changes to postgres db with Ariga Atlas
	@atlas schema apply -u ${DSN0} --to "file://migrations/postgres" --auto-approve

.PHONY: my-import
my-import: ## run mysql zoneimport from tests/pri.miek.nl for domain miek.nl 
	@go run cli/main.go -u ${DSN1} zoneImport -f tests/pri.miek.nl -d miek.nl

.PHONY: my-status
my-status: ## get Atlas migration status for mysql db
	@atlas migrate status --url ${DSN1} --dir "file://migrations/mysql"

.PHONY: my-inspect
my-inspect: ## inspect mysql db with Ariga Atlas
	@atlas schema inspect -u ${DSN1}

.PHONY: my-apply
my-apply: ## apply changes to mysql db with Ariga Atlas
	@atlas schema apply -u ${DSN1} --to "file://migrations/mysql" --auto-approve

.PHONY: ma-import
ma-import: ## run mariadb zoneimport from tests/pri.miek.nl for domain miek.nl 
	@go run cli/main.go -u ${DSN2} zoneImport -f tests/pri.miek.nl -d miek.nl

.PHONY: ma-status
ma-status: ## get Atlas migration status for maria db
	@atlas migrate status --url ${DSN2} --dir "file://migrations/mariadb"

.PHONY: ma-inspect
ma-inspect: ## inspect maria db with Ariga Atlas
	@atlas schema inspect -u ${DSN2}

.PHONY: ma-apply
ma-apply: ## apply changes to maria db with Ariga Atlas
	@atlas schema apply -u ${DSN2} --to "file://migrations/mariadb" --auto-approve	