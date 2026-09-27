.PHONY: generate
generate:
	go tool oapi-codegen -generate types,server -package api contracts/trip-service.openapi.yaml > internal/api/api.gen.go

.PHONY: migrate
migrate:
	goose -dir migrations postgres "$(DATABASE_URL)" up

.PHONY: run
run:
	go run cmd/trip-service/main.go

.PHONY: test
test:
	# Запуск тестов с race detector
	go test -v -race ./...

