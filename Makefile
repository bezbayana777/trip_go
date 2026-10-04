-include .env
export

.PHONY: generate
generate:
	go tool oapi-codegen \
  -generate types,chi-server \
  -package api \
  -o internal/generated/api.gen.go \
  contracts/openapi/trip-service.openapi.yaml

.PHONY: migrate
migrate:
	goose -dir migrations postgres "$(DATABASE_URL)" up

.PHONY: run
run:
	go run cmd/trip-service/main.go


