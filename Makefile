createdb:
	docker exec -it go-bank-postgres createdb --username=postgres --owner=postgres go-bank

dropdb:
	docker exec -it go-bank-postgres dropdb --username=postgres --owner=postgres go-bank

migrateup:
	migrate -path=./db/migrations -database="postgres://alphaogilo:Pass123456@localhost/simple_bank?sslmode=disable" up

migratedown:
	migrate -path=./db/migrations -database="postgres://alphaogilo:Pass123456@localhost/simple_bank?sslmode=disable" down

createmigration:
	migrate create -ext sql -dir ./db/migrations -seq ${NAME}

sqlc:
	sqlc generate

test:
	go test -v -cover ./...


.PHONY: createdb dropdb migrateup migratedown createmigration test