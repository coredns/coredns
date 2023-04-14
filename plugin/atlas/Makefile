DSN="postgres://${DB_USER}:${DB_PASS}@localhost:5432/${DB_NAME}?sslmode=disable"

install:
	curl -sSf https://atlasgo.sh | sh

pg.status:
	@atlas migrate status --url ${DSN} --dir "file://migrations"

pg.inspect:
	@atlas schema inspect -u ${DSN}

docker:
	@docker compose up -d

