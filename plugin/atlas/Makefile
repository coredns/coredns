DSN="postgres://${DB_USER}:${DB_PASS}@localhost:5432/${DB_NAME}?sslmode=disable"

install:
	curl -sSf https://atlasgo.sh | sh

pg.status:
	@atlas migrate status --url ${DSN} --dir "file://migrations"

pg.inspect:
	@atlas schema inspect -u ${DSN}

docker:
	@docker compose up -d

test:
#	rm -rf coverage.html coverage.out
	go test -v ./... -coverprofile coverage.out
	go tool cover -html coverage.out -o coverage.html
# 	@go test -race -covermode=atomic -coverprofile=coverage.out

import:
	go run cli/main.go --db.name ${DB_NAME} --db.hostname 127.0.0.1 --db.user ${DB_USER} --db.password ${DB_PASS} zoneImport -f tests/pri.miek.nl -d miek.nl